FROM golang:1.21 AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o log-analysis ./cmd/rest_api

FROM ubuntu:22.04 as base

WORKDIR /app

COPY --from=builder /app/log-analysis .

EXPOSE 8094

ENTRYPOINT ["/app/log-analysis"]