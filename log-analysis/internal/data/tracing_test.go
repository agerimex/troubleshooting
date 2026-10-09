package data

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestFillNamedParams(t *testing.T) {
	got := fillNamedParams(
		`where "ParentSpanId" = {parent:String} and "UnixTime" > toInt64({timeFrom:Int64}) and x = {missing:String} limit {rowsPerPage:Int64}`,
		map[string]string{"parent": "0000000000000000", "timeFrom": "123", "rowsPerPage": "25"},
	)

	want := `where "ParentSpanId" = '0000000000000000' and "UnixTime" > toInt64(123) and x = {missing:String} limit 25`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestQuerySpanIsChildAndRecordsUsedParams(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	ctx, parent := provider.Tracer("test").Start(context.Background(), "request")
	filter := SpanFilter{ParentId: "0000000000000000", RowsPerPage: 25}
	where, limit := addFilter(filter)
	query := "SELECT 1 FROM otel_traces" + where + " " + limit

	_, span := startQuerySpan(ctx, "SELECT otel_traces", query, filterParams(filter))
	endQuerySpan(span, errors.New("boom"))
	parent.End()

	ended := recorder.Ended()
	if len(ended) != 2 {
		t.Fatalf("got %d ended spans, want 2", len(ended))
	}
	got := ended[0]
	if got.Name() != "clickhouse SELECT otel_traces" {
		t.Errorf("name = %q", got.Name())
	}
	if got.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Errorf("query span is not a child of the request span")
	}
	if got.Status().Code != codes.Error {
		t.Errorf("status = %v, want error", got.Status().Code)
	}

	attrs := map[string]string{}
	for _, kv := range got.Attributes() {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}
	if attrs["db.statement"] != query {
		t.Errorf("db.statement = %q", attrs["db.statement"])
	}
	want := map[string]string{"parent": "0000000000000000", "rowsPerPage": "25"}
	for name, value := range want {
		if attrs[queryParamPrefix+name] != value {
			t.Errorf("%s = %q, want %q", queryParamPrefix+name, attrs[queryParamPrefix+name], value)
		}
	}
	// Bound but unused parameters (e.g. timeFrom, serviceName) are not recorded.
	if _, ok := attrs[queryParamPrefix+"serviceName"]; ok {
		t.Errorf("unused parameter serviceName was recorded")
	}

	// The recorded statement reads back as the query that ran.
	params := map[string]string{}
	for name := range want {
		params[name] = attrs[queryParamPrefix+name]
	}
	filled := fillNamedParams(attrs["db.statement"], params)
	if filled != `SELECT 1 FROM otel_traces where "ParentSpanId" = '0000000000000000' limit 25` {
		t.Errorf("filled statement = %q", filled)
	}
}
