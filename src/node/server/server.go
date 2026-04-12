package server

import (
	pb "github.com/utterwqlnut/utterdb/protos"
)

type server struct {
	pb.UnimplementedNodeServer
	kv *internalKeyValueStore
}

func (s *server) GetValue(*pb.Request) (*pb.Value, error) {

}

func (s *server) InitiateMove(*pb.NodeT) error {

}

func (s *server) MoveData(*pb.VNode) *pb.Data {

}
