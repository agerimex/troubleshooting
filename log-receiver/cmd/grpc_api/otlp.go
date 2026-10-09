package main

import (
	"context"
	"fmt"
	"log"

	"log-receiver/internal/data"
	"log-receiver/internal/otlp"

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
			ErrorMessage:  fmt.Sprintf("%d spans without a valid trace or span id were dropped", rejected),
		}
	}
	return response, nil
}
