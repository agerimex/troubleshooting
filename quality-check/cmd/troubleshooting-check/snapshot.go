package main

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Evidence snapshots: the spans of each evidence trace, read once when the
// run is checked and kept in result.json and report.html. Raw spans expire
// (3 days by default), so a link in an old pull request stops working; the
// snapshot still shows what the execution did. It never changes a status:
// a failed read is only noted.

const maxSnapshotSpans = 200

type SnapSpan struct {
	SpanID   string `json:"span_id"`
	ParentID string `json:"parent_id"`
	Service  string `json:"service"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Start    string `json:"start"`       // Unix nanoseconds
	Duration int64  `json:"duration_us"` // -1: invalid
	Status   string `json:"status"`
	Peer     string `json:"peer,omitempty"` // peer.service or db.system of a CLIENT span
}

type Snapshot struct {
	Spans     []SnapSpan `json:"spans,omitempty"`
	Truncated bool       `json:"truncated,omitempty"`
	Error     string     `json:"error,omitempty"`
}

type reportSpan struct {
	Timestamp string              `json:"timeStamp"`
	Name      string              `json:"name"`
	Kind      string              `json:"spanKind"`
	Service   string              `json:"service"`
	SpanID    string              `json:"spanId"`
	Parent    string              `json:"parentSpanId"`
	Status    string              `json:"status"`
	Duration  int64               `json:"duration"`
	Tags      []map[string]string `json:"tags"`
}

// snapshotReaders bounds the trace reports read at the same time.
const snapshotReaders = 4

// readSpanLimit is how many spans of an evidence trace are read to choose
// the snapshot's maxSnapshotSpans from (the trace report's maximum).
const readSpanLimit = 1000

// traceRead is the spans of one evidence trace and window as read, in start
// order; db marks the database calls (CLIENT spans with db.system).
type traceRead struct {
	spans     []SnapSpan
	db        []bool
	truncated bool
	err       string
}

// addSnapshots reads the spans of every evidence that has no snapshot yet (a
// recorded evaluation comes with the API's): one request per distinct trace
// and window, up to snapshotReaders at a time. Each evidence then keeps its
// counterexample first (snapshotFor).
func addSnapshots(o options, ev *Evaluation) {
	type window struct{ trace, from, to string }
	reads := map[window]*traceRead{}
	var order []window
	for _, c := range ev.Checks {
		for _, e := range c.Evidence {
			if e.Snapshot != nil {
				continue
			}
			w := window{e.TraceID, e.TimeFrom, e.TimeTo}
			if _, ok := reads[w]; !ok {
				reads[w] = nil
				order = append(order, w)
			}
		}
	}
	results := make([]traceRead, len(order))
	var wg sync.WaitGroup
	sem := make(chan struct{}, snapshotReaders)
	for i, w := range order {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, w window) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = readTrace(o, w.trace, w.from, w.to)
		}(i, w)
	}
	wg.Wait()
	for i, w := range order {
		reads[w] = &results[i]
	}
	for i := range ev.Checks {
		for j := range ev.Checks[i].Evidence {
			e := &ev.Checks[i].Evidence[j]
			if e.Snapshot == nil {
				if t := reads[window{e.TraceID, e.TimeFrom, e.TimeTo}]; t != nil {
					e.Snapshot = snapshotFor(*t, e.SpanID, ev.Checks[i].Type == "db_calls")
				}
			}
		}
	}
}

// snapshotFor keeps at most maxSnapshotSpans of a read trace: the evidence
// span with its ancestors, for a db_calls check its database calls, then the
// spans below the evidence span, then the rest (the API's rule,
// SnapshotSelection in log-analysis).
func snapshotFor(t traceRead, focusSpan string, dbCheck bool) *Snapshot {
	if t.err != "" {
		return &Snapshot{Error: t.err}
	}
	ids := make([]string, len(t.spans))
	parents := make([]string, len(t.spans))
	focus := []string{focusSpan}
	for i, s := range t.spans {
		ids[i], parents[i] = s.SpanID, s.ParentID
		if dbCheck && t.db[i] {
			focus = append(focus, s.SpanID)
		}
	}
	keep, cut := snapshotSelection(ids, parents, focus, maxSnapshotSpans)
	s := &Snapshot{Truncated: t.truncated || cut}
	for _, i := range keep {
		s.Spans = append(s.Spans, t.spans[i])
	}
	return s
}

// snapshotSelection is log-analysis's SnapshotSelection (this module doesn't
// depend on it): the chosen indexes in start order, and whether spans were
// left out.
func snapshotSelection(ids, parents, focus []string, limit int) ([]int, bool) {
	index := map[string]int{}
	children := map[string][]int{}
	for i, id := range ids {
		if _, ok := index[id]; !ok {
			index[id] = i
		}
		children[parents[i]] = append(children[parents[i]], i)
	}
	chosen := make([]bool, len(ids))
	n := 0
	take := func(i int) {
		if !chosen[i] && n < limit {
			chosen[i] = true
			n++
		}
	}
	for _, id := range focus {
		f, ok := index[id]
		if !ok {
			continue
		}
		for i, seen := f, map[int]bool{}; !seen[i]; {
			seen[i] = true
			take(i)
			p, ok := index[parents[i]]
			if !ok {
				break
			}
			i = p
		}
	}
	if len(focus) > 0 {
		if f, ok := index[focus[0]]; ok {
			below := make([]bool, len(ids))
			stack := []int{f}
			for len(stack) > 0 {
				i := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				for _, c := range children[ids[i]] {
					if !below[c] && c != f {
						below[c] = true
						stack = append(stack, c)
					}
				}
			}
			for i := range ids {
				if below[i] {
					take(i)
				}
			}
		}
	}
	for i := range ids {
		take(i)
	}
	out := make([]int, 0, n)
	for i := range ids {
		if chosen[i] {
			out = append(out, i)
		}
	}
	return out, len(ids) > limit
}

func readTrace(o options, trace, from, to string) traceRead {
	status, raw, err := postJSON(o, "/api/v1/trace-report", map[string]any{"trace_id": trace, "time_from": from, "time_to": to,
		"span_limit": readSpanLimit, "sections": []string{"spans"}}, 32<<20)
	if err != nil {
		return traceRead{err: err.Error()}
	}
	var out struct {
		Error   bool   `json:"error"`
		Message string `json:"message"`
		Data    struct {
			Report struct {
				Spans *struct {
					Status    string       `json:"status"`
					Error     string       `json:"error"`
					Records   []reportSpan `json:"records"`
					Truncated bool         `json:"truncated"`
				} `json:"spans"`
			} `json:"Report"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || status != http.StatusOK || out.Error {
		return traceRead{err: fmt.Sprintf("HTTP %d %s", status, out.Message)}
	}
	// Only "ok" has records: "limit" means a query limit or the deadline was
	// hit and nothing was returned (report contract), which must not read as
	// an empty trace.
	sec := out.Data.Report.Spans
	if sec == nil || sec.Status != "ok" {
		msg := "spans unavailable"
		if sec != nil && sec.Status == "limit" {
			msg = "query limit"
		}
		if sec != nil && sec.Error != "" {
			msg += ": " + sec.Error
		}
		return traceRead{err: msg}
	}
	t := traceRead{truncated: sec.Truncated}
	for _, r := range sec.Records {
		sp := SnapSpan{SpanID: r.SpanID, ParentID: r.Parent, Service: r.Service, Name: r.Name, Kind: r.Kind,
			Start: r.Timestamp, Duration: r.Duration, Status: r.Status}
		db := false
		for _, tag := range r.Tags {
			if tag["key"] == "peer.service" || tag["key"] == "db.system" || tag["key"] == "db.system.name" {
				if sp.Peer == "" {
					sp.Peer = tag["value"]
				}
				db = db || tag["key"] != "peer.service"
			}
		}
		t.spans = append(t.spans, sp)
		t.db = append(t.db, db && r.Kind == "client")
	}
	return t
}

// snapshotRows orders the spans as a tree (children under their parent, by
// start time) with their depth. Spans whose parent isn't in the snapshot
// are roots.
func snapshotRows(spans []SnapSpan) ([]SnapSpan, []int) {
	byID := map[string]bool{}
	for _, s := range spans {
		byID[s.SpanID] = true
	}
	children := map[string][]SnapSpan{}
	var roots []SnapSpan
	for _, s := range spans {
		if byID[s.ParentID] && s.ParentID != s.SpanID {
			children[s.ParentID] = append(children[s.ParentID], s)
		} else {
			roots = append(roots, s)
		}
	}
	byStart := func(l []SnapSpan) {
		sort.SliceStable(l, func(i, j int) bool { return startOf(l[i]) < startOf(l[j]) })
	}
	byStart(roots)
	var rows []SnapSpan
	var depth []int
	seen := map[string]bool{}
	var walk func(s SnapSpan, d int)
	walk = func(s SnapSpan, d int) {
		if seen[s.SpanID] {
			return
		}
		seen[s.SpanID] = true
		rows, depth = append(rows, s), append(depth, d)
		kids := children[s.SpanID]
		byStart(kids)
		for _, k := range kids {
			walk(k, d+1)
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
	return rows, depth
}

func startOf(s SnapSpan) int64 {
	n, _ := strconv.ParseInt(s.Start, 10, 64)
	return n
}

// snapshotHTML is a static timeline: offset, duration bar, depth, names.
func snapshotHTML(s *Snapshot, highlight string) string {
	e := html.EscapeString
	if s == nil {
		return ""
	}
	if s.Error != "" {
		return `<p class="meta">Snapshot of the execution unavailable: ` + e(s.Error) + `</p>`
	}
	if len(s.Spans) == 0 {
		return `<p class="meta">Snapshot: no spans of this trace were found in its window.</p>`
	}
	rows, depth := snapshotRows(s.Spans)
	first, last := startOf(rows[0]), int64(0)
	for _, r := range rows {
		st := startOf(r)
		if st < first {
			first = st
		}
		end := st
		if r.Duration > 0 {
			end += r.Duration * 1000
		}
		if end > last {
			last = end
		}
	}
	total := float64(last - first)
	if total <= 0 {
		total = 1
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<details class="snap"><summary>Snapshot of the execution: %d spans`, len(rows))
	if s.Truncated {
		fmt.Fprintf(&b, ` (first %d)`, maxSnapshotSpans)
	}
	b.WriteString(`, kept in this report</summary><table class="snap"><thead><tr><th>Start</th><th>Duration</th><th>Span</th><th class="bar"></th></tr></thead><tbody>`)
	for i, r := range rows {
		st := startOf(r) - first
		left := float64(st) / total * 100
		dur := "invalid"
		width := 0.0
		if r.Duration >= 0 {
			dur = fmtMsUS(r.Duration)
			width = float64(r.Duration*1000) / total * 100
		}
		cls := ""
		if r.SpanID == highlight {
			cls = ` class="hl"`
		}
		name := e(r.Name)
		if r.Peer != "" {
			name += ` <span class="meta">→ ` + e(r.Peer) + `</span>`
		}
		status := ""
		if r.Status == "error" {
			status = ` <span class="fail">error</span>`
		}
		fmt.Fprintf(&b, `<tr%s><td>+%s</td><td>%s</td><td style="padding-left:%dpx">%s<br><span class="meta">%s · %s</span>%s</td>`+
			`<td class="bar"><i style="margin-left:%.2f%%;width:%.2f%%"></i></td></tr>`,
			cls, fmtMsUS(st/1000), dur, 8+depth[i]*14, name, e(r.Service), e(r.Kind), status, left, minf(maxf(width, 0.4), 100-left))
	}
	b.WriteString(`</tbody></table></details>`)
	return b.String()
}

func fmtMsUS(us int64) string {
	return strconv.FormatFloat(float64(us)/1000, 'f', 1, 64) + " ms"
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
