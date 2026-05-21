package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/utterwqlnut/utterdb/src/config"
	"github.com/utterwqlnut/utterdb/src/hashing"
	"gopkg.in/yaml.v3"
)

var GLOBAL_LOCK sync.RWMutex = sync.RWMutex{}

func forwardToManipulator(addr string, msg string) (string, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	_, err = conn.Write([]byte(msg + "\n"))
	if err != nil {
		return "", err
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}

	return string(buf[:n]), nil
}
func handleTcp(hR **hashing.HashRing, conn net.Conn, manipulator string) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for {
		msg, err := reader.ReadString('\n') // CMD|ARG1|ARG2...

		if err != nil {
			return // connection closed
		}

		msg = strings.TrimSpace(msg)
		cmd := strings.Split(msg, "|")
		fmt.Println(cmd[0])
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
			GLOBAL_LOCK.RLock()
			err := (*hR).Write(cmd[1], cmd[2], cmd[3], cmd[4])
			GLOBAL_LOCK.RUnlock()
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
			GLOBAL_LOCK.RLock()
			value, err := (*hR).Get(cmd[1], cmd[2])
			GLOBAL_LOCK.RUnlock()

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
			GLOBAL_LOCK.RLock()
			err := (*hR).Erase(cmd[1], cmd[2])
			GLOBAL_LOCK.RUnlock()
			if err != nil {
				conn.Write([]byte("ERR " + err.Error() + "\n"))
				continue
			}
			conn.Write([]byte("OK\n"))

		case "NEWRING":
			fmt.Println("Recieved here")
			GLOBAL_LOCK.Lock()
			(*hR) = hashing.FromString(cmd[1])
			GLOBAL_LOCK.Unlock()

		case "ADDNODE":
			if len(cmd) != 2 {
				conn.Write([]byte("ERR invalid AddNode command\n"))
				continue
			}
			GLOBAL_LOCK.RLock()
			resp, err := forwardToManipulator(manipulator, "ADDNODE|"+cmd[1])
			GLOBAL_LOCK.RUnlock()
			if err != nil {
				conn.Write([]byte("ERR " + err.Error() + "\n"))
				continue
			}

			conn.Write([]byte(resp))

		case "REMOVENODE":
			if len(cmd) != 2 {
				conn.Write([]byte("ERR invalid RemoveNode command\n"))
				continue
			}
			GLOBAL_LOCK.RLock()
			resp, err := forwardToManipulator(manipulator, "REMOVENODE|"+cmd[1])
			GLOBAL_LOCK.RUnlock()
			if err != nil {
				conn.Write([]byte("ERR " + err.Error() + "\n"))
				continue
			}

			conn.Write([]byte(resp))

		case "GETRAM":
			if len(cmd) != 1 {
				conn.Write([]byte("ERR invalid Get Ram command\n"))
				continue
			}
			GLOBAL_LOCK.RLock()
			value := (*hR).GetRam()
			GLOBAL_LOCK.RUnlock()
			conn.Write([]byte(value + "\n"))

		case "GETCPU":
			if len(cmd) != 1 {
				conn.Write([]byte("ERR invalid Get Cpu command\n"))
				continue
			}
			GLOBAL_LOCK.RLock()
			value := (*hR).GetCpu()
			GLOBAL_LOCK.RUnlock()
			conn.Write([]byte(value + "\n"))

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
	hashRing := hashing.NewHashRing(cfg.Nodes)

	for i := range hashRing.Ring {
		defer hashRing.Ring[i].NodeConn.Conn.Close()
	}
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

		go handleTcp(&hashRing, conn, cfg.Manipulator)
	}
}
