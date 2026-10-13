import { formatNs, toNs } from '@/services/spanTree'
import { escapeHtml, exportJson as reportJson, htmlPage, reportHtmlBody } from '@/services/report'
import { type Report } from '@/types/report'
import {
  type Analysis, type CallStats, type Classification, type ErrorGroup, type GroupComparison, type MetricComparison,
  type Percentile, type PlanPart, type Population, type PopulationStats, type ReportContext
} from '@/types/stats'

// Pure helpers of the statistics and comparison views. Every value that could
// not be determined is shown as unavailable, never as 0.

export const UNAVAILABLE = 'unavailable'

export const INTERPRETATION_SHORT =
  'Signals point to what to investigate. They are rule-based heuristics, not proof that a release caused a change; results within thresholds are not proof of identical behavior.'

export function classificationLabel (c: Classification | string): string {
  switch (c) {
    case 'regression_signal': return 'Regression signal'
    case 'improvement_signal': return 'Improvement signal'
    case 'within_threshold': return 'Within threshold'
    case 'insufficient_evidence': return 'Insufficient evidence'
    case 'incomparable': return 'Not comparable'
    case 'descriptive': return 'Descriptive only'
  }
  return c
}

export function classificationSeverity (c: Classification | string): string {
  switch (c) {
    case 'regression_signal': return 'danger'
    case 'improvement_signal': return 'success'
    case 'within_threshold': return 'info'
    case 'insufficient_evidence':
    case 'incomparable': return 'warning'
  }
  return 'secondary'
}

export function metricLabel (metric: string): string {
  switch (metric) {
    case 'calls': return 'Recorded calls'
    case 'rate': return 'Observed call rate'
    case 'error_fraction': return 'Recorded error fraction'
  }
  return metric
}

// formatMetric shows a metric value with its unit.
export function formatMetric (metric: string, value: number | null | undefined): string {
  if (value === null || value === undefined) {
    return UNAVAILABLE
  }
  switch (metric) {
    case 'calls': return String(Math.round(value))
    case 'rate': return value.toFixed(2) + ' calls/min'
    case 'error_fraction': return value.toFixed(2) + ' %'
  }
  return value.toFixed(1) + ' ms'
}

function signed (value: number, digits: number): string {
  const text = value.toFixed(digits)
  return value > 0 ? '+' + text : text
}

// formatAbsolute is candidate minus baseline in the metric's change unit
// (percentage points for the error fraction).
export function formatAbsolute (m: MetricComparison): string {
  if (m.absolute === null) {
    return UNAVAILABLE
  }
  switch (m.metric) {
    case 'calls': return signed(m.absolute, 0)
    case 'rate': return signed(m.absolute, 2) + ' calls/min'
    case 'error_fraction': return signed(m.absolute, 2) + ' pp'
  }
  return signed(m.absolute, 1) + ' ms'
}

// formatRelative is the relative change in percent, rounded for display only
// (classifications use the unrounded values).
export function formatRelative (m: MetricComparison): string {
  if (m.relative !== null) {
    return signed(m.relative, 2) + ' %'
  }
  return m.relativeNote ? 'unavailable (baseline is zero)' : UNAVAILABLE
}

export function percentileText (p: Percentile | undefined): string {
  if (!p) {
    return UNAVAILABLE
  }
  switch (p.status) {
    case 'available': return (p.valueMs ?? 0).toFixed(1) + ' ms'
    case 'insufficient_sample': return `insufficient sample (${p.samples} of ${p.minimum})`
  }
  return UNAVAILABLE
}

export function errorFractionText (s: CallStats | null | undefined): string {
  if (!s || s.errorFraction === null) {
    return UNAVAILABLE
  }
  return (s.errorFraction * 100).toFixed(2) + ' %'
}

// populationText describes a population definition for people.
export function populationText (p: Population): string {
  const dim = (name: string, v: string | undefined) => v === undefined ? null : (v === '' ? `${name} not recorded` : `${name} ${v}`)
  return [p.service, dim('operation', p.operation), dim('span kind', p.spanKind), dim('environment', p.environment),
    dim('version', p.version), dim('installation', p.installation), dim('segment', p.segment)]
    .filter((x) => x !== null).join(' · ')
}

export function windowText (from: string, to: string): string {
  return `${formatNs(from)} – ${formatNs(to)} [from, to)`
}

export function durationText (seconds: number): string {
  if (seconds % 3600 === 0) {
    return `${seconds / 3600} h`
  }
  if (seconds % 60 === 0) {
    return `${seconds / 60} min`
  }
  return `${seconds} s`
}

// planSummary says where the values come from, e.g. "2 stored buckets (1
// provisional), 1 raw interval".
export function planSummary (plan: PlanPart[]): string {
  const stored = plan.filter((p) => p.source === 'stored')
  const provisional = stored.filter((p) => p.state === 'provisional').length
  const raw = plan.filter((p) => p.source === 'raw').length
  const missing = plan.filter((p) => p.source === 'unavailable').length
  const parts: string[] = []
  if (stored.length) {
    parts.push(`${stored.length} stored bucket${stored.length === 1 ? '' : 's'}` + (provisional ? ` (${provisional} provisional)` : ' (final)'))
  }
  if (raw) {
    parts.push(`${raw} interval${raw === 1 ? '' : 's'} computed from raw records`)
  }
  if (missing) {
    parts.push(`${missing} unavailable`)
  }
  return parts.join(', ') || 'no intervals'
}

export function statsStatusText (s: PopulationStats): string {
  switch (s.status) {
    case 'ok': return s.condition === 'ok' ? 'ok' : s.condition
    case 'timeout': return 'unavailable: statistics query timed out'
    case 'unavailable': return 'unavailable: the statistics source is unavailable'
    case 'outside_retention': return s.condition
    case 'exact_bounds_unavailable': return s.condition
  }
  return 'unavailable: ' + (s.error ?? s.condition)
}

export function sourceKindLabel (kind: string): string {
  switch (kind) {
    case 'span_status': return 'ERROR-status spans'
    case 'exception': return 'exception events'
    case 'log': return 'ERROR/FATAL logs'
  }
  return kind
}

// groupTitle names an error group by what it was grouped on.
export function groupTitle (g: ErrorGroup): string {
  if (g.method === 'exception') {
    return `${g.exceptionType} at ${g.frame || 'unknown frame'}`
  }
  return g.signature || '(no message recorded)'
}

export function groupLabelText (label: string): string {
  switch (label) {
    case 'recurring': return 'Recurring in both scopes'
    case 'newly_observed_in_candidate': return 'Newly observed in the candidate scope'
    case 'not_observed_in_candidate': return 'Not observed in the candidate scope'
    case 'undetermined': return 'Undetermined'
    case 'incomparable': return 'Not comparable'
  }
  return label
}

export function occurrencesText (n: number | null | undefined): string {
  return n === null || n === undefined ? UNAVAILABLE : String(n)
}

// slowThreshold is the displayed slow-example threshold of a row: the
// candidate p95 when it is available.
export function slowThreshold (row: { candidate: CallStats | null }): number | null {
  const p95 = row.candidate?.percentiles?.p95
  return p95 && p95.valueMs !== null ? p95.valueMs : null
}

// --- export

export const COMPARISON_FORMAT = 'troubleshooting-comparison'
export const COMPARISON_VERSION = 1

// IncludedReport is a combined report the user explicitly selected as a
// supporting example of a comparison, exactly as it was generated.
export interface IncludedReport {
  report: Report
  context: ReportContext
  includedAt: string
}

// exportComparisonJson is the stored analysis exactly as reviewed (never
// recomputed) with the selected reports embedded.
export function exportComparisonJson (analysis: Analysis, included: IncludedReport[], exportedAt: Date) {
  return {
    format: COMPARISON_FORMAT,
    version: COMPARISON_VERSION,
    exportedAt: (BigInt(exportedAt.getTime()) * BigInt(1000000)).toString(),
    statement: 'A reviewed comparison of two explicitly selected populations. Embedded reports are selected examples, not the whole population. ' + INTERPRETATION_SHORT,
    analysis,
    embeddedReports: included.map((r) => ({
      role: 'selected example',
      example: r.context.example,
      population: r.context.population,
      includedAt: r.includedAt,
      note: 'Generated at its own time; it may include evidence that arrived after the comparison.',
      report: reportJson(r.report, exportedAt)
    })),
    liveLinks: []
  }
}

// exportComparisonHtml is a self-contained, script-free page of the reviewed
// comparison and its embedded reports; every recorded value is escaped.
export function exportComparisonHtml (analysis: Analysis, included: IncludedReport[], exportedAt: Date): string {
  const e = escapeHtml
  const head = (cells: string[]) => '<tr>' + cells.map((c) => `<th>${e(c)}</th>`).join('') + '</tr>'
  const row = (cells: unknown[]) => '<tr>' + cells.map((c) => `<td>${e(c)}</td>`).join('') + '</tr>'
  const pop = (label: string, s: PopulationStats) =>
    row([label, s.label, windowText(String(s.population.time_from), String(s.population.time_to)), durationText(s.durationSeconds),
      statsStatusText(s), s.coverage.status, planSummary(s.plan), formatNs(s.cutoff)])
  const metricRows = analysis.rows.flatMap((r) => r.metrics.map((m) => row([
    `${r.operation} (${r.spanKind})`, metricLabel(m.metric), formatMetric(m.metric, m.baseline), formatMetric(m.metric, m.candidate),
    formatAbsolute(m), formatRelative(m), classificationLabel(m.classification), m.reasons.join('; ')
  ]))).join('')
  const samples = analysis.rows.map((r) => row([`${r.operation} (${r.spanKind})`,
    r.baseline ? `${r.baseline.calls} calls, ${r.baseline.validDurations} valid durations, ${r.baseline.invalidDurations} excluded` : 'no calls observed',
    r.candidate ? `${r.candidate.calls} calls, ${r.candidate.validDurations} valid durations, ${r.candidate.invalidDurations} excluded` : 'no calls observed',
    r.notes.join(' ')])).join('')
  const groupRows = analysis.errorGroups.map((g: GroupComparison) => row([
    sourceKindLabel(g.sourceKind), groupTitle(g), g.operation || 'not recorded', g.method === 'exception' ? 'exception type + frame' : 'normalized message',
    occurrencesText(g.baseline.occurrences), occurrencesText(g.candidate.occurrences), groupLabelText(g.label) + (g.reason ? ` (${g.reason})` : '')
  ])).join('')
  const notices = [...analysis.notices, ...analysis.baseline.notices.map((n) => ({ ...n, message: 'Baseline: ' + n.message })),
    ...analysis.candidate.notices.map((n) => ({ ...n, message: 'Candidate: ' + n.message }))]
  const embedded = included.map((r, i) => `<section class="embedded">
<p><b>Selected example ${i + 1} of ${included.length}</b> (not the whole population): ${e(r.context.example)} · ${e(r.context.population)}.
This report was generated at its own time and may include evidence that arrived after the comparison.</p>
${reportHtmlBody(r.report, exportedAt)}
</section>`).join('\n')
  const body = `<h1>Release comparison</h1>
<p>${e(analysis.title)}</p>
<p>Analysis revision <code>${e(analysis.id)}</code>${analysis.regenerates ? ` (regenerates <code>${e(analysis.regenerates)}</code>)` : ''} · generated at ${e(formatNs(analysis.generatedAt))} · exported at ${e(formatNs((BigInt(exportedAt.getTime()) * BigInt(1000000)).toString()))}. This file is a copy of the reviewed comparison; it does not query the service.</p>
<p><i>${e(analysis.interpretation)}</i></p>
<h2>Populations</h2>
<table>${head(['', 'population', 'window', 'duration', 'status', 'coverage', 'values from', 'analysis cutoff'])}${pop('Baseline', analysis.baseline)}${pop('Candidate', analysis.candidate)}</table>
${analysis.mismatches.length ? '<ul>' + analysis.mismatches.map((m) => `<li class="bad">Populations differ in ${e(m.dimension)}: baseline ${e(m.baseline)}, candidate ${e(m.candidate)}; conclusions for ${e(m.affects.join(', '))} are unavailable.</li>`).join('') + '</ul>' : ''}
<h2>Metrics</h2>
<table>${head(['operation', 'metric', 'baseline', 'candidate', 'absolute change', 'relative change', 'result', 'reasons'])}${metricRows}</table>
<table>${head(['operation', 'baseline sample', 'candidate sample', 'notes'])}${samples}</table>
<h2>Error groups</h2>
<p>Occurrences of different source kinds can describe the same failure; they are never added together.</p>
${groupRows ? `<table>${head(['source', 'group', 'operation', 'grouped by', 'baseline', 'candidate', 'label'])}${groupRows}</table>` : '<p>No error groups were recorded in either scope.</p>'}
<h2>Quality notices</h2>
${notices.length ? '<ul>' + notices.map((n) => `<li>${e(n.message)}</li>`).join('') + '</ul>' : '<p>None.</p>'}
<h2>Rules and definitions</h2>
<p>${e(analysis.rules.version)}: ${e(analysis.rules.description)}</p>
<table>${head(['contract', 'version'])}${Object.entries(analysis.contracts).map(([k, v]) => row([k, v])).join('')}</table>
<h2>Selected investigation reports (${included.length})</h2>
${included.length ? embedded : '<p>No combined report was included.</p>'}
<p>Live links: none. Every included report is embedded above and can be read offline.</p>`
  return htmlPage('Release comparison ' + analysis.id, body)
}

const hourMs = 60 * 60 * 1000

// defaultWindow is the last six whole UTC hours, so it maps to whole buckets.
export function defaultWindow (now = Date.now()): { timeFrom: string, timeTo: string } {
  const end = Math.floor(now / hourMs) * hourMs
  return { timeFrom: toNs(new Date(end - 6 * hourMs)), timeTo: toNs(new Date(end)) }
}
