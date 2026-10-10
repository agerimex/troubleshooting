package otlp

import (
	"encoding/json"
	"testing"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

var (
	traceID  = []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	spanID   = []byte{0xa1, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6, 0xa7, 0xa8}
	parentID = []byte{0xb1, 0xb2, 0xb3, 0xb4, 0xb5, 0xb6, 0xb7, 0xb8}
)

func str(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}}
}

func request(service *commonpb.KeyValue, spans ...*tracepb.Span) []*tracepb.ResourceSpans {
	resource := &resourcepb.Resource{}
	if service != nil {
		resource.Attributes = []*commonpb.KeyValue{service, str("host.name", "dev")}
	}
	return []*tracepb.ResourceSpans{{
		Resource: resource,
		ScopeSpans: []*tracepb.ScopeSpans{{
			Scope: &commonpb.InstrumentationScope{Name: "net/http", Version: "1.2.3"},
			Spans: spans,
		}},
	}}
}

func TestSpansConvertsFields(t *testing.T) {
	start := time.Date(2026, 10, 9, 12, 0, 0, 123456789, time.UTC)
	span := &tracepb.Span{
		TraceId:           traceID,
		SpanId:            spanID,
		ParentSpanId:      parentID,
		TraceState:        "k=v",
		Name:              "GET /orders",
		Kind:              tracepb.Span_SPAN_KIND_SERVER,
		StartTimeUnixNano: uint64(start.UnixNano()),
		EndTimeUnixNano:   uint64(start.Add(1500 * time.Microsecond).UnixNano()),
		Attributes:        []*commonpb.KeyValue{str("http.method", "GET")},
		Status:            &tracepb.Status{Code: tracepb.Status_STATUS_CODE_ERROR, Message: "boom"},
		Events: []*tracepb.Span_Event{
			{TimeUnixNano: uint64(start.UnixNano()), Name: "exception", Attributes: []*commonpb.KeyValue{str("exception.message", "boom")}},
		},
		Links: []*tracepb.Span_Link{{TraceId: traceID, SpanId: parentID, TraceState: "a=b", Attributes: []*commonpb.KeyValue{str("kind", "retry")}}},
	}

	spans, rejected := Spans(request(str("service.name", "orders-api"), span))

	if rejected != 0 || len(spans) != 1 {
		t.Fatalf("got %d spans, %d rejected", len(spans), rejected)
	}
	got := spans[0]
	checks := []struct{ name, got, want string }{
		{"TraceId", got.TraceId, "0102030405060708090a0b0c0d0e0f10"},
		{"SpanId", got.SpanId, "a1a2a3a4a5a6a7a8"},
		{"ParentSpanId", got.ParentSpanId, "b1b2b3b4b5b6b7b8"},
		{"TraceState", got.TraceState, "k=v"},
		{"SpanName", got.SpanName, "GET /orders"},
		{"SpanKind", got.SpanKind, "server"},
		{"ServiceName", got.ServiceName, "orders-api"},
		{"StatusMessage", got.StatusMessage, "boom"},
		{"resource host.name", got.ResourceAttributes["host.name"], "dev"},
		{"span http.method", got.SpanAttributes["http.method"], "GET"},
		{"otel.scope.name", got.SpanAttributes["otel.scope.name"], "net/http"},
		{"otel.scope.version", got.SpanAttributes["otel.scope.version"], "1.2.3"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if !got.Timestamp.AsTime().Equal(start) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp.AsTime(), start)
	}
	if got.Duration != 1500 {
		t.Errorf("Duration = %d µs, want 1500", got.Duration)
	}
	if got.StatusCode != 1 {
		t.Errorf("StatusCode = %d, want 1 (error)", got.StatusCode)
	}

	if len(got.Events_Name) != 1 || got.Events_Name[0] != "exception" || len(got.Events_Timestamp) != 1 || len(got.Events_Attributes) != 1 {
		t.Fatalf("events = %v / %v / %v", got.Events_Name, got.Events_Timestamp, got.Events_Attributes)
	}
	var eventAttrs map[string]string
	if err := json.Unmarshal([]byte(got.Events_Attributes[0].Value), &eventAttrs); err != nil || eventAttrs["exception.message"] != "boom" {
		t.Errorf("event attributes = %q (%v)", got.Events_Attributes[0].Value, err)
	}
	if len(got.Links_TraceId) != 1 || got.Links_SpanId[0] != "b1b2b3b4b5b6b7b8" || got.Links_TraceState[0] != "a=b" || len(got.Links_Attributes) != 1 {
		t.Errorf("links = %v / %v / %v / %v", got.Links_TraceId, got.Links_SpanId, got.Links_TraceState, got.Links_Attributes)
	}
}

// OTLP and the OpenTelemetry Go API number Ok and Error the other way round.
func TestSpansMapsStatusCodes(t *testing.T) {
	tests := []struct {
		otlp tracepb.Status_StatusCode
		want int32
	}{
		{tracepb.Status_STATUS_CODE_UNSET, 0},
		{tracepb.Status_STATUS_CODE_OK, 2},
		{tracepb.Status_STATUS_CODE_ERROR, 1},
	}
	for _, tt := range tests {
		span := &tracepb.Span{TraceId: traceID, SpanId: spanID, Status: &tracepb.Status{Code: tt.otlp}}
		spans, _ := Spans(request(nil, span))
		if spans[0].StatusCode != tt.want {
			t.Errorf("%v -> %d, want %d", tt.otlp, spans[0].StatusCode, tt.want)
		}
	}

	// A span without a status is unset.
	spans, _ := Spans(request(nil, &tracepb.Span{TraceId: traceID, SpanId: spanID}))
	if spans[0].StatusCode != 0 {
		t.Errorf("missing status -> %d, want 0", spans[0].StatusCode)
	}
}

func TestSpansRootAndDefaults(t *testing.T) {
	spans, _ := Spans(request(nil, &tracepb.Span{
		TraceId:           traceID,
		SpanId:            spanID,
		StartTimeUnixNano: 2000,
		EndTimeUnixNano:   1000, // end before start must not give a negative duration
	}))

	got := spans[0]
	if got.ParentSpanId != RootParentSpanID {
		t.Errorf("ParentSpanId = %q, want the root marker", got.ParentSpanId)
	}
	if got.ServiceName != unknownService {
		t.Errorf("ServiceName = %q, want %q", got.ServiceName, unknownService)
	}
	if got.SpanKind != "unspecified" {
		t.Errorf("SpanKind = %q", got.SpanKind)
	}
	if got.Duration != 0 {
		t.Errorf("Duration = %d, want 0", got.Duration)
	}
	if got.Events_Name != nil || got.Links_TraceId != nil {
		t.Errorf("expected no events or links")
	}
}

func TestSpansRejectsInvalidIDs(t *testing.T) {
	spans, rejected := Spans(request(nil,
		&tracepb.Span{TraceId: traceID[:8], SpanId: spanID},
		&tracepb.Span{TraceId: traceID},
		&tracepb.Span{TraceId: make([]byte, 16), SpanId: spanID},
		&tracepb.Span{TraceId: traceID, SpanId: make([]byte, 8)},
		&tracepb.Span{TraceId: traceID, SpanId: spanID, ParentSpanId: parentID[:4]},
		&tracepb.Span{TraceId: traceID, SpanId: spanID},
	))

	if len(spans) != 1 || rejected != 5 {
		t.Errorf("got %d spans, %d rejected; want 1 and 5", len(spans), rejected)
	}
}

func TestValueToString(t *testing.T) {
	array := &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: []*commonpb.AnyValue{
		{Value: &commonpb.AnyValue_StringValue{StringValue: "a"}},
		{Value: &commonpb.AnyValue_IntValue{IntValue: 2}},
		{Value: &commonpb.AnyValue_BoolValue{BoolValue: true}},
	}}}}
	kvlist := &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{Values: []*commonpb.KeyValue{
		{Key: "n", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: 1.5}}},
	}}}}

	tests := []struct {
		name  string
		value *commonpb.AnyValue
		want  string
	}{
		{"string", &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "привет"}}, "привет"},
		{"bool", &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: false}}, "false"},
		{"int", &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: -42}}, "-42"},
		{"double", &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: 0.25}}, "0.25"},
		{"bytes", &commonpb.AnyValue{Value: &commonpb.AnyValue_BytesValue{BytesValue: []byte("hi")}}, "aGk="},
		{"array", array, `["a",2,true]`},
		{"kvlist", kvlist, `{"n":1.5}`},
		{"empty", &commonpb.AnyValue{}, ""},
		{"nil", nil, ""},
	}
	for _, tt := range tests {
		if got := valueToString(tt.value); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
