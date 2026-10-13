import { describe, expect, it } from 'vitest'
import {
  graphLayout, headline, initialSelection, scopedLogs, siblingOverlaps, spanKey, spanLogCounts, TASK_NOT_RECORDED, taskLanes, timeline, traceStart, validateRules
} from '@/services/problems'
import { type Report } from '@/types/report'
import { type Problem, type DetectionRules } from '@/types/problems'
import { referenceReports } from './problemReference'

function report (name: 'failed' | 'logonly', extraLogs: Report['logs']['records'] = []): Report {
  const r = referenceReports[name]
  const section = <T>(records: T[]) => ({ status: 'ok' as const, records, limit: 1000, shown: records.length, truncated: false, total: records.length, totalStatus: 'ok' as const, retrievedAt: '1' })
  return { traceId: r.traceId, timeFrom: '0', timeTo: '9', generatedAt: '1', spans: section(r.spans), logs: section([...r.logs, ...extraLogs]), warnings: [] }
}
const key = (r: Report, spanId: string) => r.traceId + ':' + spanId
const UPLOAD = '0000000000000002'
const THUMBNAIL = '0000000000000003'

describe('trace graph (PW-INV-002/003)', () => {
  it('draws recorded parent → child edges only, siblings in their own columns', () => {
    const r = report('failed')
    const g = graphLayout(r.spans.records, 60)
    expect(g.nodes).toHaveLength(4)
    expect(g.edges.map((e) => [e.from.split(':')[1], e.to.split(':')[1]]).sort()).toEqual([
      ['0000000000000001', UPLOAD], ['0000000000000001', THUMBNAIL], [UPLOAD, '0000000000000004']
    ].sort())
    const col = (id: string) => g.nodes.find((n) => n.span.spanId === id)!.column
    expect(col(UPLOAD)).not.toBe(col(THUMBNAIL))
    expect(g.hidden).toBe(0)
  })

  it('reports the 120 ms overlap of upload [25,225) and thumbnail [30,150)', () => {
    const o = siblingOverlaps(report('failed').spans.records)
    expect(o).toHaveLength(1)
    expect([o[0].a.name, o[0].b.name].sort()).toEqual(['create_thumbnail', 'upload_recording'])
    expect(o[0].fromMs).toBe(30)
    expect(o[0].toMs).toBe(150)
    expect(o[0].overlapMs).toBe(120)
  })

  it('a span starting after two overlapping siblings keeps only its recorded parent', () => {
    const spans = report('failed').spans.records
    const later = { ...spans[2], spanId: '00000000000000d0', name: 'later', parentSpanId: '0000000000000001', timeStamp: (BigInt(spans[0].timeStamp) + 230_000_000n).toString(), duration: 5000 }
    const g = graphLayout([...spans, later], 60)
    expect(g.edges.filter((e) => e.to.endsWith('00000000000000d0')).map((e) => e.from.split(':')[1])).toEqual(['0000000000000001'])
  })

  it('a missing parent is detached, never given an invented edge', () => {
    const spans = report('failed').spans.records.filter((s) => s.spanId !== '0000000000000001')
    const g = graphLayout(spans, 60)
    expect(g.nodes.filter((n) => n.detached).map((n) => n.span.name).sort()).toEqual(['create_thumbnail', 'upload_recording'])
    expect(g.edges).toHaveLength(1)
  })

  it('a render limit hides spans from the drawing but says how many', () => {
    const g = graphLayout(report('failed').spans.records, 2)
    expect(g.nodes).toHaveLength(2)
    expect(g.hidden).toBe(2)
  })
})

describe('timeline (PW-INV-002)', () => {
  it('uses one axis from the trace start; nothing is stacked or summed', () => {
    const tl = timeline(report('failed').spans.records)
    const row = (name: string) => tl.rows.find((r) => r.span.name === name)!
    expect(row('upload_recording')).toMatchObject({ startMs: 25, endMs: 225 })
    expect(row('create_thumbnail')).toMatchObject({ startMs: 30, endMs: 150 })
    expect(tl.totalMs).toBe(240)
  })

  it('an invalid duration has no end instead of a zero-length interval', () => {
    const spans = report('failed').spans.records.map((s) => s.spanId === THUMBNAIL ? { ...s, duration: -1 } : s)
    expect(timeline(spans).rows.find((r) => r.span.spanId === THUMBNAIL)!.endMs).toBeNull()
    expect(siblingOverlaps(spans)).toHaveLength(0)
  })
})

describe('logs (PW-INV-005/006/007/008)', () => {
  it('task lanes come from the recorded task.id; a missing one is "Task not recorded"', () => {
    const r = report('failed')
    const upload = r.spans.records.find((s) => s.spanId === UPLOAD)!
    const lanes = taskLanes(scopedLogs(r, upload, 'span').map((l) => l.log), traceStart(r.spans.records)!)
    expect(lanes.map((l) => l.task)).toEqual(['checksum', 'transfer', TASK_NOT_RECORDED])
    expect(lanes.flatMap((l) => l.points)).toHaveLength(6)
  })

  it('all trace logs include logs without a SpanId and with an unknown span, labeled', () => {
    const r = report('failed', [
      { timeUnixNano: '1791709092100000000', recordId: 'x-trace', traceId: referenceReports.failed.traceId, spanId: '', severity: 'INFO', service: 'recording-api', message: 'trace-level', attributes: {} },
      { timeUnixNano: '1791709092110000000', recordId: 'x-gone', traceId: referenceReports.failed.traceId, spanId: '00000000000000ee', severity: 'WARN', service: 'recording-worker', message: 'unknown span', attributes: {} }
    ])
    const upload = r.spans.records.find((s) => s.spanId === UPLOAD)!
    const all = scopedLogs(r, upload, 'trace')
    expect(all).toHaveLength(14)
    expect(all.find((l) => l.log.recordId === 'x-trace')!.association.kind).toBe('no-span-id')
    expect(all.find((l) => l.log.recordId === 'x-gone')!.association.kind).toBe('span-not-found')
    const own = scopedLogs(r, upload, 'span').map((l) => l.log.recordId)
    expect(own).not.toContain('x-trace')
    expect(own).not.toContain('x-gone')
  })

  it('a log of another trace with the same SpanId is not the span\'s log', () => {
    const r = report('failed', [{ timeUnixNano: '1791709092100000000', recordId: 'other', traceId: 'ffffffffffffffffffffffffffffff01', spanId: UPLOAD, severity: 'ERROR', service: 'x', message: 'other trace', attributes: {} }])
    const upload = r.spans.records.find((s) => s.spanId === UPLOAD)!
    expect(scopedLogs(r, upload, 'span').map((l) => l.log.recordId)).not.toContain('other')
    expect(spanLogCounts(r).get(key(r, UPLOAD))).toEqual({ logs: 6, errors: 1 })
  })

  it('an OK span keeps its status and shows its ERROR log separately', () => {
    const r = report('logonly')
    expect(r.spans.records.every((s) => s.status === 'ok')).toBe(true)
    expect(spanLogCounts(r).get(key(r, UPLOAD))!.errors).toBe(1)
  })

  it('selection: the entry span, else the previous identity if present, else none', () => {
    const r = report('failed')
    expect(initialSelection(r, UPLOAD, null)).toBe(key(r, UPLOAD))
    expect(initialSelection(r, '', key(r, THUMBNAIL))).toBe(key(r, THUMBNAIL))
    expect(initialSelection(r, '', 'ffffffffffffffffffffffffffffff01:' + THUMBNAIL)).toBeNull()
    expect(spanKey(r.spans.records[0])).toContain(r.traceId)
  })
})

describe('problem rows (PW-GRP-001/004/005)', () => {
  const base = { id: 'PRB-1', title: 't', service: 's', operation: 'upload', environment: 'prod', qualified: true, explanation: {} as any }
  const group = { key: 'k', sourceKind: 'log', service: 's', operation: 'upload', environment: 'prod', method: 'message', fingerprint: 'f', exceptionType: '', frame: '', signature: 'denied', occurrences: 240, unlinked: 0, affectedTraces: 100, firstSeen: '1', lastSeen: '2', versions: [] } as any

  it('leads with affected calls when they are attributable', () => {
    const h = headline({ ...base, type: 'error', error: { ...group, affectedCalls: 100 } } as Problem)
    expect(h).toMatchObject({ value: '100', label: 'Affected calls' })
    expect(h.detail).toContain('240 matching ERROR logs')
  })

  it('leads with matching records when calls can\'t be attributed', () => {
    const h = headline({ ...base, type: 'error', qualified: false, error: { ...group, affectedCalls: 0 } } as Problem)
    expect(h).toMatchObject({ value: '240', label: 'Matching records', detail: 'Call attribution unavailable' })
  })

  it('a regression shows the metric change and evaluated calls, not failed calls', () => {
    const h = headline({
      ...base,
      type: 'release_regression',
      regression: { metric: { metric: 'p95', baseline: 100, candidate: 160, absolute: 60, relative: 60 }, baselineCalls: 1000, candidateCalls: 1000, baseline: {}, candidate: {} }
    } as unknown as Problem)
    expect(h).toEqual({ value: 'p95 +60%', label: '100 ms → 160 ms', detail: '1,000 / 1,000 calls evaluated' })
  })
})

describe('detection rules (PW-RUL-002)', () => {
  const rules: DetectionRules = {
    scope: 'installation',
    revision: 1,
    errors: { enabled: true, minAffectedCalls: 1, grouping: 'operation_signature' },
    slow: { enabled: true, durationMs: 500, minCalls: 5 },
    regressions: { enabled: true, p95RelativePct: 20, p95AbsoluteMs: 20 }
  }
  it('validates enabled templates only', () => {
    expect(validateRules(rules)).toEqual([])
    const bad = { ...rules, slow: { ...rules.slow, durationMs: -5 } }
    expect(validateRules(bad).map((e) => e.field)).toEqual(['slow.durationMs'])
    expect(validateRules({ ...bad, slow: { ...bad.slow, enabled: false } })).toEqual([])
    expect(validateRules({ ...rules, errors: { ...rules.errors, minAffectedCalls: 1.5 } }).map((e) => e.field)).toEqual(['errors.minAffectedCalls'])
  })
})
