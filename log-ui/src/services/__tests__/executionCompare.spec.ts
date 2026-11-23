import { describe, expect, it } from 'vitest'
import { compareExecutions, differences } from '@/services/executionCompare'
import { groupRepeats, REPEAT_MIN, timeline } from '@/services/problems'
import { type Span } from '@/types/filter'

const ROOT = '0000000000000000'
const MS = 1_000_000 // ns
const base = 1_700_000_000_000 * MS

// span: start and duration in ms from a fixed base.
const span = (trace: string, spanId: string, parentSpanId: string, name: string, atMs: number, ms: number, extra: Partial<Span> = {}): Span => ({
  timeStamp: String(BigInt(base) + BigInt(atMs * MS)), name, spanKind: 'client', service: 'api', traceId: trace.repeat(32), spanId, parentSpanId,
  msg: '', tags: [], serviceTags: [], childSpanCount: 0, statusCode: 0, status: 'ok', statusMessage: '', duration: ms * 1000,
  events: [], links: [], ...extra
})

// The fixture's start_session: n permission lookups, an insert, and, in the
// new version, a call to the directory service.
function startSession (trace: string, lookups: number, total: number, directory: boolean): Span[] {
  const spans = [
    span(trace, 'a000000000000000', ROOT, 'start_session', 0, total, { spanKind: 'server' }),
    span(trace, 'b000000000000000', 'a000000000000000', 'check_permissions', 1, lookups * 23, { spanKind: 'internal' })
  ]
  for (let i = 0; i < lookups; i++) {
    spans.push(span(trace, 'c0000000000000' + i.toString(16).padStart(2, '0'), 'b000000000000000', 'permission_lookup', 2 + 23 * i, 22))
  }
  spans.push(span(trace, 'd000000000000000', 'a000000000000000', 'insert_session', 2 + lookups * 23 + 3, 4))
  if (directory) {
    spans.push(span(trace, 'e000000000000000', 'a000000000000000', 'GET /users', 2 + lookups * 23 + 10, 45))
    spans.push(span(trace, 'e100000000000000', 'e000000000000000', 'GET /users/{id}', 2 + lookups * 23 + 11, 43, { service: 'directory', spanKind: 'server' }))
  }
  return spans
}

describe('repeated calls in the timeline', () => {
  it('folds a run of repeated leaf calls into one row with every call kept', () => {
    const tl = timeline(startSession('1', 20, 910, true))
    const items = groupRepeats(tl.rows, new Set(), null)
    expect(items.map((i) => i.type === 'row' ? i.row.span.name : 'group ' + i.group.name + ' × ' + i.group.rows.length)).toEqual([
      'start_session', 'check_permissions', 'group permission_lookup × 20', 'insert_session', 'GET /users', 'GET /users/{id}'
    ])
    const g = items[2]
    expect(g.type === 'group' && g.group).toMatchObject({ startMs: 2, endMs: 461, minMs: 22, maxMs: 22, errors: 0, depth: 2 })
  })

  it('keeps short runs as rows', () => {
    const items = groupRepeats(timeline(startSession('1', REPEAT_MIN - 1, 420, false)).rows, new Set(), null)
    expect(items.every((i) => i.type === 'row')).toBe(true)
  })

  it('expands a group that was opened or holds the selected span', () => {
    const rows = timeline(startSession('1', 5, 420, false)).rows
    const key = 'repeat:' + rows[2].key
    const opened = groupRepeats(rows, new Set([key]), null)
    expect(opened.filter((i) => i.type === 'row').length).toBe(rows.length)
    expect(opened.find((i) => i.type === 'group')).toMatchObject({ expanded: true })
    const selected = groupRepeats(rows, new Set(), rows[4].key)
    expect(selected.find((i) => i.type === 'group')).toMatchObject({ expanded: true })
  })

  it('does not fold calls that have children, detached spans, or different names', () => {
    const t = '2'
    const spans = [span(t, 'a000000000000000', ROOT, 'job', 0, 100)]
    for (let i = 0; i < 4; i++) {
      const id = 'b00000000000000' + i
      spans.push(span(t, id, 'a000000000000000', 'batch', 1 + i * 20, 10))
      spans.push(span(t, 'c00000000000000' + i, id, 'write', 2 + i * 20, 5))
    }
    spans.push(span(t, 'd000000000000000', 'ffffffffffffffff', 'orphan', 90, 1))
    spans.push(span(t, 'd100000000000000', 'ffffffffffffffff', 'orphan', 91, 1))
    spans.push(span(t, 'd200000000000000', 'ffffffffffffffff', 'orphan', 92, 1))
    const items = groupRepeats(timeline(spans).rows, new Set(), null)
    expect(items.some((i) => i.type === 'group')).toBe(false)
  })

  it('counts failed calls of a group', () => {
    const spans = startSession('3', 4, 300, false)
    spans[3] = { ...spans[3], status: 'error' }
    const g = groupRepeats(timeline(spans).rows, new Set(), null).find((i) => i.type === 'group')
    expect(g?.type === 'group' && g.group.errors).toBe(1)
  })
})

describe('comparing two executions', () => {
  const baseline = startSession('1', 1, 420, false)
  const candidate = startSession('2', 20, 910, true)

  it('matches calls by path and counts repeats on each side', () => {
    const c = compareExecutions(baseline, candidate)
    expect(c.rows.map((r) => [r.name, r.reference.count, r.current.count, r.change])).toEqual([
      ['start_session', 1, 1, 'same'],
      ['check_permissions', 1, 1, 'same'],
      ['permission_lookup', 1, 20, 'more'],
      ['insert_session', 1, 1, 'same'],
      ['GET /users', 0, 1, 'new'],
      ['GET /users/{id}', 0, 1, 'new']
    ])
    expect(c).toMatchObject({ axisMs: 910, referenceMs: 420, currentMs: 910, differentEntry: false })
    // Each side is timed from its own start.
    expect(c.rows[2].current.segments[19]).toEqual({ startMs: 439, endMs: 461, error: false })
    expect(c.rows[2].reference.segments).toEqual([{ startMs: 2, endMs: 24, error: false }])
  })

  it('puts calls only the reference made after their parent subtree', () => {
    const c = compareExecutions(candidate, baseline)
    expect(c.rows.map((r) => [r.name, r.change])).toEqual([
      ['start_session', 'same'],
      ['check_permissions', 'same'],
      ['permission_lookup', 'fewer'],
      ['insert_session', 'same'],
      ['GET /users', 'missing'],
      ['GET /users/{id}', 'missing']
    ])
  })

  it('says which calls differ, most calls first', () => {
    expect(differences(compareExecutions(baseline, candidate))).toEqual([
      'permission_lookup: 1 → 20 calls', 'GET /users: a new call', 'GET /users/{id}: a new call'
    ])
    expect(differences(compareExecutions(candidate, baseline))).toEqual([
      'permission_lookup: 20 → 1 call', 'GET /users: not called (1 before)', 'GET /users/{id}: not called (1 before)'
    ])
    expect(differences(compareExecutions(baseline, startSession('3', 1, 400, false)))).toEqual([])
  })

  it('flags executions that start with different operations', () => {
    const other = [span('4', 'a000000000000000', ROOT, 'end_session', 0, 50, { spanKind: 'server' })]
    expect(compareExecutions(baseline, other).differentEntry).toBe(true)
    expect(compareExecutions([], candidate)).toMatchObject({ referenceMs: 0, currentMs: 910, differentEntry: false })
  })
})
