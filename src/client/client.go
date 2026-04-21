// Client
package client

import (
	"log"

	pb "github.com/utterwqlnut/utterdb/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func GetClient(ip string) (pb.NodeClient, *grpc.ClientConn) {
	conn, err := grpc.Dial(
		ip,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("Failed to connect to the grpc server")
	}
	client := pb.NewNodeClient(conn)

	return client, conn
}
