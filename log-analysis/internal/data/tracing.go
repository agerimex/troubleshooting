package data

import (
	"context"
	"fmt"
	"regexp"
	"sync"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const queryParamPrefix = "db.query.parameter."

var tracer = otel.Tracer("logs-backend/internal/data")

// namedPlaceholder matches ClickHouse query parameters such as {timeFrom:Int64}.
var namedPlaceholder = regexp.MustCompile(`\{(\w+):([^}]+)\}`)

// startQuerySpan starts a child span of ctx for one ClickHouse query. It records
// the statement and the values of the parameters the statement actually uses,
// so the span list can show the query as it ran (see fillNamedParams).
func startQuerySpan(ctx context.Context, name, query string, params []any) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String("db.system", "clickhouse"),
		attribute.String("db.statement", query),
	}
	used := make(map[string]bool)
	for _, m := range namedPlaceholder.FindAllStringSubmatch(query, -1) {
		used[m[1]] = true
	}
	for _, p := range params {
		if nv, ok := p.(driver.NamedValue); ok && used[nv.Name] {
			attrs = append(attrs, attribute.String(queryParamPrefix+nv.Name, fmt.Sprint(nv.Value)))
		}
	}
	ctx, span := tracer.Start(ctx, "clickhouse "+name,
		trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attrs...))
	if !span.IsRecording() {
		return ctx, span
	}
	// The query runs with the span's ids as its query id, so its row in
	// system.query_log can be found (querylog.go). What ClickHouse reports
	// during the query (profile events and progress) is recorded on the span
	// when it ends. Contexts derived from ctx (withLimits) keep both; a query
	// span runs one query at a time (two running queries can't share an id).
	sc := span.SpanContext()
	queryID := sc.TraceID().String() + "-" + sc.SpanID().String()
	span.SetAttributes(attribute.String(AttrDBQueryID, queryID))
	profile := &queryProfile{}
	ctx = clickhouse.Context(ctx,
		clickhouse.WithQueryID(queryID),
		clickhouse.WithProfileEvents(profile.events),
		clickhouse.WithProgress(profile.progress))
	return ctx, &profiledSpan{Span: span, profile: profile}
}

func endQuerySpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// Attributes of a query span measured by the database server: CPU time of the
// query's threads (only from query_log, querylog.go), its peak memory, and the
// rows and bytes it read.
const (
	AttrDBServerCPU      = "db.server.cpu_us"
	AttrDBServerMemory   = "db.server.memory_peak_bytes"
	AttrDBServerReadRows = "db.server.read_rows"
	AttrDBServerReadSize = "db.server.read_bytes"
)

// queryProfile keeps what ClickHouse sends during one query: the largest
// memory gauge (the query's peak; it matches query_log's memory_usage) and the
// progress packets, which are increments of rows and bytes read. CPU time is
// not taken from profile events: threads that finish after the last packet
// are missing from them (2 ms of a query query_log puts at 10 ms).
type queryProfile struct {
	mu        sync.Mutex
	seen      bool
	memory    int64
	readRows  uint64
	readBytes uint64
}

func (p *queryProfile) events(events []clickhouse.ProfileEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range events {
		if e.Type == "gauge" && (e.Name == "MemoryTrackerPeakUsage" || e.Name == "MemoryTrackerUsage") {
			p.seen = true
			if e.Value > p.memory {
				p.memory = e.Value
			}
		}
	}
}

func (p *queryProfile) progress(pr *clickhouse.Progress) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seen = true
	p.readRows += pr.Rows
	p.readBytes += pr.Bytes
}

func (p *queryProfile) attributes() []attribute.KeyValue {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.seen {
		return nil
	}
	var attrs []attribute.KeyValue
	if p.memory > 0 {
		attrs = append(attrs, attribute.Int64(AttrDBServerMemory, p.memory))
	}
	return append(attrs,
		attribute.Int64(AttrDBServerReadRows, int64(p.readRows)),
		attribute.Int64(AttrDBServerReadSize, int64(p.readBytes)))
}

// profiledSpan records the query profile when the span ends.
type profiledSpan struct {
	trace.Span
	profile *queryProfile
}

func (s *profiledSpan) End(options ...trace.SpanEndOption) {
	if attrs := s.profile.attributes(); len(attrs) > 0 {
		s.Span.SetAttributes(attrs...)
	}
	s.Span.End(options...)
}

// fillNamedParams replaces {name:Type} placeholders with their recorded values,
// quoting String values. Unknown placeholders are left as they are.
func fillNamedParams(statement string, params map[string]string) string {
	return namedPlaceholder.ReplaceAllStringFunc(statement, func(placeholder string) string {
		m := namedPlaceholder.FindStringSubmatch(placeholder)
		value, ok := params[m[1]]
		if !ok {
			return placeholder
		}
		if m[2] == "String" {
			return "'" + value + "'"
		}
		return value
	})
}
