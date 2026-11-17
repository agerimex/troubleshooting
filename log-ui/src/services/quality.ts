import { type ExecutionTarget, type ExecutionView } from '@/services/navigation'
import { ROOT_PARENT } from '@/services/spanTree'
import { type Span } from '@/types/filter'
import { type Report } from '@/types/report'
import { escapeHtml, htmlPage } from '@/services/report'
import { type CatalogueEntry, type CheckStatus, type Gate, type HistoryPoint, type QualityCheckResult, type QualityEvaluation, type QualityEvidence, type QualityRecord, type QualitySource, type SavedSnapshot } from '@/types/quality'

// Pure parts of the Quality page (docs/quality-gate.md): windows, labels,
// evidence targets, reading a CLI result file and the HTML export.

export const QUALITY_CONTRACT = 'quality-gate-v1'

export const EXAMPLE_RULES = `version: 1
entry:
  service: recording-api
  operation: start_session
  span_kind: server
checks:
  - id: startup-p95
    type: latency
    max_ms: 800
    target_ms: 500
  - id: db-calls
    type: db_calls
    max_per_execution: 3
  - id: no-directory
    type: forbidden_call
    to: directory-api
`

export const WINDOW_PRESETS: { value: string, label: string, minutes: number }[] = [
  { value: '15m', label: 'Last 15 minutes', minutes: 15 },
  { value: '1h', label: 'Last hour', minutes: 60 },
  { value: '6h', label: 'Last 6 hours', minutes: 360 },
  { value: '24h', label: 'Last 24 hours', minutes: 1440 }
]

// presetWindow is [now - preset, now) in Unix nanoseconds.
export function presetWindow (preset: string, nowMs: number): { from: string, to: string } {
  const p = WINDOW_PRESETS.find((w) => w.value === preset) ?? WINDOW_PRESETS[1]
  const to = BigInt(nowMs) * 1000000n
  return { from: (to - BigInt(p.minutes) * 60000000000n).toString(), to: to.toString() }
}

export const GATE_LABEL: Record<Gate, string> = { passed: 'Passed', failed: 'Failed', unresolved: 'Unresolved: not enough data' }
export const STATUS_LABEL: Record<CheckStatus, string> = { pass: 'Pass', fail: 'Fail', insufficient: 'No data' }

export function gateLabel (gate: string): string {
  return GATE_LABEL[gate as Gate] ?? gate
}

export function statusLabel (status: string): string {
  return STATUS_LABEL[status as CheckStatus] ?? status
}

// valueText shows a value in its unit: milliseconds with one decimal, counts
// as they are, "-" when there is none.
export function valueText (value: number | null | undefined, unit: string): string {
  if (value === null || value === undefined) {
    return '-'
  }
  if (unit === 'ms') {
    return String(Math.round(value * 10) / 10) + ' ms'
  }
  return String(value)
}

export function entryText (c: QualityCheckResult): string {
  return c.entry.service + ' / ' + c.entry.operation + (c.entry.span_kind ? ' (' + c.entry.span_kind + ')' : '')
}

export function limitText (c: QualityCheckResult): string {
  let s = valueText(c.limit, c.unit)
  if (c.target !== null && c.target !== undefined) {
    s += ' (target ' + valueText(c.target, c.unit) + (c.target_status ? ', ' + c.target_status : '') + ')'
  }
  return s
}

export function baselineText (c: QualityCheckResult): string {
  if (!c.baseline) {
    return ''
  }
  return c.baseline.status === 'insufficient' ? 'no data' : valueText(c.baseline.value, c.unit)
}

// evidenceView: calls between services open as a sequence, the rest as a
// timeline.
export function evidenceView (c: QualityCheckResult): ExecutionView {
  return c.type === 'forbidden_call' ? 'sequence' : 'timeline'
}

const traceIdPattern = /^[0-9a-f]{32}$/
const spanIdPattern = /^[0-9a-f]{16}$/
const nsPattern = /^-?\d{1,20}$/

// evidenceTarget opens one piece of evidence in the execution viewer; null
// for evidence without valid identifiers (a hand-edited result file).
export function evidenceTarget (c: QualityCheckResult, e: QualityEvidence, view: ExecutionView = evidenceView(c)): ExecutionTarget | null {
  if (!traceIdPattern.test(e.trace_id) || !nsPattern.test(e.time_from) || !nsPattern.test(e.time_to) || BigInt(e.time_to) <= BigInt(e.time_from)) {
    return null
  }
  const t: ExecutionTarget = { traceId: e.trace_id, window: { from: e.time_from, to: e.time_to }, view, origin: { page: 'quality', label: 'the quality check ' + c.id } }
  if (spanIdPattern.test(e.span_id)) {
    t.spanId = e.span_id
  }
  if (savedSpans(e.snapshot, e.trace_id).length > 0) {
    t.snapshot = e.snapshot
  }
  return t
}

// parseResult reads a result file (the CLI's --json, or this page's JSON
// export). It never throws; anything else than a quality-gate-v1 evaluation
// is an error.
export function parseResult (text: string): [string | null, QualityEvaluation | null] {
  let data: any
  try {
    data = JSON.parse(text)
  } catch {
    return ['The file is not JSON.', null]
  }
  if (data && typeof data === 'object' && data.data?.Evaluation) {
    data = data.data.Evaluation
  }
  if (!data || typeof data !== 'object' || data.contract !== QUALITY_CONTRACT || !Array.isArray(data.checks) || typeof data.gate !== 'string') {
    return ['The file is not a ' + QUALITY_CONTRACT + ' evaluation (troubleshooting-check --json).', null]
  }
  for (const c of data.checks) {
    if (!c || typeof c.id !== 'string' || typeof c.status !== 'string' || !c.entry || typeof c.entry.service !== 'string') {
      return ['The evaluation has an invalid check.', null]
    }
    c.details = Array.isArray(c.details) ? c.details.filter((d: unknown) => typeof d === 'string') : []
    c.evidence = Array.isArray(c.evidence) ? c.evidence.filter((e: any) => e && typeof e.trace_id === 'string') : []
  }
  // Results from before requirements have none.
  data.requirements = Array.isArray(data.requirements)
    ? data.requirements.filter((r: any) => r && typeof r.id === 'string' && typeof r.status === 'string').map((r: any) => ({
      ...r, title: typeof r.title === 'string' ? r.title : '', summary: typeof r.summary === 'string' ? r.summary : '',
      checks: Array.isArray(r.checks) ? r.checks.filter((x: unknown) => typeof x === 'string') : []
    }))
    : []
  return [null, data as QualityEvaluation]
}

// --- Requirements and history (stage 5)

export const REQUIREMENT_LABEL: Record<string, string> = {
  met: 'Met', violated: 'Violated', insufficient: 'Not enough data', not_evaluated: 'Not evaluated', absent: 'Not in the rules'
}

export function requirementLabel (status: string): string {
  return REQUIREMENT_LABEL[status] ?? status
}

// requirementCounts: "1 violated, 2 met, 1 not evaluated", worst first.
export function requirementCounts (statuses: { status: string }[]): string {
  const n = new Map<string, number>()
  for (const s of statuses) {
    n.set(s.status, (n.get(s.status) ?? 0) + 1)
  }
  const order = ['violated', 'insufficient', 'met', 'not_evaluated']
  const keys = [...order.filter((k) => n.has(k)), ...[...n.keys()].filter((k) => !order.includes(k)).sort()]
  return keys.map((k) => n.get(k) + ' ' + requirementLabel(k).toLowerCase()).join(', ')
}

// streakText says since when a requirement's status holds, e.g. "Violated
// since commit 0123456789ab (2 evaluations); met before."
export function streakText (e: CatalogueEntry): string {
  const s = e.streak
  if (!s || !s.since || s.evaluations < 1) {
    return ''
  }
  const label = requirementLabel(e.status)
  const n = s.evaluations === 1 ? 'this evaluation only' : s.evaluations + ' evaluations'
  if (s.at_least) {
    return label + ' in all ' + (s.evaluations === 1 ? 'one recorded evaluation' : s.evaluations + ' recorded evaluations') + ' read.'
  }
  const at = s.since.commit ? 'commit ' + short(s.since.commit) : nsLabel(s.since.recorded_at)
  return label + ' since ' + at + ' (' + n + ')' + (s.status_before ? '; ' + requirementLabel(s.status_before).toLowerCase() + ' before.' : '.')
}

const nsLabel = (iso: string) => (iso ? iso.replace('T', ' ').replace(/\.\d+Z$|Z$/, ' UTC') : '')

// recordedText shows a recorded time as "2026-11-17 12:05:05 UTC".
export function recordedText (iso: string | undefined): string {
  return nsLabel(iso ?? '')
}

// historyCells: the history oldest first (left to right), each with a label.
export function historyCells (history: HistoryPoint[]): (HistoryPoint & { label: string })[] {
  return [...history].reverse().map((h) => ({
    ...h,
    label: requirementLabel(h.status) + ' · ' + (h.commit ? 'commit ' + short(h.commit) + (h.ref ? ' (' + h.ref + ')' : '') + ' · ' : '') + nsLabel(h.recorded_at)
  }))
}

// rulesChanged: the rules (sha256) differ from the next older evaluation's.
export function rulesChanged (history: HistoryPoint[], i: number): boolean {
  return i + 1 < history.length && !!history[i].rules_sha256 && !!history[i + 1].rules_sha256 && history[i].rules_sha256 !== history[i + 1].rules_sha256
}

const evaluationIdPattern = /^[0-9a-f]{24}$/

export function validEvaluationId (id: string): boolean {
  return evaluationIdPattern.test(id)
}

// evaluationHash is the link of a recorded evaluation.
export function evaluationHash (id: string): string {
  return '#/quality?evaluation=' + id
}

// parseRecord checks a recorded evaluation as the API returns it.
export function parseRecord (data: any): [string | null, QualityRecord | null] {
  if (!data || typeof data !== 'object') {
    return ['The recorded evaluation is missing.', null]
  }
  const [err, ev] = parseResult(JSON.stringify(data.evaluation ?? null))
  if (err || !ev) {
    return [err, null]
  }
  return [null, { evaluation: ev, rules: typeof data.rules === 'string' ? data.rules : '' }]
}

export function nsToIso (ns: string | undefined): string {
  if (!ns || !nsPattern.test(ns)) {
    return ''
  }
  return new Date(Number(BigInt(ns) / 1000000n)).toISOString()
}

// exportHtml is a self-contained page of the evaluation as shown: no
// scripts, every recorded value escaped.
export function exportHtml (ev: QualityEvaluation): string {
  const e = escapeHtml
  const hasBaseline = !!ev.baseline
  const rows = ev.checks.map((c) => {
    const items = [...c.details.map((d) => `<li>${e(d)}</li>`),
      ...c.evidence.map((x) => `<li>Evidence: ${e(x.note)} (trace <code>${e(x.trace_id)}</code>, span <code>${e(x.span_id)}</code>)</li>`)]
    return `<tr><td>${e(statusLabel(c.status))}</td><td><b>${e(c.id)}</b>${c.advisory ? ' (advisory)' : ''}<br>${e(entryText(c))}</td>` +
      `<td>${e(valueText(c.value, c.unit))}</td><td>${e(limitText(c))}</td>${hasBaseline ? `<td>${e(baselineText(c))}</td>` : ''}` +
      `<td>${e(c.executions)}</td><td>${e(c.summary)}${items.length ? '<ul>' + items.join('') + '</ul>' : ''}</td></tr>`
  }).join('')
  const reqs = ev.requirements ?? []
  const reqTable = reqs.length
    ? `\n<h2>Requirements: ${e(requirementCounts(reqs))}</h2>\n<table><thead><tr><th>Status</th><th>Requirement</th><th>Owner</th><th>Why</th><th>Checks</th></tr></thead><tbody>${
      reqs.map((r) => `<tr><td>${e(requirementLabel(r.status))}</td><td><b>${e(r.id)}</b><br>${e(r.title)}</td><td>${e(r.owner ?? '')}</td><td>${e(r.why ?? '')}</td><td>${e(r.checks.join(', '))}<br>${e(r.summary)}</td></tr>`).join('')
    }</tbody></table>\n<h2>Checks</h2>`
    : ''
  const recorded = ev.id ? `\n<p>Recorded evaluation <code>${e(ev.id)}</code>${ev.project ? ` of project <code>${e(ev.project)}</code>` : ''}, ${e(recordedText(ev.recorded_at))}.</p>` : ''
  const body = `<h1>Quality gate: ${e(gateLabel(ev.gate))}</h1>
<p>Run <code>${e(ev.run)}</code>, ${e(nsToIso(ev.time_from))} to ${e(nsToIso(ev.time_to))}${hasBaseline ? `; baseline <code>${e(ev.baseline)}</code>, ${e(nsToIso(ev.baseline_time_from))} to ${e(nsToIso(ev.baseline_time_to))}` : ''}. ${e(ev.contract)}, evaluated ${e(ev.evaluated_at)}.</p>${sourceText(ev.source) ? `\n<p>${e(sourceText(ev.source))}</p>` : ''}${recorded}${reqTable}
<table><thead><tr><th>Status</th><th>Check</th><th>Value</th><th>Limit</th>${hasBaseline ? '<th>Baseline</th>' : ''}<th>Executions</th><th>Result</th></tr></thead><tbody>${rows}</tbody></table>
<p>${e(ev.method)}. Statuses describe this run only: a pass means "not observed in this run", not a guarantee; a missing measurement is never a pass.</p>`
  return htmlPage('Quality gate: ' + gateLabel(ev.gate), body)
}

export function exportJson (ev: QualityEvaluation): string {
  return JSON.stringify(ev, null, 2) + '\n'
}

const short = (s: string) => s.slice(0, 12)

// sourceText says what was tested and which rules decided (the CLI's
// "source", absent for a check made on this page), e.g.
// "commit 0123456789ab (feat/x) · rules main@89ab01234567:quality.yaml, sha256 d80ff3781a3d".
export function sourceText (s: QualitySource | undefined | null): string {
  if (!s || typeof s !== 'object') {
    return ''
  }
  const str = (v: unknown) => (typeof v === 'string' ? v : '')
  const parts: string[] = []
  const commit = str(s.commit)
  const ref = str(s.ref)
  if (commit) {
    const head = str(s.pr_head)
    parts.push('commit ' + short(commit) + (ref ? ' (' + ref + ')' : '') + (head ? ', merging head ' + short(head) : ''))
  } else if (ref) {
    parts.push('ref ' + ref)
  }
  const rules = str(s.rules_ref) || str(s.rules)
  if (rules) {
    parts.push('rules ' + rules + (str(s.rules_sha256) ? ', sha256 ' + short(str(s.rules_sha256)) : ''))
  }
  return parts.join(' · ')
}

// ciRunLink is the CI run's link, only if it is http(s).
export function ciRunLink (s: QualitySource | undefined | null): string {
  const u = s && typeof s.ci_run === 'string' ? s.ci_run : ''
  return /^https?:\/\//i.test(u) ? u : ''
}

const STATUS_CODE: Record<string, number> = { unset: 0, error: 1, ok: 2 }

// savedSpans are a snapshot's spans as Spans of the trace, leaving out any
// that a hand-edited file made invalid.
export function savedSpans (snap: SavedSnapshot | undefined | null, traceId: string): Span[] {
  if (!snap || !Array.isArray(snap.spans)) {
    return []
  }
  const str = (v: unknown) => (typeof v === 'string' ? v : '')
  return snap.spans.filter((s) => s && spanIdPattern.test(str(s.span_id)) && nsPattern.test(str(s.start))).map((s) => {
    const status = str(s.status) || 'unset'
    const peer = str(s.peer)
    return {
      timeStamp: s.start,
      name: str(s.name),
      spanKind: str(s.kind),
      service: str(s.service),
      traceId,
      spanId: s.span_id,
      parentSpanId: spanIdPattern.test(str(s.parent_id)) ? s.parent_id : ROOT_PARENT,
      msg: '',
      // The snapshot keeps the CLIENT's peer (peer.service or db.system):
      // the sequence draws the call to it.
      tags: peer ? [{ key: 'peer.service', value: peer }] : [],
      serviceTags: [],
      childSpanCount: 0,
      statusCode: STATUS_CODE[status] ?? 0,
      status,
      statusMessage: '',
      duration: typeof s.duration_us === 'number' ? s.duration_us : -1,
      events: [],
      links: []
    }
  })
}

export const SNAPSHOT_LOGS_TEXT = 'Logs are not kept in a saved snapshot.'

// snapshotReport is a report of a saved snapshot: its spans, and a logs
// section that says logs were not kept (never "no logs").
export function snapshotReport (traceId: string, window: { from: string, to: string }, snap: SavedSnapshot, generatedAt: string): Report {
  const spans = savedSpans(snap, traceId)
  return {
    traceId,
    timeFrom: window.from,
    timeTo: window.to,
    generatedAt,
    spans: {
      status: 'ok', records: spans, limit: spans.length, shown: spans.length, truncated: !!snap.truncated,
      total: snap.truncated ? null : spans.length, totalStatus: snap.truncated ? 'unavailable' : 'ok', retrievedAt: generatedAt
    },
    logs: {
      status: 'unavailable', error: SNAPSHOT_LOGS_TEXT, records: [], limit: 0, shown: 0, truncated: false,
      total: null, totalStatus: 'unavailable', retrievedAt: generatedAt
    },
    warnings: []
  }
}
