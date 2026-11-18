package data

import (
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// Stage 9: the quick wins after the first external pilot.

// A request without a window takes the run's own (and the baseline's own);
// half a window is still an error, and an explicit run window is still the
// baseline's default.
func TestQualityWindowFromRun(t *testing.T) {
	req := QualityRequest{Rules: referenceRules(), Run: "candidate", Baseline: "baseline"}
	if err := req.Validate(); err != nil {
		t.Fatalf("no window: %v", err)
	}
	if !req.windowFromRun || !req.baselineFromRun || req.BaselineFrom != "" {
		t.Errorf("from run %v, baseline from run %v, baseline window %q", req.windowFromRun, req.baselineFromRun, req.BaselineFrom)
	}
	half := QualityRequest{Rules: referenceRules(), Run: "candidate", TimeFrom: num(qBase)}
	if err := half.Validate(); err == nil || !strings.Contains(err.Error(), "time_to") {
		t.Errorf("half a window: %v", err)
	}
	explicit := validRequest(t, referenceRules(), "candidate", "baseline")
	if explicit.windowFromRun || explicit.baselineFromRun || explicit.BaselineFrom != explicit.TimeFrom {
		t.Errorf("explicit window: %+v", explicit)
	}
	// A run window from the run with an explicit baseline window keeps it.
	mixed := QualityRequest{Rules: referenceRules(), Run: "candidate", Baseline: "baseline",
		BaselineFrom: num(qBase), BaselineTo: num(qBase + int64(time.Hour))}
	if err := mixed.Validate(); err != nil || !mixed.windowFromRun || mixed.baselineFromRun {
		t.Errorf("mixed: %v %+v", err, mixed)
	}

	// The window found is [first, last+1); none found is the whole search
	// range (checks then report no executions), and always fits the limit.
	now := time.Unix(0, qBase+int64(48*time.Hour))
	lo, hi := runWindowBounds(now, 72*3600)
	if time.Duration(hi-lo) > MaxQualityWindow {
		t.Errorf("search range %s is longer than %s", time.Duration(hi-lo), MaxQualityWindow)
	}
	if from, to := runWindow(3, 100, 200, lo, hi); from != 100 || to != 201 {
		t.Errorf("window %d..%d", from, to)
	}
	if from, to := runWindow(0, 0, 0, lo, hi); from != lo || to != hi {
		t.Errorf("empty run: %d..%d", from, to)
	}
	// Retention wins over the 24 h look-back.
	if lo2, _ := runWindowBounds(now, 3600); lo2 != now.Add(-time.Hour).UnixNano() {
		t.Errorf("retention bound %d", lo2)
	}

	res := buildQualityResult(validRequest(t, referenceRules(), "candidate", ""), runSpans{}, runSpans{}, now)
	if res.WindowFromRun {
		t.Error("an explicit window is not from the run")
	}
	req.TimeFrom, req.TimeTo = num(qBase), num(qBase+int64(time.Hour))
	req.BaselineFrom, req.BaselineTo = req.TimeFrom, req.TimeTo
	if res := buildQualityResult(req, runSpans{}, runSpans{}, now); !res.WindowFromRun || !res.BaselineFromRun {
		t.Errorf("result: from run %v, baseline %v", res.WindowFromRun, res.BaselineFromRun)
	}
}

// The window query binds only the run, and the run is never inlined.
func TestQualityRunRangeQuery(t *testing.T) {
	query := qualityRunRangeQuery(ruleEntries(validRequest(t, referenceRules(), "x", "").Rules), 1, 2)
	bound := map[string]bool{}
	for _, p := range []any{clickhouse.Named("qualityRun", "x")} {
		bound[p.(driver.NamedValue).Name] = true
	}
	for _, m := range namedPlaceholder.FindAllStringSubmatch(query, -1) {
		if !bound[m[1]] {
			t.Errorf("placeholder %q is used but not bound", m[1])
		}
	}
	for _, want := range []string{`"UnixTime" >= 1 AND "UnixTime" < 2`, `{qualityRun:String}`, `('recording-api','start_session')`} {
		if !strings.Contains(query, want) {
			t.Errorf("query is missing %q:\n%s", want, query)
		}
	}
}

// otelsql's rows spans carry db.system but make no call: counting them
// doubled db_calls. The evidence lists the statements, so a drop from 5
// calls to 4 shows which one is missing.
func TestQualityDBCallsStatements(t *testing.T) {
	var spans []qSpan
	for _, s := range referenceRun("candidate", "baseline", 420) {
		if s.DB {
			s.Statement = "select  allowed\n from permissions where user_id = ?"
			if s.Name == "insert_session" {
				s.Statement = "insert into sessions (id) values (?)"
			}
			spans = append(spans, s)
			rows := s
			rows.SpanID, rows.Name, rows.Statement = s.SpanID[:14]+"ff", "sql:rows_close", ""
			spans = append(spans, rows)
			continue
		}
		spans = append(spans, s)
	}
	res := buildQualityResult(validRequest(t, referenceRules(), "candidate", ""), runSpans{Spans: spans}, runSpans{}, time.Unix(0, qBase))
	c := checkByID(t, res, "db-calls")
	if c.Value == nil || *c.Value != 2 || c.Status != CheckPass {
		t.Fatalf("db-calls %s value %v: rows spans must not count", c.Status, c.Value)
	}
	details := strings.Join(c.Details, "|")
	for _, want := range []string{
		"insert into sessions (id) values (?) × 1",
		"select allowed from permissions where user_id = ? × 1", // whitespace folded
		"not counted, they report on a call rather than make one: sql:rows_close × 2",
	} {
		if !strings.Contains(details, want) {
			t.Errorf("details are missing %q: %s", want, details)
		}
	}
	// Without statements the span names stay (the reference fixture's form).
	plain := buildQualityResult(validRequest(t, referenceRules(), "candidate", ""), runSpans{Spans: referenceRun("candidate", "candidate", 910)}, runSpans{}, time.Unix(0, qBase))
	if d := strings.Join(checkByID(t, plain, "db-calls").Details, "|"); !strings.Contains(d, "permission_lookup × 20|insert_session × 1") {
		t.Errorf("plain details %s", d)
	}
}
