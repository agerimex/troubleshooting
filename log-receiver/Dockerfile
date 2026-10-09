# Build from the repository root (see Makefile): go.mod replaces protos with ../protos.
FROM golang:1.21 AS builder

WORKDIR /src

COPY protos/go.mod protos/
COPY log-receiver/go.mod log-receiver/go.sum log-receiver/

RUN cd log-receiver && go mod download

COPY protos/ protos/
COPY log-receiver/ log-receiver/

RUN cd log-receiver && CGO_ENABLED=0 GOOS=linux go build -o /app/grpc_server ./cmd/grpc_api

FROM ubuntu:22.04 as base

WORKDIR /app

COPY --from=builder /app/grpc_server .

EXPOSE 50055

ENTRYPOINT ["/app/grpc_server"]
