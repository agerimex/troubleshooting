package main

import (
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Report files: Markdown for a CI job summary or a pull request comment, and
// a self-contained HTML page. Both show exactly the evaluation the API
// returned; recorded names are escaped, and the HTML has no scripts.

var gateTitle = map[string]string{"passed": "Passed", "failed": "Failed", "unresolved": "Unresolved: not enough data"}

var statusTitle = map[string]string{"pass": "pass", "fail": "FAIL", "insufficient": "no data"}

var statusIcon = map[string]string{"pass": "✅", "fail": "❌", "insufficient": "⚠️"}

var gateIcon = map[string]string{"passed": "✅", "failed": "❌", "unresolved": "⚠️"}

var requirementWord = map[string]string{"met": "MET", "violated": "VIOLATED", "insufficient": "NO DATA", "not_evaluated": "NOT EVALUATED"}

var requirementTitle = map[string]string{"met": "met", "violated": "violated", "insufficient": "not enough data", "not_evaluated": "not evaluated"}

var requirementIcon = map[string]string{"met": "✅", "violated": "❌", "insufficient": "⚠️", "not_evaluated": "➖"}

// requirementCounts is e.g. "1 violated, 1 met, 1 not evaluated", worst first.
func requirementCounts(rs []RequirementResult) string {
	n := map[string]int{}
	for _, r := range rs {
		n[r.Status]++
	}
	var parts []string
	for _, st := range []string{"violated", "insufficient", "met", "not_evaluated"} {
		if n[st] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n[st], requirementTitle[st]))
			delete(n, st)
		}
	}
	other := make([]string, 0, len(n))
	for st := range n {
		other = append(other, st)
	}
	sort.Strings(other)
	for _, st := range other {
		parts = append(parts, fmt.Sprintf("%d %s", n[st], st))
	}
	return strings.Join(parts, ", ")
}

func titleOf(m map[string]string, k string) string {
	if v, ok := m[k]; ok {
		return v
	}
	return k
}

// mdText escapes text for a Markdown table cell or line: recorded names can
// contain |, backticks, brackets or HTML.
func mdText(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "|", `\|`, "`", "\\`", "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`,
		"<", "&lt;", ">", "&gt;", "\n", " ", "\r", " ")
	return r.Replace(s)
}

// webLink: only http(s) links become links in a report.
func webLink(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func baselineText(c CheckResult) string {
	if c.Baseline == nil {
		return ""
	}
	if c.Baseline.Status == "insufficient" {
		return "no data"
	}
	return fmtValue(c.Baseline.Value, c.Unit)
}

func valueText(c CheckResult) string {
	if c.Value == nil {
		return "-"
	}
	return fmtValue(c.Value, c.Unit)
}

func limitText(c CheckResult) string {
	s := fmtValue(c.Limit, c.Unit)
	if c.Target != nil {
		s += " (target " + fmtValue(c.Target, c.Unit)
		if c.TargetStatus != "" {
			s += ", " + c.TargetStatus
		}
		s += ")"
	}
	return s
}

// mdURL escapes the characters that would end a Markdown link target.
var mdURL = strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29", "<", "%3C", ">", "%3E")

// markdownReport is the summary for a job page or a pull request comment;
// htmlName names the HTML report written next to it ("" if none), where
// evidence without a link points for its snapshot.
func markdownReport(ev Evaluation, htmlName string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### %s Quality gate: %s\n\n", titleOf(gateIcon, ev.Gate), titleOf(gateTitle, ev.Gate))
	fmt.Fprintf(&b, "Run `%s`", strings.ReplaceAll(ev.Run, "`", "'"))
	hasBaseline := ev.Baseline != ""
	if hasBaseline {
		fmt.Fprintf(&b, " against baseline `%s`", strings.ReplaceAll(ev.Baseline, "`", "'"))
	}
	fmt.Fprintf(&b, ", %d checks.", len(ev.Checks))
	if s := sourceText(ev.Source); s != "" {
		fmt.Fprintf(&b, " %s.", mdText(s))
	}
	if ev.Source != nil && webLink(ev.Source.CIRun) {
		fmt.Fprintf(&b, " [CI run](%s)", mdURL.Replace(ev.Source.CIRun))
	}
	b.WriteString("\n\n")
	if len(ev.Requirements) > 0 {
		fmt.Fprintf(&b, "**Requirements:** %s\n\n| | Requirement | Owner | Status |\n|---|---|---|---|\n", requirementCounts(ev.Requirements))
		for _, r := range ev.Requirements {
			status := titleOf(requirementTitle, r.Status) + ": " + r.Summary
			if r.GuardrailSummary != "" {
				status += "; " + r.GuardrailSummary
			}
			fmt.Fprintf(&b, "| %s | **%s** %s | %s | %s |\n", titleOf(requirementIcon, r.Status), mdText(r.ID), mdText(r.Title),
				mdText(r.Owner), mdText(status))
		}
		if hasGuardrails(ev.Requirements) {
			b.WriteString("\n" + mdText(guardrailDefinition) + "\n")
		}
		b.WriteString("\n")
	}
	if hasBaseline {
		b.WriteString("| | Check | Value | Limit | Baseline | Executions |\n|---|---|---|---|---|---|\n")
	} else {
		b.WriteString("| | Check | Value | Limit | Executions |\n|---|---|---|---|---|\n")
	}
	for _, c := range ev.Checks {
		id := mdText(c.ID)
		if c.Advisory {
			id += " (advisory)"
		}
		fmt.Fprintf(&b, "| %s | **%s**<br>%s | %s | %s |", titleOf(statusIcon, c.Status), id, mdText(entryText(c.Entry)), mdText(valueText(c)), mdText(limitText(c)))
		if hasBaseline {
			fmt.Fprintf(&b, " %s |", mdText(baselineText(c)))
		}
		fmt.Fprintf(&b, " %d |\n", c.Executions)
	}
	// Only what needs attention gets details; passes are in the table (and in
	// full in the HTML and JSON reports).
	for _, c := range ev.Checks {
		if c.Status == "pass" {
			continue
		}
		fmt.Fprintf(&b, "\n**%s** %s: %s\n", mdText(c.ID), titleOf(statusTitle, c.Status), mdText(c.Summary))
		for _, d := range c.Details {
			fmt.Fprintf(&b, "- %s\n", mdText(d))
		}
		for _, e := range c.Evidence {
			if webLink(e.Link) {
				fmt.Fprintf(&b, "- Evidence: [%s](%s)\n", mdText(e.Note), mdURL.Replace(e.Link))
			} else {
				fmt.Fprintf(&b, "- Evidence: %s, trace `%s`%s\n", mdText(e.Note), e.TraceID, snapshotNote(e.Snapshot, htmlName))
			}
		}
	}
	fmt.Fprintf(&b, "\n<sub>%s · %s · evaluated %s. Statuses describe this run only; a pass is \"not observed in this run\", not a guarantee.",
		mdText(ev.Contract), mdText(ev.Method), mdText(ev.EvaluatedAt))
	for _, a := range ev.Assumptions {
		fmt.Fprintf(&b, " %s", mdText(a))
	}
	b.WriteString("</sub>\n")
	return b.String()
}

const htmlStyle = `body{font:14px/1.5 system-ui,sans-serif;margin:24px;color:#1b2316;background:#fff}
h1{font-size:22px;margin:0 0 4px}h2{font-size:16px;margin:18px 0 0}table{border-collapse:collapse;margin:16px 0;width:100%}
th,td{border:1px solid #d5dccf;padding:6px 8px;text-align:left;vertical-align:top}th{background:#f3f6ef}
.pass{color:#2f6b12}.fail{color:#a1260d;font-weight:600}.insufficient{color:#8a5a00;font-weight:600}
.meta{color:#5a6553;font-size:12px}code{font-size:12px}li{margin:2px 0}td.chk{min-width:170px}table.res>tbody>tr>td:not(.chk):not(:last-child){white-space:nowrap}
details.snap{margin:6px 0 10px}details.snap summary{cursor:pointer;color:#3f6b12}
table.snap{margin:6px 0;font-size:12px}table.snap td{padding:3px 6px;white-space:nowrap}table.snap tr.hl td{background:#fff4d6}
table.snap td.bar,table.snap th.bar{width:40%;min-width:120px}td.bar i{display:block;height:8px;background:#7aa45a;border-radius:2px}`

func htmlReport(ev Evaluation) string {
	e := html.EscapeString
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8">`)
	b.WriteString(`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'">`)
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">`)
	fmt.Fprintf(&b, "<title>Quality gate: %s</title><style>%s</style></head><body>", e(titleOf(gateTitle, ev.Gate)), htmlStyle)
	cls := map[string]string{"passed": "pass", "failed": "fail", "unresolved": "insufficient"}[ev.Gate]
	fmt.Fprintf(&b, `<h1 class="%s">Quality gate: %s</h1>`, cls, e(titleOf(gateTitle, ev.Gate)))
	fmt.Fprintf(&b, `<p class="meta">Run <code>%s</code>, %s%s`, e(ev.Run), e(windowText(ev.TimeFrom, ev.TimeTo)), fromRun(ev.WindowFromRun))
	if ev.Baseline != "" {
		fmt.Fprintf(&b, ` · baseline <code>%s</code>, %s%s`, e(ev.Baseline), e(windowText(ev.BaselineFrom, ev.BaselineTo)), fromRun(ev.BaselineFromRun))
	}
	fmt.Fprintf(&b, ` · %s · evaluated %s`, e(ev.Contract), e(ev.EvaluatedAt))
	if s := sourceText(ev.Source); s != "" {
		fmt.Fprintf(&b, `<br>%s`, e(s))
	}
	if ev.Source != nil && webLink(ev.Source.CIRun) {
		fmt.Fprintf(&b, ` · <a href="%s">CI run</a>`, e(ev.Source.CIRun))
	}
	b.WriteString(`</p>`)
	if len(ev.Requirements) > 0 {
		fmt.Fprintf(&b, `<h2>Requirements: %s</h2><table class="req"><thead><tr><th>Status</th><th>Requirement</th><th>Owner</th><th>Why</th><th>Checks</th></tr></thead><tbody>`,
			e(requirementCounts(ev.Requirements)))
		rcls := map[string]string{"met": "pass", "violated": "fail", "insufficient": "insufficient"}
		for _, r := range ev.Requirements {
			guard := ""
			if len(r.Guardrails) > 0 {
				guard = `<br>Guardrails: ` + e(strings.Join(r.Guardrails, ", ")) + `<br><span class="meta">` + e(r.GuardrailSummary) + `</span>`
			}
			fmt.Fprintf(&b, `<tr><td class="%s">%s</td><td class="chk"><b>%s</b><br>%s</td><td>%s</td><td>%s</td><td>%s<br><span class="meta">%s</span>%s</td></tr>`,
				rcls[r.Status], e(titleOf(requirementTitle, r.Status)), e(r.ID), e(r.Title), e(r.Owner), e(r.Why), e(strings.Join(r.Checks, ", ")), e(r.Summary), guard)
		}
		b.WriteString(`</tbody></table>`)
		if hasGuardrails(ev.Requirements) {
			fmt.Fprintf(&b, `<p class="meta">%s</p>`, e(guardrailDefinition))
		}
		b.WriteString(`<h2>Checks</h2>`)
	}
	b.WriteString(`<table class="res"><thead><tr><th>Status</th><th>Check</th><th>Value</th><th>Limit</th>`)
	if ev.Baseline != "" {
		b.WriteString(`<th>Baseline</th>`)
	}
	b.WriteString(`<th>Executions</th><th>Result</th></tr></thead><tbody>`)
	for _, c := range ev.Checks {
		adv := ""
		if c.Advisory {
			adv = " (advisory)"
		}
		fmt.Fprintf(&b, `<tr><td class="%s">%s</td><td class="chk"><b>%s</b>%s<br><span class="meta">%s</span></td><td>%s</td><td>%s</td>`,
			e(c.Status), e(titleOf(statusTitle, c.Status)), e(c.ID), adv, e(entryText(c.Entry)), e(valueText(c)), e(limitText(c)))
		if ev.Baseline != "" {
			fmt.Fprintf(&b, `<td>%s</td>`, e(baselineText(c)))
		}
		fmt.Fprintf(&b, `<td>%d</td><td>%s`, c.Executions, e(c.Summary))
		if len(c.Details) > 0 || len(c.Evidence) > 0 {
			b.WriteString(`<ul>`)
			for _, d := range c.Details {
				fmt.Fprintf(&b, `<li>%s</li>`, e(d))
			}
			for _, ev := range c.Evidence {
				if webLink(ev.Link) {
					fmt.Fprintf(&b, `<li>Evidence: <a href="%s">%s</a>%s</li>`, e(ev.Link), e(ev.Note), snapshotHTML(ev.Snapshot, ev.SpanID))
				} else {
					fmt.Fprintf(&b, `<li>Evidence: %s, trace <code>%s</code>%s</li>`, e(ev.Note), e(ev.TraceID), snapshotHTML(ev.Snapshot, ev.SpanID))
				}
			}
			b.WriteString(`</ul>`)
		}
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</tbody></table>`)
	fmt.Fprintf(&b, `<p class="meta">%s. Statuses describe this run only: a pass means "not observed in this run", not a guarantee; a missing measurement is never a pass.</p>`, e(ev.Method))
	if len(ev.Assumptions) > 0 {
		b.WriteString(`<ul class="meta">`)
		for _, a := range ev.Assumptions {
			fmt.Fprintf(&b, `<li>%s</li>`, e(a))
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`</body></html>`)
	return b.String()
}

// sourceText says what was tested and which rules decided, e.g.
// "commit 1a2b3c4d5e6f (feat/x), merging head 0a1b2c3d4e5f · rules
// main@89ab01234567:quality.yaml, sha256 0123456789ab".
// Empty if nothing is known.
func sourceText(s *Source) string {
	if s == nil {
		return ""
	}
	var parts []string
	if s.Commit != "" {
		c := "commit " + short(s.Commit)
		if s.Ref != "" {
			c += " (" + s.Ref + ")"
		}
		if s.PRHead != "" {
			c += ", merging head " + short(s.PRHead)
		}
		parts = append(parts, c)
	} else if s.Ref != "" {
		parts = append(parts, "ref "+s.Ref)
	}
	if s.Rules != "" {
		r := "rules " + s.Rules // the file as given, or where it was read from
		if s.RulesRef != "" {
			r = "rules " + s.RulesRef
		}
		if s.RulesSHA256 != "" {
			r += ", sha256 " + short(s.RulesSHA256)
		}
		parts = append(parts, r)
	}
	return strings.Join(parts, " · ")
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// windowText shows a [from, to) window of Unix nanoseconds in UTC, e.g.
// "2026-11-17 12:05:05 to 12:08:25 UTC"; values that aren't nanoseconds are
// shown as they are.
// fromRun marks a window found from the run's own spans.
func fromRun(yes bool) string {
	if yes {
		return " (the run's window)"
	}
	return ""
}

func windowText(from, to string) string {
	f, err1 := strconv.ParseInt(from, 10, 64)
	t, err2 := strconv.ParseInt(to, 10, 64)
	if err1 != nil || err2 != nil {
		return "[" + from + ", " + to + ")"
	}
	ft, tt := time.Unix(0, f).UTC(), time.Unix(0, t).UTC()
	end := tt.Format("2006-01-02 15:04:05")
	if ft.Format("2006-01-02") == tt.Format("2006-01-02") {
		end = tt.Format("15:04:05")
	}
	return ft.Format("2006-01-02 15:04:05") + " to " + end + " UTC"
}

// snapshotNote points an evidence without a link to its snapshot in the HTML
// report, if one is written and the snapshot has spans.
func snapshotNote(s *Snapshot, htmlName string) string {
	if s == nil || len(s.Spans) == 0 || htmlName == "" {
		return ""
	}
	return "; its spans are in " + htmlName
}
