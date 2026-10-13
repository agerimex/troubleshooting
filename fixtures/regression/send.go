package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type spanIdentity struct{ trace, span string }

// identitiesOf lists the (trace id, span id) pairs of a request.
func identitiesOf(req *collectortracepb.ExportTraceServiceRequest) []spanIdentity {
	var out []spanIdentity
	for _, rs := range req.GetResourceSpans() {
		for _, ss := range rs.GetScopeSpans() {
			for _, s := range ss.GetSpans() {
				out = append(out, spanIdentity{hex.EncodeToString(s.GetTraceId()), hex.EncodeToString(s.GetSpanId())})
			}
		}
	}
	return out
}

type otlpSender struct {
	conn   *grpc.ClientConn
	client collectortracepb.TraceServiceClient
}

func dialOTLP(addr string) (*otlpSender, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &otlpSender{conn: conn, client: collectortracepb.NewTraceServiceClient(conn)}, nil
}

func (s *otlpSender) Close() error { return s.conn.Close() }

// send exports one request and returns how many spans the receiver rejected.
// The receiver writes the spans before it answers, so they are queryable
// once send returns.
func (s *otlpSender) send(ctx context.Context, req *collectortracepb.ExportTraceServiceRequest) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := s.client.Export(ctx, req)
	if err != nil {
		return 0, err
	}
	return resp.GetPartialSuccess().GetRejectedSpans(), nil
}

// sendAll exports the requests in order; any rejected span is an error.
func (s *otlpSender) sendAll(ctx context.Context, reqs []*collectortracepb.ExportTraceServiceRequest) error {
	for i, req := range reqs {
		rejected, err := s.send(ctx, req)
		if err != nil {
			return fmt.Errorf("request %d: %w", i, err)
		}
		if rejected != 0 {
			return fmt.Errorf("request %d: %d spans rejected", i, rejected)
		}
	}
	return nil
}
