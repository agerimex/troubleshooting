package data

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

func TestFillSQLArgs(t *testing.T) {
	args := map[int]string{}
	for i := 1; i <= 11; i++ {
		args[i] = string(rune('a' + i - 1))
	}
	args[1] = "$2" // a value that looks like a placeholder must not be re-substituted

	got := fillSQLArgs("select * from t where a=$1 and j=$10 and k=$11 and z=$12", args)

	want := "select * from t where a='$2' and j='j' and k='k' and z=$12"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

const (
	traceA = "0123456789abcdef0123456789abcdef"
	spanA  = "0123456789abcdef"
)

func validFilter() SpanFilter {
	return SpanFilter{TimeFrom: "1000", TimeTo: "2000"}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(f *SpanFilter)
		wantErr string
	}{
		{"window only", func(f *SpanFilter) {}, ""},
		{"missing time_from", func(f *SpanFilter) { f.TimeFrom = "" }, "time_from"},
		{"missing time_to", func(f *SpanFilter) { f.TimeTo = "" }, "time_to"},
		{"reversed window", func(f *SpanFilter) { f.TimeTo = "999" }, "after time_from"},
		{"empty window", func(f *SpanFilter) { f.TimeTo = f.TimeFrom }, "after time_from"},
		{"window too wide", func(f *SpanFilter) { f.TimeTo = fmt.Sprint(1000 + int64(limits.MaxWindow) + 1) }, "longer"},
		{"window exactly the maximum", func(f *SpanFilter) { f.TimeTo = fmt.Sprint(1000 + int64(limits.MaxWindow)) }, ""},
		{"parent without trace", func(f *SpanFilter) { f.ParentId = spanA }, "needs trace_id"},
		{"parent with trace", func(f *SpanFilter) { f.TraceId = traceA; f.ParentId = spanA }, ""},
		{"upper-case ids are accepted", func(f *SpanFilter) { f.TraceId = strings.ToUpper(traceA) }, ""},
		{"short trace id", func(f *SpanFilter) { f.TraceId = "abc" }, "trace_id"},
		{"non-hex parent", func(f *SpanFilter) { f.TraceId = traceA; f.ParentId = "zzzzzzzzzzzzzzzz" }, "parent_id"},
		{"bad scope", func(f *SpanFilter) { f.Scope = "children" }, "scope"},
		{"bad status", func(f *SpanFilter) { f.Status = "failed" }, "status"},
		{"partial cursor", func(f *SpanFilter) { f.AfterTime = "1500" }, "cursor"},
		{"cursor without time", func(f *SpanFilter) { f.AfterTraceId = traceA; f.AfterSpanId = spanA }, "after_time"},
		{"full cursor", func(f *SpanFilter) { f.AfterTime = "1500"; f.AfterTraceId = traceA; f.AfterSpanId = spanA }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := validFilter()
			tt.edit(&f)
			err := f.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				return
			}
			if err == nil || !errors.Is(err, ErrInvalidFilter) || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want an ErrInvalidFilter mentioning %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateDefaults(t *testing.T) {
	f := validFilter()
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	if f.Scope != ScopeRoot || f.RowsPerPage != DefaultRowsPerPage {
		t.Errorf("search defaults: scope %q rows %d", f.Scope, f.RowsPerPage)
	}

	f = validFilter()
	f.TraceId = strings.ToUpper(traceA)
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	if f.Scope != ScopeAll || f.TraceId != traceA {
		t.Errorf("a trace defaults to every depth: scope %q trace %q", f.Scope, f.TraceId)
	}
}

func TestConditions(t *testing.T) {
	tests := []struct {
		name       string
		filter     SpanFilter
		withCursor bool
		want       []string
		notWant    []string
	}{
		{
			name:   "root spans",
			filter: SpanFilter{Scope: ScopeRoot},
			want:   []string{`"ParentSpanId" = '0000000000000000'`},
		},
		{
			name:    "any depth has no parent condition",
			filter:  SpanFilter{Scope: ScopeAll, Status: "error"},
			want:    []string{`"StatusCode" = {statusCode:Int8}`},
			notWant: []string{"ParentSpanId"},
		},
		{
			name:   "children are matched by trace and parent",
			filter: SpanFilter{Scope: ScopeRoot, TraceId: traceA, ParentId: spanA},
			want:   []string{`"TraceId" = {traceId:String}`, `"ParentSpanId" = {parentId:String}`},
		},
		{
			name:       "the cursor compares the whole sort key",
			filter:     SpanFilter{Scope: ScopeAll, AfterTime: "5", AfterTraceId: traceA, AfterSpanId: spanA},
			withCursor: true,
			want:       []string{`("UnixTime", "TraceId", "SpanId") > (toInt64({afterTime:Int64}), {afterTraceId:String}, {afterSpanId:String})`},
		},
		{
			name:    "counts ignore the cursor",
			filter:  SpanFilter{Scope: ScopeAll, AfterTime: "5", AfterTraceId: traceA, AfterSpanId: spanA},
			notWant: []string{"afterTime"},
		},
		{
			name:   "service and method filters",
			filter: SpanFilter{Scope: ScopeAll, ServiceName: "bill", MethodName: "GET"},
			want:   []string{`"ServiceName" ILIKE {serviceName:String}`, `"SpanName" ILIKE {spanName:String}`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conds, _ := tt.filter.conditions(tt.withCursor)
			where := whereClause(conds)
			for _, want := range tt.want {
				if !strings.Contains(where, want) {
					t.Errorf("where = %q, missing %q", where, want)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(where, notWant) {
					t.Errorf("where = %q, must not contain %q", where, notWant)
				}
			}
		})
	}
}

// Every placeholder in a query must be bound, or ClickHouse rejects it.
func TestEveryPlaceholderIsBound(t *testing.T) {
	filter := SpanFilter{TimeFrom: "1", TimeTo: "2", TraceId: traceA, ParentId: spanA, Status: "ok",
		ServiceName: "s", MethodName: "m", AfterTime: "1", AfterTraceId: traceA, AfterSpanId: spanA}
	conds, params := filter.conditions(true)
	params = append(params, filter.windowParams()...)
	query := canonicalSpans(conds) + whereClause(conds)

	bound := map[string]bool{}
	for _, param := range params {
		bound[param.(driver.NamedValue).Name] = true
	}
	for _, m := range namedPlaceholder.FindAllStringSubmatch(query, -1) {
		if !bound[m[1]] {
			t.Errorf("placeholder %q is used but not bound", m[1])
		}
	}
}

// The canonical record must be chosen before filters apply, so the
// pre-selection may only narrow which spans are canonicalized.
func TestCanonicalSpansChoosesBeforeFiltering(t *testing.T) {
	query := canonicalSpans([]string{`"StatusCode" = 1`})
	for _, want := range []string{
		`("TraceId", "SpanId") IN (SELECT "TraceId", "SpanId" FROM otel_traces WHERE ` + windowCondition + ` AND "StatusCode" = 1)`,
		`LIMIT 1 BY "TraceId", "SpanId"`,
		`ORDER BY "TraceId", "SpanId", "StatusCode" = 1 DESC`,
	} {
		if !strings.Contains(query, want) {
			t.Errorf("query is missing %q:\n%s", want, query)
		}
	}
	if strings.Contains(canonicalSpans(nil), " IN (") {
		t.Errorf("no conditions must mean no pre-selection")
	}
}

// A page reads its candidates in table order (UnixTime alone, so ClickHouse can
// stop early) and chooses canonical records among the candidate spans' rows in
// the page's time range before the filters apply again.
func TestPageQueries(t *testing.T) {
	filter := SpanFilter{TimeFrom: "1", TimeTo: "2", Scope: ScopeRoot, Status: "error",
		AfterTime: "1", AfterTraceId: traceA, AfterSpanId: spanA}
	conds, params := filter.conditions(true)
	params = append(params, filter.windowParams()...)
	params = append(params, clickhouse.Named("scanRows", "52"), clickhouse.Named("pageFrom", "1"),
		clickhouse.Named("pageTo", "2"), clickhouse.Named("limit", "26"))

	candidates := pageCandidates(conds)
	for _, want := range []string{
		windowCondition + ` AND "ParentSpanId" = '` + RootParentSpanID + `' AND "StatusCode" = {statusCode:Int8}`,
		`("UnixTime", "TraceId", "SpanId") > (`,
		`ORDER BY "UnixTime" LIMIT {scanRows:UInt32}`,
	} {
		if !strings.Contains(candidates, want) {
			t.Errorf("candidates are missing %q:\n%s", want, candidates)
		}
	}

	page := pageCanonicalSpans(conds)
	for _, want := range []string{
		`"UnixTime" >= toInt64({pageFrom:Int64}) AND "UnixTime" < toInt64({pageTo:Int64})`,
		`("TraceId", "SpanId") IN (SELECT "TraceId", "SpanId" FROM (` + candidates + `))`,
		`ORDER BY "TraceId", "SpanId", "StatusCode" = 1 DESC`,
		`LIMIT 1 BY "TraceId", "SpanId"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page query is missing %q:\n%s", want, page)
		}
	}
	// The canonical choice is made before filters: the filters appear only in
	// the candidate subquery, not on the rows being ranked.
	if strings.Count(page, `"StatusCode" = {statusCode:Int8}`) != 1 {
		t.Errorf("filters must only select candidates:\n%s", page)
	}

	bound := map[string]bool{}
	for _, param := range params {
		bound[param.(driver.NamedValue).Name] = true
	}
	for _, m := range namedPlaceholder.FindAllStringSubmatch(page+whereClause(conds), -1) {
		if !bound[m[1]] {
			t.Errorf("placeholder %q is used but not bound", m[1])
		}
	}
}

// The count groups rows by start time, matching conds per row, and ranks only
// rows at shared times with the canonical rule before conds apply.
func TestCountQueries(t *testing.T) {
	filter := SpanFilter{TimeFrom: "1", TimeTo: "2", Scope: ScopeRoot, Status: "error"}
	conds, params := filter.conditions(false)
	params = append(params, filter.windowParams()...)
	params = append(params, clickhouse.Named("sharedTimes", "[1,2]"))

	groups := timeGroups("otel_traces", conds)
	if want := `sum("ParentSpanId" = '` + RootParentSpanID + `' AND "StatusCode" = {statusCode:Int8}) AS s`; !strings.Contains(groups, want) {
		t.Errorf("time groups are missing %q:\n%s", want, groups)
	}
	if !strings.Contains(timeGroups("otel_traces", nil), "n AS s") || strings.Contains(timeGroups("otel_traces", nil), " IN (") {
		t.Errorf("without conditions every row matches:\n%s", timeGroups("otel_traces", nil))
	}

	shared := sharedTimeSpans(conds, `{sharedTimes:Array(Int64)}`)
	for _, want := range []string{
		`argMax("ParentSpanId" = '` + RootParentSpanID + `' AND "StatusCode" = {statusCode:Int8},`,
		`tuple("StatusCode" = 1, length("Events.Name"), length("Links.SpanId"), ` + spanPayloadHash + `)`,
		`"UnixTime" IN {sharedTimes:Array(Int64)}`,
		`GROUP BY "UnixTime", "TraceId", "SpanId"`,
	} {
		if !strings.Contains(shared, want) {
			t.Errorf("shared-time count is missing %q:\n%s", want, shared)
		}
	}
	// The tuple ranks like winnerOrder, which lists the same keys.
	if !strings.Contains(winnerOrder, `"StatusCode" = 1 DESC, length("Events.Name") DESC, length("Links.SpanId") DESC`) {
		t.Errorf("winnerOrder changed; update the ranking tuple of sharedTimeSpans")
	}

	bound := map[string]bool{}
	for _, param := range params {
		bound[param.(driver.NamedValue).Name] = true
	}
	for _, m := range namedPlaceholder.FindAllStringSubmatch(groups+shared, -1) {
		if !bound[m[1]] {
			t.Errorf("placeholder %q is used but not bound", m[1])
		}
	}
}

// Child counts read the children's copies by start time and group them per
// span, so copies that agree on the parent are counted without ranking; the
// others are listed for the canonical rule.
func TestChildGroups(t *testing.T) {
	for _, want := range []string{
		`"UnixTime" IN (SELECT "UnixTime" FROM otel_traces WHERE ` + windowCondition + ` AND ` + isChildOf + `)`,
		`GROUP BY "UnixTime", "TraceId", "SpanId"`,
		`if(min("ParentSpanId") = max("ParentSpanId"), min("ParentSpanId"), '') AS parent`,
		`groupArrayIf("SpanId", parent = '')`,
	} {
		if !strings.Contains(childGroups, want) {
			t.Errorf("child groups are missing %q:\n%s", want, childGroups)
		}
	}
	bound := map[string]bool{"timeFrom": true, "timeTo": true, "parents": true}
	for _, m := range namedPlaceholder.FindAllStringSubmatch(childGroups, -1) {
		if !bound[m[1]] {
			t.Errorf("placeholder %q is used but not bound", m[1])
		}
	}
}

// Log pages and totals work like span ones: candidates in table order, the
// canonical row among a record's rows in the page's time range before filters,
// and totals that rank only rows at shared times.
func TestLogPageAndCountQueries(t *testing.T) {
	filter := LogFilter{TimeFrom: "1", TimeTo: "2", Severity: "ERROR", Message: "declined",
		AfterTime: "1", AfterRecordId: "r1"}
	conds, params := filter.conditions(true)
	params = append(params, filter.windowParams()...)
	params = append(params, clickhouse.Named("scanRows", "52"), clickhouse.Named("pageFrom", "1"),
		clickhouse.Named("pageTo", "2"), clickhouse.Named("limit", "26"), clickhouse.Named("sharedTimes", "[1]"))

	candidates := logPageCandidates(conds)
	page := logPageCanonical(conds)
	for _, want := range []string{
		`ORDER BY "UnixTime" LIMIT {scanRows:UInt32}`,
		`("UnixTime", "RecordId") > (`,
	} {
		if !strings.Contains(candidates, want) {
			t.Errorf("log candidates are missing %q:\n%s", want, candidates)
		}
	}
	for _, want := range []string{
		`"UnixTime" >= toInt64({pageFrom:Int64}) AND "UnixTime" < toInt64({pageTo:Int64})`,
		`"RecordId" IN (SELECT "RecordId" FROM (` + candidates + `))`,
		`ORDER BY "RecordId", cityHash64(` + logPayloadColumns + `) DESC`,
		`LIMIT 1 BY "RecordId"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("log page query is missing %q:\n%s", want, page)
		}
	}
	if strings.Count(page, `"Severity" = {severity:String}`) != 1 {
		t.Errorf("filters must only select candidates:\n%s", page)
	}

	countConds, _ := filter.conditions(false)
	shared := sharedTimeLogs(countConds, `{sharedTimes:Array(Int64)}`)
	for _, want := range []string{
		`argMax("Severity" = {severity:String} AND positionCaseInsensitiveUTF8("Body", {message:String}) > 0, cityHash64(` + logPayloadColumns + `))`,
		`GROUP BY "UnixTime", "RecordId"`,
	} {
		if !strings.Contains(shared, want) {
			t.Errorf("shared-time log count is missing %q:\n%s", want, shared)
		}
	}
	if !strings.Contains(timeGroups("log_records", countConds), `FROM log_records WHERE`) {
		t.Errorf("log totals must group log_records")
	}

	bound := map[string]bool{}
	for _, param := range params {
		bound[param.(driver.NamedValue).Name] = true
	}
	for _, m := range namedPlaceholder.FindAllStringSubmatch(page+whereClause(conds)+shared+timeGroups("log_records", countConds), -1) {
		if !bound[m[1]] {
			t.Errorf("placeholder %q is used but not bound", m[1])
		}
	}
}

// Statistics pass a record alone at its time straight through (eligible rows
// only) and group per identity only the rows at shared times, so memory
// doesn't grow with the records of the hour; both branches give the same
// columns.
func TestStatRecordsGroupOnlySharedTimes(t *testing.T) {
	parts := []interval{{0, hourNs}}
	for name, q := range map[string]string{
		"spans": statSpans(parts, 5, time.Minute, ""),
		"logs":  statLogs(parts, 5, time.Minute, ""),
	} {
		branches := strings.Split(q, "UNION ALL")
		if len(branches) != 2 {
			t.Fatalf("%s: want a pass-through and a grouped branch:\n%s", name, q)
		}
		single, grouped := branches[0], branches[1]
		if !strings.Contains(single, `"UnixTime" NOT IN (SELECT "UnixTime" FROM`) || !strings.Contains(single, eligibleCondition(5, time.Minute)) ||
			strings.Contains(single, `GROUP BY "TraceId"`) || strings.Contains(single, `GROUP BY "RecordId"`) {
			t.Errorf("%s: the pass-through branch must take eligible rows at unshared times without grouping:\n%s", name, single)
		}
		if !strings.Contains(grouped, `"UnixTime" IN (SELECT "UnixTime" FROM`) || !strings.Contains(grouped, `GROUP BY "UnixTime" HAVING count() > 1`) {
			t.Errorf("%s: the grouped branch must read only rows at shared times:\n%s", name, grouped)
		}
		for _, col := range []string{`AS "Eligible"`, `AS "Payloads"`, `AS "UnknownReceipt"`} {
			if !strings.Contains(single, col) || !strings.Contains(grouped, col) {
				t.Errorf("%s: both branches need %s", name, col)
			}
		}
	}
	if !strings.Contains(statSpans(parts, 5, time.Minute, ""), `"Events.Timestamp" AS "EventTimes"`) {
		t.Errorf("the pass-through branch must name event columns like the grouped one")
	}
	// A preselection is grouped in one pass: the window isn't read again for
	// shared times (that ran past the rows-to-read limit for problems).
	for name, q := range map[string]string{
		"spans": statSpans(parts, 5, time.Minute, `"ServiceName" = 'a'`),
		"logs":  statLogs(parts, 5, time.Minute, `"ServiceName" = 'a'`),
	} {
		if strings.Contains(q, "UNION ALL") || strings.Contains(q, "HAVING count() > 1") || !strings.Contains(q, `"ServiceName" = 'a'`) {
			t.Errorf("%s with a preselection must be one grouping of the preselected records:\n%s", name, q)
		}
	}
}

func TestArrayLiterals(t *testing.T) {
	got := arrayLiteral([]string{"a1b2", `it's`, `back\slash`})
	want := `['a1b2','it\'s','back\\slash']`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}

	got = pairsLiteral([][2]string{{"t1", "s1"}, {"t'2", "s2"}})
	want = `[('t1','s1'),('t\'2','s2')]`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestZipEventsAndLinks(t *testing.T) {
	ts := time.Unix(0, 1700000000123456789)
	events := zipEvents([]time.Time{ts}, []string{"exception"}, []map[string]string{{"exception.message": "denied"}})
	if len(events) != 1 || events[0].TimeUnixNano != "1700000000123456789" || events[0].Name != "exception" ||
		events[0].Attributes["exception.message"] != "denied" {
		t.Errorf("events = %+v", events)
	}

	links := zipLinks([]string{traceA}, []string{spanA}, []string{""}, []map[string]string{nil})
	if len(links) != 1 || links[0].TraceId != traceA || links[0].SpanId != spanA || links[0].Attributes == nil {
		t.Errorf("links = %+v", links)
	}

	if got := zipEvents(nil, nil, nil); got == nil || len(got) != 0 {
		t.Errorf("no events must encode as [], got %#v", got)
	}
}

func TestExplainQueryError(t *testing.T) {
	for _, err := range []error{
		context.DeadlineExceeded,
		&clickhouse.Exception{Code: chTimeoutExceeded},
		&clickhouse.Exception{Code: chTooManyRows},
		fmt.Errorf("wrapped: %w", &clickhouse.Exception{Code: chMemoryLimitExceeded}),
	} {
		if got := ExplainQueryError(context.Background(), err); !errors.Is(got, ErrQueryLimit) {
			t.Errorf("ExplainQueryError(%v) = %v, want ErrQueryLimit", err, got)
		}
	}
	other := &clickhouse.Exception{Code: 60}
	if got := ExplainQueryError(context.Background(), other); got != other {
		t.Errorf("other errors must pass through, got %v", got)
	}

	for _, err := range []error{
		&net.OpError{Op: "dial", Err: errors.New("connection refused")},
		fmt.Errorf("wrapped: %w", &net.DNSError{Name: "clickhouse-server", Err: "server misbehaving"}),
	} {
		if got := ExplainQueryError(context.Background(), err); !errors.Is(got, ErrUnavailable) {
			t.Errorf("ExplainQueryError(%v) = %v, want ErrUnavailable", err, got)
		}
	}

	// After the request deadline the driver reports only a network timeout.
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if got := ExplainQueryError(expired, errors.New("read tcp: i/o timeout")); !errors.Is(got, ErrQueryLimit) {
		t.Errorf("an error after the deadline = %v, want ErrQueryLimit", got)
	}
}

func TestQueryLimitSettings(t *testing.T) {
	s := DefaultQueryLimits.settings()
	if s["max_execution_time"] != 10 || s["timeout_overflow_mode"] != "throw" || s["read_overflow_mode"] != "throw" {
		t.Errorf("settings = %v", s)
	}
}

func TestStatusCodeToString(t *testing.T) {
	for status, code := range StatusCodeMap {
		if got := StatusCodeToString(code); got != status {
			t.Errorf("StatusCodeToString(%q) = %q, want %q", code, got, status)
		}
	}
}

// Concurrent first calls used to race on a lazily filled map; run with -race.
func TestStatusCodeToStringConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			StatusCodeToString("1")
		}()
	}
	wg.Wait()
}

func TestLimitRows(t *testing.T) {
	for _, tc := range []struct{ in, want int }{
		{0, DefaultRowsPerPage},
		{-1, DefaultRowsPerPage},
		{10, 10},
		{MaxRowsPerPage, MaxRowsPerPage},
		{100000, MaxRowsPerPage},
	} {
		f := SpanFilter{RowsPerPage: tc.in}
		f.LimitRows()
		if f.RowsPerPage != tc.want {
			t.Errorf("LimitRows(%d) = %d, want %d", tc.in, f.RowsPerPage, tc.want)
		}
	}
}

// Windows are [time_from, time_to): a record at exactly time_to belongs to the
// next window (features/milestone_1/README.md).
func TestWindowIsHalfOpen(t *testing.T) {
	want := `"UnixTime" >= toInt64({timeFrom:Int64}) AND "UnixTime" < toInt64({timeTo:Int64})`
	if windowCondition != want {
		t.Fatalf("windowCondition = %s, want %s", windowCondition, want)
	}
	for _, q := range []string{canonicalSpans(nil), canonicalLogs(nil)} {
		if !strings.Contains(q, want) || strings.Contains(q, "BETWEEN") {
			t.Errorf("query does not use the half-open window:\n%s", q)
		}
	}
}
