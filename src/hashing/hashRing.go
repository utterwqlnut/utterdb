package hashing

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spaolacci/murmur3"
	pb "github.com/utterwqlnut/utterdb/protos"
	"github.com/utterwqlnut/utterdb/src/client"

	"google.golang.org/grpc"
)

type NodeConn struct {
	client pb.NodeClient
	Conn   *grpc.ClientConn
}

type Node struct {
	ip       string
	name     string
	hash     uint64
	NodeConn NodeConn
}

type HashRing struct {
	Ring              []*Node
	globalLock        sync.RWMutex
	rebalanceLock     sync.Mutex
	ReplicationFactor int
}

func (hR *HashRing) String() string {
	var s string = ""
	for i, node := range hR.Ring {
		s += node.name + "," + node.ip
		if i != len(hR.Ring)-1 {
			s += " "
		}
	}
	return s
}

func FromString(s string, replicationFactor int) *HashRing {
	nodes := make([]*Node, 0)
	nodeStrings := strings.Split(s, " ")
	sort.Strings(nodeStrings)

	for i := 0; i < len(nodeStrings); i++ {
		splt := strings.Split(nodeStrings[i], ",")
		nodes = append(nodes, NewNode(splt[1], "node_"+strconv.Itoa(i)))
	}

	hR := HashRing{nodes, sync.RWMutex{}, sync.Mutex{}, replicationFactor}
	hR.Sort()

	return &hR
}

func NewHashRing(nodeNames []string, replicationFactor int) *HashRing {
	nodes := make([]*Node, 0)

	for i := 0; i < len(nodeNames); i++ {
		nodes = append(nodes, NewNode(nodeNames[i], "node_"+strconv.Itoa(i)))
	}
	hR := HashRing{nodes, sync.RWMutex{}, sync.Mutex{}, replicationFactor}
	hR.Sort()
	return &hR
}

func (hR *HashRing) Sort() {
	sort.Slice(hR.Ring, func(i, j int) bool {
		return hR.Ring[i].hash < hR.Ring[j].hash
	})
}

func (hR *HashRing) effectiveReplication() int {
	n := hR.ReplicationFactor
	if n > len(hR.Ring) {
		return len(hR.Ring)
	}
	if n < 1 {
		return 1
	}
	return n
}

func (hR *HashRing) ringRange(startIdx, endIdx int) (uint64, uint64) {
	ringLen := len(hR.Ring)
	return hR.Ring[(startIdx%ringLen+ringLen)%ringLen].hash,
		hR.Ring[(endIdx%ringLen+ringLen)%ringLen].hash
}

func (hR *HashRing) initiateMove(dest *Node, start, end uint64, sourceIP string) error {
	ctx := context.Background()
	_, err := dest.NodeConn.client.InitiateMove(ctx, &pb.Rebalance{Start: start, End: end, Ip: sourceIP})
	return err
}

func (hR *HashRing) clearRange(node *Node, start, end uint64) error {
	ctx := context.Background()
	_, err := node.NodeConn.client.ClearOldData(ctx, &pb.Range{Start: start, End: end})
	return err
}

// These helper methods are NOT thread safe
func (hR *HashRing) AddNodeHelper(n *Node) (nodeBefore *Node, insertIdx int, startHash uint64, endHash uint64) {
	insertIdx = sort.Search(len(hR.Ring), func(i int) bool {
		return hR.Ring[i].hash >= n.hash
	}) % len(hR.Ring)

	beforeIdx := (insertIdx - 1 + len(hR.Ring)) % len(hR.Ring)
	nodeBefore = hR.Ring[beforeIdx]
	startHash = nodeBefore.hash
	endHash = n.hash

	return nodeBefore, insertIdx, startHash, endHash
}

func (hR *HashRing) RemoveNodeHelper(n *Node) (nodeBefore *Node, delIdx int, startHash uint64, endHash uint64) {
	delIdx = sort.Search(len(hR.Ring), func(i int) bool {
		return hR.Ring[i].hash >= n.hash
	})

	beforeIdx := (delIdx - 1 + len(hR.Ring)) % len(hR.Ring)
	nodeBefore = hR.Ring[beforeIdx]
	startHash = nodeBefore.hash
	endHash = n.hash

	return nodeBefore, delIdx, startHash, endHash
}

func (hR *HashRing) ownerIndex(keyHash uint64) int {
	return sort.Search(len(hR.Ring), func(i int) bool {
		return hR.Ring[i].hash >= keyHash
	}) % len(hR.Ring)
}

func (hR *HashRing) nodesForKey(key string) []*Node {
	keyHash := murmur3.Sum64([]byte(key))
	idx := hR.ownerIndex(keyHash)
	n := hR.effectiveReplication()
	nodes := make([]*Node, n)
	for i := 0; i < n; i++ {
		nodes[i] = hR.Ring[(idx+i)%len(hR.Ring)]
	}
	return nodes
}

func (hR *HashRing) GetNode(key string) *Node {
	return hR.nodesForKey(key)[0]
}

func NewNode(ip string, name string) *Node {
	nodeClient, conn := client.GetClient(ip)
	return &Node{
		ip:       ip,
		name:     name,
		hash:     murmur3.Sum64([]byte(name)),
		NodeConn: NodeConn{nodeClient, conn},
	}
}

func (hR *HashRing) Write(key string, keyType string, value string, valueType string) error {
	hR.globalLock.RLock()
	defer hR.globalLock.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	data := &pb.Data{Key: key, KeyType: keyType, Value: value, ValueType: valueType}
	for _, node := range hR.nodesForKey(key) {
		_, err := node.NodeConn.client.Write(ctx, data)
		if err != nil {
			return err
		}
	}
	return nil
}

func (hR *HashRing) Erase(key string, keyType string) error {
	hR.globalLock.RLock()
	defer hR.globalLock.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	req := &pb.Request{Key: key, Type: keyType}
	for _, node := range hR.nodesForKey(key) {
		_, err := node.NodeConn.client.Erase(ctx, req)
		if err != nil {
			return err
		}
	}
	return nil
}

func (hR *HashRing) Get(key string, keyType string) (string, error) {
	hR.globalLock.RLock()
	defer hR.globalLock.RUnlock()
	node := hR.GetNode(key)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	value, err := node.NodeConn.client.Get(ctx, &pb.Request{Key: key, Type: keyType})
	if err != nil {
		return "", err
	}

	return value.Value, nil
}

func (hR *HashRing) GetRam() string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	returnString := ""

	hR.globalLock.RLock()
	for i := range hR.Ring {
		ramUse, _ := hR.Ring[i].NodeConn.client.RamUse(ctx, &pb.Empty{})
		returnString += strconv.FormatFloat(float64(ramUse.Value), 'e', -1, 32)
		if i != len(hR.Ring)-1 {
			returnString += " "
		}
	}
	hR.globalLock.RUnlock()

	return returnString
}

func (hR *HashRing) GetCpu() string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	returnString := ""

	hR.globalLock.RLock()
	for i := range hR.Ring {
		cpuUse, _ := hR.Ring[i].NodeConn.client.CpuUse(ctx, &pb.Empty{})
		returnString += strconv.FormatFloat(float64(cpuUse.Value), 'e', -1, 32)
		if i != len(hR.Ring)-1 {
			returnString += " "
		}
	}
	hR.globalLock.RUnlock()

	return returnString
}

func (hR *HashRing) rebalanceReplicasOnAdd(node *Node, insertIdx int) error {
	ringLen := len(hR.Ring)
	n := hR.effectiveReplication()

	for i := 1; i < n; i++ {
		start, end := hR.ringRange(insertIdx-i-1, insertIdx-i)
		source := hR.Ring[(insertIdx-i+ringLen)%ringLen]
		if err := hR.initiateMove(node, start, end, source.ip); err != nil {
			return fmt.Errorf("replica move %d on add: %w", i, err)
		}
	}
	return nil
}

func (hR *HashRing) clearStaleReplicasOnAdd(newIdx int) error {
	ringLen := len(hR.Ring)
	n := hR.effectiveReplication()

	for j := 1; j <= n; j++ {
		clearNode := hR.Ring[(newIdx+j)%ringLen]
		start, end := hR.ringRange(newIdx-j-1, newIdx-j)
		if err := hR.clearRange(clearNode, start, end); err != nil {
			return fmt.Errorf("clear replica %d on add: %w", j, err)
		}
	}
	return nil
}

func (hR *HashRing) AddNode(ip string) error {
	hR.rebalanceLock.Lock()
	defer hR.rebalanceLock.Unlock()

	if hR.ReplicationFactor > len(hR.Ring)+1 {
		return errors.New("replication factor exceeds ring size after add")
	}

	node := NewNode(ip, "node_"+strconv.Itoa(len(hR.Ring)))
	nodeBefore, insertIdx, start, end := hR.AddNodeHelper(node)

	if err := hR.initiateMove(node, start, end, nodeBefore.ip); err != nil {
		return fmt.Errorf("primary move on add: %w", err)
	}

	if err := hR.rebalanceReplicasOnAdd(node, insertIdx); err != nil {
		return err
	}

	hR.globalLock.Lock()
	hR.Ring = append(hR.Ring, node)
	hR.Sort()
	newIdx := sort.Search(len(hR.Ring), func(i int) bool {
		return hR.Ring[i].ip == node.ip
	})
	hR.globalLock.Unlock()

	if err := hR.clearStaleReplicasOnAdd(newIdx); err != nil {
		return err
	}

	if err := hR.clearRange(nodeBefore, start, end); err != nil {
		return fmt.Errorf("clear source on add: %w", err)
	}

	return nil
}

func (hR *HashRing) rebalanceReplicasOnRemove(delIdx int, successor *Node) error {
	ringLen := len(hR.Ring)
	n := hR.effectiveReplication()

	for i := 1; i < n; i++ {
		start, end := hR.ringRange(delIdx-i-1, delIdx-i)
		source := hR.Ring[(delIdx+i-1+ringLen)%ringLen]
		if err := hR.initiateMove(successor, start, end, source.ip); err != nil {
			return fmt.Errorf("replica move %d on remove: %w", i, err)
		}
	}
	return nil
}

func (hR *HashRing) clearStaleReplicasOnRemove(delIdx int) error {
	ringLen := len(hR.Ring)
	n := hR.effectiveReplication()

	// j=1 would clear data just moved onto the successor
	for j := 2; j <= n; j++ {
		clearNode := hR.Ring[(delIdx+j)%ringLen]
		start, end := hR.ringRange(delIdx-j-1, delIdx-j)
		if err := hR.clearRange(clearNode, start, end); err != nil {
			return fmt.Errorf("clear replica %d on remove: %w", j, err)
		}
	}
	return nil
}

func (hR *HashRing) RemoveNode(ip string) error {
	hR.rebalanceLock.Lock()
	defer hR.rebalanceLock.Unlock()

	found := false
	var idx int
	for i := range hR.Ring {
		if hR.Ring[i].ip == ip {
			found = true
			idx = i
			break
		}
	}

	if !found {
		return errors.New("IP not found")
	}

	if hR.ReplicationFactor > len(hR.Ring) {
		return errors.New("cannot remove node: replication factor exceeds ring size")
	}

	deletingNode := hR.Ring[idx]
	_, delIdx, start, end := hR.RemoveNodeHelper(deletingNode)
	successorIdx := (delIdx + 1) % len(hR.Ring)
	successor := hR.Ring[successorIdx]

	hR.initiateMove(successor, start, end, deletingNode.ip)

	hR.rebalanceReplicasOnRemove(delIdx, successor)

	hR.clearStaleReplicasOnRemove(delIdx)

	hR.globalLock.Lock()
	hR.Ring = append(hR.Ring[:idx], hR.Ring[idx+1:]...)
	hR.Sort()
	hR.globalLock.Unlock()

	return nil
}

func (hR *HashRing) HeartBeat(proxies []string, fails map[string]int) {
	toBeDeleted := make([]string, 0)
	hR.globalLock.RLock()
	for _, node := range hR.Ring {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := node.NodeConn.client.Health(ctx, &pb.Empty{})

		if err != nil {
			fails[node.name] += 1
		} else {
			fails[node.name] = 0
		}

		if fails[node.name] >= 1 {
			toBeDeleted = append(toBeDeleted, node.ip)
		}
		cancel()
	}
	hR.globalLock.RUnlock()

	for _, ip := range toBeDeleted {
		_ = hR.RemoveNode(ip)
		time.Sleep(5 * time.Second)
	}
}
