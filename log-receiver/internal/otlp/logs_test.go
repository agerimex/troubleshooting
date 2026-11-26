package otlp

import (
	"testing"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func logsRequest(service string, records ...*logspb.LogRecord) []*logspb.ResourceLogs {
	return []*logspb.ResourceLogs{{
		Resource:  &resourcepb.Resource{Attributes: []*commonpb.KeyValue{str("service.name", service), str("host.name", "dev")}},
		ScopeLogs: []*logspb.ScopeLogs{{Scope: &commonpb.InstrumentationScope{Name: "Serilog"}, LogRecords: records}},
	}}
}

func kv(key string, value *commonpb.AnyValue) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: value}
}

func strValue(s string) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: s}}
}

// A record as the Serilog OpenTelemetry sink sends it.
func TestLogsMapsASerilogRecord(t *testing.T) {
	at := time.Date(2026, 11, 26, 20, 0, 0, 123456789, time.UTC)
	record := &logspb.LogRecord{
		TimeUnixNano:         uint64(at.UnixNano()),
		ObservedTimeUnixNano: uint64(at.Add(time.Millisecond).UnixNano()),
		SeverityText:         "Information",
		SeverityNumber:       logspb.SeverityNumber_SEVERITY_NUMBER_INFO,
		Body:                 strValue("Call quality report for participant 7"),
		TraceId:              traceID,
		SpanId:               spanID,
		Attributes: []*commonpb.KeyValue{
			str("ParticipantId", "7"),
			kv("Report", &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{Values: []*commonpb.KeyValue{
				kv("jitterMs", &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 12}}),
				str("codec", "opus"),
			}}}}),
		},
	}
	records, rejected := Logs(logsRequest("controller", record))
	if rejected != 0 || len(records) != 1 {
		t.Fatalf("records %d, rejected %d", len(records), rejected)
	}
	r := records[0]
	if r.UnixTime != at.UnixNano() || r.Severity != "INFO" || r.ServiceName != "controller" || r.Body != "Call quality report for participant 7" {
		t.Errorf("record = %+v", r)
	}
	if r.TraceId != "0102030405060708090a0b0c0d0e0f10" || r.SpanId != "a1a2a3a4a5a6a7a8" {
		t.Errorf("ids = %q %q, want lower-case hex", r.TraceId, r.SpanId)
	}
	if r.Attributes["ParticipantId"] != "7" || r.Attributes["Report"] != `{"codec":"opus","jitterMs":12}` {
		t.Errorf("attributes = %v", r.Attributes)
	}
	if _, ok := r.Attributes["host.name"]; ok {
		t.Error("resource attributes other than service.name must not be added")
	}
	if len(r.RecordId) != 32 {
		t.Errorf("record id %q, want a 32-character hash", r.RecordId)
	}
}

func TestLogsFieldRules(t *testing.T) {
	at := uint64(time.Date(2026, 11, 26, 20, 0, 0, 0, time.UTC).UnixNano())
	base := func(edit func(*logspb.LogRecord)) *logspb.LogRecord {
		r := &logspb.LogRecord{TimeUnixNano: at, Body: strValue("x"), TraceId: traceID, SpanId: spanID}
		edit(r)
		return r
	}
	tests := []struct {
		name   string
		record *logspb.LogRecord
		check  func(t *testing.T, records int, rejected int64, severity, trace, span string, unix int64, body string)
	}{
		{"severity from the number when the text is empty", base(func(r *logspb.LogRecord) { r.SeverityNumber = logspb.SeverityNumber_SEVERITY_NUMBER_WARN2 }),
			func(t *testing.T, n int, rej int64, sev, _, _ string, _ int64, _ string) {
				if sev != "WARN" {
					t.Errorf("severity %q, want WARN", sev)
				}
			}},
		{"the number's ranges end at FATAL", base(func(r *logspb.LogRecord) { r.SeverityNumber = logspb.SeverityNumber_SEVERITY_NUMBER_FATAL4 }),
			func(t *testing.T, n int, rej int64, sev, _, _ string, _ int64, _ string) {
				if sev != "FATAL" {
					t.Errorf("severity %q, want FATAL", sev)
				}
			}},
		{"no severity at all stays empty", base(func(r *logspb.LogRecord) {}),
			func(t *testing.T, n int, rej int64, sev, _, _ string, _ int64, _ string) {
				if sev != "" {
					t.Errorf("severity %q, want empty", sev)
				}
			}},
		{"Serilog's Verbose and .NET's Critical", base(func(r *logspb.LogRecord) { r.SeverityText = "Verbose" }),
			func(t *testing.T, n int, rej int64, sev, _, _ string, _ int64, _ string) {
				if sev != "TRACE" {
					t.Errorf("Verbose: severity %q, want TRACE", sev)
				}
			}},
		{"all-zero ids are not recorded", base(func(r *logspb.LogRecord) { r.TraceId, r.SpanId = make([]byte, 16), make([]byte, 8) }),
			func(t *testing.T, n int, rej int64, _, trace, span string, _ int64, _ string) {
				if n != 1 || trace != "" || span != "" {
					t.Errorf("records %d, ids %q %q, want one record without ids", n, trace, span)
				}
			}},
		{"a wrong-length span id is rejected", base(func(r *logspb.LogRecord) { r.SpanId = []byte{1, 2, 3} }),
			func(t *testing.T, n int, rej int64, _, _, _ string, _ int64, _ string) {
				if n != 0 || rej != 1 {
					t.Errorf("records %d, rejected %d, want 0 and 1", n, rej)
				}
			}},
		{"the observed time stands in for a missing time", base(func(r *logspb.LogRecord) { r.TimeUnixNano, r.ObservedTimeUnixNano = 0, at+5 }),
			func(t *testing.T, n int, rej int64, _, _, _ string, unix int64, _ string) {
				if unix != int64(at+5) {
					t.Errorf("time %d, want the observed time %d", unix, at+5)
				}
			}},
		{"no time at all is rejected", base(func(r *logspb.LogRecord) { r.TimeUnixNano = 0 }),
			func(t *testing.T, n int, rej int64, _, _, _ string, _ int64, _ string) {
				if n != 0 || rej != 1 {
					t.Errorf("records %d, rejected %d, want 0 and 1", n, rej)
				}
			}},
		{"a non-string body is text", base(func(r *logspb.LogRecord) {
			r.Body = &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: []*commonpb.AnyValue{strValue("a"), {Value: &commonpb.AnyValue_BoolValue{BoolValue: true}}}}}}
		}),
			func(t *testing.T, n int, rej int64, _, _, _ string, _ int64, body string) {
				if body != `["a",true]` {
					t.Errorf("body %q, want JSON", body)
				}
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records, rejected := Logs(logsRequest("svc", tt.record))
			var sev, trace, span, body string
			var unix int64
			if len(records) == 1 {
				r := records[0]
				sev, trace, span, unix, body = r.Severity, r.TraceId, r.SpanId, r.UnixTime, r.Body
			}
			tt.check(t, len(records), rejected, sev, trace, span, unix, body)
		})
	}
}

// Valid records of a request are kept when others are rejected.
func TestLogsKeepsTheValidRecordsOfARequest(t *testing.T) {
	at := uint64(time.Now().UnixNano())
	records, rejected := Logs(logsRequest("svc",
		&logspb.LogRecord{TimeUnixNano: at, Body: strValue("ok")},
		&logspb.LogRecord{Body: strValue("no time")},
		&logspb.LogRecord{TimeUnixNano: at, TraceId: []byte{1}, Body: strValue("bad trace id")},
	))
	if len(records) != 1 || rejected != 2 || records[0].Body != "ok" {
		t.Errorf("records %d (%v), rejected %d", len(records), records, rejected)
	}
}

// An exporter's retry sends the same records: they get the same ids, so reads
// keep one row each. A different record gets a different id; log.record.uid
// wins when the sender sets it.
func TestLogsRecordIDs(t *testing.T) {
	at := uint64(time.Date(2026, 11, 26, 20, 0, 0, 0, time.UTC).UnixNano())
	record := func(body string, attrs ...*commonpb.KeyValue) *logspb.LogRecord {
		return &logspb.LogRecord{TimeUnixNano: at, SeverityText: "Error", Body: strValue(body), TraceId: traceID, SpanId: spanID, Attributes: attrs}
	}
	first, _ := Logs(logsRequest("svc", record("a", str("k1", "v1"), str("k2", "v2")), record("b")))
	retry, _ := Logs(logsRequest("svc", record("a", str("k2", "v2"), str("k1", "v1")), record("b")))
	if first[0].RecordId != retry[0].RecordId || first[1].RecordId != retry[1].RecordId {
		t.Errorf("a retry got other ids: %s %s vs %s %s", first[0].RecordId, first[1].RecordId, retry[0].RecordId, retry[1].RecordId)
	}
	if first[0].RecordId == first[1].RecordId {
		t.Error("two different records got one id")
	}
	other, _ := Logs(logsRequest("other-service", record("a", str("k1", "v1"), str("k2", "v2"))))
	if other[0].RecordId == first[0].RecordId {
		t.Error("the service is not part of the id")
	}
	uid, _ := Logs(logsRequest("svc", record("a", str(LogRecordUIDAttribute, "01JDX7Y3Q4"))))
	if uid[0].RecordId != "01JDX7Y3Q4" {
		t.Errorf("record id %q, want log.record.uid", uid[0].RecordId)
	}
}
