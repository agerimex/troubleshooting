package data

import (
	"strings"
	"sync"
	"testing"
	"time"

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

func TestAddFilter(t *testing.T) {
	tests := []struct {
		name      string
		filter    SpanFilter
		wantWhere []string
		wantLimit string
	}{
		{
			name:      "empty filter",
			filter:    SpanFilter{},
			wantWhere: nil,
		},
		{
			name:      "first page",
			filter:    SpanFilter{TimeFrom: "100", ParentId: "0000000000000000", RowsPerPage: 25},
			wantWhere: []string{`"UnixTime" > toInt64({timeFrom:Int64})`, `"ParentSpanId" = {parent:String}`},
			wantLimit: `limit {rowsPerPage:Int64}`,
		},
		{
			name:      "next page continues after the last row, including rows with the same time",
			filter:    SpanFilter{TimeFrom: "100", AfterSpanId: "abc"},
			wantWhere: []string{`"UnixTime" = toInt64({timeFrom:Int64}) and "SpanId" > {afterSpanId:String}`},
		},
		{
			name:      "service and method filters",
			filter:    SpanFilter{ServiceName: "bill", MethodName: "GET", Status: "error"},
			wantWhere: []string{`"ServiceName" iLike {serviceName:String}`, `"SpanName" iLike {spanName:String}`, `"StatusCode" = {statusCode:Int8}`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			where, limit := addFilter(tt.filter)
			if len(tt.wantWhere) == 0 && where != "" {
				t.Errorf("where = %q, want empty", where)
			}
			for _, want := range tt.wantWhere {
				if !strings.Contains(where, want) {
					t.Errorf("where = %q, missing %q", where, want)
				}
			}
			if limit != tt.wantLimit {
				t.Errorf("limit = %q, want %q", limit, tt.wantLimit)
			}
		})
	}
}

func TestFilterParamsBindEveryPlaceholder(t *testing.T) {
	filter := SpanFilter{TimeFrom: "1", ParentId: "p", Status: "ok", ServiceName: "s", MethodName: "m", RowsPerPage: 10, AfterSpanId: "a"}
	where, limit := addFilter(filter)

	bound := map[string]bool{}
	for _, param := range filterParams(filter) {
		bound[param.(driver.NamedValue).Name] = true
	}
	for _, name := range []string{"timeFrom", "parent", "statusCode", "serviceName", "spanName", "rowsPerPage", "afterSpanId"} {
		if strings.Contains(where+limit, "{"+name+":") && !bound[name] {
			t.Errorf("placeholder %q is used but not bound", name)
		}
	}
}

func TestArrayLiteral(t *testing.T) {
	got := arrayLiteral([]string{"a1b2", `it's`, `back\slash`})
	want := `['a1b2','it\'s','back\\slash']`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestEarliestChildStart(t *testing.T) {
	spans := []*Span{
		{Timestamp: "5000000000"},
		{Timestamp: "2000000000"}, // earliest start
		{Timestamp: "4000000000"},
	}

	from, ok := earliestChildStart(spans)

	want := 2000000000 - int64(time.Minute)
	if !ok || from != want {
		t.Errorf("got %d (ok=%v), want %d", from, ok, want)
	}

	if _, ok := earliestChildStart([]*Span{{Timestamp: "not a number"}}); ok {
		t.Errorf("an unparsable timestamp must disable the bound")
	}
	if _, ok := earliestChildStart(nil); ok {
		t.Errorf("no spans must not produce a bound")
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
