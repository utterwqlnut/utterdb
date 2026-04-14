// Client Test

package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	pb "github.com/utterwqlnut/utterdb/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	conn, err := grpc.Dial(
		"localhost:9001",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("Failed to connect to the grpc server")
	}
	defer conn.Close()

	client := pb.NewNodeClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	for i := 0; i < 1000; i++ {
		_, err = client.Write(ctx, &pb.Data{Key: strconv.Itoa(i), Value: strconv.Itoa(i * 2), KeyType: "int", ValueType: "int"})
		if err != nil {
			fmt.Println(err)
		}
	}

	for i := 0; i < 1000; i++ {
		value, err := client.Get(ctx, &pb.Request{Key: strconv.Itoa(i), Type: "int"})
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(value)
	}

	for i := 0; i < 1000; i++ {
		_, err := client.Erase(ctx, &pb.Request{Key: strconv.Itoa(i), Type: "int"})
		if err != nil {
			fmt.Println(err)
		}
	}

}
