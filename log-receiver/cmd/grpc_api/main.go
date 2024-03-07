package main

import (
	"context"
	"log"
	"net"
	"time"

	"log-receiver/internal/data"
	"log-receiver/internal/driver"

	pb "github.com/artemparygin/troubleshooting/protos/logs"

	"google.golang.org/grpc"
)

type logServiceServer struct {
	pb.UnimplementedLogServiceServer
	dataChannel chan data.Log
	models      data.Models
}

func (s *logServiceServer) flushDataToClickHouse() {
	buffer := make([]data.Log, 0, 100)
	for msg := range s.dataChannel {
		buffer = append(buffer, msg)

		if len(buffer) >= 1 {
			new_buff := make([]data.Log, 0, len(buffer))
			new_buff = append(new_buff, buffer...)

			go s.flushBufferToClickHouse(new_buff)
			buffer = buffer[:0]
		}
	}
}

func (s *logServiceServer) flushBufferToClickHouse(buffer []data.Log) {
	s.models.Log.InsertLogData(context.Background(), buffer)
}

func (s *logServiceServer) LogMessage(ctx context.Context, req *pb.LogMessageRequest) (*pb.LogMessageResponse, error) {
	log.Printf("Received log message: %s", req.Message)
	s.dataChannel <- data.Log{
		Timestamp: req.Timestamp.AsTime(),
		Message:   req.Message,
	}
	return &pb.LogMessageResponse{Success: true}, nil
}

func (s *logServiceServer) SendSpans(ctx context.Context, req *pb.Spans) (*pb.LogMessageResponse, error) {
	s.models.Log.InsertTraceData(context.Background(), req.Spans)
	return &pb.LogMessageResponse{Success: true}, nil
}

func main() {
	clickhouse, err := driver.Connect()

	ctx := context.Background()
	driver.CreateTracer(ctx, clickhouse)
	driver.СreateLogs(ctx, clickhouse)

	if err != nil {
		log.Fatalf("Failed to connect to clickhouse")
	}

	lis, err := net.Listen("tcp", ":50055")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	srv := logServiceServer{
		models: data.New(clickhouse),
	}

	srv.dataChannel = make(chan data.Log, 1000)
	go func() {
		for {
			time.Sleep(time.Second * 5)
			srv.flushDataToClickHouse()
		}
	}()

	pb.RegisterLogServiceServer(grpcServer, &srv)

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
