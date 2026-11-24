import { callIdentity, shortStatement, statementOf, tablesOf, targetOf } from '@/services/callIdentity'
import { spanOrder } from '@/services/report'
import { spanInterval, traceStart } from '@/services/problems'
import { type Span } from '@/types/filter'

// Comparison of two executions (two traces) on one time scale. Spans are
// matched by their call path: the service, kind and name of the span and of
// each ancestor up to the entry, and for a database call its normalized
// statement (callIdentity, services/callIdentity.ts). Calls with the same path are one row, with
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
  parent: string // the parent row's path; '' for the entry, '?' when the parent was not loaded
  depth: number
  name: string
  service: string
  spanKind: string
  statement: string // normalized database statement, '' when none
  target: string // what the call talks to (db.system, peer.service …), '' when not recorded
  reference: CompareSide // the execution compared with (e.g. the baseline)
  current: CompareSide // the execution being investigated
  change: CompareChange
  replacement: number | null // index into ExecutionComparison.replacements
}

// Replacement is an inferred correspondence: calls only the reference made
// and calls only the current execution made, under the same caller, in the
// same place between the calls both made, to the same target, and (when
// both name tables) touching a common table. It says the new calls likely
// do what the old ones did; it is an inference from these facts, listed in
// reasons, never a recorded fact.
export interface Replacement {
  parent: string
  reference: CompareRow[]
  current: CompareRow[]
  target: string
  tables: string[] // tables both sides name
  reasons: string[]
}

export interface ExecutionComparison {
  rows: CompareRow[]
  replacements: Replacement[]
  axisMs: number // one scale for both: the longer execution
  referenceMs: number // first start to last end of each execution
  currentMs: number
  differentEntry: boolean // the entry spans differ (service, kind or name)
}

// Steps of a path are joined with a character that names and statements
// don't contain ('/' does: "GET /users").
const SEP = '\u0001'

interface Placed { path: string, parent: string, depth: number, span: Span }

// placed lists the spans of one execution in parent order with their paths.
function placed (spans: Span[]): Placed[] {
  const pathOf = new Map<string, string>()
  return spanOrder(spans).map((o) => {
    const parent = o.parentMissing ? '?' : pathOf.get(o.span.parentSpanId) ?? ''
    const path = parent + SEP + callIdentity(o.span)
    pathOf.set(o.span.spanId, path)
    return { path, parent, depth: o.depth, span: o.span }
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
        path: p.path, parent: p.parent, depth: p.depth, name: p.span.name, service: p.span.service, spanKind: p.span.spanKind ?? '',
        statement: statementOf(p.span), target: targetOf(p.span),
        reference: { count: 0, segments: [] }, current: { count: 0, segments: [] }, change: 'same', replacement: null
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
      const parent = p.parent
      let at = rows.length
      const pi = rows.findIndex((r) => r.path === parent)
      if (pi >= 0) {
        at = pi + 1
        while (at < rows.length && rows[at].path.startsWith(parent + SEP)) {
          at++
        }
      }
      add(p, refOrigin, 'reference', at)
    }
  }
  for (const r of rows) {
    r.change = changeOf(r.reference.count, r.current.count)
  }
  const replacements = inferReplacements(rows)
  replacements.forEach((rp, i) => {
    for (const r of [...rp.reference, ...rp.current]) {
      r.replacement = i
    }
  })
  const referenceMs = totalMs(reference, refOrigin)
  const currentMs = totalMs(current, curOrigin)
  const entry = (spans: Span[]) => placed(spans)[0]?.path ?? ''
  return {
    rows,
    replacements,
    axisMs: Math.max(1, referenceMs, currentMs),
    referenceMs,
    currentMs,
    differentEntry: reference.length > 0 && current.length > 0 && entry(reference) !== entry(current)
  }
}

// firstStart: the earliest start of a row's calls on one side.
const firstStart = (r: CompareRow, side: 'reference' | 'current') => Math.min(...r[side].segments.map((s) => s.startMs))

// lcs: the longest common subsequence of two lists of paths.
function lcs (a: string[], b: string[]): string[] {
  const t = Array.from({ length: a.length + 1 }, () => new Array<number>(b.length + 1).fill(0))
  for (let i = a.length - 1; i >= 0; i--) {
    for (let j = b.length - 1; j >= 0; j--) {
      t[i][j] = a[i] === b[j] ? t[i + 1][j + 1] + 1 : Math.max(t[i + 1][j], t[i][j + 1])
    }
  }
  const out: string[] = []
  let i = 0
  let j = 0
  while (i < a.length && j < b.length) {
    if (a[i] === b[j]) {
      out.push(a[i])
      i++
      j++
    } else if (t[i + 1][j] >= t[i][j + 1]) {
      i++
    } else {
      j++
    }
  }
  return out
}

// inferReplacements pairs calls only one execution made (see Replacement).
// Under each caller both executions made, its calls are ordered by start on
// each side; the calls both made, in the same order (their longest common
// subsequence), are anchors; a one-sided call belongs to the gap after the
// anchors that started before it. In each gap, calls of one side and of the
// other to the same target are paired, unless both name tables and none is
// common.
export function inferReplacements (rows: CompareRow[]): Replacement[] {
  const byPath = new Map(rows.map((r) => [r.path, r]))
  const children = new Map<string, CompareRow[]>()
  for (const r of rows) {
    children.set(r.parent, [...(children.get(r.parent) ?? []), r])
  }
  const both = (r: CompareRow) => r.reference.count > 0 && r.current.count > 0
  const out: Replacement[] = []
  for (const [parent, kids] of children) {
    const p = byPath.get(parent)
    if (!p || !both(p)) {
      continue // the entry, a caller only one side made, or a caller not loaded
    }
    const order = (side: 'reference' | 'current') => kids.filter((r) => r[side].count > 0).sort((x, y) => firstStart(x, side) - firstStart(y, side))
    const ref = order('reference')
    const cur = order('current')
    const anchors = lcs(ref.filter(both).map((r) => r.path), cur.filter(both).map((r) => r.path))
    const anchorSet = new Set(anchors)
    // gaps: the one-sided calls of a side with the number of anchors before them.
    const gaps = (list: CompareRow[]) => {
      const g: { row: CompareRow, gap: number }[] = []
      let n = 0
      for (const r of list) {
        if (anchorSet.has(r.path)) {
          n++
        } else if (!both(r)) {
          g.push({ row: r, gap: n })
        }
      }
      return g
    }
    const refGaps = gaps(ref)
    const curGaps = gaps(cur)
    const tables = (list: CompareRow[]) => new Set(list.flatMap((r) => tablesOf(r.statement)))
    for (let gap = 0; gap <= anchors.length; gap++) {
      const targets = new Set(refGaps.filter((x) => x.gap === gap && x.row.target).map((x) => x.row.target))
      for (const target of targets) {
        let old = refGaps.filter((x) => x.gap === gap && x.row.target === target).map((x) => x.row)
        let now = curGaps.filter((x) => x.gap === gap && x.row.target === target).map((x) => x.row)
        // A call that names tables, none of which the other side names, is
        // not part of the pair (a new query of another table stays new).
        const shares = (r: CompareRow, other: Set<string>) => {
          const own = tablesOf(r.statement)
          return !own.length || !other.size || own.some((t) => other.has(t))
        }
        const ot0 = tables(old)
        const nt0 = tables(now)
        old = old.filter((r) => shares(r, nt0))
        now = now.filter((r) => shares(r, ot0))
        if (!old.length || !now.length) {
          continue
        }
        const ot = tables(old)
        const nt = tables(now)
        const common = [...ot].filter((t) => nt.has(t)).sort()
        if (ot.size && nt.size && !common.length) {
          continue
        }
        const before = gap === 0 ? null : byPath.get(anchors[gap - 1])!
        const reasons = [
          before ? `same place: after ${rowLabel(before)}` : `same place: the first calls of ${p.name}`,
          `same target: ${target}`,
          ...(common.length ? [`same ${common.length === 1 ? 'table' : 'tables'}: ${common.join(', ')}`] : [])
        ]
        out.push({ parent, reference: old, current: now, target, tables: common, reasons })
      }
    }
  }
  return out
}

// rowLabel names a row: its name, and the start of its statement.
export function rowLabel (r: CompareRow): string {
  return r.statement ? `${r.name} ${shortStatement(r.statement, 40)}` : r.name
}

const callsText = (n: number) => n === 1 ? '1 call' : n + ' calls'

// replacementText says what an inferred replacement pairs, with its reasons.
export function replacementText (rp: Replacement): string {
  const side = (list: CompareRow[], s: 'reference' | 'current') => list.map((r) => `${rowLabel(r)} (${callsText(r[s].count)})`).join(', ')
  return `likely in place of each other: ${side(rp.reference, 'reference')} → ${side(rp.current, 'current')}; ${rp.reasons.join('; ')}`
}

// differences says in words which calls differ, most calls first. Calls of
// an inferred replacement are said together, once.
export function differences (c: ExecutionComparison): string[] {
  const out: { n: number, text: string }[] = []
  for (const rp of c.replacements) {
    const n = rp.current.reduce((a, r) => a + r.current.count, 0) + rp.reference.reduce((a, r) => a + r.reference.count, 0)
    out.push({ n, text: replacementText(rp) })
  }
  for (const r of c.rows) {
    if (r.change === 'same' || r.replacement !== null) {
      continue
    }
    const name = rowLabel(r)
    const text = r.change === 'new'
      ? `${name}: ${r.current.count === 1 ? 'a new call' : r.current.count + ' new calls'}`
      : r.change === 'missing'
        ? `${name}: not called (${r.reference.count} before)`
        : `${name}: ${r.reference.count} → ${r.current.count} ${r.current.count === 1 ? 'call' : 'calls'}`
    out.push({ n: Math.abs(r.current.count - r.reference.count), text })
  }
  return out.sort((x, y) => y.n - x.n).map((d) => d.text)
}

// --- presentation

// Tone is what a row's color says: the entry, an unchanged call, more or
// fewer calls of it, a new call, one no longer made, or a call of an
// inferred replacement.
export type CompareTone = 'entry' | 'same' | 'more' | 'fewer' | 'new' | 'missing' | 'replaced'

export function toneOf (r: CompareRow): CompareTone {
  if (r.replacement !== null) {
    return 'replaced'
  }
  return r.depth === 0 && r.change === 'same' ? 'entry' : r.change
}

// RunNote describes the calls of one row on one side when there are
// several: how many, whether they ran one after another (no two overlap)
// or overlapped, and the time from the first start to the last end.
//
// A span's start comes from the wall clock and its duration from a
// monotonic one, so calls made strictly in turn can overlap by a few
// microseconds (seen on Windows: one ends at +0.881 ms, the next starts at
// +0.880 ms). An overlap up to OVERLAP_TOLERANCE of the run, at least
// OVERLAP_MIN_MS, is not counted.
export const OVERLAP_MIN_MS = 0.05
export const OVERLAP_TOLERANCE = 0.01
export interface RunNote {
  count: number
  sequential: boolean
  startMs: number
  endMs: number
  text: string
}

export function runNote (side: CompareSide): RunNote | null {
  // By start, then by end: a call of no length that starts with a longer one
  // is not inside it.
  const valid = side.segments.filter((s) => s.endMs !== null).sort((a, b) => a.startMs - b.startMs || a.endMs! - b.endMs!)
  if (side.count < 2 || valid.length < 2) {
    return null
  }
  const startMs = valid[0].startMs
  const endMs = Math.max(...valid.map((s) => s.endMs!))
  const ms = endMs - startMs
  const tolerance = Math.max(OVERLAP_MIN_MS, ms * OVERLAP_TOLERANCE)
  // Against the latest end so far: a long call can cover later ones.
  let sequential = true
  let lastEnd = valid[0].endMs!
  for (let i = 1; i < valid.length; i++) {
    if (lastEnd - valid[i].startMs > tolerance) {
      sequential = false
    }
    lastEnd = Math.max(lastEnd, valid[i].endMs!)
  }
  const span = ms.toFixed(ms >= 10 ? 0 : 1) + ' ms'
  return {
    count: side.count,
    sequential,
    startMs,
    endMs,
    text: `${side.count} calls, ${sequential ? 'one after another' : 'overlapping'}: ${span}`
  }
}

// sideRows are the rows of one execution in the stacked layout: the calls
// it made, in order.
export function sideRows (c: ExecutionComparison, side: 'reference' | 'current'): CompareRow[] {
  return c.rows.filter((r) => r[side].count > 0)
}

// deltaText: the difference of the two executions' durations.
export function deltaText (c: ExecutionComparison): string {
  const d = c.currentMs - c.referenceMs
  const v = Math.abs(d)
  return (d >= 0 ? '+' : '−') + v.toFixed(v >= 10 ? 0 : 1) + ' ms'
}
