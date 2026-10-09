package driver

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

func СreateLogs(ctx context.Context, conn driver.Conn) {
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS logs (
			id Int64 CODEC(ZSTD(1)),
			timestamp DateTime CODEC(Delta(8), ZSTD(1)),
			level String CODEC(ZSTD(1)),
			message String CODEC(ZSTD(1)),
			spanid String CODEC(ZSTD(1))
		) ENGINE = MergeTree
		PARTITION BY toDate(timestamp)
		ORDER BY (spanid)
		TTL toDateTime(timestamp) + toIntervalDay(3)
		SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1
	`

	err := conn.Exec(ctx, createTableSQL)
	if err != nil {
		log.Fatal(err)
	}
}

func CreateTracer(ctx context.Context, conn driver.Conn) {

	createTableSQL := `
		CREATE TABLE IF NOT EXISTS otel_traces
		(
			"UnixTime" Int64 CODEC(Delta(8), ZSTD(1)),
			"TraceId" String CODEC(ZSTD(1)),
			"SpanId" String CODEC(ZSTD(1)),
			"ParentSpanId" String CODEC(ZSTD(1)),
			"TraceState" String CODEC(ZSTD(1)),
			"SpanName" LowCardinality(String) CODEC(ZSTD(1)),
			"SpanKind" LowCardinality(String) CODEC(ZSTD(1)),
			"ServiceName" LowCardinality(String) CODEC(ZSTD(1)),
			"ResourceAttributes" Map(LowCardinality(String), String) CODEC(ZSTD(1)),
			"SpanAttributes" Map(LowCardinality(String), String) CODEC(ZSTD(1)),
			"Duration" Int64 CODEC(ZSTD(1)),
			"StatusCode" Int8 CODEC(ZSTD(1)),
			"StatusMessage" String CODEC(ZSTD(1)),
			"Events.Timestamp" Array(DateTime64(9)) CODEC(ZSTD(1)),
			"Events.Name" Array(LowCardinality(String)) CODEC(ZSTD(1)),
			"Events.Attributes" Array(Map(LowCardinality(String), String)) CODEC(ZSTD(1)),
			"Links.TraceId" Array(String) CODEC(ZSTD(1)),
			"Links.SpanId" Array(String) CODEC(ZSTD(1)),
			"Links.TraceState" Array(String) CODEC(ZSTD(1)),
			"Links.Attributes" Array(Map(LowCardinality(String), String)) CODEC(ZSTD(1)),
			"ChildSpanCount" Int32 CODEC(ZSTD(1)),
			INDEX idx_trace_id TraceId TYPE bloom_filter(0.001) GRANULARITY 1,
			INDEX idx_parent_trace_id ParentSpanId TYPE bloom_filter(0.001) GRANULARITY 1,
			INDEX idx_status_code StatusCode TYPE bloom_filter(0.01) GRANULARITY 1,
			INDEX idx_service_name ServiceName TYPE bloom_filter(0.01) GRANULARITY 1,
			INDEX idx_span_name SpanName TYPE bloom_filter(0.01) GRANULARITY 1,
			INDEX idx_res_attr_key mapKeys(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
			INDEX idx_res_attr_value mapValues(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
			INDEX idx_span_attr_key mapKeys(SpanAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
			INDEX idx_span_attr_value mapValues(SpanAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
			INDEX idx_duration Duration TYPE minmax GRANULARITY 1
		)
		ENGINE = MergeTree
		PARTITION BY toDate(fromUnixTimestamp64Nano(UnixTime))
		ORDER BY (UnixTime)
		TTL toDateTime(fromUnixTimestamp64Nano(UnixTime)) + toIntervalDay(3)
		SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1
	`

	err := conn.Exec(ctx, createTableSQL)
	if err != nil {
		log.Fatal(err)
	}
}

// func CreateTracer(ctx context.Context, conn driver.Conn) {

// 	createTableSQL := `
// 		CREATE TABLE IF NOT EXISTS otel_traces
// 		(
// 			"UnixTime" Int64 CODEC(Delta(8), ZSTD(1)),
// 			"Timestamp" DateTime64(9) CODEC(Delta(8), ZSTD(1)),
// 			"Timestamp2" DateTime64 CODEC(Delta(8), ZSTD(1)),
// 			"TraceId" String CODEC(ZSTD(1)),
// 			"SpanId" String CODEC(ZSTD(1)),
// 			"ParentSpanId" String CODEC(ZSTD(1)),
// 			"TraceState" String CODEC(ZSTD(1)),
// 			"SpanName" LowCardinality(String) CODEC(ZSTD(1)),
// 			"SpanKind" LowCardinality(String) CODEC(ZSTD(1)),
// 			"ServiceName" LowCardinality(String) CODEC(ZSTD(1)),
// 			"ResourceAttributes" Map(LowCardinality(String), String) CODEC(ZSTD(1)),
// 			"SpanAttributes" Map(LowCardinality(String), String) CODEC(ZSTD(1)),
// 			"Duration" Int64 CODEC(ZSTD(1)),
// 			"StatusCode" Int8 CODEC(ZSTD(1)),
// 			"StatusMessage" String CODEC(ZSTD(1)),
// 			"Events.Timestamp" Array(DateTime64(9)) CODEC(ZSTD(1)),
// 			"Events.Name" Array(LowCardinality(String)) CODEC(ZSTD(1)),
// 			"Events.Attributes" Array(Map(LowCardinality(String), String)) CODEC(ZSTD(1)),
// 			"Links.TraceId" Array(String) CODEC(ZSTD(1)),
// 			"Links.SpanId" Array(String) CODEC(ZSTD(1)),
// 			"Links.TraceState" Array(String) CODEC(ZSTD(1)),
// 			"Links.Attributes" Array(Map(LowCardinality(String), String)) CODEC(ZSTD(1)),
// 			"ChildSpanCount" Int32 CODEC(ZSTD(1)),
// 			INDEX idx_trace_id TraceId TYPE bloom_filter(0.001) GRANULARITY 1,
// 			INDEX idx_status_code StatusCode TYPE bloom_filter(0.01) GRANULARITY 1,
// 			INDEX idx_res_attr_key mapKeys(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
// 			INDEX idx_res_attr_value mapValues(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
// 			INDEX idx_span_attr_key mapKeys(SpanAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
// 			INDEX idx_span_attr_value mapValues(SpanAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
// 			INDEX idx_duration Duration TYPE minmax GRANULARITY 1
// 		)
// 		ENGINE = MergeTree
// 		PARTITION BY toDate(Timestamp)
// 		ORDER BY (ServiceName, SpanName, toUnixTimestamp(Timestamp), TraceId)
// 		TTL toDateTime(Timestamp) + toIntervalDay(3)
// 		SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1
// 	`

// 	err := conn.Exec(ctx, createTableSQL)
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// }

type Config struct {
	Addr     string
	Database string
	Username string
	Password string
}

// ConfigFromEnv reads CLICKHOUSE_ADDR, CLICKHOUSE_DATABASE, CLICKHOUSE_USER and
// CLICKHOUSE_PASSWORD. The defaults match docker-compose; for local runs
// against the compose ClickHouse use CLICKHOUSE_ADDR=localhost:19000.
func ConfigFromEnv() Config {
	return Config{
		Addr:     getenv("CLICKHOUSE_ADDR", "clickhouse-server:9000"),
		Database: getenv("CLICKHOUSE_DATABASE", "default"),
		Username: getenv("CLICKHOUSE_USER", "default"),
		Password: os.Getenv("CLICKHOUSE_PASSWORD"),
	}
}

func getenv(key, def string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return def
}

func Connect(cfg Config) (driver.Conn, error) {
	return clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},

		Debugf: func(format string, v ...interface{}) {
			fmt.Printf(format, v...)
		},
	})
}
