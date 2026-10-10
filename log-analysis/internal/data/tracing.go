package data

import (
	"context"
	"fmt"
	"regexp"

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
	return tracer.Start(ctx, "clickhouse "+name,
		trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attrs...))
}

func endQuerySpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
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
