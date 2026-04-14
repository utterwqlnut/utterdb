package server

import (
	pb "github.com/utterwqlnut/utterdb/protos"
)

type server struct {
	pb.UnimplementedNodeServer
	kv *internalKeyValueStore
}

func (s *server) Get(rq *pb.Request) (*pb.Value, error) {
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

func (s *server) Write(data *pb.Data) error {
	key, keyErr := ParseToStringable(data.Key, data.KeyType)
	value, valErr := ParseToStringable(data.Value, data.Value)

	if keyErr != nil {
		return keyErr
	}

	if valErr != nil {
		return valErr
	}

	s.kv.write(key, value)
	return nil
}

func (s *server) Erase(rq *pb.Request) error {
	key, err1 := ParseToStringable(rq.Key, rq.Type)

	if err1 != nil {
		return err1
	}

	err2 := s.kv.erase(key)

	return err2

}

func (s *server) InitiateMove(nodeT *pb.NodeT) error {

}

func (s *server) MoveData(vnode *pb.VNode) (*pb.Data, error) {

}

func (s *server) RamUse() (*pb.Float, error) {

}

func (s *server) CpuUse() (*pb.Float, error) {

}
