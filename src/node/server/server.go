package server

import (
	"context"
	"io"

	pb "github.com/utterwqlnut/utterdb/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Server struct {
	pb.UnimplementedNodeServer
	kv           *internalKeyValueStore
	migrating    bool
	migrateStart uint64
	migrateEnd   uint64
	migrateShard int
	ip           string
}

func NewNodeServer(shards int, ip string) *Server {
	return &Server{
		kv:        newInternalKeyValueStore(shards),
		migrating: false,
		ip:        ip,
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

	if keyErr != nil {
		return &pb.Empty{}, keyErr
	}

	if valErr != nil {
		return &pb.Empty{}, valErr
	}

	s.kv.write(key, value, s.migrating, s.migrateShard, s.migrateStart, s.migrateEnd)
	return &pb.Empty{}, nil
}

func (s *Server) Erase(ctx context.Context, rq *pb.Request) (*pb.Empty, error) {
	key, err1 := ParseToStringable(rq.Key, rq.Type)

	if err1 != nil {
		return &pb.Empty{}, err1
	}

	err2 := s.kv.erase(key, s.migrating, s.migrateShard, s.migrateStart, s.migrateEnd)

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

func (s *Server) MoveData(dataReq *pb.DataStreamReq, stream grpc.ServerStreamingServer[pb.Data]) error {
	s.migrateStart = dataReq.Start
	s.migrateEnd = dataReq.End
	s.migrating = true

	for i := 0; i < s.kv.shards; i++ {
		s.migrateShard = i
		mp := s.kv.getSnapShot(i, s.migrateStart, s.migrateEnd)

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
	}
	conn, err := grpc.Dial(
		dataReq.SourceIp,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewNodeClient(conn)
	ctx := context.Background()

	// Now for going through the log
	s.migrating = false
	for _, function := range s.kv.log {
		switch function.methodName {
		case "write":
			client.Write(ctx,
				&pb.Data{
					Key:       function.key.Stringify(),
					KeyType:   getTypeString(function.key),
					Value:     function.value.Stringify(),
					ValueType: getTypeString(function.value),
				})
		case "erase":
			client.Erase(ctx,
				&pb.Request{
					Key:  function.key.Stringify(),
					Type: getTypeString(function.key),
				})
		}
	}
	s.kv.clearLog()

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

		s.kv.write(key, value, false, 0, 0, 0)
	}

	return &pb.Empty{}, nil
}

func (s *Server) RamUse(ctx context.Context, _ *pb.Empty) (*pb.Float, error) {
	return &pb.Float{Value: s.kv.getRamUse()}, nil
}

func (s *Server) CpuUse(ctx context.Context, _ *pb.Empty) (*pb.Float, error) {
	return &pb.Float{Value: s.kv.getCpuUse()}, nil
}
