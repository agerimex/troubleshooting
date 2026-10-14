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

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	logBatchSize     = 100
	logBatchInterval = time.Second
	logInsertTimeout = 30 * time.Second
)

type logServiceServer struct {
	pb.UnimplementedLogServiceServer
	logs   *batch.Batcher[data.LogRecord]
	models data.Models
}

// LogMessage stores one record. It is batched with other single records and
// answers only after its batch was written: an error (or a deadline) means the
// record is not confirmed, and the sender should retry it with the same
// record_id.
func (s *logServiceServer) LogMessage(ctx context.Context, req *pb.LogMessageRequest) (*pb.LogMessageResponse, error) {
	record, err := data.LogRecordFromProto(req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "log record: %v", err)
	}
	if err := s.logs.Add(ctx, record); err != nil {
		if ctx.Err() != nil {
			return nil, status.FromContextError(err).Err()
		}
		return nil, status.Errorf(codes.Unavailable, "insert log record: %v", err)
	}
	return &pb.LogMessageResponse{Success: true}, nil
}

// SendLogs stores a batch in one insert and answers after it was written. An
// invalid record rejects the whole batch, so nothing is stored partially.
func (s *logServiceServer) SendLogs(ctx context.Context, req *pb.Logs) (*pb.LogMessageResponse, error) {
	records := make([]data.LogRecord, 0, len(req.GetLogs()))
	for i, l := range req.GetLogs() {
		record, err := data.LogRecordFromProto(l)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "log record %d: %v", i, err)
		}
		records = append(records, record)
	}
	if err := s.models.Log.InsertLogRecords(ctx, records); err != nil {
		log.Printf("Failed to insert %d log records: %v", len(records), err)
		// Unavailable tells the sender the request may be retried.
		return nil, status.Errorf(codes.Unavailable, "insert log records: %v", err)
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

	retention, lateness, err := driver.RetentionFromEnv()
	if err != nil {
		log.Fatalf("Retention: %v", err)
	}
	if err := driver.EnsureSchema(context.Background(), clickhouse, retention); err != nil {
		log.Fatalf("Schema: %v", err)
	}
	log.Printf("Retention: traces %s, logs %s, statistics %s (allowed lateness %s)", retention.Traces, retention.Logs, retention.Stats, lateness)

	listenAddr := getenv("LISTEN_ADDR", ":50055")
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	// OTLP clients default to port 4317. Both ports serve both services.
	otlpListenAddr := getenv("OTLP_LISTEN_ADDR", ":4317")
	otlpLis, err := net.Listen("tcp", otlpListenAddr)
	if err != nil {
		log.Fatalf("Failed to listen for OTLP: %v", err)
	}

	var serverOptions []grpc.ServerOption
	if token := os.Getenv(tokenEnv); token != "" {
		serverOptions = append(serverOptions, grpc.UnaryInterceptor(tokenAuth(token)))
		log.Printf("%s is set: senders must present the token", tokenEnv)
	}
	grpcServer := grpc.NewServer(serverOptions...)

	models := data.New(clickhouse)
	go heartbeat(models)
	logs := batch.New(logBatchSize, logBatchInterval, func(batch []data.LogRecord) error {
		ctx, cancel := context.WithTimeout(context.Background(), logInsertTimeout)
		defer cancel()
		if err := models.Log.InsertLogRecords(ctx, batch); err != nil {
			log.Printf("Failed to insert %d log records: %v", len(batch), err)
			return err
		}
		return nil
	})
	pb.RegisterLogServiceServer(grpcServer, &logServiceServer{models: models, logs: logs})
	collectortracepb.RegisterTraceServiceServer(grpcServer, &otlpTraceServer{models: models})

	go func() {
		stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		<-stop.Done()
		log.Println("Shutting down: finishing in-flight requests")
		grpcServer.GracefulStop()
	}()

	go func() {
		log.Printf("OTLP trace receiver listening on %s", otlpListenAddr)
		if err := grpcServer.Serve(otlpLis); err != nil {
			log.Fatalf("Failed to serve OTLP: %v", err)
		}
	}()

	log.Printf("Receiver listening on %s", listenAddr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
	// Serve returns after GracefulStop, when no request can add logs any more.
	logs.Close()
}

// heartbeatInterval is how often the receiver records that it is running.
// Statistics treat a minute without a heartbeat as a collection gap.
const heartbeatInterval = 15 * time.Second

func heartbeat(models data.Models) {
	name, err := os.Hostname()
	if err != nil {
		name = "log-receiver"
	}
	for {
		ctx, cancel := context.WithTimeout(context.Background(), heartbeatInterval)
		if err := models.Log.Heartbeat(ctx, name); err != nil {
			log.Printf("Heartbeat: %v", err)
		}
		cancel()
		time.Sleep(heartbeatInterval)
	}
}
