import { describe, expect, it } from 'vitest'
import { buildSequence, eventText, lanes, locate, pageOf, pairEndpoints, visibleEvents, OTHER_LANE } from '@/services/sequence'
import { timeline } from '@/services/problems'
import { type Report } from '@/types/report'
import { type Span } from '@/types/filter'
import { sequenceCases } from './sequenceReference'

function report (id: string, mutate?: (spans: Span[]) => Span[]): Report {
  const c = sequenceCases[id]
  const spans: Span[] = mutate ? mutate(structuredClone(c.spans)) : structuredClone(c.spans)
  const section = <T>(records: T[]) => ({ status: 'ok' as const, records, limit: 1000, shown: records.length, truncated: false, total: records.length, totalStatus: 'ok' as const, retrievedAt: '1' })
  return { traceId: c.traceId, timeFrom: '0', timeTo: '9', generatedAt: '1', spans: section(spans), logs: section(structuredClone(c.logs)), warnings: [] }
}
const key = (r: Report, id: string) => r.traceId + ':' + id
const span = (r: Report, id: string) => r.spans.records.find((s) => s.spanId === id)!

describe('pairing (SEQ-GEN-001/002/003)', () => {
  it('a CLIENT with exactly one SERVER of another service is one operation with both spans', () => {
    const r = report('error')
    const m = buildSequence(r)
    expect(m.spanCount).toBe(6)
    expect(m.operations).toHaveLength(4)
    const put = m.operations.find((o) => o.name === 'PutObject')!
    expect(put).toMatchObject({ kind: 'call', source: 'recording-api', target: 'object-storage', spanKeys: [key(r, '0000000000000003'), key(r, '0000000000000004')], status: 'error', serverStatus: 'error' })
    expect(pairEndpoints(m, key(r, '0000000000000004'))).toEqual({ client: key(r, '0000000000000003'), server: key(r, '0000000000000004') })
    expect(m.participants).toEqual(['recording-api', 'thumbnail-worker', 'object-storage'])
  })

  it('two possible SERVER children stay separate operations', () => {
    const r = report('error', (spans) => [...spans, { ...spans[3], spanId: '00000000000000d4', service: 'object-storage-b' }])
    const m = buildSequence(r)
    const put = m.operations.find((o) => o.name === 'PutObject')!
    expect(put.kind).toBe('client')
    expect(put.spanKeys).toHaveLength(1)
    expect(m.operations.filter((o) => o.kind === 'server' && o.name === 'PUT /recordings')).toHaveLength(2)
  })

  it('INTERNAL spans stay local operations on their own service', () => {
    const m = buildSequence(report('error'))
    const upload = m.operations.find((o) => o.name === 'upload_recording')!
    expect(upload).toMatchObject({ kind: 'local', source: 'recording-api', target: 'recording-api' })
    expect(m.participants).not.toContain('upload_recording')
  })

  it('a CLIENT without a captured server reaches its recorded peer as not instrumented', () => {
    const r = report('error', (spans) => spans.filter((s) => s.spanId !== '0000000000000004'))
    const m = buildSequence(r)
    const put = m.operations.find((o) => o.name === 'PutObject')!
    expect(put).toMatchObject({ kind: 'client', target: 'object-storage (not instrumented)', targetObserved: false })
    expect(m.unobserved).toEqual(['object-storage (not instrumented)'])
  })
})

describe('events (SEQ-GEN-004/008/009/011/012)', () => {
  it('ends are completion markers, not responses; no event claims success', () => {
    const m = buildSequence(report('logs-only'))
    expect(m.events.filter((e) => e.kind === 'end')).toHaveLength(4)
    expect(m.events.map((e) => eventText(e, m)).join('\n')).not.toMatch(/response|200 OK|success/i)
  })

  it('ERROR logs are markers on the span that recorded them, even when every span is OK', () => {
    const r = report('logs-only')
    const m = buildSequence(r)
    const logs = m.events.filter((e) => e.kind === 'log')
    expect(logs).toHaveLength(4)
    const storageLog = logs.find((e) => e.label.startsWith('AccessDenied'))!
    expect(storageLog).toMatchObject({ spanKey: key(r, '0000000000000004'), participant: 'object-storage', operationId: key(r, '0000000000000003') })
    expect(m.operations.every((o) => o.status === 'ok')).toBe(true)
  })

  it('a child that outlives its parent keeps its recorded end', () => {
    const r = report('error', (spans) => spans.map((s) => s.spanId === '0000000000000001' ? { ...s, duration: 30000 } : s.spanId === '0000000000000002' ? { ...s, duration: 205000 } : s))
    const row = timeline(r.spans.records).rows.find((x) => x.span.spanId === '0000000000000002')!
    expect(row.endMs).toBe(230)
  })

  it('an unknown end creates no completion marker and no zero duration', () => {
    const r = report('error', (spans) => spans.map((s) => s.spanId === '0000000000000002' ? { ...s, duration: -1 } : s))
    const m = buildSequence(r)
    const upload = m.operations.find((o) => o.name === 'upload_recording')!
    expect(upload.endMs).toBeNull()
    expect(m.events.filter((e) => e.operationId === upload.id && e.kind === 'end')).toHaveLength(0)
  })

  it('a log outside its span\'s interval stays visible with a note', () => {
    const r = report('error')
    const late = { ...r.logs.records.find((l) => l.severity === 'ERROR' && l.spanId === '0000000000000002')!, recordId: 'late', timeUnixNano: (BigInt(span(r, '0000000000000001').timeStamp) + 500_000_000n).toString() }
    r.logs.records.push(late)
    const m = buildSequence(r)
    expect(m.events.find((e) => e.logId === 'late')!.note).toContain('outside')
  })

  it('repeated names are repeats, never retries', () => {
    const m = buildSequence(report('large'))
    const repeated = m.operations.filter((o) => o.name === 'validate_chunk')
    expect(repeated.length).toBeGreaterThan(1)
    expect(repeated[0].repeat).toEqual({ index: 1, count: repeated.length })
    expect(JSON.stringify(m)).not.toMatch(/retry/i)
  })

  it('is deterministic and leaves the report unchanged', () => {
    const r = report('error')
    const before = JSON.stringify(r)
    expect(JSON.stringify(buildSequence(r))).toBe(JSON.stringify(buildSequence(r)))
    expect(JSON.stringify(r)).toBe(before)
  })
})

describe('overlap, links and clocks (SEQ-GEN-005/006/007/010)', () => {
  it('siblings [25,225) and [30,152) overlap 122 ms; nesting is not an overlap', () => {
    const m = buildSequence(report('error'))
    expect(m.overlaps).toHaveLength(1)
    expect(m.overlaps[0]).toMatchObject({ aName: 'upload_recording', bName: 'GenerateThumbnail', fromMs: 30, toMs: 152, overlapMs: 122 })
  })

  it('a generic link stays an association; nothing is fetched or paired', () => {
    const r = report('async')
    const m = buildSequence(r)
    expect(m.links).toEqual([{ fromKey: key(r, '0000000000000016'), fromName: 'process_job', traceId: r.traceId, spanId: '0000000000000015', inReport: true }])
    expect(m.operations.map((o) => o.kind).sort()).toEqual(['consumer', 'producer', 'server'])
  })

  it('a missing parent is explicit and no root is invented', () => {
    const m = buildSequence(report('partial'))
    expect(m.operations.find((o) => o.name === 'PutObject')!.missingParent).toBe(true)
    expect(m.limitations.join(' ')).toContain('parent that isn\'t in this report')
    expect(m.missingSpanLogs).toBeGreaterThan(0)
  })

  it('a server outside its client interval withholds cross-service overlaps, not same-clock ones', () => {
    const r = report('error', (spans) => spans.map((s) => s.spanId === '0000000000000006' ? { ...s, timeStamp: (BigInt(s.timeStamp) - 20_000_000n).toString() } : s))
    const m = buildSequence(r)
    expect(m.clockUncertain).toHaveLength(1)
    // upload and GenerateThumbnail are both recorded by recording-api: one clock.
    expect(m.overlaps.map((o) => o.overlapMs)).toEqual([122])
    const cross = buildSequence(report('error', (spans) => spans.map((s) => s.spanId === '0000000000000006' ? { ...s, timeStamp: (BigInt(s.timeStamp) - 20_000_000n).toString() } : s.spanId === '0000000000000005' ? { ...s, service: 'other-api' } : s)))
    expect(cross.overlaps).toHaveLength(0)
    expect(m.limitations.join(' ')).toContain('clocks')
  })
})

describe('bounded rendering (SEQ-RES-001)', () => {
  it('pages marks, folds extra lanes and locates the selection', () => {
    const r = report('large')
    const m = buildSequence(r)
    expect(m.events.length).toBeGreaterThan(80)
    const page = pageOf(m.events, 0, 80)
    expect(page.events).toHaveLength(80)
    expect(page.total).toBe(m.events.length)
    const last = m.events[m.events.length - 1]
    const first = m.events.findIndex((e) => e.spanKey === last.spanKey || e.operationId === last.spanKey)
    expect(locate(m.events, last.spanKey, 80)).toBe(Math.floor(first / 80) * 80)
    const l = lanes({ ...m, participants: ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'] }, 6)
    expect(l.shown).toHaveLength(5)
    expect(l.other).toEqual(['f', 'g', 'h'])
    expect(OTHER_LANE).toBe('other services')
  })

  it('hiding internal operations hides their marks but keeps logs and the selection', () => {
    const r = report('error')
    const m = buildSequence(r)
    const upload = key(r, '0000000000000002')
    const hidden = visibleEvents(m, false, null)
    expect(hidden.some((e) => e.operationId === upload && e.kind !== 'log')).toBe(false)
    expect(hidden.some((e) => e.kind === 'log' && e.spanKey === upload)).toBe(true)
    expect(visibleEvents(m, false, upload).some((e) => e.operationId === upload && e.kind === 'start')).toBe(true)
    expect(m.events).toHaveLength(visibleEvents(m, true, null).length)
  })
})
