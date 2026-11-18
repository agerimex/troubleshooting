package data

import (
	"regexp"
	"sort"
	"strings"

	pb "github.com/agerimex/troubleshooting/protos/logs"
)

// Redaction applies log-sender's redaction rules (log-sender/redact.go) to
// every span and log record the receiver stores, so a plain OpenTelemetry
// sender is held to them too: SQL arguments are dropped unless the span's
// resource says the application opted in (troubleshooting.sql_args=sent);
// user:password is removed from URLs in keys and values; query strings are
// removed from http.target, http.url, url.full and url.query. A record that
// lost something lists the keys in troubleshooting.redacted. The rules are
// the same in both modules (kept in step by the same test cases); the
// receiver's are the second layer, log-sender's the first.
//
// Off with TROUBLESHOOTING_REDACT=off (cmd/grpc_api), e.g. for an
// installation that must keep everything as sent.
var Redaction = true

const (
	attrRedacted = "troubleshooting.redacted"
	attrSQLArgs  = "troubleshooting.sql_args"
)

var (
	userinfoPattern = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/?#@\s]+@`)
	queryKeys       = map[string]bool{"http.target": true, "http.url": true, "url.full": true}
)

func redactText(s string) string {
	if !strings.Contains(s, "@") {
		return s
	}
	return userinfoPattern.ReplaceAllString(s, "$1")
}

func isSQLArg(key string) bool {
	return strings.HasPrefix(key, "db.sql.args.") || strings.HasPrefix(key, "db.query.parameter.")
}

// redactMap cleans one attribute map in place and returns the keys it
// dropped or cleaned.
func redactMap(m map[string]string, keepSQLArgs bool) []string {
	var removed []string
	for key, v := range m {
		if (!keepSQLArgs && isSQLArg(key)) || key == "url.query" {
			delete(m, key)
			removed = append(removed, key)
			continue
		}
		clean := redactText(key)
		out := redactText(v)
		if queryKeys[clean] {
			if i := strings.IndexByte(out, '?'); i >= 0 {
				out = out[:i]
			}
		}
		if clean != key {
			delete(m, key)
			removed = append(removed, clean)
		}
		if out != v {
			removed = append(removed, clean)
		}
		m[clean] = out
	}
	return removed
}

func markRedacted(m map[string]string, removed []string) {
	if len(removed) == 0 {
		return
	}
	if prev := m[attrRedacted]; prev != "" {
		removed = append(removed, strings.Split(prev, ",")...)
	}
	sort.Strings(removed)
	out := removed[:0]
	for i, k := range removed {
		if i == 0 || k != removed[i-1] {
			out = append(out, k)
		}
	}
	m[attrRedacted] = strings.Join(out, ",")
}

// redactSpan cleans a span and its decoded events and links before insert.
func redactSpan(d *pb.OneSpan, events *spanEvents, links *spanLinks) {
	if d.SpanAttributes == nil {
		d.SpanAttributes = map[string]string{}
	}
	keep := d.ResourceAttributes[attrSQLArgs] == "sent"
	removed := redactMap(d.SpanAttributes, keep)
	removed = append(removed, redactMap(d.ResourceAttributes, true)...)
	if n := redactText(d.SpanName); n != d.SpanName {
		d.SpanName, removed = n, append(removed, "span.name")
	}
	if s := redactText(d.StatusMessage); s != d.StatusMessage {
		d.StatusMessage, removed = s, append(removed, "status.message")
	}
	for i := range events.attributes {
		events.names[i] = redactText(events.names[i])
		if events.attributes[i] != nil {
			removed = append(removed, redactMap(events.attributes[i], keep)...)
		}
	}
	for i := range links.attributes {
		if links.attributes[i] != nil {
			removed = append(removed, redactMap(links.attributes[i], keep)...)
		}
	}
	markRedacted(d.SpanAttributes, removed)
}

// redactRecord cleans a log record before insert.
func redactRecord(r *LogRecord) {
	if r.Attributes == nil {
		r.Attributes = map[string]string{}
	}
	removed := redactMap(r.Attributes, false)
	if b := redactText(r.Body); b != r.Body {
		r.Body, removed = b, append(removed, "message")
	}
	markRedacted(r.Attributes, removed)
}
