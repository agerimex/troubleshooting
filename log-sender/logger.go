package sender

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

// ClickHouseWriter is an unfinished early integration and stores nothing.
//
// Deprecated: use NewLogWriter with a LogExporter.
type ClickHouseWriter struct {
	url    string
	client *http.Client
}

// Deprecated: use NewLogWriter with a LogExporter.
func NewClickHouseWriter(url string) *ClickHouseWriter {
	return &ClickHouseWriter{
		url:    url,
		client: &http.Client{},
	}
}

// LogMessage is the JSON shape of the early zerolog integration.
//
// Deprecated: use LogRecord.
type LogMessage struct {
	Level    string    `json:"level"`
	Time     time.Time `json:"time"`
	Message  string    `json:"message"`
	TraceId  string    `json:"trace_id"`
	File     string    `json:"file"`
	Function string    `json:"function"`
	Line     int       `json:"line"`
}

// Write discards p.
func (w *ClickHouseWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

// CustomHook adds the caller's file, function and line to every event.
type CustomHook struct {
}

func (h CustomHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	pc, file, line, _ := runtime.Caller(3)
	functionName := runtime.FuncForPC(pc).Name()
	e.Str("file", file)
	e.Str("function", functionName)
	e.Int("line", line)
}

// CustomLogger adds the trace_id and span_id of the span in ctx. With a
// LogWriter, prefer logger.Info().Ctx(ctx) and TraceHook.
type CustomLogger struct {
	zerolog.Logger
}

func withSpan(e *zerolog.Event, ctx context.Context) *zerolog.Event {
	// Without a valid span nothing is added: an all-zero id would be rejected.
	if sc := trace.SpanFromContext(ctx).SpanContext(); sc.IsValid() {
		e = e.Str("trace_id", sc.TraceID().String()).Str("span_id", sc.SpanID().String())
	}
	return e
}

func (l *CustomLogger) InfoWithContext(ctx context.Context) *zerolog.Event {
	return withSpan(l.Logger.Info(), ctx)
}

func (l *CustomLogger) DebugWithContext(ctx context.Context) *zerolog.Event {
	return withSpan(l.Logger.Debug(), ctx)
}

func (l *CustomLogger) WarningWithContext(ctx context.Context) *zerolog.Event {
	return withSpan(l.Logger.Warn(), ctx)
}

func (l *CustomLogger) ErrorWithContext(ctx context.Context) *zerolog.Event {
	return withSpan(l.Logger.Error(), ctx)
}
