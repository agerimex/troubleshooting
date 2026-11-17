package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
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

// addSnapshots reads the spans of every evidence (one request per distinct
// trace and window).
func addSnapshots(o options, ev *Evaluation) {
	cache := map[string]*Snapshot{}
	for i := range ev.Checks {
		for j := range ev.Checks[i].Evidence {
			e := &ev.Checks[i].Evidence[j]
			key := e.TraceID + "|" + e.TimeFrom + "|" + e.TimeTo
			if cache[key] == nil {
				cache[key] = readSnapshot(o, e.TraceID, e.TimeFrom, e.TimeTo)
			}
			e.Snapshot = cache[key]
		}
	}
}

func readSnapshot(o options, trace, from, to string) *Snapshot {
	body, _ := json.Marshal(map[string]any{"trace_id": trace, "time_from": from, "time_to": to,
		"span_limit": maxSnapshotSpans, "sections": []string{"spans"}})
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	endpoint := strings.TrimRight(o.url, "/") + "/api/v1/trace-report"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return &Snapshot{Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	if user, pass, ok := strings.Cut(o.basicAuth, ":"); ok {
		req.SetBasicAuth(user, pass)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &Snapshot{Error: err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
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
	if err := json.Unmarshal(raw, &out); err != nil || resp.StatusCode != http.StatusOK || out.Error {
		return &Snapshot{Error: fmt.Sprintf("HTTP %d %s", resp.StatusCode, out.Message)}
	}
	sec := out.Data.Report.Spans
	if sec == nil || sec.Status != "ok" && sec.Status != "limit" {
		msg := "spans unavailable"
		if sec != nil && sec.Error != "" {
			msg += ": " + sec.Error
		}
		return &Snapshot{Error: msg}
	}
	s := &Snapshot{Truncated: sec.Truncated || len(sec.Records) > maxSnapshotSpans}
	for i, r := range sec.Records {
		if i == maxSnapshotSpans {
			break
		}
		sp := SnapSpan{SpanID: r.SpanID, ParentID: r.Parent, Service: r.Service, Name: r.Name, Kind: r.Kind,
			Start: r.Timestamp, Duration: r.Duration, Status: r.Status}
		for _, t := range r.Tags {
			if t["key"] == "peer.service" || t["key"] == "db.system" || t["key"] == "db.system.name" {
				sp.Peer = t["value"]
				break
			}
		}
		s.Spans = append(s.Spans, sp)
	}
	return s
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
