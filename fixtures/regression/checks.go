package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

// Expected answers are written out here from the scenario descriptions
// (README.md); none of them is computed with the API's SQL or filter code.

type checker struct {
	api   *api
	d     dataset
	state *state
	// statePath is where state changes are saved.
	statePath string
	// notes collects facts worth reporting, such as which RF03 tie version won.
	notes map[string]string
}

func (c *checker) window() filter {
	from, to := c.d.window()
	return windowFilter(from, to)
}

// wide covers every fixture span, including the RF06 parent.
func (c *checker) wide() filter {
	return windowFilter(c.d.at(-90*time.Minute), c.d.at(5*time.Minute))
}

func names(spans []apiSpan) []string {
	out := make([]string, len(spans))
	for i, s := range spans {
		out[i] = s.Name
	}
	sort.Strings(out)
	return out
}

func expectNames(what string, spans []apiSpan, want ...string) error {
	sort.Strings(want)
	if got := names(spans); len(got) != len(want) || (len(want) > 0 && !reflect.DeepEqual(got, want)) {
		return fmt.Errorf("%s: got %v, want %v", what, got, want)
	}
	return nil
}

func byName(spans []apiSpan, name string) (apiSpan, bool) {
	for _, s := range spans {
		if s.Name == name {
			return s, true
		}
	}
	return apiSpan{}, false
}

func (c *checker) children(ctx context.Context, f filter, trace, parent string) ([]apiSpan, error) {
	f.TraceID, f.ParentID, f.Scope = trace, parent, "all"
	f.RowsPerPage = childCap
	return c.api.all(ctx, f)
}

func (c *checker) trace(ctx context.Context, f filter, trace string) ([]apiSpan, error) {
	f.TraceID, f.Scope, f.RowsPerPage = trace, "all", childCap
	return c.api.all(ctx, f)
}

func (c *checker) service(f filter, suffix, scope string) filter {
	f.ServiceName, f.Scope, f.RowsPerPage = c.d.service(suffix), scope, childCap
	return f
}

// RF01: parents with the same span id in two traces keep their own children.
func (c *checker) rf01(ctx context.Context) error {
	want := map[string][]string{
		c.d.traceID(scRF01, 1): {"rf01.a.child1", "rf01.a.child2"},
		c.d.traceID(scRF01, 2): {"rf01.b.child1"},
	}
	for trace, kids := range want {
		got, err := c.children(ctx, c.window(), trace, idRoot)
		if err != nil {
			return err
		}
		if err := expectNames("children of "+trace, got, kids...); err != nil {
			return err
		}
		f := c.window()
		f.TraceID, f.Scope = trace, "root"
		roots, err := c.api.all(ctx, f)
		if err != nil {
			return err
		}
		if len(roots) != 1 || roots[0].ChildSpanCount != len(kids) {
			return fmt.Errorf("root of %s: %d rows, childSpanCount %v, want 1 row with %d", trace, len(roots), childCounts(roots), len(kids))
		}
		f.ParentID, f.Scope = idRoot, "all"
		if n, err := c.api.count(ctx, f); err != nil || n != len(kids) {
			return fmt.Errorf("count of children of %s = %d (%v), want %d", trace, n, err, len(kids))
		}
	}
	return nil
}

func childCounts(spans []apiSpan) []int {
	out := make([]int, len(spans))
	for i, s := range spans {
		out[i] = s.ChildSpanCount
	}
	return out
}

// expectedTotal is the number of logical spans of this run the API should
// list: the distinct (trace id, span id) pairs generated, counted here from
// the generator's own data.
func (c *checker) expectedTotal() int {
	identities := map[string]bool{}
	add := func(spans []spanIdentity) {
		for _, s := range spans {
			identities[s.trace+":"+s.span] = true
		}
	}
	for _, req := range c.d.deliveries() {
		add(identitiesOf(req))
	}
	if c.state.LateSent {
		add(identitiesOf(c.d.late()))
	}
	if c.state.M1LateSent {
		add(identitiesOf(c.d.m1Late()))
	}
	total := len(identities)
	if c.state.ParitySent {
		total += 2 * len(paritySpanNames)
	}
	if c.state.InvalidSent {
		total++ // only the valid span of invalidIDs
	}
	return total
}

// rf02 checks the logical total of the whole run; run after replays it shows
// that retries did not add spans.
func (c *checker) rf02(ctx context.Context) error {
	f := c.wide()
	f.ServiceName, f.Scope = "fx"+c.d.run, "all"
	n, err := c.api.count(ctx, f)
	if err != nil {
		return err
	}
	if want := c.expectedTotal(); n != want {
		return fmt.Errorf("logical spans of the run: count %d, want %d", n, want)
	}
	f.RowsPerPage = childCap
	rows, err := c.api.all(ctx, f)
	if err != nil {
		return err
	}
	if len(rows) != n {
		return fmt.Errorf("listing the run returns %d rows, count says %d", len(rows), n)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.key()] {
			return fmt.Errorf("%s is listed twice across pages", r.key())
		}
		seen[r.key()] = true
	}
	return nil
}

// RF03: conflicting payloads resolve to one whole record, the same way in
// both arrival orders and in every endpoint.
func (c *checker) rf03(ctx context.Context) error {
	// Error beats ok (rule 1), whichever arrived first.
	list, err := c.api.all(ctx, c.service(c.window(), "rf03-status", "root"))
	if err != nil {
		return err
	}
	if len(list) != 2 {
		return fmt.Errorf("status conflict: %d rows, want 2 (one per trace)", len(list))
	}
	for _, s := range list {
		if s.Status != "error" || s.StatusMessage != "conflict version error" || tagValue(s.Tags, "variant") != "error" ||
			len(s.Events) != 1 || s.Events[0].Name != "retry" {
			return fmt.Errorf("status conflict in %s: got status %q variant %q with %d events, want the whole error version",
				s.TraceID, s.Status, tagValue(s.Tags, "variant"), len(s.Events))
		}
		detail, err := c.trace(ctx, c.window(), s.TraceID)
		if err != nil {
			return err
		}
		if len(detail) != 1 || tagValue(detail[0].Tags, "variant") != "error" {
			return fmt.Errorf("trace %s shows %d spans / variant %q, want the error version once", s.TraceID, len(detail), variantOf(detail))
		}
	}
	for status, want := range map[string]int{"ok": 0, "error": 2, "": 2} {
		f := c.service(c.window(), "rf03-status", "root")
		f.Status = status
		if n, err := c.api.count(ctx, f); err != nil || n != want {
			return fmt.Errorf("status conflict: count with status %q = %d (%v), want %d", status, n, err, want)
		}
		rows, err := c.api.all(ctx, f)
		if err != nil || len(rows) != want {
			return fmt.Errorf("status conflict: list with status %q has %d rows (%v), want %d", status, len(rows), err, want)
		}
	}

	// Equal rank: the hash decides, identically for both orders.
	tie, err := c.api.all(ctx, c.service(c.window(), "rf03-tie", "root"))
	if err != nil {
		return err
	}
	if len(tie) != 2 {
		return fmt.Errorf("tie: %d rows, want 2", len(tie))
	}
	v3, v4 := variantOf(tie[:1]), variantOf(tie[1:])
	if v3 != v4 {
		return fmt.Errorf("tie: arrival order changed the winner: %s vs %s", v3, v4)
	}
	wantDuration := map[string]int64{"c": 100000, "d": 200000}[v3]
	for _, s := range tie {
		if s.Duration != wantDuration {
			return fmt.Errorf("tie: variant %s with duration %dµs, want %dµs (fields mixed from two versions)", v3, s.Duration, wantDuration)
		}
		detail, err := c.trace(ctx, c.window(), s.TraceID)
		if err != nil {
			return err
		}
		if variantOf(detail) != v3 || len(detail) != 1 {
			return fmt.Errorf("tie: trace %s shows variant %q, list shows %q", s.TraceID, variantOf(detail), v3)
		}
	}
	c.notes["RF03 tie winner"] = "variant " + v3
	return nil
}

func variantOf(spans []apiSpan) string {
	if len(spans) == 0 {
		return ""
	}
	return tagValue(spans[0].Tags, "variant")
}

// RF04: an error search finds the failed grandchild under a successful root.
func (c *checker) rf04(ctx context.Context) error {
	l := c.d.traceID(scRF04, 1)
	f := c.service(c.window(), "rf04", "all")
	f.Status = "error"
	found, err := c.api.all(ctx, f)
	if err != nil {
		return err
	}
	if len(found) != 1 || found[0].Name != "UploadToStorage" || found[0].TraceID != l {
		return fmt.Errorf("error search at any depth: got %v, want UploadToStorage in %s", names(found), l)
	}
	if n, err := c.api.count(ctx, f); err != nil || n != 1 {
		return fmt.Errorf("error count at any depth = %d (%v), want 1", n, err)
	}
	f.Scope = "root"
	if roots, err := c.api.all(ctx, f); err != nil || len(roots) != 0 {
		return fmt.Errorf("error search over root spans: %v (%v), want none (the root succeeded)", names(roots), err)
	}

	spans, err := c.trace(ctx, c.window(), l)
	if err != nil {
		return err
	}
	if err := expectNames("trace "+l, spans, "POST /recordings", "upload pipeline", "UploadToStorage"); err != nil {
		return err
	}
	root, _ := byName(spans, "POST /recordings")
	pipeline, _ := byName(spans, "upload pipeline")
	upload, _ := byName(spans, "UploadToStorage")
	if root.ParentSpanID != "0000000000000000" || pipeline.ParentSpanID != root.SpanID || upload.ParentSpanID != pipeline.SpanID {
		return fmt.Errorf("trace %s: parent chain is broken", l)
	}
	if root.Status != "ok" || upload.Status != "error" || upload.StatusMessage != "upload failed" {
		return fmt.Errorf("trace %s: statuses root %q upload %q (%q)", l, root.Status, upload.Status, upload.StatusMessage)
	}
	return expectException(upload, c.d.at(-10*time.Minute).Add(219*time.Millisecond), nil)
}

func expectException(s apiSpan, at time.Time, extra map[string]string) error {
	want := map[string]string{
		"exception.type":       exceptionType,
		"exception.message":    exceptionMessage,
		"exception.stacktrace": exceptionStack,
	}
	for k, v := range extra {
		want[k] = v
	}
	if len(s.Events) != 1 {
		return fmt.Errorf("%s: %d events, want 1", s.Name, len(s.Events))
	}
	e := s.Events[0]
	if e.Name != "exception" || e.TimeUnixNano != ns(at) || !reflect.DeepEqual(e.Attributes, want) {
		return fmt.Errorf("%s: event %q at %s with %v, want exception at %s with %v", s.Name, e.Name, e.TimeUnixNano, e.Attributes, ns(at), want)
	}
	return nil
}

// rf05 is the state of RF05 after the late parent was delivered (or before,
// with delivered false).
func (c *checker) rf05(ctx context.Context, delivered bool) error {
	m := c.d.traceID(scRF05, 1)
	found, err := c.api.all(ctx, c.service(c.window(), "rf05", "all"))
	if err != nil {
		return err
	}
	spans, err := c.trace(ctx, c.window(), m)
	if err != nil {
		return err
	}
	if !delivered {
		if err := expectNames("searchable child without parent", found, "rf05.child"); err != nil {
			return err
		}
		if err := expectNames("trace before the parent arrived", spans, "rf05.child"); err != nil {
			return err
		}
		if spans[0].ParentSpanID != idLateRoot {
			return fmt.Errorf("child's parent id = %s, want %s", spans[0].ParentSpanID, idLateRoot)
		}
		return nil
	}
	if err := expectNames("search after the parent arrived", found, "rf05.child", "rf05.parent"); err != nil {
		return err
	}
	if err := expectNames("trace after the parent arrived", spans, "rf05.child", "rf05.parent"); err != nil {
		return err
	}
	parent, _ := byName(spans, "rf05.parent")
	if kid, _ := byName(spans, "rf05.child"); kid.ParentSpanID != parent.SpanID || parent.ChildSpanCount != 1 {
		return fmt.Errorf("the late parent is not attached: child's parent %s, parent %s with %d children", kid.ParentSpanID, parent.SpanID, parent.ChildSpanCount)
	}
	return nil
}

// RF06: a parent outside the window appears once the window is widened.
func (c *checker) rf06(ctx context.Context) error {
	n := c.d.traceID(scRF06, 1)
	spans, err := c.trace(ctx, c.window(), n)
	if err != nil {
		return err
	}
	if err := expectNames("trace in the default window", spans, "rf06.child"); err != nil {
		return err
	}
	// The UI widens by one hour on each side.
	from, to := c.d.window()
	spans, err = c.trace(ctx, windowFilter(from.Add(-time.Hour), to.Add(time.Hour)), n)
	if err != nil {
		return err
	}
	return expectNames("trace in the widened window", spans, "rf06.child", "rf06.parent")
}

// RF07: recorded exception evidence and span links come back exactly.
func (c *checker) rf07(ctx context.Context) error {
	spans, err := c.trace(ctx, c.window(), c.d.traceID(scRF07, 1))
	if err != nil {
		return err
	}
	if len(spans) != 1 {
		return fmt.Errorf("%d spans, want 1", len(spans))
	}
	s := spans[0]
	if s.Status != "error" || s.StatusMessage != "retry failed" {
		return fmt.Errorf("status %q %q", s.Status, s.StatusMessage)
	}
	if err := expectException(s, c.d.at(-10*time.Minute).Add(time.Second+40*time.Millisecond), map[string]string{"retry.count": "3"}); err != nil {
		return err
	}
	want := apiLink{TraceID: c.d.traceID(scRF04, 1), SpanID: idRoot, Attributes: map[string]string{"link.reason": "retry of upload"}}
	if len(s.Links) != 1 || !reflect.DeepEqual(s.Links[0], want) {
		return fmt.Errorf("links %+v, want [%+v]", s.Links, want)
	}
	return nil
}

// RF08: paging over equal times and span ids returns each span exactly once.
func (c *checker) rf08(ctx context.Context) error {
	want := map[string]bool{}
	for i := 1; i <= 8; i++ {
		id := idShared
		if i == 6 {
			id = "00000000000000e1"
		} else if i == 7 {
			id = "00000000000000ef"
		}
		want[c.d.traceID(scRF08, i)+":"+id] = true
	}
	for _, pageSize := range []int{1, 2, 3, 7} {
		f := c.service(c.window(), "rf08", "root")
		f.RowsPerPage = pageSize
		rows, err := c.api.all(ctx, f)
		if err != nil {
			return err
		}
		got := map[string]bool{}
		for i, r := range rows {
			if got[r.key()] {
				return fmt.Errorf("page size %d: %s returned twice", pageSize, r.key())
			}
			got[r.key()] = true
			if i > 0 && !before(rows[i-1], r) {
				return fmt.Errorf("page size %d: rows %d and %d are out of order", pageSize, i-1, i)
			}
		}
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("page size %d: got %d spans %v, want %v", pageSize, len(got), sortedKeys(got), sortedKeys(want))
		}
	}
	if n, err := c.api.count(ctx, c.service(c.window(), "rf08", "root")); err != nil || n != 8 {
		return fmt.Errorf("count = %d (%v), want 8", n, err)
	}
	return nil
}

// before is the documented order (start time, trace id, span id).
func before(a, b apiSpan) bool {
	if len(a.TimeStamp) != len(b.TimeStamp) {
		return len(a.TimeStamp) < len(b.TimeStamp)
	}
	if a.TimeStamp != b.TimeStamp {
		return a.TimeStamp < b.TimeStamp
	}
	if a.TraceID != b.TraceID {
		return a.TraceID < b.TraceID
	}
	return a.SpanID < b.SpanID
}

// RF09: each expansion of a four-level trace shows exactly the next level.
func (c *checker) rf09(ctx context.Context) error {
	p := c.d.traceID(scRF09, 1)
	f := c.window()
	f.TraceID, f.Scope = p, "root"
	level, err := c.api.all(ctx, f)
	if err != nil {
		return err
	}
	if err := expectNames("level 1", level, "rf09.level1"); err != nil {
		return err
	}
	for i, want := range []string{"rf09.level2", "rf09.level3", "rf09.level4"} {
		if level[0].ChildSpanCount != 1 {
			return fmt.Errorf("level %d: childSpanCount %d, want 1", i+1, level[0].ChildSpanCount)
		}
		if level, err = c.children(ctx, c.window(), p, level[0].SpanID); err != nil {
			return err
		}
		if err := expectNames(fmt.Sprintf("level %d", i+2), level, want); err != nil {
			return err
		}
	}
	if level[0].ChildSpanCount != 0 {
		return fmt.Errorf("level 4: childSpanCount %d, want 0", level[0].ChildSpanCount)
	}
	return nil
}

// RF10: exactly childCap children are complete; childCap+1 are truncated.
func (c *checker) rf10(ctx context.Context) error {
	for i, n := range []int{childCap, childCap + 1} {
		trace := c.d.traceID(scRF10, i+1)
		f := c.window()
		f.TraceID, f.ParentID, f.Scope, f.RowsPerPage = trace, idRoot, "all", childCap
		page, more, err := c.api.spans(ctx, f)
		if err != nil {
			return err
		}
		if len(page) != childCap || more != (n > childCap) {
			return fmt.Errorf("%d children: page of %d with HasMore=%v, want %d with HasMore=%v", n, len(page), more, childCap, n > childCap)
		}
		if count, err := c.api.count(ctx, f); err != nil || count != n {
			return fmt.Errorf("%d children: count %d (%v)", n, count, err)
		}
		f.ParentID, f.Scope = "", "root"
		roots, _, err := c.api.spans(ctx, f)
		if err != nil || len(roots) != 1 || roots[0].ChildSpanCount != n {
			return fmt.Errorf("%d children: parent's childSpanCount %v (%v)", n, childCounts(roots), err)
		}
	}
	return nil
}

// RF12: children before the parent (clock skew) and after it ended (async)
// are found inside the declared window, and windows are [from, to): a span at
// exactly "from" is in, one at exactly "to" is out.
func (c *checker) rf12(ctx context.Context) error {
	q := c.d.traceID(scRF12, 1)
	t0 := c.d.at(-10 * time.Minute)
	skewed, async := t0.Add(-2*time.Minute), t0.Add(time.Second+5*time.Minute)
	cases := []struct {
		from, to time.Time
		want     []string
	}{
		{skewed, async.Add(time.Nanosecond), []string{"rf12.async-child", "rf12.skewed-child"}},
		{skewed.Add(time.Nanosecond), async.Add(time.Nanosecond), []string{"rf12.async-child"}},
		{skewed, async, []string{"rf12.skewed-child"}}, // "to" itself is excluded
		{t0, t0.Add(time.Second), nil},
	}
	for _, tc := range cases {
		kids, err := c.children(ctx, windowFilter(tc.from, tc.to), q, idRoot)
		if err != nil {
			return err
		}
		if err := expectNames(fmt.Sprintf("children in [%s, %s)", ns(tc.from), ns(tc.to)), kids, tc.want...); err != nil {
			return err
		}
		if !tc.from.After(t0) && tc.to.After(t0) {
			f := windowFilter(tc.from, tc.to)
			f.TraceID, f.Scope = q, "root"
			roots, err := c.api.all(ctx, f)
			if err != nil || len(roots) != 1 || roots[0].ChildSpanCount != len(tc.want) {
				return fmt.Errorf("parent in [%s, %s): childSpanCount %v (%v), want %d", ns(tc.from), ns(tc.to), childCounts(roots), err, len(tc.want))
			}
		}
	}
	return nil
}

// RF16: two versions of one operation keep their version, population and
// exact durations.
func (c *checker) rf16(ctx context.Context) error {
	rows, err := c.api.all(ctx, c.service(c.window(), "rf16", "root"))
	if err != nil {
		return err
	}
	durations := map[string][]int64{}
	for _, r := range rows {
		if r.Name != "TranscodeRecording" || tagValue(r.Tags, "recording.format") != "mp4" || r.Status != "ok" {
			return fmt.Errorf("unexpected span %s %q", r.key(), r.Name)
		}
		v := tagValue(r.ServiceTags, "service.version")
		durations[v] = append(durations[v], r.Duration)
	}
	var wantV1, wantV2 []int64
	for _, d := range rf16Durations() {
		wantV1 = append(wantV1, d.Microseconds())
		wantV2 = append(wantV2, 2*d.Microseconds())
	}
	for version, want := range map[string][]int64{"1.0.0": wantV1, "2.0.0": wantV2} {
		got := durations[version]
		sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("version %s: durations %v, want %v", version, got, want)
		}
		c.notes["RF16 "+version] = summary(got)
	}
	if len(durations) != 2 {
		return fmt.Errorf("versions %v, want 1.0.0 and 2.0.0", sortedKeys(durations))
	}
	return nil
}

func summary(micros []int64) string {
	var sum int64
	for _, m := range micros {
		sum += m
	}
	return fmt.Sprintf("%d calls, %d–%d µs, mean %d µs", len(micros), micros[0], micros[len(micros)-1], sum/int64(len(micros)))
}

// limited shows that a query over its row limit fails explicitly.
func (c *checker) limited(ctx context.Context, limitedAPI *api) error {
	f := c.wide()
	f.ServiceName, f.Scope = "fx"+c.d.run, "all"
	rows, _, err := limitedAPI.spans(ctx, f)
	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("got %d rows and error %v, want an explicit limit error", len(rows), err)
	}
	if apiErr.Status != 504 || !strings.Contains(apiErr.Message, "limit") {
		return fmt.Errorf("got %v, want HTTP 504 naming the limit", apiErr)
	}
	c.notes["limit error"] = apiErr.Message
	return nil
}

// scenarioChecks run on every pass (initial, after replay, after recreation).
func (c *checker) scenarioChecks() []namedCheck {
	return []namedCheck{
		{"RF01", "Matching parent span ids in two traces", c.rf01},
		{"RF02", "Logical total of the run, no duplicates across pages", c.rf02},
		{"RF03", "Conflicting payloads, both arrival orders", c.rf03},
		{"RF04", "Failed grandchild under an OK root", c.rf04},
		{"RF05", "Late parent is attached", func(ctx context.Context) error { return c.rf05(ctx, true) }},
		{"RF06", "Parent outside the window, then widened", c.rf06},
		{"RF07", "Exception event, stack trace and link", c.rf07},
		{"RF08", "Paging over equal times and span ids", c.rf08},
		{"RF09", "Four-level trace", c.rf09},
		{"RF10", "Exactly N and N+1 children", c.rf10},
		{"RF12", "Clock-skewed and async children, window edges", c.rf12},
		{"RF16", "v1/v2 populations and durations", c.rf16},
	}
}

type namedCheck struct {
	id, title string
	run       func(ctx context.Context) error
}
