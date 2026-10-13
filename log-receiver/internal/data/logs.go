package data

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	pb "github.com/agerimex/troubleshooting/protos/logs"
)

// LogRecord is one row of the log_records table.
type LogRecord struct {
	UnixTime    int64
	RecordId    string
	TraceId     string
	SpanId      string
	Severity    string
	ServiceName string
	Body        string
	Attributes  map[string]string
}

// severityAliases maps the level names of common loggers (zerolog, slog,
// logrus) to the stored severities.
var severityAliases = map[string]string{
	"TRACE":    "TRACE",
	"DEBUG":    "DEBUG",
	"INFO":     "INFO",
	"WARN":     "WARN",
	"WARNING":  "WARN",
	"ERROR":    "ERROR",
	"ERR":      "ERROR",
	"FATAL":    "FATAL",
	"PANIC":    "FATAL",
	"CRITICAL": "FATAL",
}

// NormalizeSeverity upper-cases a level name and maps known aliases. An empty
// severity stays empty (not recorded); an unknown one is kept upper-cased.
func NormalizeSeverity(severity string) string {
	upper := strings.ToUpper(strings.TrimSpace(severity))
	if mapped, ok := severityAliases[upper]; ok {
		return mapped
	}
	return upper
}

// LogRecordFromProto validates and normalizes one received record. Identifiers
// are kept as recorded (lower-cased hex); a missing TraceId or SpanId stays
// empty, never inferred. A record without record_id gets a random one, so it
// is stored but a retry of it can't be recognized.
func LogRecordFromProto(req *pb.LogMessageRequest) (LogRecord, error) {
	if req.GetTimestamp() == nil {
		return LogRecord{}, fmt.Errorf("timestamp is required")
	}
	traceID := strings.ToLower(req.GetTraceId())
	if traceID != "" && !validHexID(traceID, 32) {
		return LogRecord{}, fmt.Errorf("trace_id %q is not 32 hex digits or is all zeros", req.GetTraceId())
	}
	spanID := strings.ToLower(req.GetSpanId())
	if spanID != "" && !validHexID(spanID, 16) {
		return LogRecord{}, fmt.Errorf("span_id %q is not 16 hex digits or is all zeros", req.GetSpanId())
	}
	recordID := req.GetRecordId()
	if recordID == "" {
		recordID = randomID()
	}
	if len(recordID) > 128 {
		return LogRecord{}, fmt.Errorf("record_id is longer than 128 bytes")
	}
	attributes := make(map[string]string, len(req.GetAttributes()))
	for key, value := range req.GetAttributes() {
		attributes[validUTF8(key)] = validUTF8(value)
	}
	return LogRecord{
		UnixTime:    req.GetTimestamp().AsTime().UnixNano(),
		RecordId:    validUTF8(recordID),
		TraceId:     traceID,
		SpanId:      spanID,
		Severity:    NormalizeSeverity(req.GetSeverity()),
		ServiceName: validUTF8(req.GetServiceName()),
		Body:        validUTF8(req.GetMessage()),
		Attributes:  attributes,
	}, nil
}

func validHexID(id string, size int) bool {
	if len(id) != size {
		return false
	}
	if _, err := hex.DecodeString(id); err != nil {
		return false
	}
	return strings.Trim(id, "0") != ""
}

func validUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "�")
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// InsertLogRecords writes records in one insert and returns only after
// ClickHouse has accepted it; on error none of them may be confirmed.
// A retried record is stored again with the same RecordId; reads keep one row
// per RecordId.
func (l *Log) InsertLogRecords(ctx context.Context, records []LogRecord) error {
	if len(records) == 0 {
		return nil
	}
	received := time.Now().UnixNano()
	batch, err := clickhouseDB.PrepareBatch(ctx, "INSERT INTO log_records (UnixTime, RecordId, TraceId, SpanId, Severity, ServiceName, Body, Attributes, ReceivedTime)")
	if err != nil {
		return err
	}
	for _, r := range records {
		// Column order must match the INSERT above.
		if err := batch.Append(
			r.UnixTime,
			r.RecordId,
			r.TraceId,
			r.SpanId,
			r.Severity,
			r.ServiceName,
			r.Body,
			r.Attributes,
			received,
		); err != nil {
			batch.Abort()
			return err
		}
	}
	return batch.Send()
}
