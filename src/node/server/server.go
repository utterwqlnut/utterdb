package server

import (
	pb "github.com/utterwqlnut/utterdb/protos"
)

type server struct {
	pb.UnimplementedNodeServer
	kv *internalKeyValueStore
}

func (s *server) Get(*pb.Request) (*pb.Value, error) {

}

func (s *server) Write(*pb.Request) error {

}

func (s *server) Erase(*pb.Data) error {

}

func (s *server) InitiateMove(*pb.NodeT) error {

}

func (s *server) MoveData(*pb.VNode) (*pb.Data, error) {

}
