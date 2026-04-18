// Client Test

package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	pb "github.com/utterwqlnut/utterdb/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func getClient(ip string) (pb.NodeClient, *grpc.ClientConn, context.Context, context.CancelFunc) {
	conn, err := grpc.Dial(
		ip,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("Failed to connect to the grpc server")
	}
	client := pb.NewNodeClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)

	return client, conn, ctx, cancel
}

func main() {
	args := os.Args
	client1, conn, ctx1, cancel1 := getClient("localhost" + args[1])

	defer conn.Close()
	defer cancel1()

	for i := 0; i < 1000; i++ {
		_, err := client1.Write(ctx1, &pb.Data{Key: strconv.Itoa(i), Value: strconv.Itoa(i * 2), KeyType: "int", ValueType: "int"})
		if err != nil {
			fmt.Println(err)
		}
	}

	for i := 0; i < 1000; i++ {
		value, err := client1.Get(ctx1, &pb.Request{Key: strconv.Itoa(i), Type: "int"})
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println("Node1", value)
	}
	// Now start a migration

	client2, conn2, ctx2, cancel2 := getClient("localhost" + args[2])

	defer conn2.Close()
	defer cancel2()

	go client2.InitiateMove(ctx1, &pb.Rebalance{
		Ip:    "localhost" + args[1],
		Start: 0,
		End:   math.MaxUint64,
	})

	for i := 0; i < 1000; i++ {
		_, err := client1.Erase(ctx1, &pb.Request{Key: strconv.Itoa(i), Type: "int"})
		if err != nil {
			fmt.Println(err)
		}
	}

	for i := 0; i < 1000; i++ {
		value, err := client2.Get(ctx2, &pb.Request{Key: strconv.Itoa(i), Type: "int"})
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println("Node2", value)
	}
}
