package data

import (
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Executions are traces whose root starts in the window; their spans are read
// to shapeLookahead after it, and a shape is the entry plus the sorted set of
// distinct caller -> callee calls.
func TestTraceShapes(t *testing.T) {
	q := traceShapes([]interval{{0, hourNs}}, 5, time.Minute)
	for _, want := range []string{
		`(("UnixTime" >= 0 AND "UnixTime" < ` + num(hourNs+int64(shapeLookahead)) + `))`,
		`WHERE ((tupleElement(r, 1) >= 0 AND tupleElement(r, 1) < ` + num(hourNs) + `))`,
		`arraySort(arrayDistinct(arrayFilter(e -> e != 0,`,
		`cityHash64(arrayFirst(x -> x.1 = s.2, spans).3, s.3, s.4)`,
		`cityHash64("EntryService", "EntryOperation", "Edges") AS "ShapeId"`,
		`WHERE "Eligible"`,
		`HAVING roots > 0`,
	} {
		if !strings.Contains(q, want) {
			t.Errorf("trace shapes are missing %q:\n%s", want, q)
		}
	}
	b := bucketShapes([]interval{{0, hourNs}}, 5, time.Minute)
	for _, want := range []string{
		`quantilesTDigestIfState(0.5, 0.95, 0.99)("RootDuration" / 1000, "RootDuration" >= 0) AS "Durations"`,
		`groupArraySample(64, 1)(tuple("RootDuration", "TraceId", "RootTime")) AS "Samples"`,
		`GROUP BY "BucketStart", "ShapeId"`,
	} {
		if !strings.Contains(b, want) {
			t.Errorf("bucket shapes are missing %q:\n%s", want, b)
		}
	}
}

// A stored revision from before shapes-v1 has no shapes: its hour is computed
// from raw spans while they are retained, and is unavailable after that.
func TestShapePlan(t *testing.T) {
	h := hourNs
	revisions := map[int64]Revision{
		0:     {Revision: 1, State: StateFinal, Definitions: "metrics-v1;errors-v1;canonical-v1"},
		h:     {Revision: 2, State: StateFinal, Definitions: "metrics-v1;errors-v1;canonical-v1"},
		2 * h: {Revision: 3, State: StateProvisional, Definitions: Definitions},
	}
	plan := shapePlan(0, 3*h, revisions, h, 0)
	got := []string{}
	for _, p := range plan {
		got = append(got, p.Source)
	}
	if strings.Join(got, ",") != "unavailable,raw,stored" {
		t.Fatalf("plan sources %v", got)
	}
	if !strings.Contains(plan[0].Reason, "before the sequence library") {
		t.Errorf("reason %q", plan[0].Reason)
	}
	q := shapeLibraryQuery(plan, 5, time.Minute, nil)
	if !strings.Contains(q, `FROM stat_shapes WHERE (toInt64(toUnixTimestamp("BucketStart")), "Revision") IN ((7200, 3))`) {
		t.Errorf("stored part not read from stat_shapes:\n%s", q)
	}
	if !strings.Contains(q, ` UNION ALL `) || !strings.Contains(q, `quantilesTDigestIfMerge(0.5, 0.95, 0.99)("Durations")`) {
		t.Errorf("stored and raw shapes must merge:\n%s", q)
	}
}

func TestShapeFiltersAreBound(t *testing.T) {
	conds, params := shapeFilters(ShapeFilter{Service: "pay", Operation: "POST", ErrorsOnly: true})
	if len(conds) != 3 || conds[2] != "e > 0" {
		t.Fatalf("conds %v", conds)
	}
	q := shapeLibraryQuery([]PlanPart{{Source: SourceRaw, from: 0, to: hourNs}}, 5, time.Minute, conds)
	bound := map[string]bool{}
	for _, p := range params {
		bound[p.(driver.NamedValue).Name] = true
	}
	for _, m := range namedPlaceholder.FindAllStringSubmatch(q, -1) {
		if !bound[m[1]] {
			t.Errorf("placeholder %q is used but not bound", m[1])
		}
	}
}

func TestShapeFilterValidate(t *testing.T) {
	for _, f := range []ShapeFilter{{TimeFrom: "2", TimeTo: "1"}, {TimeFrom: "x", TimeTo: "1"}, {TimeFrom: "0", TimeTo: num(int64(MaxStatsWindow) + 1)}} {
		if f.Validate() == nil {
			t.Errorf("%+v must be invalid", f)
		}
	}
	if err := (ShapeFilter{TimeFrom: "0", TimeTo: num(hourNs)}).Validate(); err != nil {
		t.Errorf("valid window rejected: %v", err)
	}
}

func TestExampleOf(t *testing.T) {
	ex := exampleOf([]any{int64(121000), "abc", int64(7)})
	if ex == nil || ex.TraceId != "abc" || ex.DurationUs != 121000 || ex.Time != "7" {
		t.Errorf("example %+v", ex)
	}
	if exampleOf([]any{int64(0), "", int64(0)}) != nil || exampleOf(nil) != nil {
		t.Errorf("an empty sample is no example")
	}
}
