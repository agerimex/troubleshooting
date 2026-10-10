package main

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestTokenAuth(t *testing.T) {
	interceptor := tokenAuth("secret")
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: "/logs.LogService/SendSpans"}

	tests := []struct {
		name   string
		header []string
		want   codes.Code
	}{
		{"valid token", []string{"authorization", "Bearer secret"}, codes.OK},
		{"wrong token", []string{"authorization", "Bearer nope"}, codes.Unauthenticated},
		{"no scheme", []string{"authorization", "secret"}, codes.Unauthenticated},
		{"no header", nil, codes.Unauthenticated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.header != nil {
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(tt.header...))
			}
			_, err := interceptor(ctx, nil, info, handler)
			if got := status.Code(err); got != tt.want {
				t.Errorf("code = %v, want %v", got, tt.want)
			}
		})
	}
}
