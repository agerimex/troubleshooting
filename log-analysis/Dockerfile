# Build from the repository root (see Makefile): go.mod replaces log-sender
# and protos with ../log-sender and ../protos.
FROM golang:1.21 AS builder

WORKDIR /src

COPY protos/go.mod protos/
COPY log-sender/go.mod log-sender/go.sum log-sender/
COPY log-analysis/go.mod log-analysis/go.sum log-analysis/

RUN cd log-analysis && go mod download

COPY protos/ protos/
COPY log-sender/ log-sender/
COPY log-analysis/ log-analysis/

RUN cd log-analysis && CGO_ENABLED=0 GOOS=linux go build -o /app/log-analysis ./cmd/rest_api

FROM ubuntu:22.04 as base

WORKDIR /app

COPY --from=builder /app/log-analysis .

EXPOSE 8094

ENTRYPOINT ["/app/log-analysis"]
