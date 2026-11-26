package main

import (
	"context"
	"fmt"
	"log"

	"log-receiver/internal/data"
	"log-receiver/internal/otlp"

	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	// OTLP senders such as the OpenTelemetry Collector compress with gzip.
	_ "google.golang.org/grpc/encoding/gzip"
)

// otlpTraceServer accepts traces over the standard OpenTelemetry protocol, so
// applications in any language can send them without log-sender.
type otlpTraceServer struct {
	collectortracepb.UnimplementedTraceServiceServer
	models data.Models
}

func (s *otlpTraceServer) Export(ctx context.Context, req *collectortracepb.ExportTraceServiceRequest) (*collectortracepb.ExportTraceServiceResponse, error) {
	spans, rejected := otlp.Spans(req.GetResourceSpans())

	if len(spans) > 0 {
		if err := s.models.Log.InsertTraceData(ctx, spans); err != nil {
			log.Printf("Failed to insert %d OTLP spans: %v", len(spans), err)
			// Unavailable tells OTLP exporters the request may be retried.
			return nil, status.Errorf(codes.Unavailable, "insert spans: %v", err)
		}
	}

	response := &collectortracepb.ExportTraceServiceResponse{}
	if rejected > 0 {
		response.PartialSuccess = &collectortracepb.ExportTracePartialSuccess{
			RejectedSpans: rejected,
			ErrorMessage:  fmt.Sprintf("%d spans with an invalid trace, span or parent id were dropped", rejected),
		}
	}
	return response, nil
}

// otlpLogsServer accepts logs over the standard OpenTelemetry protocol (any
// OpenTelemetry SDK or Collector), stored like log-sender's: redacted, and
// answered only after they are written (internal/otlp/logs.go for the mapping).
type otlpLogsServer struct {
	collectorlogspb.UnimplementedLogsServiceServer
	models data.Models
}

func (s *otlpLogsServer) Export(ctx context.Context, req *collectorlogspb.ExportLogsServiceRequest) (*collectorlogspb.ExportLogsServiceResponse, error) {
	records, rejected := otlp.Logs(req.GetResourceLogs())

	if len(records) > 0 {
		if err := s.models.Log.InsertLogRecords(ctx, records); err != nil {
			log.Printf("Failed to insert %d OTLP log records: %v", len(records), err)
			// Unavailable tells OTLP exporters the request may be retried.
			return nil, status.Errorf(codes.Unavailable, "insert log records: %v", err)
		}
	}

	response := &collectorlogspb.ExportLogsServiceResponse{}
	if rejected > 0 {
		response.PartialSuccess = &collectorlogspb.ExportLogsPartialSuccess{
			RejectedLogRecords: rejected,
			ErrorMessage:       fmt.Sprintf("%d log records without a timestamp or with an invalid trace or span id were dropped", rejected),
		}
	}
	return response, nil
}
