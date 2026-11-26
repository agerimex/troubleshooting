package data

import (
	"strings"
	"testing"
)

// The name list reads each part of the window from exactly one source: whole
// hours with a committed revision from the hourly aggregates, everything else
// from raw spans; unavailable hours from neither.
func TestSpanNamesQueryUsesOneSourcePerHour(t *testing.T) {
	h := hourNs
	start := int64(1_800_000_000) * 1e9 / h * h
	revisions := map[int64]Revision{start: {BucketStart: start, Revision: 7, State: StateFinal}}
	// [start, start+1h) is stored, [start+1h, start+1h30m) is raw.
	plan := planWindow(start, start+h+h/2, revisions, 0, 0)

	q := spanNamesQuery(plan)
	for _, want := range []string{
		`FROM stat_calls WHERE (toInt64(toUnixTimestamp("BucketStart")), "Revision") IN ((` + num(start/1e9) + `, 7))`,
		`FROM stat_shapes WHERE (toInt64(toUnixTimestamp("BucketStart")), "Revision") IN ((` + num(start/1e9) + `, 7))`,
		`FROM otel_traces WHERE (("UnixTime" >= ` + num(start+h) + ` AND "UnixTime" < ` + num(start+h+h/2) + `))`,
		`GROUP BY s, n`,
		`LIMIT 5001`,
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query is missing %q:\n%s", want, q)
		}
	}
	// The stored hour is not read from raw spans as well.
	if strings.Contains(q, `"UnixTime" >= `+num(start)+` `) {
		t.Errorf("the stored hour is also read from raw spans:\n%s", q)
	}

	rawOnly := spanNamesQuery(planWindow(start, start+h, nil, 0, 0))
	if strings.Contains(rawOnly, "stat_calls") || !strings.Contains(rawOnly, "otel_traces") {
		t.Errorf("an hour without a revision must come from raw spans only:\n%s", rawOnly)
	}

	// Raw spans expired and no statistics: nothing to read.
	if q := spanNamesQuery(planWindow(start, start+h, nil, start+2*h, start+2*h)); q != "" {
		t.Errorf("a window outside retention got a query:\n%s", q)
	}
}

func TestPickedNamesFilter(t *testing.T) {
	f := validFilter()
	f.ServiceNames = []string{"api", "worker"}
	f.SpanNames = [][2]string{{"api", "GET /a"}}
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	conds, params := f.conditions(false)
	where := whereClause(conds)
	// A whole picked service, or a picked (service, name) pair.
	for _, want := range []string{`("ServiceName" IN {serviceNames:Array(String)} OR ("SpanName" IN {spanNames:Array(String)} AND ("ServiceName", "SpanName") IN {spanPairs:Array(Tuple(String, String))}))`} {
		if !strings.Contains(where, want) {
			t.Errorf("where = %q, missing %q", where, want)
		}
	}
	if len(params) == 0 {
		t.Fatal("no parameters")
	}

	for name, edit := range map[string]func(*SpanFilter){
		"an empty name":   func(f *SpanFilter) { f.SpanNames = [][2]string{{"api", ""}} },
		"too many names":  func(f *SpanFilter) { f.ServiceNames = make([]string, MaxPickedNames+1) },
		"a too long name": func(f *SpanFilter) { f.SpanNames = [][2]string{{"api", strings.Repeat("x", 1025)}} },
	} {
		f := validFilter()
		edit(&f)
		if err := f.Validate(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
