// Package otlp converts OpenTelemetry Protocol (OTLP) trace data into the
// spans log-sender sends, so both paths share one insert into otel_traces.
package otlp

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

	pb "github.com/agerimex/troubleshooting/protos/logs"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RootParentSpanID marks root spans in otel_traces (log-analysis and the UI
// look for it). OTLP marks them with an empty parent_span_id instead.
const RootParentSpanID = "0000000000000000"

// unknownService is what OpenTelemetry SDKs report when service.name is unset.
const unknownService = "unknown_service"

// statusCodes maps OTLP status codes (Unset=0, Ok=1, Error=2) to the codes
// stored in otel_traces, which follow the OpenTelemetry Go API
// (Unset=0, Error=1, Ok=2).
var statusCodes = map[tracepb.Status_StatusCode]int32{
	tracepb.Status_STATUS_CODE_UNSET: 0,
	tracepb.Status_STATUS_CODE_OK:    2,
	tracepb.Status_STATUS_CODE_ERROR: 1,
}

// spanKinds uses the same names as SpanKind.String() in the OpenTelemetry Go
// SDK, which is what log-sender stores.
var spanKinds = map[tracepb.Span_SpanKind]string{
	tracepb.Span_SPAN_KIND_UNSPECIFIED: "unspecified",
	tracepb.Span_SPAN_KIND_INTERNAL:    "internal",
	tracepb.Span_SPAN_KIND_SERVER:      "server",
	tracepb.Span_SPAN_KIND_CLIENT:      "client",
	tracepb.Span_SPAN_KIND_PRODUCER:    "producer",
	tracepb.Span_SPAN_KIND_CONSUMER:    "consumer",
}

// Spans converts an OTLP export request into spans for InsertTraceData.
// Spans without a valid trace or span id are skipped and counted in rejected.
//
// ChildSpanCount is left at 0: OTLP doesn't carry it, so log-analysis counts
// children when it reads spans.
func Spans(resourceSpans []*tracepb.ResourceSpans) (spans []*pb.OneSpan, rejected int64) {
	for _, rs := range resourceSpans {
		resourceAttrs := attributesToMap(rs.GetResource().GetAttributes())
		serviceName := serviceNameOf(rs.GetResource())

		for _, ss := range rs.GetScopeSpans() {
			scope := ss.GetScope()
			for _, span := range ss.GetSpans() {
				if len(span.GetTraceId()) != 16 || len(span.GetSpanId()) != 8 {
					rejected++
					continue
				}
				converted := convertSpan(span, scope)
				converted.ServiceName = serviceName
				converted.ResourceAttributes = resourceAttrs
				spans = append(spans, converted)
			}
		}
	}
	return spans, rejected
}

func convertSpan(span *tracepb.Span, scope *commonpb.InstrumentationScope) *pb.OneSpan {
	start := span.GetStartTimeUnixNano()
	end := span.GetEndTimeUnixNano()

	parent := RootParentSpanID
	if len(span.GetParentSpanId()) == 8 {
		parent = hex.EncodeToString(span.GetParentSpanId())
	}

	attrs := attributesToMap(span.GetAttributes())
	// otel_traces has no scope columns; keep the instrumentation library visible
	// the way non-OTLP exporters do.
	if scope.GetName() != "" {
		attrs["otel.scope.name"] = scope.GetName()
	}
	if scope.GetVersion() != "" {
		attrs["otel.scope.version"] = scope.GetVersion()
	}

	out := &pb.OneSpan{
		Timestamp:      timestamppb.New(unixNano(start)),
		TraceId:        hex.EncodeToString(span.GetTraceId()),
		SpanId:         hex.EncodeToString(span.GetSpanId()),
		ParentSpanId:   parent,
		TraceState:     span.GetTraceState(),
		SpanName:       span.GetName(),
		SpanKind:       spanKinds[span.GetKind()],
		SpanAttributes: attrs,
		StatusCode:     statusCodes[span.GetStatus().GetCode()],
		StatusMessage:  span.GetStatus().GetMessage(),
	}
	if end > start {
		out.Duration = int64((end - start) / 1000) // microseconds, as log-sender sends
	}

	for _, event := range span.GetEvents() {
		out.Events_Timestamp = append(out.Events_Timestamp, timestamppb.New(unixNano(event.GetTimeUnixNano())))
		out.Events_Name = append(out.Events_Name, event.GetName())
		out.Events_Attributes = append(out.Events_Attributes, encodeAttributes(event.GetName(), event.GetAttributes()))
	}
	for _, link := range span.GetLinks() {
		out.Links_TraceId = append(out.Links_TraceId, hex.EncodeToString(link.GetTraceId()))
		out.Links_SpanId = append(out.Links_SpanId, hex.EncodeToString(link.GetSpanId()))
		out.Links_TraceState = append(out.Links_TraceState, link.GetTraceState())
		out.Links_Attributes = append(out.Links_Attributes, encodeAttributes("", link.GetAttributes()))
	}
	return out
}

func unixNano(ns uint64) time.Time {
	return time.Unix(0, int64(ns)).UTC()
}

func serviceNameOf(resource *resourcepb.Resource) string {
	for _, kv := range resource.GetAttributes() {
		if kv.GetKey() == "service.name" {
			if name := valueToString(kv.GetValue()); name != "" {
				return name
			}
		}
	}
	return unknownService
}

func attributesToMap(attrs []*commonpb.KeyValue) map[string]string {
	result := make(map[string]string, len(attrs))
	for _, kv := range attrs {
		result[kv.GetKey()] = valueToString(kv.GetValue())
	}
	return result
}

// encodeAttributes packs one event's or link's attributes the same way
// log-sender does: the whole map as a JSON object in Attribute.Value.
func encodeAttributes(name string, attrs []*commonpb.KeyValue) *pb.Attribute {
	value, err := json.Marshal(attributesToMap(attrs))
	if err != nil {
		value = []byte("{}")
	}
	return &pb.Attribute{Key: name, Value: string(value)}
}

// valueToString formats an OTLP attribute value like attribute.Value.Emit in
// the OpenTelemetry Go SDK: scalars as text, arrays and maps as JSON.
func valueToString(v *commonpb.AnyValue) string {
	switch value := v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return value.StringValue
	case *commonpb.AnyValue_BoolValue:
		return strconv.FormatBool(value.BoolValue)
	case *commonpb.AnyValue_IntValue:
		return strconv.FormatInt(value.IntValue, 10)
	case *commonpb.AnyValue_DoubleValue:
		return strconv.FormatFloat(value.DoubleValue, 'g', -1, 64)
	case *commonpb.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(value.BytesValue)
	case *commonpb.AnyValue_ArrayValue, *commonpb.AnyValue_KvlistValue:
		encoded, err := json.Marshal(toJSON(v))
		if err != nil {
			return ""
		}
		return string(encoded)
	default:
		return ""
	}
}

// toJSON turns a value into plain Go values for json.Marshal, keeping numbers
// and booleans typed inside arrays and maps.
func toJSON(v *commonpb.AnyValue) any {
	switch value := v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return value.StringValue
	case *commonpb.AnyValue_BoolValue:
		return value.BoolValue
	case *commonpb.AnyValue_IntValue:
		return value.IntValue
	case *commonpb.AnyValue_DoubleValue:
		return value.DoubleValue
	case *commonpb.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(value.BytesValue)
	case *commonpb.AnyValue_ArrayValue:
		items := make([]any, 0, len(value.ArrayValue.GetValues()))
		for _, item := range value.ArrayValue.GetValues() {
			items = append(items, toJSON(item))
		}
		return items
	case *commonpb.AnyValue_KvlistValue:
		fields := make(map[string]any, len(value.KvlistValue.GetValues()))
		for _, kv := range value.KvlistValue.GetValues() {
			fields[kv.GetKey()] = toJSON(kv.GetValue())
		}
		return fields
	default:
		return nil
	}
}
