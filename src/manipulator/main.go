package main

import (
	"os"
	"sort"
	"strconv"

	"github.com/spaolacci/murmur3"
)

type Node struct {
	ip   string
	name string
	hash uint64
}

type HashRing struct {
	ring []Node
}

func (hR *HashRing) sort() {
	sort.Slice(hR.ring, func(i, j int) bool {
		return hR.ring[i].hash < hR.ring[j].hash
	})
}

func (hR *HashRing) addNode(n Node) (nodeBefore Node, startHash uint64, endHash uint64) {
	hR.ring = append(hR.ring, n)
	hR.sort()

	idx := sort.Search(len(hR.ring),func(i int) bool {
		return hR.ring[i].hash == n.hash
	})

	before := ((idx - 1) + len(hR.ring)) % (len(hR.ring)) // Find the node before this new added node
	after := (idx + 1) % len(hR.ring)

	return hR.ring[before], n.hash, hR.ring[after].hash
}

func (hR *HashRing) getNode(key string) Node {
	keyHash := murmur3.Sum64([]byte(key))
	idx := sort.Search(len(hR.ring),func(i int) bool {
		return hR.ring[i].hash >= keyHash
	})

	return hR.ring[idx]
}

func newNode(ip string, name string) *Node {
	return &Node{
		ip:   ip,
		name: name,
		hash: murmur3.Sum64([]byte(name)),
	}
}

func main() {
	args := os.Args
	nodes := make([]Node, 0)

	for i := 1; i < len(args); i++ {
		nodes = append(nodes, Node{ip: args[i], name: "node_" + strconv.Itoa(i)})
	}

}
