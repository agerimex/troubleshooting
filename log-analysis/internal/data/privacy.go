package data

import (
	"context"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// Privacy scan (troubleshooting-check privacy): finds stored spans and log
// records of one run or service that carry what an application must never
// send. It reports the kind of finding, how many records have it and one
// example (trace and span id), never the value itself. Raw rows are scanned
// (not canonical ones): every stored copy matters.

// PrivacyRequest selects what to scan: a run, a service or both, in a window
// (left out: the last 24 h).
type PrivacyRequest struct {
	Run      string `json:"run,omitempty"`
	Service  string `json:"service,omitempty"`
	TimeFrom string `json:"time_from,omitempty"`
	TimeTo   string `json:"time_to,omitempty"`
}

// PrivacyFinding is one kind of leak.
type PrivacyFinding struct {
	Kind    string `json:"kind"`
	Source  string `json:"source"` // spans or logs
	Records uint64 `json:"records"`
	TraceID string `json:"trace_id,omitempty"`
	SpanID  string `json:"span_id,omitempty"`
	What    string `json:"what"`
}

// PrivacyResult is the scan's answer.
type PrivacyResult struct {
	Run      string           `json:"run,omitempty"`
	Service  string           `json:"service,omitempty"`
	TimeFrom string           `json:"time_from"`
	TimeTo   string           `json:"time_to"`
	Spans    uint64           `json:"spans"`
	Logs     uint64           `json:"logs"`
	Findings []PrivacyFinding `json:"findings"`
}

// privacyKinds are the patterns, matched against a record's text (name,
// status or body, and the attribute maps as text, keys included).
var privacyKinds = []struct{ kind, pattern, what string }{
	{"url_credentials", `[A-Za-z][A-Za-z0-9+.-]*://[^/?#@ '",]+@`, "a URL with user and password (scheme://user:pass@host), in a key or a value"},
	{"query_secret", `[?&](token|access_token|api_key|apikey|key|password|secret)=`, "a query string with a token, key, password or secret"},
	{"bearer_token", `(?i)bearer [A-Za-z0-9._~+/=-]{16,}`, "a bearer token"},
	{"password_hash", `\$2[aby]\$[0-9]{2}\$|\$argon2(id|i|d)\$`, "a bcrypt or argon2 password hash"},
}

// Validate checks the request and fills the default window.
func (q *PrivacyRequest) Validate() error {
	now := clock.Now()
	q.Run, q.Service = strings.TrimSpace(q.Run), strings.TrimSpace(q.Service)
	if q.Run == "" && q.Service == "" {
		return invalid("name a run or a service to scan")
	}
	if q.Run != "" && !validRunID(q.Run) {
		return invalid("run must be a run id of at most 128 characters")
	}
	if q.TimeFrom == "" && q.TimeTo == "" {
		q.TimeFrom, q.TimeTo = num(now.Add(-MaxQualityWindow+time.Minute).UnixNano()), num(now.Add(time.Minute).UnixNano())
	}
	_, _, err := parseWindow(q.TimeFrom, q.TimeTo, "")
	return err
}

// privacySpanQuery counts, over the selected spans, every kind of finding
// and SQL arguments from senders that didn't opt in, each with an example.
func privacySpanQuery(q PrivacyRequest) (string, []any) {
	from, to, _ := parseWindow(q.TimeFrom, q.TimeTo, "")
	text := `concat("SpanName", ' ', "StatusMessage", ' ', toString(mapKeys("SpanAttributes")), ' ', toString(mapValues("SpanAttributes")), ' ', toString("ResourceAttributes"), ' ', toString("Events.Attributes"), ' ', toString("Links.Attributes"))`
	sqlArgs := `(arrayExists(k -> startsWith(k, 'db.sql.args.') OR startsWith(k, 'db.query.parameter.'), mapKeys("SpanAttributes")) AND "ResourceAttributes"['troubleshooting.sql_args'] != 'sent')`
	cols := []string{"count()", "countIf(" + sqlArgs + ")", "anyIf(concat(\"TraceId\", ':', \"SpanId\"), " + sqlArgs + ")"}
	for _, k := range privacyKinds {
		cond := "match(" + text + ", " + quoteString(k.pattern) + ")"
		cols = append(cols, "countIf("+cond+")", "anyIf(concat(\"TraceId\", ':', \"SpanId\"), "+cond+")")
	}
	where, params := privacyWhere(q, from, to, spanAttr(AttrRun), `"ServiceName"`)
	return "SELECT " + strings.Join(cols, ", ") + " FROM otel_traces WHERE " + where, params
}

// privacyLogQuery does the same for log records.
func privacyLogQuery(q PrivacyRequest) (string, []any) {
	from, to, _ := parseWindow(q.TimeFrom, q.TimeTo, "")
	text := `concat("Body", ' ', toString(mapKeys("Attributes")), ' ', toString(mapValues("Attributes")))`
	sqlArgs := `arrayExists(k -> startsWith(k, 'db.sql.args.') OR startsWith(k, 'db.query.parameter.'), mapKeys("Attributes"))`
	cols := []string{"count()", "countIf(" + sqlArgs + ")", "anyIf(concat(\"TraceId\", ':', \"SpanId\"), " + sqlArgs + ")"}
	for _, k := range privacyKinds {
		cond := "match(" + text + ", " + quoteString(k.pattern) + ")"
		cols = append(cols, "countIf("+cond+")", "anyIf(concat(\"TraceId\", ':', \"SpanId\"), "+cond+")")
	}
	where, params := privacyWhere(q, from, to, `"Attributes"['`+AttrRun+`']`, `"ServiceName"`)
	return "SELECT " + strings.Join(cols, ", ") + " FROM log_records WHERE " + where, params
}

func privacyWhere(q PrivacyRequest, from, to int64, runCol, serviceCol string) (string, []any) {
	conds := []string{`"UnixTime" >= ` + num(from), `"UnixTime" < ` + num(to)}
	var params []any
	if q.Run != "" {
		conds = append(conds, runCol+` = {privacyRun:String}`)
		params = append(params, clickhouse.Named("privacyRun", q.Run))
	}
	if q.Service != "" {
		conds = append(conds, serviceCol+` = {privacyService:String}`)
		params = append(params, clickhouse.Named("privacyService", q.Service))
	}
	return strings.Join(conds, " AND "), params
}

// scanPrivacy runs one of the queries and turns its row into findings: SQL
// arguments first, then privacyKinds, each with its count and an example
// ("trace:span").
func scanPrivacy(ctx context.Context, name, source, query string, params []any) (uint64, []PrivacyFinding, error) {
	ctx, span := startQuerySpan(ctx, name, query, params)
	var err error
	defer func() { endQuerySpan(span, err) }()
	n := 1 + len(privacyKinds)
	var total uint64
	counts := make([]uint64, n)
	examples := make([]string, n)
	dest := []any{&total}
	for i := 0; i < n; i++ {
		dest = append(dest, &counts[i], &examples[i])
	}
	if err = dbFor(ctx).QueryRow(withLimits(ctx), query, params...).Scan(dest...); err != nil {
		return 0, nil, err
	}
	var out []PrivacyFinding
	for i := 0; i < n; i++ {
		if counts[i] == 0 {
			continue
		}
		kind, what := "sql_arguments", "SQL arguments (db.sql.args.*, db.query.parameter.*) from a sender that didn't opt in"
		if i > 0 {
			kind, what = privacyKinds[i-1].kind, privacyKinds[i-1].what
		}
		f := PrivacyFinding{Kind: kind, Source: source, Records: counts[i], What: what}
		f.TraceID, f.SpanID, _ = strings.Cut(examples[i], ":")
		out = append(out, f)
	}
	return total, out, nil
}

// ScanPrivacy scans the spans and log records of a validated request.
func ScanPrivacy(ctx context.Context, q PrivacyRequest) (PrivacyResult, error) {
	res := PrivacyResult{Run: q.Run, Service: q.Service, TimeFrom: q.TimeFrom, TimeTo: q.TimeTo, Findings: []PrivacyFinding{}}
	query, params := privacySpanQuery(q)
	spans, found, err := scanPrivacy(ctx, "SELECT privacy spans", "spans", query, params)
	if err != nil {
		return res, err
	}
	res.Spans, res.Findings = spans, append(res.Findings, found...)
	query, params = privacyLogQuery(q)
	logs, found, err := scanPrivacy(ctx, "SELECT privacy logs", "logs", query, params)
	if err != nil {
		return res, err
	}
	res.Logs, res.Findings = logs, append(res.Findings, found...)
	return res, nil
}
