package server

import (
	"context"
	"io"
	"sync"

	pb "github.com/utterwqlnut/utterdb/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Server struct {
	pb.UnimplementedNodeServer
	kv            *internalKeyValueStore
	migrating     bool
	migrateLock   sync.RWMutex
	ip            string
	migrateIp     string
	migrateClient pb.NodeClient
}

func NewNodeServer(shards int, ip string) *Server {
	return &Server{
		kv:          newInternalKeyValueStore(shards),
		migrating:   false,
		ip:          ip,
		migrateLock: sync.RWMutex{},
	}
}
func (s *Server) Health(ctx context.Context, _ *pb.Empty) (*pb.Empty, error) {
	return &pb.Empty{}, nil
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

	if keyErr != nil {
		return &pb.Empty{}, keyErr
	}

	if valErr != nil {
		return &pb.Empty{}, valErr
	}

	s.kv.write(key, value)

	s.migrateLock.RLock()
	if s.migrating {
		_, err := s.migrateClient.Write(ctx, data)
		if err != nil {
			return nil, err
		}
	}
	s.migrateLock.RUnlock()

	return &pb.Empty{}, nil
}

func (s *Server) Erase(ctx context.Context, rq *pb.Request) (*pb.Empty, error) {
	key, err1 := ParseToStringable(rq.Key, rq.Type)

	if err1 != nil {
		return &pb.Empty{}, err1
	}

	err2 := s.kv.erase(key, false)

	s.migrateLock.RLock()
	if s.migrating {
		_, err := s.migrateClient.Erase(ctx, rq)
		if err != nil {
			return nil, err
		}
	}
	s.migrateLock.RUnlock()

	return &pb.Empty{}, err2

}

func getTypeString(x Stringable) string {
	var xType string
	switch x.(type) {
	case Int:
		xType = "int"
	case Float:
		xType = "float"
	case String:
		xType = "string"
	}
	return xType
}

func (s *Server) ClearOldData(ctx context.Context, rng *pb.Range) (*pb.Empty, error) {
	for i := 0; i < s.kv.shards; i++ {
		s.kv.lockShard(i, true)
		mp := s.kv.getSnapShot(i, rng.Start, rng.End)

		for key, _ := range mp {
			s.kv.erase(key, true)
		}

		s.kv.unlockShard(i, true)
	}
	return &pb.Empty{}, nil
}

func (s *Server) MoveData(dataReq *pb.DataStreamReq, stream grpc.ServerStreamingServer[pb.Data]) error {
	s.migrateLock.Lock()
	s.migrating = true

	conn, err := grpc.Dial(
		dataReq.SourceIp,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return err
	}
	s.migrateClient = pb.NewNodeClient(conn)
	s.migrateLock.Unlock()

	defer conn.Close()

	for i := 0; i < s.kv.shards; i++ {
		s.kv.lockShard(i, false)
		mp := s.kv.getSnapShot(i, dataReq.Start, dataReq.End)

		for key, value := range mp {
			keyType := getTypeString(key)
			valueType := getTypeString(value)
			stream.Send(&pb.Data{
				Key:       key.Stringify(),
				Value:     value.Stringify(),
				KeyType:   keyType,
				ValueType: valueType,
			})
		}
		s.kv.unlockShard(i, false)
	}

	s.migrateLock.Lock()
	s.migrating = false
	s.migrateLock.Unlock()

	return nil

}

func (s *Server) InitiateMove(ctx context.Context, reb *pb.Rebalance) (*pb.Empty, error) {
	conn, err := grpc.Dial(
		reb.Ip,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return &pb.Empty{}, err
	}
	defer conn.Close()
	client := pb.NewNodeClient(conn)
	dataStreamReq := &pb.DataStreamReq{
		Start:    reb.Start,
		End:      reb.End,
		SourceIp: s.ip,
	}
	stream, err := client.MoveData(ctx, dataStreamReq)

	if err != nil {
		return &pb.Empty{}, err
	}

	for {
		res, err := stream.Recv()

		if err == io.EOF {
			break
		}

		if err != nil {
			return &pb.Empty{}, err
		}

		key, keyErr := ParseToStringable(res.Key, res.KeyType)
		value, valErr := ParseToStringable(res.Value, res.ValueType)

		if keyErr != nil {
			return &pb.Empty{}, keyErr
		}

		if valErr != nil {
			return &pb.Empty{}, valErr
		}

		s.kv.write(key, value)
	}

	return &pb.Empty{}, nil
}

func (s *Server) RamUse(ctx context.Context, _ *pb.Empty) (*pb.Float, error) {
	return &pb.Float{Value: s.kv.getRamUse()}, nil
}

func (s *Server) CpuUse(ctx context.Context, _ *pb.Empty) (*pb.Float, error) {
	return &pb.Float{Value: s.kv.getCpuUse()}, nil
}
