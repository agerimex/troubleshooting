package orders

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TestMain exports spans over OTLP when OTEL_EXPORTER_OTLP_ENDPOINT is set
// (in CI, to the Troubleshooting started by quality-check/stack). The run id
// comes from OTEL_RESOURCE_ATTRIBUTES=troubleshooting.run=<id>; nothing in
// the application knows about it. Without the variable the tests just run.
func TestMain(m *testing.M) {
	ctx := context.Background()
	var shutdown func(context.Context) error
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		exp, err := otlptracegrpc.New(ctx) // endpoint and insecure from the OTEL_EXPORTER_OTLP_* variables
		if err != nil {
			panic(err)
		}
		res, err := resource.New(ctx, resource.WithFromEnv(), resource.WithAttributes(attribute.String("service.name", service)))
		if err != nil {
			panic(err)
		}
		tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp), sdktrace.WithResource(res))
		otel.SetTracerProvider(tp)
		shutdown = tp.Shutdown
	}
	code := m.Run()
	if shutdown != nil {
		if err := shutdown(ctx); err != nil { // flushes the spans
			println("exporting spans:", err.Error())
			code = 1
		}
	}
	os.Exit(code)
}

func TestOrders(t *testing.T) {
	s := httptest.NewServer(Handler(Store{Regression: os.Getenv("EXAMPLE_REGRESSION") == "1"}))
	defer s.Close()
	for i := 0; i < 40; i++ { // enough executions for a p95 (at least 20)
		resp, err := http.Get(s.URL + "/orders/o" + strconv.Itoa(i))
		if err != nil {
			t.Fatal(err)
		}
		var o Order
		err = json.NewDecoder(resp.Body).Decode(&o)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK || len(o.Items) != 8 || o.Total == 0 {
			t.Fatalf("order %d: %v %d %+v", i, err, resp.StatusCode, o)
		}
	}
	resp, err := http.Get(s.URL + "/orders/")
	if err != nil || resp.StatusCode != http.StatusNotFound {
		t.Errorf("no id: %v %v", err, resp)
	}
}
