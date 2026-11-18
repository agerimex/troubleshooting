package sender

import (
	"regexp"
	"sort"
	"strings"

	"go.opentelemetry.io/otel/attribute"
)

// Redaction: log-sender keeps secrets and personal data out of what it sends,
// before anything leaves the application. Applied to every span (its name,
// status message, span, resource, event and link attributes) and every log
// record (message and attributes):
//
//   - SQL arguments (db.sql.args.*, db.query.parameter.*) are dropped, unless
//     the application opts in with WithSQLArgs;
//   - user and password in URLs (scheme://user:pass@host) are removed, in
//     attribute keys and values alike;
//   - query strings are removed from http.target, http.url, url.full and
//     url.query (tokens travel as ?token=…);
//   - keys named by WithRedactKeys are dropped.
//
// A span or record that lost something says so in AttrRedacted (the keys,
// comma-separated), so a reader knows the value was removed rather than
// never recorded. The receiver applies the same rules to every sender.

// AttrRedacted lists the attribute keys log-sender removed or cleaned.
const AttrRedacted = "troubleshooting.redacted"

// AttrSQLArgs is set to "sent" on the resource of an application that opted
// in to sending SQL arguments (WithSQLArgs); the receiver keeps them only
// then.
const AttrSQLArgs = "troubleshooting.sql_args"

var (
	userinfoPattern = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/?#@\s]+@`)
	sqlArgPrefixes  = []string{"db.sql.args.", "db.query.parameter."}
	queryKeys       = map[string]bool{"http.target": true, "http.url": true, "url.full": true}
)

type redaction struct {
	sqlArgs bool            // WithSQLArgs: keep SQL arguments
	keys    map[string]bool // WithRedactKeys
}

// WithSQLArgs keeps SQL arguments (db.sql.args.*, db.query.parameter.*) in
// spans. They are often personal data; log-sender logs once that it sends
// them, and marks the resource with AttrSQLArgs.
func WithSQLArgs() Option {
	return func(c *config) { c.redact.sqlArgs = true }
}

// WithRedactKeys drops these attribute keys from every span and log record,
// in addition to the built-in rules.
func WithRedactKeys(keys ...string) Option {
	return func(c *config) {
		if c.redact.keys == nil {
			c.redact.keys = map[string]bool{}
		}
		for _, k := range keys {
			c.redact.keys[k] = true
		}
	}
}

// RedactText removes user and password from every URL in s.
func RedactText(s string) string {
	if !strings.Contains(s, "@") {
		return s
	}
	return userinfoPattern.ReplaceAllString(s, "$1")
}

func isSQLArg(key string) bool {
	for _, p := range sqlArgPrefixes {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

func withoutQuery(v string) string {
	if i := strings.IndexByte(v, '?'); i >= 0 {
		return v[:i]
	}
	return v
}

// value cleans one string value of key; changed reports whether it did.
func (r redaction) value(key, v string) (string, bool) {
	out := RedactText(v)
	if queryKeys[key] {
		out = withoutQuery(out)
	}
	return out, out != v
}

// attributes applies the rules to a span's, resource's, event's or link's
// attributes; removed are the keys it dropped or cleaned.
func (r redaction) attributes(in []attribute.KeyValue) (out []attribute.KeyValue, removed []string) {
	out = make([]attribute.KeyValue, 0, len(in))
	for _, kv := range in {
		key := string(kv.Key)
		if (!r.sqlArgs && isSQLArg(key)) || r.keys[key] || key == "url.query" {
			removed = append(removed, key)
			continue
		}
		if clean := RedactText(key); clean != key {
			removed = append(removed, clean)
			kv.Key = attribute.Key(clean)
		}
		switch kv.Value.Type() {
		case attribute.STRING:
			if v, changed := r.value(string(kv.Key), kv.Value.AsString()); changed {
				removed = append(removed, string(kv.Key))
				kv.Value = attribute.StringValue(v)
			}
		case attribute.STRINGSLICE:
			vals := kv.Value.AsStringSlice()
			changed := false
			for i, v := range vals {
				if c, ch := r.value(string(kv.Key), v); ch {
					vals[i], changed = c, true
				}
			}
			if changed {
				removed = append(removed, string(kv.Key))
				kv.Value = attribute.StringSliceValue(vals)
			}
		}
		out = append(out, kv)
	}
	return out, removed
}

// stringMap applies the rules to attributes already flattened to strings (log
// records); removed are the keys it dropped or cleaned.
func (r redaction) stringMap(in map[string]string) (map[string]string, []string) {
	out := make(map[string]string, len(in))
	var removed []string
	for key, v := range in {
		if (!r.sqlArgs && isSQLArg(key)) || r.keys[key] || key == "url.query" {
			removed = append(removed, key)
			continue
		}
		clean := RedactText(key)
		if clean != key {
			removed = append(removed, clean)
		}
		if c, changed := r.value(clean, v); changed {
			removed = append(removed, clean)
			v = c
		}
		out[clean] = v
	}
	return out, removed
}

// marker joins removed keys for AttrRedacted: sorted, without duplicates.
func marker(removed []string) string {
	if len(removed) == 0 {
		return ""
	}
	sort.Strings(removed)
	out := removed[:0]
	for i, k := range removed {
		if i == 0 || k != removed[i-1] {
			out = append(out, k)
		}
	}
	return strings.Join(out, ",")
}
