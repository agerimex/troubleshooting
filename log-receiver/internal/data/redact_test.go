package data

import (
	"strings"
	"testing"

	pb "github.com/agerimex/troubleshooting/protos/logs"
)

// The same leaks as log-sender's TestRedactionByDefault, arriving from a
// sender that doesn't redact (plain OTLP): the receiver cleans them.
func TestReceiverRedaction(t *testing.T) {
	d := &pb.OneSpan{SpanName: "GET /x", StatusMessage: "dial rtsp://root:pw@10.0.0.5/live: timeout",
		ResourceAttributes: map[string]string{"service.name": "app"},
		SpanAttributes: map[string]string{
			"db.statement":                 "select * from users where login = ?",
			"db.sql.args.0":                "alice",
			"db.query.parameter.password":  "hash",
			"http.target":                  "/wsplayercommand?token=SECRET123&room=1",
			"url.query":                    "token=SECRET123",
			"rtsp://root:pw@10.0.0.5/live": "rtsp://root:pw@10.0.0.5/live",
		}}
	events := spanEvents{names: []string{"retry"}, attributes: []map[string]string{{"url.full": "https://u:p@api.example/x?key=k"}}}
	links := spanLinks{attributes: []map[string]string{nil}}
	redactSpan(d, &events, &links)
	all := d.SpanName + " " + d.StatusMessage + " " + events.attributes[0]["url.full"]
	for k, v := range d.SpanAttributes {
		all += " " + k + "=" + v
	}
	for _, leak := range []string{"alice", "hash", "SECRET123", "root:pw", "u:p@", "key=k"} {
		if strings.Contains(all, leak) {
			t.Errorf("%q was stored: %s", leak, all)
		}
	}
	if got := d.SpanAttributes[attrRedacted]; got != "db.query.parameter.password,db.sql.args.0,http.target,rtsp://10.0.0.5/live,status.message,url.full,url.query" {
		t.Errorf("marker %q", got)
	}
	if d.SpanAttributes["http.target"] != "/wsplayercommand" || d.SpanAttributes["rtsp://10.0.0.5/live"] != "rtsp://10.0.0.5/live" {
		t.Errorf("attributes %v", d.SpanAttributes)
	}

	// A sender that opted in to SQL arguments keeps them; nothing else changes.
	opted := &pb.OneSpan{ResourceAttributes: map[string]string{attrSQLArgs: "sent"},
		SpanAttributes: map[string]string{"db.query.parameter.run": "ci-1", "http.target": "/a?b=c"}}
	redactSpan(opted, &spanEvents{}, &spanLinks{})
	if opted.SpanAttributes["db.query.parameter.run"] != "ci-1" || opted.SpanAttributes["http.target"] != "/a" {
		t.Errorf("opted in: %v", opted.SpanAttributes)
	}

	// Log records: message and attributes, never SQL arguments.
	r := LogRecord{Body: "camera rtsp://root:pw@10.0.0.5/live offline", Attributes: map[string]string{"db.sql.args.1": "bob", "room": "1"}}
	redactRecord(&r)
	if r.Body != "camera rtsp://10.0.0.5/live offline" || r.Attributes["db.sql.args.1"] != "" || r.Attributes["room"] != "1" || r.Attributes[attrRedacted] != "db.sql.args.1,message" {
		t.Errorf("record %q %v", r.Body, r.Attributes)
	}

	// A clean span is unchanged and unmarked.
	clean := &pb.OneSpan{SpanAttributes: map[string]string{"http.target": "/a", "db.statement": "select 1"}}
	redactSpan(clean, &spanEvents{}, &spanLinks{})
	if _, ok := clean.SpanAttributes[attrRedacted]; ok || len(clean.SpanAttributes) != 2 {
		t.Errorf("clean span changed: %v", clean.SpanAttributes)
	}
}
