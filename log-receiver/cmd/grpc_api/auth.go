package main

import (
	"context"
	"crypto/subtle"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// tokenEnv enables authentication when set. log-sender reads the same variable.
const tokenEnv = "TROUBLESHOOTING_TOKEN"

// tokenAuth accepts only requests carrying "authorization: Bearer <token>".
func tokenAuth(token string) grpc.UnaryServerInterceptor {
	want := []byte("Bearer " + token)
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		for _, got := range md.Get("authorization") {
			if subtle.ConstantTimeCompare([]byte(got), want) == 1 {
				return handler(ctx, req)
			}
		}
		return nil, status.Error(codes.Unauthenticated, "missing or invalid troubleshooting token")
	}
}
