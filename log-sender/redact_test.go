package sender

import (
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// leakySpan carries the three leaks a real integration had: SQL arguments, a
// token in the request URI, and a camera's user:password in an attribute key
// (and value).
func leakySpan() sdktrace.ReadOnlySpan {
	start := time.Unix(1700000000, 0)
	return tracetest.SpanStub{
		Name:      "GET /wsplayercommand",
		StartTime: start,
		EndTime:   start.Add(time.Millisecond),
		Resource:  resource.NewSchemaless(attribute.String("service.name", "app")),
		Attributes: []attribute.KeyValue{
			attribute.String("db.statement", "select * from users where login = ?"),
			attribute.String("db.sql.args.0", "alice"),
			attribute.String("db.query.parameter.password", "hash"),
			attribute.String("http.target", "/wsplayercommand?token=SECRET123&room=1"),
			attribute.String("url.query", "token=SECRET123"),
			attribute.String("rtsp://root:pw@10.0.0.5/live", "rtsp://root:pw@10.0.0.5/live"),
			attribute.StringSlice("upstreams", []string{"postgres://app:s3cret@db/app", "http://plain/x"}),
			attribute.String("custom.secret", "x"),
			attribute.Int64("http.status_code", 200),
		},
		Status: sdktrace.Status{Description: "dial rtsp://root:pw@10.0.0.5/live: timeout"},
		Events: []sdktrace.Event{{Name: "retry", Time: start,
			Attributes: []attribute.KeyValue{attribute.String("url.full", "https://u:p@api.example/x?key=k")}}},
	}.Snapshot()
}

func TestRedactionByDefault(t *testing.T) {
	r := redaction{keys: map[string]bool{"custom.secret": true}}
	got := spanToProtoWith(leakySpan(), r)
	all := got.SpanName + " " + got.StatusMessage
	for k, v := range got.SpanAttributes {
		all += " " + k + "=" + v
	}
	for _, e := range got.Events_Attributes {
		all += " " + e.Value
	}
	for _, leak := range []string{"alice", "hash", "SECRET123", "root:pw", "app:s3cret", "u:p@", "key=k", "custom.secret="} {
		if strings.Contains(all, leak) {
			t.Errorf("%q was sent: %s", leak, all)
		}
	}
	for k, want := range map[string]string{
		"db.statement":         "select * from users where login = ?",
		"http.target":          "/wsplayercommand",
		"rtsp://10.0.0.5/live": "rtsp://10.0.0.5/live",
		"upstreams":            "[postgres://db/app http://plain/x]",
		"http.status_code":     "200",
		AttrRedacted:           "custom.secret,db.query.parameter.password,db.sql.args.0,http.target,rtsp://10.0.0.5/live,status.message,upstreams,url.full,url.query",
	} {
		if got.SpanAttributes[k] != want {
			t.Errorf("%s = %q, want %q", k, got.SpanAttributes[k], want)
		}
	}
	if got.StatusMessage != "dial rtsp://10.0.0.5/live: timeout" {
		t.Errorf("status %q", got.StatusMessage)
	}
	if ev := got.Events_Attributes[0].Value; ev != `{"url.full":"https://api.example/x"}` {
		t.Errorf("event %s", ev)
	}
	// A clean span gets no marker.
	if _, ok := spanToProto(testSpan()).SpanAttributes[AttrRedacted]; ok {
		t.Error("a clean span is marked redacted")
	}
}

func TestRedactionWithSQLArgs(t *testing.T) {
	cfg := newConfig([]Option{WithSQLArgs()})
	got := spanToProtoWith(leakySpan(), cfg.redact)
	if got.SpanAttributes["db.sql.args.0"] != "alice" || got.SpanAttributes["db.query.parameter.password"] != "hash" {
		t.Errorf("WithSQLArgs keeps the arguments: %v", got.SpanAttributes)
	}
	if strings.Contains(got.SpanAttributes["http.target"], "SECRET123") {
		t.Error("WithSQLArgs keeps only SQL arguments, not query strings")
	}
}

func TestRedactionOfLogRecords(t *testing.T) {
	e := &LogExporter{service: "app", redact: redaction{}}
	p := e.toProto(LogRecord{Message: "camera rtsp://root:pw@10.0.0.5/live offline", Severity: "WARN",
		Attributes: map[string]string{"http.target": "/x?token=SECRET123", "db.sql.args.1": "bob", "room": "1"}})
	if p.Message != "camera rtsp://10.0.0.5/live offline" || p.Attributes["http.target"] != "/x" || p.Attributes["db.sql.args.1"] != "" || p.Attributes["room"] != "1" {
		t.Errorf("record %q %v", p.Message, p.Attributes)
	}
	if p.Attributes[AttrRedacted] != "db.sql.args.1,http.target,message" {
		t.Errorf("marker %q", p.Attributes[AttrRedacted])
	}
}

func TestRedactText(t *testing.T) {
	for in, want := range map[string]string{
		"rtsp://root:pw@cam/a":           "rtsp://cam/a",
		"postgres://u@db/x":              "postgres://db/x",
		"mail me at a@b.c":               "mail me at a@b.c",
		"http://host/a?x=y@z":            "http://host/a?x=y@z",
		"two: http://a:b@h1/ ftp://c@h2": "two: http://h1/ ftp://h2",
	} {
		if got := RedactText(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
