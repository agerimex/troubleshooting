package sender

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"

	pb "github.com/agerimex/troubleshooting/protos/logs"

	"google.golang.org/protobuf/types/known/timestamppb"
)

type ClickHouseWriter struct {
	url    string
	client *http.Client
}

func NewClickHouseWriter(url string) *ClickHouseWriter {
	return &ClickHouseWriter{
		url:    url,
		client: &http.Client{},
	}
}

type LogMessage struct {
	Level    string    `json:"level"`
	Time     time.Time `json:"time"`
	Message  string    `json:"message"`
	TraceId  string    `json:"trace_id"`
	File     string    `json:"file"`
	Function string    `json:"function"`
	Line     int       `json:"line"`
}

// writeLogToBackend is not wired up yet (see "UI for find zerolog by trace" in
// the README). It takes the client so it can share the exporter's connection.
func writeLogToBackend(ctx context.Context, client pb.LogServiceClient, message []byte) error {
	var logData LogMessage
	if err := json.Unmarshal(message, &logData); err != nil {
		return fmt.Errorf("troubleshooting: decode log message: %w", err)
	}

	_, err := client.LogMessage(ctx, &pb.LogMessageRequest{Message: logData.Message, Timestamp: timestamppb.New(logData.Time)})
	return err
}

func (w *ClickHouseWriter) Write(p []byte) (n int, err error) {

	return len(p), nil
}

type CustomHook struct {
}

func (h CustomHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	pc, file, line, _ := runtime.Caller(3)
	functionName := runtime.FuncForPC(pc).Name()
	e.Str("file", file)
	e.Str("function", functionName)
	e.Int("line", line)
}

type CustomLogger struct {
	zerolog.Logger
}

func (l *CustomLogger) InfoWithContext(ctx context.Context) *zerolog.Event {
	span := trace.SpanFromContext(ctx)
	return l.Logger.Info().Str("trace_id", span.SpanContext().TraceID().String())
}

func (l *CustomLogger) DebugWithContext(ctx context.Context) *zerolog.Event {
	span := trace.SpanFromContext(ctx)
	return l.Logger.Debug().Str("trace_id", span.SpanContext().TraceID().String())
}

func (l *CustomLogger) WarningWithContext(ctx context.Context) *zerolog.Event {
	span := trace.SpanFromContext(ctx)
	return l.Logger.Warn().Str("trace_id", span.SpanContext().TraceID().String())
}
