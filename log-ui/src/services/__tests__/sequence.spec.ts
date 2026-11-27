import { describe, expect, it } from 'vitest'
import { activationOf, agentPrompt, buildSequence, endpointLabels, eventText, hiddenOperations, lanes, locate, pageOf, pairEndpoints, sequenceExport, visibleEvents, OTHER_LANE, type SeqEvent } from '@/services/sequence'
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

describe('activation bar of the selected operation', () => {
  const ev = (id: string, participant = 'api'): SeqEvent => ({ id, kind: id.endsWith(':end') ? 'end' : 'call', timeMs: 0, operationId: id.split(':')[0], spanKey: id.split(':')[0], participant, label: id })
  // a starts, b starts, a ends, b ends: a's end is not next to its start.
  const events = [ev('r:start', 'web'), ev('a:start'), ev('b:start'), ev('b:end'), ev('a:end'), ev('r:end', 'web')]

  it('runs from the start mark to the end mark on the operation\'s lifeline', () => {
    expect(activationOf(events, pageOf(events, 0, 80), 'a')).toEqual({ participant: 'api', from: 1, to: 4, startsHere: true, endsHere: true })
  })
  it('is clipped to the page and says which end is elsewhere', () => {
    expect(activationOf(events, pageOf(events, 2, 2), 'a')).toEqual({ participant: 'api', from: 0, to: 1, startsHere: false, endsHere: false })
    expect(activationOf(events, pageOf(events, 0, 3), 'a')).toEqual({ participant: 'api', from: 1, to: 2, startsHere: true, endsHere: false })
  })
  it('is absent without a valid end or when the operation is on other pages', () => {
    const noEnd = events.filter((e) => e.id !== 'a:end')
    expect(activationOf(noEnd, pageOf(noEnd, 0, 80), 'a')).toBeNull()
    expect(activationOf(events, pageOf(events, 5, 1), 'a')).toBeNull()
  })
})

describe('messages: PRODUCER → CONSUMER', () => {
  const TRACE = 'abcdef00000000000000000000000001'
  const T0 = 1791792000000000000n
  // at and dur in ms from T0
  const s = (id: string, parent: string, service: string, kind: string, name: string, at: number, dur: number): Span => ({
    timeStamp: (T0 + BigInt(at * 1_000_000)).toString(), name, spanKind: kind, service, traceId: TRACE, spanId: id.padStart(16, '0'), parentSpanId: parent.padStart(16, '0'),
    msg: '', tags: [], serviceTags: [], childSpanCount: 0, statusCode: 0, status: 'unset', statusMessage: '', duration: dur * 1000, events: [], links: []
  })
  const rep = (spans: Span[]): Report => {
    const section = <T>(records: T[]) => ({ status: 'ok' as const, records, limit: 1000, shown: records.length, truncated: false, total: records.length, totalStatus: 'ok' as const, retrievedAt: '1' })
    return { traceId: TRACE, timeFrom: '0', timeTo: '9', generatedAt: '1', spans: section(spans), logs: section([]), warnings: [] }
  }
  const k = (id: string) => TRACE + ':' + id.padStart(16, '0')
  const root = s('1', '0', 'api', 'server', 'POST /jobs', 0, 100)
  const producer = s('2', '1', 'api', 'producer', 'publish jobs', 10, 2)

  it('a PRODUCER with one CONSUMER of another service is one message: an arrow, then the consumer\'s processing; no end of the send', () => {
    const m = buildSequence(rep([root, producer, s('3', '2', 'worker', 'consumer', 'process jobs', 15, 40)]))
    const msg = m.operations.find((o) => o.kind === 'message')!
    expect(msg).toMatchObject({ id: k('2'), name: 'publish jobs', source: 'api', target: 'worker', spanKeys: [k('2'), k('3')], serverName: 'process jobs', startMs: 10, consumerStartMs: 15, endMs: 55 })
    expect(m.operations).toHaveLength(2)
    const evs = m.events.filter((e) => e.operationId === msg.id)
    expect(evs.map((e) => [e.kind, e.participant, e.timeMs, e.spanKey])).toEqual([
      ['message', 'api', 10, k('2')], ['start', 'worker', 15, k('3')], ['end', 'worker', 55, k('3')]
    ])
    expect(evs.find((e) => e.kind === 'end')!.durationMs).toBe(40)
    expect(eventText(evs[0], m)).toBe('+10 ms · api → worker · message publish jobs (consumer span process jobs)')
    expect(m.participants).toEqual(['api', 'worker'])
  })

  it('fan-out (two CONSUMERs of other services) is not paired', () => {
    const m = buildSequence(rep([root, producer, s('3', '2', 'worker', 'consumer', 'process jobs', 15, 40), s('4', '2', 'audit', 'consumer', 'audit jobs', 16, 5)]))
    expect(m.operations.map((o) => o.kind).sort()).toEqual(['consumer', 'consumer', 'producer', 'server'])
    expect(m.events.some((e) => e.kind === 'message')).toBe(false)
  })

  it('a CONSUMER of the same service is not paired', () => {
    const m = buildSequence(rep([root, producer, s('3', '2', 'api', 'consumer', 'process jobs', 15, 40)]))
    expect(m.operations.map((o) => o.kind).sort()).toEqual(['consumer', 'producer', 'server'])
  })

  it('a CONSUMER that starts after the producer ended is normal async delivery, not a clock doubt', () => {
    const m = buildSequence(rep([root, producer, s('3', '2', 'worker', 'consumer', 'process jobs', 500, 40)]))
    expect(m.operations.find((o) => o.kind === 'message')).toBeDefined()
    expect(m.clockUncertain).toEqual([])
    expect(m.limitations.join(' ')).not.toContain('clock')
  })

  it('a CONSUMER that starts before its producer is paired and flagged as a clock doubt', () => {
    const m = buildSequence(rep([root, producer, s('3', '2', 'worker', 'consumer', 'process jobs', 8, 40)]))
    expect(m.operations.find((o) => o.kind === 'message')).toBeDefined()
    expect(m.clockUncertain).toEqual([k('2')])
    expect(m.limitations.join(' ')).toContain('1 consumer span(s) start before their producer span')
  })

  it('an RPC over a broker: one call arrow, the reply and an event as two message arrows; both message spans selectable', () => {
    const spans = [
      s('a1', '0', 'api', 'server', 'POST /producers', 0, 300),
      s('a2', 'a1', 'api', 'client', 'API_TO_WORKER.req', 5, 200),
      s('b1', 'a2', 'worker', 'server', 'handle WORKER_PRODUCE', 20, 60),
      s('b2', 'b1', 'worker', 'producer', 'publish <default>', 70, 1),
      s('a3', 'b2', 'api', 'consumer', 'Receive RPC_REPLY_QUEUE', 72, 3),
      s('b3', 'b1', 'worker', 'producer', 'publish <default>', 75, 1),
      s('a4', 'b3', 'api', 'consumer', 'Process API_TO_API', 90, 50),
      s('a5', 'a4', 'api', 'internal', 'save producer', 95, 30)
    ]
    const m = buildSequence(rep(spans))
    expect(m.operations.filter((o) => o.kind === 'call').map((o) => [o.source, o.target, o.name])).toEqual([['api', 'worker', 'API_TO_WORKER.req']])
    expect(m.operations.filter((o) => o.kind === 'message').map((o) => [o.source, o.target, o.serverName])).toEqual([
      ['worker', 'api', 'Receive RPC_REPLY_QUEUE'], ['worker', 'api', 'Process API_TO_API']
    ])
    expect(m.events.filter((e) => e.kind === 'message')).toHaveLength(2)
    expect(m.operations.find((o) => o.name === 'save producer')).toMatchObject({ kind: 'local', source: 'api' })
    expect(m.clockUncertain).toEqual([])
    expect(pairEndpoints(m, k('a4'))).toEqual({ client: k('b3'), server: k('a4') })
    expect(endpointLabels(m, k('a4'))).toEqual(['PRODUCER', 'CONSUMER'])
    expect(endpointLabels(m, k('b1'))).toEqual(['CLIENT', 'SERVER'])
    // The activation bar of a selected message runs over the consumer's processing.
    const msg = m.operations.find((o) => o.serverName === 'Process API_TO_API')!
    const a = activationOf(m.events, pageOf(m.events, 0, 80), msg.id)!
    expect(a.participant).toBe('api')
    expect(m.events[a.from].spanKey).toBe(k('a4'))
    // Exports carry messages as they carry calls.
    const out = sequenceExport(m, rep(spans), { view: 'sequence', selected: null, showInternal: true, logScope: 'span' })
    expect(out.operations.filter((o) => o.kind === 'message')).toHaveLength(2)
    expect(out.events.filter((e) => e.text.includes('worker → api · message publish <default>'))).toHaveLength(2)
  })
})

describe('hidden participants and the agent prompt', () => {
  it('hiding a participant leaves out every operation to or from it and its log markers, but not the selected one', () => {
    const m = buildSequence(report('error'))
    const all = visibleEvents(m, true, null)
    const hidden = new Set(['object-storage'])
    const shown = visibleEvents(m, true, null, hidden)
    const put = m.operations.find((o) => o.name === 'PutObject')!
    expect(all.some((e) => e.operationId === put.id)).toBe(true)
    expect(shown.some((e) => e.operationId === put.id)).toBe(false)
    expect(shown.some((e) => e.kind === 'log' && e.participant === 'object-storage')).toBe(false)
    expect(hiddenOperations(m, hidden)).toBe(1)
    // The selected operation stays, even when it touches a hidden participant.
    expect(visibleEvents(m, true, put.id, hidden).some((e) => e.operationId === put.id)).toBe(true)
  })

  it('the prompt asks for an explanation from the evidence and carries what the view shows', () => {
    const r = report('error')
    const m = buildSequence(r)
    const hidden = new Set(['object-storage'])
    const events = visibleEvents(m, true, null, hidden)
    const text = agentPrompt(m, r, { events, hidden: [...hidden], showInternal: true, selectedKey: null })
    expect(text).toContain('Use only the evidence below')
    expect(text).toContain('do not invent')
    expect(text).toContain(r.traceId)
    expect(text).toContain('Left out of this view by the user: object-storage')
    expect(text).toContain('## Failed spans')
    expect(text).toContain('## Logs')
    expect(text).not.toContain('call PutObject')
    for (const e of events) expect(text).toContain(eventText(e, m))
    const focused = agentPrompt(m, r, { events, hidden: [], showInternal: true, selectedKey: m.operations.find((o) => o.name === 'PutObject')!.id })
    expect(focused).toContain('Focus on the selected operation "PutObject"')
  })
})
