import { ROOT_PARENT } from '@/services/spanTree'
import { type Span } from '@/types/filter'
import { type ExecutionTarget } from '@/services/navigation'
import { type Shape, type ShapeExampleKind } from '@/types/shapes'

// Pure helpers of the sequence library (shapes-v1).

export const LIBRARY_PAGE_SIZE = 10

export const EXAMPLE_KINDS: ShapeExampleKind[] = ['p50', 'p95', 'p99', 'slowest']

// Spans of an execution are read from its root start to this much later
// (the server reads 15 minutes after an hour for shapes; the viewer can widen).
const exampleSpanMs = 60 * 60 * 1000
const exampleLeadMs = 60 * 1000

// pageOf is one page of shapes (0-based).
export function pageOf (shapes: Shape[], page: number, size = LIBRARY_PAGE_SIZE): Shape[] {
  return shapes.slice(page * size, (page + 1) * size)
}

export function pageCount (total: number, size = LIBRARY_PAGE_SIZE): number {
  return Math.max(1, Math.ceil(total / size))
}

// failedShare is the failed executions as a percentage text.
export function failedShare (s: Shape): string {
  if (s.executions === 0) {
    return '—'
  }
  const pct = (100 * s.failedExecutions) / s.executions
  if (pct === 0) {
    return '0%'
  }
  return pct < 0.1 ? '<0.1%' : pct < 10 ? pct.toFixed(1) + '%' : pct.toFixed(0) + '%'
}

// fmtDuration formats milliseconds for the table.
export function fmtDuration (ms: number | null | undefined): string {
  if (ms === null || ms === undefined) {
    return '—'
  }
  if (ms >= 10_000) {
    return (ms / 1000).toFixed(1) + ' s'
  }
  return (ms >= 10 ? ms.toFixed(0) : ms.toFixed(1)) + ' ms'
}

// percentileHint explains a missing percentile (the comparison-v1 minimums).
export function percentileHint (s: Shape, p: 'p50' | 'p95' | 'p99'): string {
  const need = { p50: 20, p95: 200, p99: 1000 }[p]
  if (s.percentiles[p] !== null) {
    return ''
  }
  return `${p} needs at least ${need} executions with a valid duration (this shape has ${s.executions - s.invalidDurations})`
}

// exampleLabel names a representative execution.
export function exampleLabel (kind: ShapeExampleKind): string {
  return kind === 'slowest' ? 'slowest' : kind
}

// exampleTarget opens the representative execution of a shape in the viewer
// as a sequence, with the library as its origin.
export function exampleTarget (s: Shape, kind: ShapeExampleKind): ExecutionTarget | null {
  const ex = s.examples[kind]
  if (!ex) {
    return null
  }
  const start = BigInt(ex.time)
  const ms = 1_000_000n
  const label = `the sequence library (${s.entryOperation} · ${exampleLabel(kind)} · ${fmtDuration(ex.durationUs / 1000)})`
  return {
    traceId: ex.traceId,
    window: { from: String(start - BigInt(exampleLeadMs) * ms), to: String(start + BigInt(exampleSpanMs) * ms) },
    view: 'sequence',
    origin: { page: 'executions', label }
  }
}

export interface ShapeCall {
  caller: string // service · operation, or "unknown caller" when the parent wasn't recorded
  callee: string // service · operation
  kind: string
  count: number // in the example execution
}

// callsOf lists the distinct calls (caller -> callee) of one execution, the
// way the server builds a shape, in order of their first start; count is how
// often each call happened in that execution.
export function callsOf (spans: Span[]): ShapeCall[] {
  const name = (s: Span) => `${s.service} · ${s.name}`
  const byId = new Map(spans.map((s) => [s.spanId, s]))
  const calls = new Map<string, ShapeCall & { first: bigint }>()
  for (const s of spans) {
    if (s.parentSpanId === ROOT_PARENT) {
      continue
    }
    const parent = byId.get(s.parentSpanId)
    const caller = parent ? name(parent) : 'unknown caller'
    const kind = s.spanKind ?? ''
    const key = `${caller}\u0000${name(s)}\u0000${kind}`
    const at = BigInt(s.timeStamp)
    const c = calls.get(key)
    if (c) {
      c.count++
      if (at < c.first) {
        c.first = at
      }
    } else {
      calls.set(key, { caller, callee: name(s), kind, count: 1, first: at })
    }
  }
  return [...calls.values()]
    .sort((a, b) => a.first < b.first ? -1 : a.first > b.first ? 1 : (a.callee < b.callee ? -1 : 1))
    .map(({ caller, callee, kind, count }) => ({ caller, callee, kind, count }))
}

// servicesText lists the services of a shape, entry first.
export function servicesText (s: Shape, max = 4): string {
  const rest = s.services.filter((x) => x !== s.entryService)
  const shown = rest.slice(0, max)
  const more = rest.length - shown.length
  return shown.join(' · ') + (more > 0 ? ` · +${more}` : '')
}
