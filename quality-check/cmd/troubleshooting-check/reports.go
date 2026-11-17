package main

import (
	"fmt"
	"html"
	"strings"
)

// Report files: Markdown for a CI job summary or a pull request comment, and
// a self-contained HTML page. Both show exactly the evaluation the API
// returned; recorded names are escaped, and the HTML has no scripts.

var gateTitle = map[string]string{"passed": "Passed", "failed": "Failed", "unresolved": "Unresolved: not enough data"}

var statusTitle = map[string]string{"pass": "pass", "fail": "FAIL", "insufficient": "no data"}

var statusIcon = map[string]string{"pass": "✅", "fail": "❌", "insufficient": "⚠️"}

var gateIcon = map[string]string{"passed": "✅", "failed": "❌", "unresolved": "⚠️"}

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

func markdownReport(ev Evaluation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### %s Quality gate: %s\n\n", titleOf(gateIcon, ev.Gate), titleOf(gateTitle, ev.Gate))
	fmt.Fprintf(&b, "Run `%s`", strings.ReplaceAll(ev.Run, "`", "'"))
	hasBaseline := ev.Baseline != ""
	if hasBaseline {
		fmt.Fprintf(&b, " against baseline `%s`", strings.ReplaceAll(ev.Baseline, "`", "'"))
	}
	fmt.Fprintf(&b, ", %d checks.\n\n", len(ev.Checks))
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
				fmt.Fprintf(&b, "- Evidence: [%s](%s)\n", mdText(e.Note), strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29", "<", "%3C", ">", "%3E").Replace(e.Link))
			} else {
				fmt.Fprintf(&b, "- Evidence: %s (trace `%s`)\n", mdText(e.Note), e.TraceID)
			}
		}
	}
	fmt.Fprintf(&b, "\n<sub>%s · %s · evaluated %s. Statuses describe this run only; a pass is \"not observed in this run\", not a guarantee.</sub>\n",
		mdText(ev.Contract), mdText(ev.Method), mdText(ev.EvaluatedAt))
	return b.String()
}

const htmlStyle = `body{font:14px/1.5 system-ui,sans-serif;margin:24px;color:#1b2316;background:#fff}
h1{font-size:22px;margin:0 0 4px}table{border-collapse:collapse;margin:16px 0;width:100%}
th,td{border:1px solid #d5dccf;padding:6px 8px;text-align:left;vertical-align:top}th{background:#f3f6ef}
.pass{color:#2f6b12}.fail{color:#a1260d;font-weight:600}.insufficient{color:#8a5a00;font-weight:600}
.meta{color:#5a6553;font-size:12px}code{font-size:12px}li{margin:2px 0}`

func htmlReport(ev Evaluation) string {
	e := html.EscapeString
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8">`)
	b.WriteString(`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'">`)
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">`)
	fmt.Fprintf(&b, "<title>Quality gate: %s</title><style>%s</style></head><body>", e(titleOf(gateTitle, ev.Gate)), htmlStyle)
	cls := map[string]string{"passed": "pass", "failed": "fail", "unresolved": "insufficient"}[ev.Gate]
	fmt.Fprintf(&b, `<h1 class="%s">Quality gate: %s</h1>`, cls, e(titleOf(gateTitle, ev.Gate)))
	fmt.Fprintf(&b, `<p class="meta">Run <code>%s</code> [%s, %s)`, e(ev.Run), e(ev.TimeFrom), e(ev.TimeTo))
	if ev.Baseline != "" {
		fmt.Fprintf(&b, ` · baseline <code>%s</code> [%s, %s)`, e(ev.Baseline), e(ev.BaselineFrom), e(ev.BaselineTo))
	}
	fmt.Fprintf(&b, ` · %s · evaluated %s</p>`, e(ev.Contract), e(ev.EvaluatedAt))
	b.WriteString(`<table><thead><tr><th>Status</th><th>Check</th><th>Value</th><th>Limit</th>`)
	if ev.Baseline != "" {
		b.WriteString(`<th>Baseline</th>`)
	}
	b.WriteString(`<th>Executions</th><th>Result</th></tr></thead><tbody>`)
	for _, c := range ev.Checks {
		adv := ""
		if c.Advisory {
			adv = " (advisory)"
		}
		fmt.Fprintf(&b, `<tr><td class="%s">%s</td><td><b>%s</b>%s<br><span class="meta">%s</span></td><td>%s</td><td>%s</td>`,
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
					fmt.Fprintf(&b, `<li>Evidence: <a href="%s">%s</a></li>`, e(ev.Link), e(ev.Note))
				} else {
					fmt.Fprintf(&b, `<li>Evidence: %s (trace <code>%s</code>)</li>`, e(ev.Note), e(ev.TraceID))
				}
			}
			b.WriteString(`</ul>`)
		}
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</tbody></table>`)
	fmt.Fprintf(&b, `<p class="meta">%s. Statuses describe this run only: a pass means "not observed in this run", not a guarantee; a missing measurement is never a pass.</p>`, e(ev.Method))
	b.WriteString(`</body></html>`)
	return b.String()
}
