package hashing

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spaolacci/murmur3"
	"github.com/utterwqlnut/utterdb/protos"
	pb "github.com/utterwqlnut/utterdb/protos"
	"github.com/utterwqlnut/utterdb/src/client"

	"google.golang.org/grpc"
)

type NodeConn struct {
	client protos.NodeClient
	Conn   *grpc.ClientConn
}

type Node struct {
	ip       string
	name     string
	hash     uint64
	NodeConn NodeConn
}

type HashRing struct {
	Ring          []*Node
	globalLock    sync.RWMutex
	rebalanceLock sync.Mutex
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

func FromString(s string) *HashRing {
	nodes := make([]*Node, 0)
	nodeStrings := strings.Split(s, " ")
	sort.Strings(nodeStrings)

	for i := 0; i < len(nodeStrings); i++ {
		splt := strings.Split(nodeStrings[i], ",")
		nodes = append(nodes, NewNode(splt[1], "node_"+strconv.Itoa(i)))
	}

	hR := HashRing{nodes, sync.RWMutex{}, sync.Mutex{}}
	hR.Sort()

	return &hR
}

func NewHashRing(nodeNames []string) *HashRing {
	nodes := make([]*Node, 0)

	for i := 0; i < len(nodeNames); i++ {
		nodes = append(nodes, NewNode(nodeNames[i], "node_"+strconv.Itoa(i)))
	}
	hR := HashRing{nodes, sync.RWMutex{}, sync.Mutex{}}
	hR.Sort()
	return &hR
}

func (hR *HashRing) Sort() {
	sort.Slice(hR.Ring, func(i, j int) bool {
		return hR.Ring[i].hash < hR.Ring[j].hash
	})
}

// These 3 helper methods are NOT thread safe
func (hR *HashRing) AddNodeHelper(n *Node) (successor *Node, startHash uint64, endHash uint64) {
	idx := sort.Search(len(hR.Ring), func(i int) bool {
		return hR.Ring[i].hash >= n.hash
	}) % len(hR.Ring)

	successor = hR.Ring[idx]

	beforeIdx := (idx - 1 + len(hR.Ring)) % len(hR.Ring)
	startHash = hR.Ring[beforeIdx].hash
	endHash = n.hash

	return successor, startHash, endHash
}
func (hR *HashRing) RemoveNodeHelper(n *Node) (successor *Node, startHash uint64, endHash uint64) {
	idx := sort.Search(len(hR.Ring), func(i int) bool {
		return hR.Ring[i].hash == n.hash
	})

	afterIdx := (idx + 1) % len(hR.Ring)
	successor = hR.Ring[afterIdx]

	beforeIdx := (idx - 1 + len(hR.Ring)) % len(hR.Ring)

	startHash = hR.Ring[beforeIdx].hash
	endHash = n.hash // The range the dying node owned

	return successor, startHash, endHash
}

func (hR *HashRing) GetNode(key string) *Node {
	keyHash := murmur3.Sum64([]byte(key))

	idx := sort.Search(len(hR.Ring), func(i int) bool {
		return hR.Ring[i].hash >= keyHash
	}) % len(hR.Ring)

	return hR.Ring[idx]
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

	node := hR.GetNode(key)
	_, err := node.NodeConn.client.Write(ctx, &pb.Data{Key: key,
		KeyType: keyType, Value: value, ValueType: valueType})

	return err
}

func (hR *HashRing) Erase(key string, keyType string) error {
	hR.globalLock.RLock()
	defer hR.globalLock.RUnlock()

	node := hR.GetNode(key)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := node.NodeConn.client.Erase(ctx, &pb.Request{Key: key,
		Type: keyType})

	return err
}

func (hR *HashRing) Get(key string, keyType string) (string, error) {
	hR.globalLock.RLock()
	defer hR.globalLock.RUnlock()
	node := hR.GetNode(key)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	value, err := node.NodeConn.client.Get(ctx, &pb.Request{Key: key,
		Type: keyType})

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

func (hR *HashRing) AddNode(ip string) error {
	hR.rebalanceLock.Lock()
	defer hR.rebalanceLock.Unlock()
	node := NewNode(ip, "node_"+strconv.Itoa(len(hR.Ring)))

	nodeBefore, start, end := hR.AddNodeHelper(node)
	ctx := context.Background()
	_, err := node.NodeConn.client.InitiateMove(ctx, &pb.Rebalance{Start: start, End: end, Ip: nodeBefore.ip})

	if err != nil {
		return err
	}
	hR.globalLock.Lock()
	hR.Ring = append(hR.Ring, node)
	hR.Sort()
	hR.globalLock.Unlock()

	nodeBefore.NodeConn.client.ClearOldData(ctx, &pb.Range{Start: start, End: end})

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

	nodeBefore, start, end := hR.RemoveNodeHelper(hR.Ring[idx])
	ctx := context.Background()
	_, err := nodeBefore.NodeConn.client.InitiateMove(ctx, &pb.Rebalance{Start: start, End: end, Ip: hR.Ring[idx].ip})

	if err != nil {
		return err
	}

	hR.globalLock.Lock()
	hR.Ring = append(hR.Ring[:idx], hR.Ring[idx+1:]...)
	hR.Sort()
	hR.globalLock.Unlock()

	return nil
}
