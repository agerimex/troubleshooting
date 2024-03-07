FROM golang:1.21 AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o grpc_server ./cmd/grpc_api

FROM ubuntu:22.04 as base

WORKDIR /app

COPY --from=builder /app/grpc_server .

EXPOSE 50055

ENTRYPOINT ["/app/grpc_server"]