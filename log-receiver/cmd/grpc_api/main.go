package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"log-receiver/internal/batch"
	"log-receiver/internal/data"
	"log-receiver/internal/driver"

	pb "github.com/agerimex/troubleshooting/protos/logs"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	logBatchSize     = 100
	logBatchInterval = time.Second
)

type logServiceServer struct {
	pb.UnimplementedLogServiceServer
	logs   *batch.Batcher[data.Log]
	models data.Models
}

func (s *logServiceServer) LogMessage(ctx context.Context, req *pb.LogMessageRequest) (*pb.LogMessageResponse, error) {
	log.Printf("Received log message: %s", req.Message)
	err := s.logs.Add(ctx, data.Log{
		Timestamp: req.Timestamp.AsTime(),
		Message:   req.Message,
	})
	if err != nil {
		return nil, status.FromContextError(err).Err()
	}
	return &pb.LogMessageResponse{Success: true}, nil
}

func (s *logServiceServer) SendSpans(ctx context.Context, req *pb.Spans) (*pb.LogMessageResponse, error) {
	if err := s.models.Log.InsertTraceData(ctx, req.Spans); err != nil {
		log.Printf("Failed to insert %d spans: %v", len(req.Spans), err)
		return nil, status.Errorf(codes.Internal, "insert spans: %v", err)
	}
	return &pb.LogMessageResponse{Success: true}, nil
}

func getenv(key, def string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return def
}

func main() {
	clickhouse, err := driver.Connect(driver.ConfigFromEnv())
	if err != nil {
		log.Fatalf("Failed to connect to clickhouse: %v", err)
	}

	ctx := context.Background()
	driver.CreateTracer(ctx, clickhouse)
	driver.СreateLogs(ctx, clickhouse)

	listenAddr := getenv("LISTEN_ADDR", ":50055")
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	var serverOptions []grpc.ServerOption
	if token := os.Getenv(tokenEnv); token != "" {
		serverOptions = append(serverOptions, grpc.UnaryInterceptor(tokenAuth(token)))
		log.Printf("%s is set: senders must present the token", tokenEnv)
	}
	grpcServer := grpc.NewServer(serverOptions...)

	models := data.New(clickhouse)
	logs := batch.New(logBatchSize, logBatchInterval, func(batch []data.Log) {
		models.Log.InsertLogData(context.Background(), batch)
	})
	pb.RegisterLogServiceServer(grpcServer, &logServiceServer{models: models, logs: logs})

	go func() {
		stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		<-stop.Done()
		log.Println("Shutting down: finishing in-flight requests")
		grpcServer.GracefulStop()
	}()

	log.Printf("Receiver listening on %s", listenAddr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
	// Serve returns after GracefulStop, when no request can add logs any more.
	logs.Close()
}
