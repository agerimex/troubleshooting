package sender

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/agerimex/troubleshooting/protos/logs"
)

// LogRecord is one log event. RecordID is its identity: a retry must send the
// same RecordID, a separate event gets a new one (NewLogRecord does that).
// TraceID and SpanID are the recorded correlation; leave them empty when the
// record was not written inside a span. Nothing is inferred from timestamps.
type LogRecord struct {
	RecordID    string
	Time        time.Time
	Severity    string // TRACE, DEBUG, INFO, WARN, ERROR or FATAL
	Message     string
	TraceID     string
	SpanID      string
	ServiceName string // empty: the exporter's service name
	Attributes  map[string]string
}

// NewLogRecord creates a record with a new RecordID, the current time and the
// trace and span of the span active in ctx (if any).
func NewLogRecord(ctx context.Context, severity, message string) LogRecord {
	r := LogRecord{RecordID: NewRecordID(), Time: time.Now(), Severity: severity, Message: message}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.TraceID = sc.TraceID().String()
		r.SpanID = sc.SpanID().String()
	}
	return r
}

// NewRecordID returns a random 128-bit record identifier as 32 hex digits.
func NewRecordID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail on supported platforms; a time-based id
		// is still unique enough for one process.
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// LogExporter sends log records to the log-receiver over one gRPC connection.
type LogExporter struct {
	conn     *grpc.ClientConn
	client   pb.LogServiceClient
	token    string
	service  string
	resource map[string]string
}

// NewLogExporter connects to the log-receiver (WithAddress, WithToken work as
// for NewTracer). Records without a ServiceName get svcName.
func NewLogExporter(svcName string, opts ...Option) (*LogExporter, error) {
	cfg := newConfig(opts)
	conn, err := grpc.Dial(cfg.address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("troubleshooting: connect to %s: %w", cfg.address, err)
	}
	return &LogExporter{conn: conn, client: pb.NewLogServiceClient(conn), token: cfg.token, service: svcName, resource: cfg.resource}, nil
}

// Send stores records and returns nil only after the receiver confirmed that
// all of them were written. On an error none of them may be counted as
// stored (some may still have been written): retry with the same records;
// the same RecordIDs make the retry count once.
func (e *LogExporter) Send(ctx context.Context, records []LogRecord) error {
	if len(records) == 0 {
		return nil
	}
	if e == nil || e.client == nil {
		return errors.New("troubleshooting: log exporter is not connected, create it with NewLogExporter")
	}
	req := &pb.Logs{Logs: make([]*pb.LogMessageRequest, 0, len(records))}
	for _, r := range records {
		req.Logs = append(req.Logs, e.toProto(r))
	}
	if e.token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+e.token)
	}
	if _, err := e.client.SendLogs(ctx, req); err != nil {
		return fmt.Errorf("troubleshooting: send %d log records (not confirmed, retry them): %w", len(records), err)
	}
	return nil
}

func (e *LogExporter) toProto(r LogRecord) *pb.LogMessageRequest {
	service := r.ServiceName
	if service == "" {
		service = e.service
	}
	attributes := make(map[string]string, len(r.Attributes)+len(e.resource))
	for k, v := range e.resource {
		attributes[toValidUTF8(k)] = toValidUTF8(v)
	}
	for k, v := range r.Attributes {
		attributes[toValidUTF8(k)] = toValidUTF8(v)
	}
	return &pb.LogMessageRequest{
		Message:     toValidUTF8(r.Message),
		Timestamp:   timestamppb.New(r.Time),
		RecordId:    r.RecordID,
		TraceId:     r.TraceID,
		SpanId:      r.SpanID,
		Severity:    r.Severity,
		ServiceName: toValidUTF8(service),
		Attributes:  attributes,
	}
}

// Close closes the connection.
func (e *LogExporter) Close() error {
	if e == nil || e.conn == nil {
		return nil
	}
	return e.conn.Close()
}

// TraceHook is a zerolog hook that adds the trace_id and span_id of the span
// in the event's context (logger.Info().Ctx(ctx)...) and a record_id. Use it
// with a LogWriter.
type TraceHook struct{}

func (TraceHook) Run(e *zerolog.Event, _ zerolog.Level, _ string) {
	if sc := trace.SpanContextFromContext(e.GetCtx()); sc.IsValid() {
		e.Str("trace_id", sc.TraceID().String())
		e.Str("span_id", sc.SpanID().String())
	}
	e.Str("record_id", NewRecordID())
	e.Str("record_time", time.Now().UTC().Format(time.RFC3339Nano))
}

// DefaultLogBufferSize bounds the records a LogWriter keeps while the receiver
// is unavailable.
const DefaultLogBufferSize = 10000

// LogWriter is an io.Writer for zerolog that turns each JSON event into a
// LogRecord and sends them in batches in the background:
//
//	exporter, _ := sender.NewLogExporter("my-service")
//	w := sender.NewLogWriter(exporter, time.Second)
//	defer w.Close(context.Background())
//	logger := zerolog.New(io.MultiWriter(os.Stdout, w)).Hook(sender.TraceHook{})
//	logger.Error().Ctx(ctx).Msg("upload failed")
//
// Write never blocks the application on the network. Records that were not
// confirmed stay buffered and are retried with the same RecordID; when the
// buffer is full the oldest records are dropped and counted (Dropped).
type LogWriter struct {
	exporter *LogExporter
	limit    int

	mu      sync.Mutex
	buffer  []LogRecord
	dropped uint64
	lastErr error

	flushMu sync.Mutex
	stop    chan struct{}
	done    chan struct{}
}

// NewLogWriter starts a background flush every interval (no background flush
// if interval <= 0; call Flush yourself).
func NewLogWriter(exporter *LogExporter, interval time.Duration) *LogWriter {
	w := &LogWriter{exporter: exporter, limit: DefaultLogBufferSize, stop: make(chan struct{}), done: make(chan struct{})}
	if interval <= 0 {
		close(w.done)
		return w
	}
	go func() {
		defer close(w.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				_ = w.Flush(ctx)
				cancel()
			case <-w.stop:
				return
			}
		}
	}()
	return w
}

// Write parses one zerolog JSON event. It returns an error only for input that
// is not a JSON object.
func (w *LogWriter) Write(p []byte) (int, error) {
	record, err := parseZerologEvent(p, time.Now())
	if err != nil {
		return 0, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buffer) >= w.limit {
		drop := len(w.buffer) - w.limit + 1
		w.buffer = w.buffer[drop:]
		w.dropped += uint64(drop)
	}
	w.buffer = append(w.buffer, record)
	return len(p), nil
}

// Flush sends the buffered records and returns nil only after the receiver
// confirmed them. On error they stay buffered for the next Flush.
func (w *LogWriter) Flush(ctx context.Context) error {
	w.flushMu.Lock()
	defer w.flushMu.Unlock()

	w.mu.Lock()
	batch := append([]LogRecord(nil), w.buffer...)
	w.mu.Unlock()
	if len(batch) == 0 {
		return nil
	}

	const chunk = 500
	for start := 0; start < len(batch); start += chunk {
		end := start + chunk
		if end > len(batch) {
			end = len(batch)
		}
		if err := w.exporter.Send(ctx, batch[start:end]); err != nil {
			w.mu.Lock()
			w.lastErr = err
			w.mu.Unlock()
			return err
		}
		w.remove(batch[start:end])
	}
	w.mu.Lock()
	w.lastErr = nil
	w.mu.Unlock()
	return nil
}

// remove drops sent records from the head of the buffer. Records dropped by
// Write meanwhile may already be gone, so it matches by RecordID.
func (w *LogWriter) remove(sent []LogRecord) {
	ids := make(map[string]struct{}, len(sent))
	for _, r := range sent {
		ids[r.RecordID] = struct{}{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	kept := w.buffer[:0]
	for _, r := range w.buffer {
		if _, ok := ids[r.RecordID]; !ok {
			kept = append(kept, r)
		}
	}
	w.buffer = kept
}

// Pending is the number of records not yet confirmed.
func (w *LogWriter) Pending() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.buffer)
}

// Dropped is the number of records discarded because the buffer was full.
func (w *LogWriter) Dropped() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.dropped
}

// Err is the error of the last failed flush, nil after a successful one.
func (w *LogWriter) Err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastErr
}

// Close stops the background flush and makes a final Flush with ctx. It does
// not close the exporter.
func (w *LogWriter) Close(ctx context.Context) error {
	select {
	case <-w.stop:
	default:
		close(w.stop)
	}
	<-w.done
	return w.Flush(ctx)
}

// zerologFields are the fields that become LogRecord fields rather than
// attributes.
var zerologFields = map[string]bool{
	zerolog.LevelFieldName: true, zerolog.MessageFieldName: true, zerolog.TimestampFieldName: true,
	"trace_id": true, "span_id": true, "record_id": true, "record_time": true, "service": true,
}

func parseZerologEvent(p []byte, now time.Time) (LogRecord, error) {
	var fields map[string]any
	if err := json.Unmarshal(p, &fields); err != nil {
		return LogRecord{}, fmt.Errorf("troubleshooting: log event is not a JSON object: %w", err)
	}
	str := func(key string) string {
		if v, ok := fields[key].(string); ok {
			return v
		}
		return ""
	}
	r := LogRecord{
		RecordID:    str("record_id"),
		Time:        now,
		Severity:    str(zerolog.LevelFieldName),
		Message:     str(zerolog.MessageFieldName),
		TraceID:     str("trace_id"),
		SpanID:      str("span_id"),
		ServiceName: str("service"),
	}
	// An all-zero id means "no span" (an invalid SpanContext).
	if strings.Trim(r.TraceID, "0") == "" {
		r.TraceID = ""
	}
	if strings.Trim(r.SpanID, "0") == "" {
		r.SpanID = ""
	}
	if r.RecordID == "" {
		r.RecordID = NewRecordID()
	}
	if t, err := time.Parse(time.RFC3339Nano, str("record_time")); err == nil {
		r.Time = t
	}
	for key, value := range fields {
		if zerologFields[key] {
			continue
		}
		if r.Attributes == nil {
			r.Attributes = make(map[string]string)
		}
		if s, ok := value.(string); ok {
			r.Attributes[key] = s
		} else if b, err := json.Marshal(value); err == nil {
			r.Attributes[key] = string(b)
		}
	}
	return r, nil
}
