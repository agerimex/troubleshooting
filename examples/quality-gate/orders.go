// Package orders is a small service for the quality gate example
// (docs/quality-gate.md, "An application's CI"): GET /orders/{id} reads an
// order's items and their prices.
//
// Its tests send spans over OTLP to the Troubleshooting started in CI, and
// the quality gate then checks the run against quality.yaml. Setting
// EXAMPLE_REGRESSION=1 plants a typical change that passes every functional
// test: prices read one by one (a query per item) and a call to the
// legacy-pricing service. The gate must fail on it.
package orders

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const service = "example-orders"

var tracer = otel.Tracer(service)

type Item struct {
	ID    string `json:"id"`
	Price int    `json:"price_cents"`
}

type Order struct {
	ID    string `json:"id"`
	Items []Item `json:"items"`
	Total int    `json:"total_cents"`
}

// Store simulates the database: every query is a CLIENT span with
// db.system, the way database instrumentation records it.
type Store struct {
	// Regression reads prices one by one and asks legacy-pricing for
	// discounts: the change the quality gate must catch.
	Regression bool
}

func (s Store) query(ctx context.Context, name string) func() {
	_, span := tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("db.system", "postgresql"), attribute.String("db.operation", strings.Fields(name)[0])))
	time.Sleep(time.Millisecond)
	return func() { span.End() }
}

func (s Store) items(ctx context.Context, order string) []string {
	defer s.query(ctx, "SELECT order_items")()
	return []string{order + "-1", order + "-2", order + "-3", order + "-4", order + "-5", order + "-6", order + "-7", order + "-8"}
}

func (s Store) prices(ctx context.Context, ids []string) map[string]int {
	out := map[string]int{}
	price := func(id string) int { return 100 + len(id)*7 }
	if !s.Regression {
		defer s.query(ctx, "SELECT prices WHERE item_id IN (...)")()
		for _, id := range ids {
			out[id] = price(id)
		}
		return out
	}
	for _, id := range ids {
		end := s.query(ctx, "SELECT prices WHERE item_id = ?")
		out[id] = price(id)
		end()
	}
	// An HTTP call to a service that is not instrumented: the client span
	// names it in peer.service.
	_, span := tracer.Start(ctx, "GET /discounts", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("peer.service", "legacy-pricing"), attribute.String("http.request.method", "GET")))
	time.Sleep(2 * time.Millisecond)
	span.End()
	return out
}

// Handler serves GET /orders/{id}.
func Handler(s Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "GET /orders/{id}", trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String("http.request.method", r.Method), attribute.String("url.path", r.URL.Path)))
		defer span.End()
		id := strings.TrimPrefix(r.URL.Path, "/orders/")
		if r.Method != http.MethodGet || id == "" || strings.Contains(id, "/") {
			span.SetStatus(codes.Error, "not found")
			http.NotFound(w, r)
			return
		}
		ids := s.items(ctx, id)
		prices := s.prices(ctx, ids)
		o := Order{ID: id}
		for _, it := range ids {
			o.Items = append(o.Items, Item{ID: it, Price: prices[it]})
			o.Total += prices[it]
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(o)
	})
}
