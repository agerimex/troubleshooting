package data

import (
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
)

func profileAttrs(p *queryProfile) map[string]int64 {
	m := map[string]int64{}
	for _, kv := range p.attributes() {
		m[string(kv.Key)] = kv.Value.AsInt64()
	}
	return m
}

func TestQueryProfileKeepsPeakMemoryAndReadRows(t *testing.T) {
	p := &queryProfile{}
	p.events([]clickhouse.ProfileEvent{
		{ThreadID: 0, Type: "increment", Name: "UserTimeMicroseconds", Value: 500},
		{ThreadID: 0, Type: "gauge", Name: "MemoryTrackerUsage", Value: 1 << 20},
	})
	p.events([]clickhouse.ProfileEvent{
		{ThreadID: 0, Type: "gauge", Name: "MemoryTrackerPeakUsage", Value: 3 << 20},
		{ThreadID: 0, Type: "gauge", Name: "MemoryTrackerUsage", Value: 2 << 20},
	})
	p.progress(&clickhouse.Progress{Rows: 10, Bytes: 1000})
	p.progress(&clickhouse.Progress{Rows: 5, Bytes: 500})

	got := profileAttrs(p)
	want := map[string]int64{AttrDBServerMemory: 3 << 20, AttrDBServerReadRows: 15, AttrDBServerReadSize: 1500}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %d, want %d", k, got[k], v)
		}
	}
	if _, ok := got[AttrDBServerCPU]; ok {
		t.Error("CPU time must not come from profile events (incomplete); query_log has it")
	}
}

func TestQueryProfileWithoutDataRecordsNothing(t *testing.T) {
	if attrs := (&queryProfile{}).attributes(); attrs != nil {
		t.Fatalf("attrs = %v, want none: an unreported query has no measured zero", attrs)
	}
}

func TestSetTagReplacesOrAppends(t *testing.T) {
	s := &Span{Tags: []map[string]string{{"key": AttrDBServerMemory, "value": "1"}}}
	setTag(s, AttrDBServerMemory, "2")
	setTag(s, AttrDBServerCPU, "3")
	if tagValue(s.Tags, AttrDBServerMemory) != "2" || tagValue(s.Tags, AttrDBServerCPU) != "3" || len(s.Tags) != 2 {
		t.Fatalf("tags = %v", s.Tags)
	}
}
