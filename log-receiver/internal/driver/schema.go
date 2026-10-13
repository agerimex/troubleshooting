package driver

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// The receiver owns the schema: on startup it creates every table and brings
// existing installations up to date (EnsureSchema). log-analysis only reads
// raw records and writes statistics, coverage declarations and analyses.

// otelTracesSQL is the raw span table. Retries and replays add rows with the
// same (TraceId, SpanId); log-analysis reads one canonical row per identity.
// ReceivedTime is when the receiver stored the row (0 for rows stored before
// it was recorded); statistics use the first one per identity.
const otelTracesSQL = `
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
		"ReceivedTime" Int64 DEFAULT 0 CODEC(Delta(8), ZSTD(1)),
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
	TTL %s
	SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1`

// logRecordsSQL replaces the old "logs" table, which had no trace correlation
// and is no longer written (an existing one expires through its TTL).
// Retries add rows with the same RecordId; log-analysis reads one row per
// RecordId.
const logRecordsSQL = `
	CREATE TABLE IF NOT EXISTS log_records
	(
		"UnixTime" Int64 CODEC(Delta(8), ZSTD(1)),
		"RecordId" String CODEC(ZSTD(1)),
		"TraceId" String CODEC(ZSTD(1)),
		"SpanId" String CODEC(ZSTD(1)),
		"Severity" LowCardinality(String) CODEC(ZSTD(1)),
		"ServiceName" LowCardinality(String) CODEC(ZSTD(1)),
		"Body" String CODEC(ZSTD(1)),
		"Attributes" Map(LowCardinality(String), String) CODEC(ZSTD(1)),
		"ReceivedTime" Int64 DEFAULT 0 CODEC(Delta(8), ZSTD(1)),
		INDEX idx_trace_id TraceId TYPE bloom_filter(0.001) GRANULARITY 1,
		INDEX idx_record_id RecordId TYPE bloom_filter(0.001) GRANULARITY 1,
		INDEX idx_severity Severity TYPE bloom_filter(0.01) GRANULARITY 1,
		INDEX idx_service_name ServiceName TYPE bloom_filter(0.01) GRANULARITY 1
	)
	ENGINE = MergeTree
	PARTITION BY toDate(fromUnixTimestamp64Nano(UnixTime))
	ORDER BY (UnixTime)
	TTL %s
	SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1`

// Statistics (written by log-analysis). Each one-hour bucket is recomputed
// from canonical raw records as a whole and stored under a new Revision; the
// rows of a revision count only once its stat_revisions row exists (the
// commit marker), and readers use the latest committed revision per bucket.
// So a refresh never adds a second copy of a bucket, and a failed refresh
// leaves the previous revision in place. A "final" revision is never
// replaced.

const statCallsSQL = `
	CREATE TABLE IF NOT EXISTS stat_calls
	(
		"BucketStart" DateTime('UTC'),
		"Revision" Int64,
		"ServiceName" LowCardinality(String),
		"Operation" LowCardinality(String),
		"SpanKind" LowCardinality(String),
		"Environment" LowCardinality(String),
		"Version" LowCardinality(String),
		"Installation" LowCardinality(String),
		"Segment" LowCardinality(String),
		"Sampling" LowCardinality(String),
		"Calls" UInt64,
		"Errors" UInt64,
		"Ok" UInt64,
		"Unset" UInt64,
		"ValidDurations" UInt64,
		"InvalidDurations" UInt64,
		"Conflicts" UInt64,
		"Durations" AggregateFunction(quantilesTDigestIf(0.5, 0.95, 0.99), Float64, UInt8)
	)
	ENGINE = MergeTree
	PARTITION BY toYYYYMM(BucketStart)
	ORDER BY (BucketStart, Revision, ServiceName, Operation)
	TTL %s`

const statErrorsSQL = `
	CREATE TABLE IF NOT EXISTS stat_errors
	(
		"BucketStart" DateTime('UTC'),
		"Revision" Int64,
		"SourceKind" LowCardinality(String),
		"ServiceName" LowCardinality(String),
		"Operation" LowCardinality(String),
		"Environment" LowCardinality(String),
		"Version" LowCardinality(String),
		"Installation" LowCardinality(String),
		"Segment" LowCardinality(String),
		"Method" LowCardinality(String),
		"Fingerprint" String,
		"ExceptionType" String,
		"Frame" String,
		"Signature" String,
		"Occurrences" UInt64,
		"Unlinked" UInt64,
		"Traces" AggregateFunction(uniqExactIf, String, UInt8),
		"FirstSeen" Int64,
		"LastSeen" Int64,
		"Calls" AggregateFunction(uniqExactIf, String, UInt8),
		"HasCalls" UInt8 DEFAULT 0
	)
	ENGINE = MergeTree
	PARTITION BY toYYYYMM(BucketStart)
	ORDER BY (BucketStart, Revision, SourceKind, Fingerprint)
	TTL %s`

// statShapesSQL holds the sequence library: per bucket and revision, one row
// per execution shape (entry operation and the set of distinct
// caller -> callee calls of its traces), with the number of executions,
// their durations and a bounded sample of them (duration µs, TraceId, root
// start ns) for opening a representative one.
const statShapesSQL = `
	CREATE TABLE IF NOT EXISTS stat_shapes
	(
		"BucketStart" DateTime('UTC'),
		"Revision" Int64,
		"ShapeId" UInt64,
		"EntryService" LowCardinality(String),
		"EntryOperation" LowCardinality(String),
		"Services" Array(LowCardinality(String)),
		"Calls" UInt32,
		"Traces" UInt64,
		"ErrorTraces" UInt64,
		"InvalidDurations" UInt64,
		"Durations" AggregateFunction(quantilesTDigestIf(0.5, 0.95, 0.99), Float64, UInt8),
		"Samples" Array(Tuple(Int64, String, Int64)),
		"Slowest" Tuple(Int64, String, Int64)
	)
	ENGINE = MergeTree
	PARTITION BY toYYYYMM(BucketStart)
	ORDER BY (BucketStart, Revision, ShapeId)
	TTL %s`

const statRevisionsSQL = `
	CREATE TABLE IF NOT EXISTS stat_revisions
	(
		"BucketStart" DateTime('UTC'),
		"Revision" Int64,
		"State" LowCardinality(String),
		"Definitions" LowCardinality(String),
		"GeneratedAt" Int64,
		"ReceiptCutoff" Int64,
		"ContentHash" UInt64,
		"Calls" UInt64,
		"ErrorEvents" UInt64,
		"UnknownReceipt" UInt64,
		"Unassignable" UInt64,
		"Trigger" LowCardinality(String)
	)
	ENGINE = MergeTree
	ORDER BY (BucketStart, Revision)
	TTL %s`

// collection_heartbeats: a running receiver that can write to ClickHouse
// records one row every few seconds. Minutes without one are collection gaps.
const heartbeatsSQL = `
	CREATE TABLE IF NOT EXISTS collection_heartbeats
	(
		"Time" DateTime('UTC'),
		"Receiver" LowCardinality(String)
	)
	ENGINE = MergeTree
	ORDER BY (Time)
	TTL %s`

// coverage_declarations record coverage the heartbeats can't know: a declared
// gap (an outage upstream of the receiver) or complete coverage of data
// collected before heartbeats existed (backfill, a replayed incident).
const coverageSQL = `
	CREATE TABLE IF NOT EXISTS coverage_declarations
	(
		"DeclaredAt" Int64,
		"Source" LowCardinality(String),
		"From" Int64,
		"To" Int64,
		"Status" LowCardinality(String),
		"Reason" String
	)
	ENGINE = MergeTree
	ORDER BY (From)
	TTL %s`

// analyses keeps generated comparisons exactly as they were shown, so an
// export or a later look never recomputes them.
const analysesSQL = `
	CREATE TABLE IF NOT EXISTS analyses
	(
		"Id" String,
		"Kind" LowCardinality(String),
		"CreatedAt" Int64,
		"Title" String,
		"Body" String
	)
	ENGINE = MergeTree
	ORDER BY (CreatedAt, Id)
	TTL %s`

// statErrorsCallColumns were added after the first statistics release: Calls
// counts distinct (TraceId, SpanId) of a group's occurrences, and HasCalls
// marks rows that have it (0 in older rows, whose call counts are unknown).
var statErrorsCallColumns = []string{
	`"Calls" AggregateFunction(uniqExactIf, String, UInt8)`,
	`"HasCalls" UInt8 DEFAULT 0`,
}

// detection_rules keeps every saved revision of the problem detection rules
// (log-analysis). It has no TTL: a configuration must not expire.
const detectionRulesSQL = `
	CREATE TABLE IF NOT EXISTS detection_rules
	(
		"Scope" LowCardinality(String),
		"Revision" Int64,
		"SavedAt" Int64,
		"EffectiveAt" Int64,
		"Body" String
	)
	ENGINE = MergeTree
	ORDER BY ("Scope", "Revision")`

type table struct {
	name   string
	create string
	ttl    string // TTL expression of this table, with %d for seconds
	keep   func(Retention) time.Duration
}

const (
	eventTimeTTL = `toDateTime(fromUnixTimestamp64Nano(UnixTime)) + toIntervalSecond(%d)`
	bucketTTL    = `BucketStart + toIntervalSecond(%d)`
	heartbeatTTL = `Time + toIntervalSecond(%d)`
	nanoTTL      = `toDateTime(fromUnixTimestamp64Nano(%s)) + toIntervalSecond(%%d)`
)

func tables() []table {
	traces := func(r Retention) time.Duration { return r.Traces }
	logs := func(r Retention) time.Duration { return r.Logs }
	stats := func(r Retention) time.Duration { return r.Stats }
	return []table{
		{"otel_traces", otelTracesSQL, eventTimeTTL, traces},
		{"log_records", logRecordsSQL, eventTimeTTL, logs},
		{"stat_calls", statCallsSQL, bucketTTL, stats},
		{"stat_errors", statErrorsSQL, bucketTTL, stats},
		{"stat_shapes", statShapesSQL, bucketTTL, stats},
		{"stat_revisions", statRevisionsSQL, bucketTTL, stats},
		{"collection_heartbeats", heartbeatsSQL, heartbeatTTL, stats},
		{"coverage_declarations", coverageSQL, fmt.Sprintf(nanoTTL, `"To"`), stats},
		{"analyses", analysesSQL, fmt.Sprintf(nanoTTL, `"CreatedAt"`), stats},
	}
}

// EnsureSchema creates the tables, adds columns missing in older
// installations and applies the configured retention to existing tables. It
// is idempotent: a TTL that already matches is left alone, because changing
// one makes ClickHouse rewrite the table's parts.
func EnsureSchema(ctx context.Context, conn driver.Conn, r Retention) error {
	for _, t := range tables() {
		ttl := fmt.Sprintf(t.ttl, int64(t.keep(r).Seconds()))
		if err := conn.Exec(ctx, fmt.Sprintf(t.create, ttl)); err != nil {
			return fmt.Errorf("create %s: %w", t.name, err)
		}
	}
	if err := conn.Exec(ctx, detectionRulesSQL); err != nil {
		return fmt.Errorf("create detection_rules: %w", err)
	}
	for _, name := range []string{"otel_traces", "log_records"} {
		err := conn.Exec(ctx, `ALTER TABLE `+name+` ADD COLUMN IF NOT EXISTS "ReceivedTime" Int64 DEFAULT 0 CODEC(Delta(8), ZSTD(1))`)
		if err != nil {
			return fmt.Errorf("add ReceivedTime to %s: %w", name, err)
		}
	}
	for _, column := range statErrorsCallColumns {
		if err := conn.Exec(ctx, `ALTER TABLE stat_errors ADD COLUMN IF NOT EXISTS `+column); err != nil {
			return fmt.Errorf("add a call column to stat_errors: %w", err)
		}
	}
	for _, t := range tables() {
		seconds := int64(t.keep(r).Seconds())
		current, err := tableTTLSeconds(ctx, conn, t.name)
		if err != nil {
			return err
		}
		if current == seconds {
			continue
		}
		log.Printf("retention of %s: %s -> %s (removal of expired parts happens in the background)",
			t.name, time.Duration(current)*time.Second, time.Duration(seconds)*time.Second)
		if err := conn.Exec(ctx, `ALTER TABLE `+t.name+` MODIFY TTL `+fmt.Sprintf(t.ttl, seconds)); err != nil {
			return fmt.Errorf("set retention of %s: %w", t.name, err)
		}
	}
	return nil
}

// tableTTLSeconds reads the retention a table really has from its engine
// definition (-1 if it has none that can be read).
func tableTTLSeconds(ctx context.Context, conn driver.Conn, name string) (int64, error) {
	var engine string
	err := conn.QueryRow(ctx, `SELECT engine_full FROM system.tables WHERE database = currentDatabase() AND name = {name:String}`,
		clickhouse.Named("name", name)).Scan(&engine)
	if err != nil {
		return 0, fmt.Errorf("read retention of %s: %w", name, err)
	}
	return TTLSeconds(engine), nil
}

// TTLSeconds parses the interval of the first TTL expression of an
// engine_full definition (toIntervalSecond/Hour/Day); -1 if there is none.
func TTLSeconds(engine string) int64 {
	i := strings.Index(engine, "TTL ")
	if i < 0 {
		return -1
	}
	rest := engine[i:]
	for unit, seconds := range map[string]int64{"toIntervalSecond(": 1, "toIntervalHour(": 3600, "toIntervalDay(": 86400} {
		j := strings.Index(rest, unit)
		if j < 0 {
			continue
		}
		end := strings.Index(rest[j:], ")")
		if end < 0 {
			continue
		}
		n, err := strconv.ParseInt(rest[j+len(unit):j+end], 10, 64)
		if err == nil {
			return n * seconds
		}
	}
	return -1
}
