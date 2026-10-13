package data

import (
	"testing"
	"time"

	pb "github.com/agerimex/troubleshooting/protos/logs"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestLogRecordFromProtoKeepsCorrelation(t *testing.T) {
	ts := time.Date(2026, 10, 11, 9, 0, 0, 123, time.UTC)
	got, err := LogRecordFromProto(&pb.LogMessageRequest{
		Message:     "Storage returned 403 AccessDenied",
		Timestamp:   timestamppb.New(ts),
		RecordId:    "log-denied",
		TraceId:     "0AF7651916CD43DD8448EB211C80319C",
		SpanId:      "b7ad6b7169203331",
		Severity:    "error",
		ServiceName: "recording-worker",
		Attributes:  map[string]string{"file": "upload.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := LogRecord{
		UnixTime:    ts.UnixNano(),
		RecordId:    "log-denied",
		TraceId:     "0af7651916cd43dd8448eb211c80319c",
		SpanId:      "b7ad6b7169203331",
		Severity:    "ERROR",
		ServiceName: "recording-worker",
		Body:        "Storage returned 403 AccessDenied",
	}
	if got.UnixTime != want.UnixTime || got.RecordId != want.RecordId || got.TraceId != want.TraceId ||
		got.SpanId != want.SpanId || got.Severity != want.Severity || got.ServiceName != want.ServiceName ||
		got.Body != want.Body || got.Attributes["file"] != "upload.go" {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestLogRecordFromProtoLegacyRequest(t *testing.T) {
	// An old sender sets only message and timestamp: nothing is invented.
	got, err := LogRecordFromProto(&pb.LogMessageRequest{Message: "hello", Timestamp: timestamppb.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if got.TraceId != "" || got.SpanId != "" || got.Severity != "" || got.ServiceName != "" {
		t.Fatalf("legacy record got correlation or severity: %+v", got)
	}
	if len(got.RecordId) != 32 {
		t.Fatalf("legacy record got record id %q, want a generated one", got.RecordId)
	}
}

func TestLogRecordFromProtoSpanWithoutTrace(t *testing.T) {
	got, err := LogRecordFromProto(&pb.LogMessageRequest{
		Message: "x", Timestamp: timestamppb.Now(), SpanId: "b7ad6b7169203331",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.TraceId != "" || got.SpanId != "b7ad6b7169203331" {
		t.Fatalf("got trace %q span %q, want no trace and the recorded span", got.TraceId, got.SpanId)
	}
}

func TestLogRecordFromProtoRejectsInvalid(t *testing.T) {
	for name, req := range map[string]*pb.LogMessageRequest{
		"no timestamp":    {Message: "x"},
		"short trace id":  {Message: "x", Timestamp: timestamppb.Now(), TraceId: "abc"},
		"zero trace id":   {Message: "x", Timestamp: timestamppb.Now(), TraceId: "00000000000000000000000000000000"},
		"non-hex span id": {Message: "x", Timestamp: timestamppb.Now(), SpanId: "zzzzzzzzzzzzzzzz"},
		"zero span id":    {Message: "x", Timestamp: timestamppb.Now(), SpanId: "0000000000000000"},
	} {
		if _, err := LogRecordFromProto(req); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestNormalizeSeverity(t *testing.T) {
	for in, want := range map[string]string{
		"warn": "WARN", "warning": "WARN", "Error": "ERROR", "panic": "FATAL", "": "", "notice": "NOTICE",
	} {
		if got := NormalizeSeverity(in); got != want {
			t.Errorf("NormalizeSeverity(%q) = %q, want %q", in, got, want)
		}
	}
}
