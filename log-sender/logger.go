package sender

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"

	pb "github.com/artemparygin/troubleshooting/protos/logs"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ClickHouseWriter is a custom Zerolog writer for sending logs to ClickHouse.
type ClickHouseWriter struct {
	url    string
	client *http.Client
}

// NewClickHouseWriter creates a new ClickHouseWriter.
func NewClickHouseWriter(url string) *ClickHouseWriter {
	return &ClickHouseWriter{
		url:    url,
		client: &http.Client{},
	}
}

const (
	defaultName = "world"
)

var (
	addr = flag.String("addr", "localhost:50051", "the address to connect to")
	name = flag.String("name", defaultName, "Name to greet")
)

// Define a struct for automatic parsing
type LogMessage struct {
	Level    string    `json:"level"`
	Time     time.Time `json:"time"`
	Message  string    `json:"message"`
	TraceId  string    `json:"trace_id"`
	File     string    `json:"file"`
	Function string    `json:"function"`
	Line     int       `json:"line"`
}

func writeLogToBackend(message []byte) {
	flag.Parse()
	conn, err := grpc.Dial(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
	}
	defer conn.Close()
	c := pb.NewLogServiceClient(conn)

	var logData LogMessage
	err = json.Unmarshal(message, &logData)
	if err != nil {
		fmt.Printf("Error encoding to JSON: %v\n", err)
	}

	response, err := c.LogMessage(context.Background(), &pb.LogMessageRequest{Message: logData.Message, Timestamp: timestamppb.New(logData.Time)})
	if err != nil {
		// log.Fatalf("Error sending log message: %v", err)
	}

	if response.Success {
		// log.Println("Log message sent successfully")
	}
}

func (w *ClickHouseWriter) Write(p []byte) (n int, err error) {

	return len(p), nil
}

type CustomHook struct {
}

func (h CustomHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	pc, file, line, _ := runtime.Caller(3)
	functionName := runtime.FuncForPC(pc).Name()
	// Add context information to the log entry
	e.Str("file", file)
	e.Str("function", functionName)
	e.Int("line", line)
}

// NewCustomLogger creates a new CustomLogger.
func NewCustomLogger() *CustomLogger {
	clickHouseWriter := NewClickHouseWriter("http://your-clickhouse-endpoint")

	return &CustomLogger{
		Logger: zerolog.New(clickHouseWriter).Hook(CustomHook{}).With().Timestamp().Logger(),
	}
}

type CustomLogger struct {
	zerolog.Logger
}

// InfoWithContext logs an info-level message with OpenTelemetry trace information.
func (l *CustomLogger) InfoWithContext(ctx context.Context) *zerolog.Event {
	span := trace.SpanFromContext(ctx)
	return l.Logger.Info().Str("trace_id", span.SpanContext().TraceID().String())
}

// DebugWithContext logs a debug-level message with OpenTelemetry trace information.
func (l *CustomLogger) DebugWithContext(ctx context.Context) *zerolog.Event {
	span := trace.SpanFromContext(ctx)
	return l.Logger.Debug().Str("trace_id", span.SpanContext().TraceID().String())
}

// DebugWithContext logs a debug-level message with OpenTelemetry trace information.
func (l *CustomLogger) WarningWithContext(ctx context.Context) *zerolog.Event {
	span := trace.SpanFromContext(ctx)
	return l.Logger.Warn().Str("trace_id", span.SpanContext().TraceID().String())
}
