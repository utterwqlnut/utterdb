package main

import (
	"log"
	"os"
	"os/exec"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Memory struct {
		Swappiness int `yaml:"swappiness"`
	} `yaml:"memory"`
}

func main() {
	data, err := os.ReadFile("../config.yaml")

	if err != nil {
		log.Fatalf("Failed to find file")
	}

	var config Config
	err = yaml.Unmarshal(data, &config)

	if err != nil {
		log.Fatalf("Failed to read yaml")
	}

	exec.Command("sysctl", "vm.swappiness="+strconv.Itoa(config.Memory.Swappiness)).Run()
}
