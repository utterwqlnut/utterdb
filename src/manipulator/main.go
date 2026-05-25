package main

import (
	"bufio"
	"net"
	"os"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/utterwqlnut/utterdb/src/config"
	"github.com/utterwqlnut/utterdb/src/hashing"
	"gopkg.in/yaml.v3"
)

func sendAllNodesHashRing(hR *hashing.HashRing, proxies []string) {
	data := []byte("NEWRING" + "|" + hR.String() + "\n")

	for _, addr := range proxies {
		addr := addr // capture for closure

		go func() {
			for i := 0; i < 5; i++ {
				conn, err := net.Dial("tcp", addr)
				if err != nil {
					time.Sleep(20 * time.Millisecond)
					continue
				}

				_, err = conn.Write(data)
				conn.Close()

				if err != nil {
					time.Sleep(20 * time.Millisecond)
					continue
				}
				return
			}
		}()
	}
}

func handleTcp(hR *hashing.HashRing, conn net.Conn, proxies []string) {
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

		case "ADDNODE":
			if len(cmd) != 2 {
				conn.Write([]byte("ERR invalid Add Node command\n"))
				continue
			}
			err := hR.AddNode(cmd[1])
			if err != nil {
				conn.Write([]byte("ERR " + err.Error() + "\n"))
				continue
			}
			go sendAllNodesHashRing(hR, proxies)
			conn.Write([]byte("OK\n"))

		case "REMOVENODE":
			if len(cmd) != 2 {
				conn.Write([]byte("ERR invalid Remove Node command\n"))
				continue
			}
			err := hR.RemoveNode(cmd[1])
			if err != nil {
				conn.Write([]byte("ERR " + err.Error() + "\n"))
				continue
			}
			go sendAllNodesHashRing(hR, proxies)
			conn.Write([]byte("OK\n"))

		default:
			conn.Write([]byte("ERR unknown command\n"))
		}
	}
}

func main() {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		panic(err)
	}

	var cfg config.Config
	err = yaml.Unmarshal(data, &cfg)
	replicationFactor := cfg.ReplicationFactor
	if replicationFactor < 1 {
		replicationFactor = 1
	}
	hashRing := hashing.NewHashRing(cfg.Nodes, replicationFactor)

	for i := range hashRing.Ring {
		defer hashRing.Ring[i].NodeConn.Conn.Close()
	}

	fails := make(map[string]int)

	c := cron.New()
	c.AddFunc("@every 5s", func() { hashRing.HeartBeat(cfg.Proxies, fails) })
	c.AddFunc("@every 5s", func() { sendAllNodesHashRing(hashRing, cfg.Proxies) })
	c.Start()

	args := os.Args
	lis, err := net.Listen("tcp", args[1])
	if err != nil {
		panic(err)
	}

	defer lis.Close()

	for {
		conn, err := lis.Accept()
		if err != nil {
			continue
		}
		go handleTcp(hashRing, conn, cfg.Proxies)
	}
}
