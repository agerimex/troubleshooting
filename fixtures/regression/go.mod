module github.com/agerimex/troubleshooting/fixtures/regression

go 1.21.5

require (
	github.com/agerimex/troubleshooting/log-sender v0.0.0-00010101000000-000000000000
	go.opentelemetry.io/otel v1.16.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.16.0
	go.opentelemetry.io/otel/sdk v1.16.0
	go.opentelemetry.io/otel/trace v1.16.0
	go.opentelemetry.io/proto/otlp v1.0.0
	google.golang.org/grpc v1.57.0
)

require (
	github.com/agerimex/troubleshooting/protos v0.0.0-20250610202227-67b33e660507 // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.16.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/rs/zerolog v1.31.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.16.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.16.0 // indirect
	go.opentelemetry.io/otel/metric v1.16.0 // indirect
	golang.org/x/net v0.10.0 // indirect
	golang.org/x/sys v0.12.0 // indirect
	golang.org/x/text v0.9.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20230530153820-e85fd2cbaebc // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230530153820-e85fd2cbaebc // indirect
	google.golang.org/protobuf v1.31.0 // indirect
)

// The fixture checks this repository's log-sender and protos, not the
// published versions.
replace (
	github.com/agerimex/troubleshooting/log-sender => ../../log-sender
	github.com/agerimex/troubleshooting/protos => ../../protos
)
