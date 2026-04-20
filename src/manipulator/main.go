package main

import (
	"os"
	"strconv"
)

type Node struct {
	ip        string
	name      string
	hashStart uint64
	hashEnd   uint64
}

func main() {
	args := os.Args
	nodes := make([]Node, 0)

	for i := 1; i < len(args); i++ {
		nodes = append(nodes, Node{ip: args[i], name: "node_" + strconv.Itoa(i)})
	}

}
