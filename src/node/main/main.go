package main

import (
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"

	pb "github.com/utterwqlnut/utterdb/protos"
	"github.com/utterwqlnut/utterdb/src/node/server"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Nodes  []string `yaml:"nodes"`
	Memory struct {
		Swappiness int `yaml:"swappiness"`
	} `yaml:"memory"`
}

func main() {
	// Carrying out config
	data, err := os.ReadFile("config.yaml")

	if err != nil {
		log.Fatalf("Failed to find file")
	}

	var config Config
	err = yaml.Unmarshal(data, &config)

	if err != nil {
		log.Fatalf("Failed to read yaml")
	}

	exec.Command("sysctl", "vm.swappiness="+strconv.Itoa(config.Memory.Swappiness)).Run()

	// Starting Server
	args := os.Args
	lis, err := net.Listen("tcp", args[1])
	if err != nil {
		log.Fatalf("Failed to start tcp server")
	}

	ipCommand := exec.Command("hostname", "-I")
	ip, _ := ipCommand.Output()

	grpcServer := grpc.NewServer()
	keyValueServer := server.NewNodeServer(2, string(ip))

	pb.RegisterNodeServer(grpcServer, keyValueServer)

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to start grpc server")
	}
}
