import { ROOT_PARENT } from '@/services/spanTree'
import { association, sortedLogs, spanOrder, type Association } from '@/services/report'
import { type Span } from '@/types/filter'
import { type LogRecord, type Report } from '@/types/report'
import { type DetectionRules, type FieldError, type Problem } from '@/types/problems'
import { type Example } from '@/types/stats'

// Pure logic of the problem workspace: headline measures, the trace graph and
// timeline of one loaded report, task lanes and log scopes. Nothing here
// queries; switching a view uses the report already loaded.

// --- span identity and time

// spanKey is a span's identity: span ids repeat across traces.
export function spanKey (s: { traceId: string, spanId: string }): string {
  return s.traceId + ':' + s.spanId
}

// Relative times are computed on BigInt nanoseconds, then turned into
// milliseconds: epoch nanoseconds don't fit a JS number.
export function offsetMs (ns: string, originNs: bigint): number {
  return Number(BigInt(ns) - originNs) / 1e6
}

// Span duration is recorded in microseconds; negative means invalid (no end,
// or an end before the start) and is never shown as an interval.
export function validDuration (s: Span): boolean {
  return s.duration >= 0
}

export function traceStart (spans: Span[]): bigint | null {
  let start: bigint | null = null
  for (const s of spans) {
    const t = BigInt(s.timeStamp)
    if (start === null || t < start) {
      start = t
    }
  }
  return start
}

export interface Interval {
  startMs: number
  endMs: number | null // null: no valid end
}

export function spanInterval (s: Span, origin: bigint): Interval {
  const startMs = offsetMs(s.timeStamp, origin)
  return { startMs, endMs: validDuration(s) ? startMs + s.duration / 1000 : null }
}

// --- problems list

export function problemTypeLabel (p: Problem): string {
  switch (p.type) {
    case 'error': return 'Error'
    case 'slow_calls': return 'Slow calls'
    case 'release_regression': return 'Release regression'
  }
  return p.type
}

export function sourceKindShort (kind: string): string {
  switch (kind) {
    case 'span_status': return 'ERROR spans'
    case 'exception': return 'exception events'
    case 'log': return 'ERROR logs'
  }
  return kind
}

const n = (v: number) => v.toLocaleString('en-US')

export interface Headline {
  value: string
  label: string
  detail: string
}

// headline is the row's impact measure, named for what it counts: affected
// calls when they can be attributed, matching records otherwise, and the
// metric change (never a failed-call count) for a regression.
export function headline (p: Problem): Headline {
  if (p.error) {
    const g = p.error
    const records = `${n(g.occurrences)} matching ${sourceKindShort(g.sourceKind)}`
    if (g.affectedCalls != null && g.affectedCalls > 0) {
      return { value: n(g.affectedCalls), label: 'Affected calls', detail: records + ` · ${n(g.affectedTraces)} traces` }
    }
    return { value: n(g.occurrences), label: 'Matching records', detail: 'Call attribution unavailable' }
  }
  if (p.slow) {
    return { value: n(p.slow.calls), label: 'Slow calls', detail: `over ${n(p.slow.thresholdMs)} ms` }
  }
  if (p.regression) {
    const m = p.regression.metric
    const rel = m.relative != null ? `${m.relative >= 0 ? '+' : ''}${m.relative.toFixed(0)}%` : `${(m.absolute ?? 0) >= 0 ? '+' : ''}${(m.absolute ?? 0).toFixed(0)} ms`
    return {
      value: `p95 ${rel}`,
      label: `${ms(m.baseline)} → ${ms(m.candidate)}`,
      detail: `${n(p.regression.baselineCalls)} / ${n(p.regression.candidateCalls)} calls evaluated`
    }
  }
  return { value: '', label: '', detail: '' }
}

export function ms (v: number | null | undefined): string {
  if (v == null) {
    return 'unavailable'
  }
  return (Math.abs(v) >= 100 ? v.toFixed(0) : v.toFixed(1)) + ' ms'
}

export function problemContext (p: Problem): string {
  const parts = [p.operation || 'operation not recorded']
  if (p.error) {
    parts.push(p.error.exceptionType || sourceKindShort(p.error.sourceKind))
  }
  if (p.regression) {
    parts.push(`${version(p.regression.baseline.version)} → ${version(p.regression.candidate.version)}`)
  }
  if (p.environment) {
    parts.push(p.environment)
  }
  return parts.join(' · ')
}

export function version (v: string | null | undefined): string {
  if (v == null) {
    return 'any version'
  }
  return v === '' ? 'version not recorded' : 'v' + v
}

// --- examples

export function exampleLabel (e: Example): string {
  const kind = e.sourceKind === 'log' ? (e.traceId ? 'ERROR log' : 'log without TraceId')
    : e.sourceKind === 'exception' ? 'exception' : e.sourceKind === 'span_status' ? 'ERROR span' : 'slow call'
  const parts = [e.operation || 'operation not recorded']
  if (e.durationMs != null) {
    parts.push(ms(e.durationMs))
  }
  parts.push(kind, version(e.version))
  return parts.join(' · ')
}

export function exampleKey (e: Example): string {
  return [e.traceId, e.spanId, e.recordId ?? '', e.eventIndex ?? 0, e.time].join(':')
}

// --- graph

export interface GraphNode {
  key: string
  span: Span
  depth: number
  column: number
  detached: boolean // its recorded parent is not in the report
}

export interface GraphEdge {
  from: string
  to: string
}

export interface GraphLayout {
  nodes: GraphNode[]
  edges: GraphEdge[]
  columns: number
  depth: number
  hidden: number // spans not drawn because of the render limit (still in the report)
}

// graphLayout places spans as a tree: one row per depth, each subtree in its
// own columns. Edges are recorded parent → child relationships only; a span
// whose parent is missing is drawn detached, without an invented edge.
export function graphLayout (spans: Span[], limit: number): GraphLayout {
  const ordered = spanOrder(spans)
  const shown = ordered.slice(0, limit)
  const keys = new Set(shown.map((o) => spanKey(o.span)))
  const children = new Map<string, string[]>()
  for (const o of shown) {
    const parent = o.span.traceId + ':' + o.span.parentSpanId
    if (o.span.parentSpanId !== ROOT_PARENT && keys.has(parent) && !o.parentMissing) {
      children.set(parent, [...(children.get(parent) ?? []), spanKey(o.span)])
    }
  }
  const byKey = new Map(shown.map((o) => [spanKey(o.span), o]))
  const column = new Map<string, number>()
  let next = 0
  // Leaves take the next column; a parent sits over its first child.
  const place = (key: string): number => {
    const kids = children.get(key) ?? []
    if (kids.length === 0) {
      column.set(key, next)
      return next++
    }
    const cols = kids.map(place)
    column.set(key, cols[0])
    return cols[0]
  }
  const nodes: GraphNode[] = []
  const edges: GraphEdge[] = []
  for (const o of shown) {
    const key = spanKey(o.span)
    const isChild = [...children.values()].some((list) => list.includes(key))
    if (!isChild) {
      place(key)
    }
  }
  for (const o of shown) {
    const key = spanKey(o.span)
    nodes.push({ key, span: o.span, depth: o.depth, column: column.get(key) ?? 0, detached: o.parentMissing })
    for (const c of children.get(key) ?? []) {
      edges.push({ from: key, to: c })
    }
  }
  return {
    nodes,
    edges,
    columns: Math.max(1, next),
    depth: Math.max(0, ...nodes.map((x) => x.depth)) + 1,
    hidden: ordered.length - shown.length
  }
}

export interface Overlap {
  a: Span
  b: Span
  fromMs: number
  toMs: number
  overlapMs: number
}

// siblingOverlaps are pairs of children of the same parent whose recorded
// intervals overlap: max(0, min(endA, endB) − max(startA, startB)). An
// overlap means both were in flight at once; it isn't a dependency.
export function siblingOverlaps (spans: Span[]): Overlap[] {
  const origin = traceStart(spans)
  if (origin === null) {
    return []
  }
  const ids = new Set(spans.map(spanKey))
  const byParent = new Map<string, Span[]>()
  for (const s of spans) {
    const parent = s.traceId + ':' + s.parentSpanId
    if (s.parentSpanId === ROOT_PARENT || !ids.has(parent)) {
      continue
    }
    byParent.set(parent, [...(byParent.get(parent) ?? []), s])
  }
  const out: Overlap[] = []
  for (const kids of byParent.values()) {
    for (let i = 0; i < kids.length; i++) {
      for (let j = i + 1; j < kids.length; j++) {
        const a = spanInterval(kids[i], origin)
        const b = spanInterval(kids[j], origin)
        if (a.endMs === null || b.endMs === null) {
          continue // timing unknown: no overlap is claimed
        }
        const from = Math.max(a.startMs, b.startMs)
        const to = Math.min(a.endMs, b.endMs)
        if (to > from) {
          out.push({ a: kids[i], b: kids[j], fromMs: from, toMs: to, overlapMs: to - from })
        }
      }
    }
  }
  return out.sort((x, y) => y.overlapMs - x.overlapMs)
}

// --- timeline

export interface TimelineRow {
  key: string
  span: Span
  depth: number
  detached: boolean
  startMs: number
  endMs: number | null
}

export interface Timeline {
  rows: TimelineRow[]
  totalMs: number // the axis: trace start to the latest recorded end
}

// timeline lists spans in parent order on one axis relative to the trace
// start. Bars are recorded intervals; nothing is stacked or summed.
export function timeline (spans: Span[]): Timeline {
  const origin = traceStart(spans)
  if (origin === null) {
    return { rows: [], totalMs: 0 }
  }
  const rows = spanOrder(spans).map((o) => {
    const iv = spanInterval(o.span, origin)
    return { key: spanKey(o.span), span: o.span, depth: o.depth, detached: o.parentMissing, startMs: iv.startMs, endMs: iv.endMs }
  })
  const totalMs = Math.max(1, ...rows.map((r) => r.endMs ?? r.startMs))
  return { rows, totalMs }
}

// --- logs

export const TASK_ATTRIBUTE = 'task.id'
export const TASK_NOT_RECORDED = 'Task not recorded'

export interface LanePoint {
  log: LogRecord
  offsetMs: number
}

export interface Lane {
  task: string
  points: LanePoint[]
}

// taskLanes groups one span's logs by their recorded task.id (an
// application-defined attribute). Each log is a point at its recorded time;
// no task duration is inferred.
export function taskLanes (logs: LogRecord[], origin: bigint): Lane[] {
  const lanes = new Map<string, LanePoint[]>()
  for (const log of sortedLogs(logs)) {
    const task = log.attributes?.[TASK_ATTRIBUTE] || TASK_NOT_RECORDED
    lanes.set(task, [...(lanes.get(task) ?? []), { log, offsetMs: offsetMs(log.timeUnixNano, origin) }])
  }
  return [...lanes.entries()]
    .sort(([a], [b]) => (a === TASK_NOT_RECORDED ? 1 : b === TASK_NOT_RECORDED ? -1 : a.localeCompare(b)))
    .map(([task, points]) => ({ task, points }))
}

export type LogScope = 'span' | 'trace'

export interface ScopedLog {
  log: LogRecord
  association: Association
}

// scopedLogs: the selected span's logs (by its recorded identity), or every
// log of the generated report with its association, including logs without a
// SpanId and logs that reference a span the report doesn't have.
export function scopedLogs (report: Report, selected: Span | null, scope: LogScope): ScopedLog[] {
  const all = sortedLogs(report.logs.records)
  const logs = scope === 'span' && selected
    ? all.filter((l) => l.traceId === selected.traceId && l.spanId === selected.spanId)
    : scope === 'span' ? [] : all
  return logs.map((log) => ({ log, association: association(log, report) }))
}

export function isErrorSeverity (severity: string): boolean {
  return severity === 'ERROR' || severity === 'FATAL'
}

export interface SpanLogCounts {
  logs: number
  errors: number
}

// spanLogCounts counts each span's logs and ERROR/FATAL logs, by identity.
export function spanLogCounts (report: Report): Map<string, SpanLogCounts> {
  const out = new Map<string, SpanLogCounts>()
  for (const l of report.logs.records) {
    if (!l.spanId) {
      continue
    }
    const key = l.traceId + ':' + l.spanId
    const c = out.get(key) ?? { logs: 0, errors: 0 }
    c.logs++
    if (isErrorSeverity(l.severity)) {
      c.errors++
    }
    out.set(key, c)
  }
  return out
}

// initialSelection: the example's own span when the report has it; else the
// previous selection if that exact identity is in the new report; else none
// (trace-level logs).
export function initialSelection (report: Report, entrySpanId: string, previous: string | null): string | null {
  const keys = new Set(report.spans.records.map(spanKey))
  const entry = report.traceId + ':' + entrySpanId
  if (entrySpanId && keys.has(entry)) {
    return entry
  }
  if (previous && keys.has(previous)) {
    return previous
  }
  return null
}

// --- detection rules

// validateRules mirrors the server's validation (rules.go) for immediate
// feedback; the server's answer is authoritative. Disabled templates keep
// their values unchecked.
export function validateRules (r: DetectionRules): FieldError[] {
  const out: FieldError[] = []
  const whole = (v: number, lo: number, hi: number) => Number.isInteger(v) && v >= lo && v <= hi
  const positive = (v: number, hi: number) => typeof v === 'number' && v > 0 && v <= hi
  if (r.errors.enabled && !whole(r.errors.minAffectedCalls, 1, 1_000_000)) {
    out.push({ field: 'errors.minAffectedCalls', message: 'Enter a whole number of calls from 1 to 1,000,000.' })
  }
  if (r.slow.enabled) {
    if (!positive(r.slow.durationMs, 3_600_000)) {
      out.push({ field: 'slow.durationMs', message: 'Enter a duration above 0 ms and at most one hour.' })
    }
    if (!whole(r.slow.minCalls, 1, 1_000_000)) {
      out.push({ field: 'slow.minCalls', message: 'Enter a whole number of calls from 1 to 1,000,000.' })
    }
  }
  if (r.regressions.enabled) {
    if (!positive(r.regressions.p95RelativePct, 10_000)) {
      out.push({ field: 'regressions.p95RelativePct', message: 'Enter a percentage above 0.' })
    }
    if (!positive(r.regressions.p95AbsoluteMs, 3_600_000)) {
      out.push({ field: 'regressions.p95AbsoluteMs', message: 'Enter a duration above 0 ms.' })
    }
  }
  return out
}
