package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
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
	ring []*Node
}

func newHashRing(nodeNames []string) *HashRing {
	nodes := make([]*Node, 0)

	for i := 0; i < len(nodeNames); i++ {
		nodes = append(nodes, newNode(nodeNames[i], "node_"+strconv.Itoa(i)))
	}

	return &HashRing{nodes}
}

func (hR *HashRing) sort() {
	sort.Slice(hR.ring, func(i, j int) bool {
		return hR.ring[i].hash < hR.ring[j].hash
	})
}

func (hR *HashRing) addNodeHelper(n Node) (nodeBefore *Node, startHash uint64, endHash uint64) {
	idx := sort.Search(len(hR.ring), func(i int) bool {
		return hR.ring[i].hash >= n.hash
	})

	before := ((idx - 1) + len(hR.ring)) % (len(hR.ring)) // Find the node before this new added node
	after := (idx + 1) % len(hR.ring)

	return hR.ring[before], n.hash, hR.ring[after].hash
}

func (hR *HashRing) removeNodeHelper(n Node) (nodeBefore *Node, startHash uint64, endHash uint64) {
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

func (hR *HashRing) makeWrite() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		key := r.URL.Query().Get("key")
		value := r.URL.Query().Get("value")
		keyType := r.URL.Query().Get("keyType")
		valueType := r.URL.Query().Get("valueType")

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		node := hR.getNode(key)
		_, err := node.nodeConn.client.Write(ctx, &pb.Data{Key: key,
			KeyType: keyType, Value: value, ValueType: valueType})

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
}

func (hR *HashRing) makeErase() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		key := r.URL.Query().Get("key")
		keyType := r.URL.Query().Get("keyType")

		node := hR.getNode(key)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		_, err := node.nodeConn.client.Erase(ctx, &pb.Request{Key: key,
			Type: keyType})

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
}

func (hR *HashRing) makeGet() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		key := r.URL.Query().Get("key")
		keyType := r.URL.Query().Get("keyType")

		node := hR.getNode(key)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		fmt.Println(node.name)
		value, err := node.nodeConn.client.Get(ctx, &pb.Request{Key: key,
			Type: keyType})

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		fmt.Fprintln(w, value.Value)
	}
}

func (hR *HashRing) makeGetRam() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		ramUseMap := make(map[string]string)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		for i := range hR.ring {
			ramUse, _ := hR.ring[i].nodeConn.client.RamUse(ctx, &pb.Empty{})
			ramUseMap[hR.ring[i].name] = ramUse.String()
		}

		json.NewEncoder(w).Encode(ramUseMap)
	}
}

func (hR *HashRing) makeGetCpu() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		cpuUseMap := make(map[string]string)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		for i := range hR.ring {
			cpuUse, _ := hR.ring[i].nodeConn.client.CpuUse(ctx, &pb.Empty{})
			cpuUseMap[hR.ring[i].name] = cpuUse.String()
		}

		json.NewEncoder(w).Encode(cpuUseMap)
	}
}

type Config struct {
	Nodes  []string `yaml:"nodes"`
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

	http.HandleFunc("/erase", hashRing.makeErase())
	http.HandleFunc("/write", hashRing.makeWrite())
	http.HandleFunc("/get", hashRing.makeGet())
	http.HandleFunc("/get-cpu", hashRing.makeGetCpu())
	http.HandleFunc("/get-ram", hashRing.makeGetRam())
	http.ListenAndServe(":8080", nil)
}
