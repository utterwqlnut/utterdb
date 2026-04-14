package server

import (
	"context"
	"fmt"

	pb "github.com/utterwqlnut/utterdb/protos"
)

type Server struct {
	pb.UnimplementedNodeServer
	kv *internalKeyValueStore
}

func NewNodeServer() *Server {
	return &Server{
		kv: newInternalKeyValueStore(),
	}
}

func (s *Server) Get(ctx context.Context, rq *pb.Request) (*pb.Value, error) {
	key, err1 := ParseToStringable(rq.Key, rq.Type)

	if err1 != nil {
		return nil, err1
	}

	value, err2 := s.kv.get(key)

	if err2 != nil {
		return nil, err2
	}

	returnVal := pb.Value{
		Value: value.Stringify(),
	}

	return &returnVal, nil
}

func (s *Server) Write(ctx context.Context, data *pb.Data) (*pb.Empty, error) {
	key, keyErr := ParseToStringable(data.Key, data.KeyType)
	value, valErr := ParseToStringable(data.Value, data.ValueType)
	fmt.Printf("%T\n", key)

	if keyErr != nil {
		return &pb.Empty{}, keyErr
	}

	if valErr != nil {
		return &pb.Empty{}, valErr
	}

	s.kv.write(key, value)
	return &pb.Empty{}, nil
}

func (s *Server) Erase(ctx context.Context, rq *pb.Request) (*pb.Empty, error) {
	key, err1 := ParseToStringable(rq.Key, rq.Type)

	if err1 != nil {
		return &pb.Empty{}, err1
	}

	err2 := s.kv.erase(key)

	return &pb.Empty{}, err2

}

func (s *Server) RamUse(ctx context.Context, _ *pb.Empty) (*pb.Float, error) {
	return &pb.Float{Value: s.kv.getRamUse()}, nil
}

func (s *Server) CpuUse(ctx context.Context, _ *pb.Empty) (*pb.Float, error) {
	return &pb.Float{Value: s.kv.getCpuUse()}, nil
}
