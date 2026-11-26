package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// otlpLogs (OTLP-LOG) checks logs sent over OTLP, the way an OpenTelemetry SDK
// (Serilog, Node) sends them: a span and a log record with that span's ids,
// both over OTLP, end up in one trace report with the log under the span; the
// SDK's level name is normalized ("Information" is INFO); an exporter's retry
// of the same request is read once; a record with a malformed span id is
// rejected as a partial success while the valid one is stored. Its ids are
// derived from the run, so running check again resends the same records.
func (c *checker) otlpLogs(ctx context.Context, otlp *otlpSender) error {
	sum := sha256.Sum256([]byte("otlp-log-" + c.d.run))
	traceID, spanID := sum[:16], sum[16:24]
	start := c.d.at(-20 * time.Minute)
	service := "fx" + c.d.run + "-otlp-logs"
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{Key: "service.name", Value: strValue(service)}}}

	span := &collectortracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{
		Resource: resource,
		ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{
			TraceId: traceID, SpanId: spanID, Name: "POST /producers", Kind: tracepb.Span_SPAN_KIND_SERVER,
			StartTimeUnixNano: uint64(start.UnixNano()), EndTimeUnixNano: uint64(start.Add(40 * time.Millisecond).UnixNano()),
		}}}},
	}}}
	if rejected, err := otlp.send(ctx, span); err != nil || rejected != 0 {
		return fmt.Errorf("send the span: rejected %d, %v", rejected, err)
	}

	logs := &collectorlogspb.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{
		Resource: resource,
		ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{
			{
				TimeUnixNano: uint64(start.Add(10 * time.Millisecond).UnixNano()), SeverityText: "Information",
				Body: strValue("Producer created"), TraceId: traceID, SpanId: spanID,
				Attributes: []*commonpb.KeyValue{{Key: "ProducerKind", Value: strValue("video")}},
			},
			// A malformed span id: rejected, the record above still stored.
			{TimeUnixNano: uint64(start.Add(11 * time.Millisecond).UnixNano()), Body: strValue("bad id"), TraceId: traceID, SpanId: []byte{1, 2, 3}},
		}}},
	}}}
	client := collectorlogspb.NewLogsServiceClient(otlp.conn)
	for attempt := 1; attempt <= 2; attempt++ { // the second is an exporter's retry
		sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		resp, err := client.Export(sendCtx, logs)
		cancel()
		if err != nil {
			return fmt.Errorf("export logs (attempt %d): %w", attempt, err)
		}
		if got := resp.GetPartialSuccess().GetRejectedLogRecords(); got != 1 {
			return fmt.Errorf("attempt %d: %d records rejected, want 1 (the malformed span id)", attempt, got)
		}
	}

	report, err := c.api.report(ctx, reportRequest{TraceID: fmt.Sprintf("%x", traceID), TimeFrom: ns(start.Add(-time.Minute)), TimeTo: ns(start.Add(time.Minute))})
	if err != nil {
		return err
	}
	if report.Spans == nil || len(report.Spans.Records) != 1 || report.Logs == nil {
		return fmt.Errorf("report: spans %v, logs %v", report.Spans, report.Logs)
	}
	got := report.Logs.Records
	if len(got) != 1 {
		return fmt.Errorf("the report has %d logs, want 1 (the retry must be read once, the malformed record dropped)", len(got))
	}
	l := got[0]
	if l.SpanID != fmt.Sprintf("%x", spanID) || l.Severity != "INFO" || l.Message != "Producer created" || l.Service != service || l.Attributes["ProducerKind"] != "video" {
		return fmt.Errorf("log = %+v, want span %x, INFO, \"Producer created\", service %s, ProducerKind=video", l, spanID, service)
	}
	return nil
}

func strValue(s string) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: s}}
}
