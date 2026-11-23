import { spanOrder } from '@/services/report'
import { spanInterval, traceStart } from '@/services/problems'
import { type Span } from '@/types/filter'

// Comparison of two executions (two traces) on one time scale. Spans are
// matched by their call path: the service, kind and name of the span and of
// each ancestor up to the entry. Calls with the same path are one row, with
// the number of calls on each side: a call made once in one execution and
// twenty times in the other is one row, 1 against 20. It shows differences
// in the recorded calls; it does not say which of them caused a difference
// in duration.

export interface CompareSegment {
  startMs: number // from the start of its own trace
  endMs: number | null // null: no valid end
  error: boolean
}

export interface CompareSide {
  count: number
  segments: CompareSegment[]
}

export type CompareChange = 'same' | 'more' | 'fewer' | 'new' | 'missing'

export interface CompareRow {
  path: string
  depth: number
  name: string
  service: string
  spanKind: string
  reference: CompareSide // the execution compared with (e.g. the baseline)
  current: CompareSide // the execution being investigated
  change: CompareChange
}

export interface ExecutionComparison {
  rows: CompareRow[]
  axisMs: number // one scale for both: the longer execution
  referenceMs: number // first start to last end of each execution
  currentMs: number
  differentEntry: boolean // the entry spans differ (service, kind or name)
}

const step = (s: Span) => [s.service, s.spanKind ?? '', s.name].join('|')

interface Placed { path: string, depth: number, span: Span }

// placed lists the spans of one execution in parent order with their paths.
function placed (spans: Span[]): Placed[] {
  const pathOf = new Map<string, string>()
  return spanOrder(spans).map((o) => {
    const parent = o.parentMissing ? '?' : pathOf.get(o.span.parentSpanId) ?? ''
    const path = parent + '/' + step(o.span)
    pathOf.set(o.span.spanId, path)
    return { path, depth: o.depth, span: o.span }
  })
}

function totalMs (spans: Span[], origin: bigint | null): number {
  if (origin === null) {
    return 0
  }
  return Math.max(0, ...spans.map((s) => {
    const iv = spanInterval(s, origin)
    return iv.endMs ?? iv.startMs
  }))
}

function changeOf (a: number, b: number): CompareChange {
  if (a === 0) {
    return 'new'
  }
  if (b === 0) {
    return 'missing'
  }
  return a === b ? 'same' : b > a ? 'more' : 'fewer'
}

// compareExecutions puts two executions side by side. Rows follow the
// current execution's order; calls only the reference made are inserted
// after the subtree of their parent's row.
export function compareExecutions (reference: Span[], current: Span[]): ExecutionComparison {
  const refOrigin = traceStart(reference)
  const curOrigin = traceStart(current)
  const rows: CompareRow[] = []
  const byPath = new Map<string, CompareRow>()
  const add = (p: Placed, origin: bigint, side: 'reference' | 'current', at?: number) => {
    let row = byPath.get(p.path)
    if (!row) {
      row = {
        path: p.path, depth: p.depth, name: p.span.name, service: p.span.service, spanKind: p.span.spanKind ?? '',
        reference: { count: 0, segments: [] }, current: { count: 0, segments: [] }, change: 'same'
      }
      byPath.set(p.path, row)
      if (at === undefined) {
        rows.push(row)
      } else {
        rows.splice(at, 0, row)
      }
    }
    const iv = spanInterval(p.span, origin)
    row[side].count++
    row[side].segments.push({ startMs: iv.startMs, endMs: iv.endMs, error: p.span.status === 'error' })
  }
  if (curOrigin !== null) {
    for (const p of placed(current)) {
      add(p, curOrigin, 'current')
    }
  }
  if (refOrigin !== null) {
    for (const p of placed(reference)) {
      if (byPath.has(p.path)) {
        add(p, refOrigin, 'reference')
        continue
      }
      // After the last row of the parent's subtree, else at the end.
      const parent = p.path.slice(0, p.path.lastIndexOf('/'))
      let at = rows.length
      const pi = rows.findIndex((r) => r.path === parent)
      if (pi >= 0) {
        at = pi + 1
        while (at < rows.length && rows[at].path.startsWith(parent + '/')) {
          at++
        }
      }
      add(p, refOrigin, 'reference', at)
    }
  }
  for (const r of rows) {
    r.change = changeOf(r.reference.count, r.current.count)
  }
  const referenceMs = totalMs(reference, refOrigin)
  const currentMs = totalMs(current, curOrigin)
  const entry = (spans: Span[]) => placed(spans)[0]?.path ?? ''
  return {
    rows,
    axisMs: Math.max(1, referenceMs, currentMs),
    referenceMs,
    currentMs,
    differentEntry: reference.length > 0 && current.length > 0 && entry(reference) !== entry(current)
  }
}

// differences says in words which calls differ, most calls first.
export function differences (c: ExecutionComparison): string[] {
  return c.rows
    .filter((r) => r.change !== 'same')
    .sort((x, y) => Math.abs(y.current.count - y.reference.count) - Math.abs(x.current.count - x.reference.count))
    .map((r) => {
      switch (r.change) {
      case 'new': return `${r.name}: ${r.current.count === 1 ? 'a new call' : r.current.count + ' new calls'}`
      case 'missing': return `${r.name}: not called (${r.reference.count} before)`
      default: return `${r.name}: ${r.reference.count} → ${r.current.count} ${r.current.count === 1 ? 'call' : 'calls'}`
      }
    })
}
