package main

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os"
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
	"gopkg.in/yaml.v3"
)

type NodeConn struct {
	client protos.NodeClient
	conn   *grpc.ClientConn
}

type Node struct {
	ip       string
	name     string
	hash     uint64
	nodeConn NodeConn
}

type HashRing struct {
	ring          []*Node
	globalLock    sync.RWMutex
	rebalanceLock sync.Mutex
}

func newHashRing(nodeNames []string) *HashRing {
	nodes := make([]*Node, 0)

	for i := 0; i < len(nodeNames); i++ {
		nodes = append(nodes, newNode(nodeNames[i], "node_"+strconv.Itoa(i)))
	}
	hR := HashRing{nodes, sync.RWMutex{}, sync.Mutex{}}
	hR.sort()
	return &hR
}

func (hR *HashRing) sort() {
	sort.Slice(hR.ring, func(i, j int) bool {
		return hR.ring[i].hash < hR.ring[j].hash
	})
}

// These 3 helper methods are NOT thread safe
func (hR *HashRing) addNodeHelper(n *Node) (nodeBefore *Node, startHash uint64, endHash uint64) {
	idx := sort.Search(len(hR.ring), func(i int) bool {
		return hR.ring[i].hash >= n.hash
	})

	before := ((idx - 1) + len(hR.ring)) % (len(hR.ring)) // Find the node before this new added node
	after := (idx) % len(hR.ring)

	return hR.ring[before], n.hash, hR.ring[after].hash
}

func (hR *HashRing) removeNodeHelper(n *Node) (nodeBefore *Node, startHash uint64, endHash uint64) {
	idx := sort.Search(len(hR.ring), func(i int) bool {
		return hR.ring[i].hash == n.hash
	})

	before := ((idx - 1) + len(hR.ring)) % (len(hR.ring)) // Find the node before this new added node
	after := (idx + 1) % len(hR.ring)

	return hR.ring[before], n.hash, hR.ring[after].hash
}

func (hR *HashRing) getNode(key string) *Node {
	keyHash := murmur3.Sum64([]byte(key))

	idx := sort.Search(len(hR.ring), func(i int) bool {
		return hR.ring[i].hash >= keyHash
	}) % len(hR.ring)

	return hR.ring[idx]
}

func newNode(ip string, name string) *Node {
	nodeClient, conn := client.GetClient(ip)
	return &Node{
		ip:       ip,
		name:     name,
		hash:     murmur3.Sum64([]byte(name)),
		nodeConn: NodeConn{nodeClient, conn},
	}
}

func (hR *HashRing) write(key string, keyType string, value string, valueType string) error {
	hR.globalLock.RLock()
	defer hR.globalLock.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	node := hR.getNode(key)
	_, err := node.nodeConn.client.Write(ctx, &pb.Data{Key: key,
		KeyType: keyType, Value: value, ValueType: valueType})

	return err
}

func (hR *HashRing) erase(key string, keyType string) error {
	hR.globalLock.RLock()
	defer hR.globalLock.RUnlock()

	node := hR.getNode(key)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := node.nodeConn.client.Erase(ctx, &pb.Request{Key: key,
		Type: keyType})

	return err
}

func (hR *HashRing) get(key string, keyType string) (string, error) {
	hR.globalLock.RLock()
	defer hR.globalLock.RUnlock()
	node := hR.getNode(key)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	value, err := node.nodeConn.client.Get(ctx, &pb.Request{Key: key,
		Type: keyType})

	if err != nil {
		return "", err
	}

	return value.Value, nil

}

func (hR *HashRing) getRam() string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	returnString := ""

	hR.globalLock.RLock()
	for i := range hR.ring {
		ramUse, _ := hR.ring[i].nodeConn.client.RamUse(ctx, &pb.Empty{})
		returnString += strconv.FormatFloat(float64(ramUse.Value), 'e', -1, 32)
		if i != len(hR.ring)-1 {
			returnString += " "
		}
	}
	hR.globalLock.RUnlock()

	return returnString
}

func (hR *HashRing) getCpu() string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	returnString := ""

	hR.globalLock.RLock()
	for i := range hR.ring {
		cpuUse, _ := hR.ring[i].nodeConn.client.CpuUse(ctx, &pb.Empty{})
		returnString += strconv.FormatFloat(float64(cpuUse.Value), 'e', -1, 32)
		if i != len(hR.ring)-1 {
			returnString += " "
		}
	}
	hR.globalLock.RUnlock()

	return returnString
}

func (hR *HashRing) addNode(ip string) error {
	hR.rebalanceLock.Lock()
	defer hR.rebalanceLock.Unlock()
	node := newNode(ip, "node_"+strconv.Itoa(len(hR.ring)))

	nodeBefore, start, end := hR.addNodeHelper(node)
	ctx := context.Background()
	_, err := node.nodeConn.client.InitiateMove(ctx, &pb.Rebalance{Start: start, End: end, Ip: nodeBefore.ip})

	if err != nil {
		return err
	}
	hR.globalLock.Lock()
	hR.ring = append(hR.ring, node)
	hR.sort()
	hR.globalLock.Unlock()

	return nil
}

func (hR *HashRing) removeNode(ip string) error {
	hR.rebalanceLock.Lock()
	defer hR.rebalanceLock.Unlock()

	found := false
	var idx int
	for i := range hR.ring {
		if hR.ring[i].ip == ip {
			found = true
			idx = i
			break
		}
	}

	if !found {
		return errors.New("IP not found")
	}

	nodeBefore, start, end := hR.removeNodeHelper(hR.ring[idx])
	ctx := context.Background()
	_, err := nodeBefore.nodeConn.client.InitiateMove(ctx, &pb.Rebalance{Start: start, End: end, Ip: hR.ring[idx].ip})

	if err != nil {
		return err
	}

	hR.globalLock.Lock()
	hR.ring = append(hR.ring[:idx], hR.ring[idx+1:]...)
	hR.sort()
	hR.globalLock.Unlock()

	return nil
}

func (hR *HashRing) handleTcp(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for {
		msg, err := reader.ReadString('\n') // CMD|ARG1|ARG2...

		if err != nil {
			return // connection closed
		}

		msg = strings.TrimSpace(msg)
		cmd := strings.Split(msg, "|")

		if len(cmd) == 0 || cmd[0] == "" {
			conn.Write([]byte("ERR empty command\n"))
			continue
		}

		switch cmd[0] {

		case "WRITE":
			if len(cmd) != 5 {
				conn.Write([]byte("ERR invalid Write command\n"))
				continue
			}
			err := hR.write(cmd[1], cmd[2], cmd[3], cmd[4])
			if err != nil {
				conn.Write([]byte("ERR " + err.Error() + "\n"))
				continue
			}
			conn.Write([]byte("OK\n"))

		case "GET":
			if len(cmd) != 3 {
				conn.Write([]byte("ERR invalid Get command\n"))
				continue
			}
			value, err := hR.get(cmd[1], cmd[2])
			if err != nil {
				conn.Write([]byte("ERR " + err.Error() + "\n"))
				continue
			}
			conn.Write([]byte(value + "\n"))

		case "ERASE":
			if len(cmd) != 3 {
				conn.Write([]byte("ERR invalid Erase command\n"))
				continue
			}
			err := hR.erase(cmd[1], cmd[2])
			if err != nil {
				conn.Write([]byte("ERR " + err.Error() + "\n"))
				continue
			}
			conn.Write([]byte("OK\n"))

		case "ADDNODE":
			if len(cmd) != 2 {
				conn.Write([]byte("ERR invalid Add Node command\n"))
				continue
			}
			err := hR.addNode(cmd[1])
			if err != nil {
				conn.Write([]byte("ERR " + err.Error() + "\n"))
				continue
			}
			conn.Write([]byte("OK\n"))

		case "REMOVENODE":
			if len(cmd) != 2 {
				conn.Write([]byte("ERR invalid Remove Node command\n"))
				continue
			}
			err := hR.removeNode(cmd[1])
			if err != nil {
				conn.Write([]byte("ERR " + err.Error() + "\n"))
				continue
			}
			conn.Write([]byte("OK\n"))

		case "GETRAM":
			if len(cmd) != 1 {
				conn.Write([]byte("ERR invalid Get Ram command\n"))
				continue
			}
			value := hR.getRam()
			conn.Write([]byte(value + "\n"))

		case "GETCPU":
			if len(cmd) != 1 {
				conn.Write([]byte("ERR invalid Get Cpu command\n"))
				continue
			}
			value := hR.getCpu()
			conn.Write([]byte(value + "\n"))

		default:
			conn.Write([]byte("ERR unknown command\n"))
		}
	}
}

type Config struct {
	Nodes  []string `yaml:"nodes"`
	Shards int      `yaml:"shards"`
	Memory struct {
		Swappiness int `yaml:"swappiness"`
	} `yaml:"memory"`
}

func main() {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		panic(err)
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	hashRing := newHashRing(cfg.Nodes)

	for i := range hashRing.ring {
		defer hashRing.ring[i].nodeConn.conn.Close()
	}

	lis, err := net.Listen("tcp", ":8080")
	if err != nil {
		panic(err)
	}

	defer lis.Close()

	for {
		conn, err := lis.Accept()
		if err != nil {
			continue
		}
		go hashRing.handleTcp(conn)
	}
}
