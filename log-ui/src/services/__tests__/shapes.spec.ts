import { describe, expect, it } from 'vitest'
import { callsOf, exampleTarget, failedShare, fmtDuration, pageCount, pageOf, percentileHint, servicesText } from '@/services/shapes'
import { type Span } from '@/types/filter'
import { type Shape } from '@/types/shapes'

const shape = (extra: Partial<Shape> = {}): Shape => ({
  shapeId: '1', entryService: 'gateway', entryOperation: 'POST /orders', services: ['auth', 'gateway', 'orders', 'payments'],
  distinctCalls: 5, executions: 1000, failedExecutions: 3, invalidDurations: 0,
  percentiles: { p50: 120, p95: 410, p99: 900 },
  examples: { p50: { traceId: 'a'.repeat(32), time: '1000000000000', durationUs: 121000 }, slowest: { traceId: 'b'.repeat(32), time: '2000000000000', durationUs: 3000000 } },
  ...extra
})

const span = (spanId: string, parentSpanId: string, service: string, name: string, t: number, spanKind = 'server'): Span => ({
  timeStamp: String(t), name, spanKind, service, traceId: 't'.repeat(32), spanId, parentSpanId, msg: '', tags: [], serviceTags: [],
  childSpanCount: 0, statusCode: 0, status: 'unset', statusMessage: '', duration: 1, events: [], links: []
})

describe('sequence library helpers', () => {
  it('pages ten shapes at a time', () => {
    const many = Array.from({ length: 23 }, (_, i) => shape({ shapeId: String(i) }))
    expect(pageOf(many, 0).map((s) => s.shapeId)).toEqual(['0', '1', '2', '3', '4', '5', '6', '7', '8', '9'])
    expect(pageOf(many, 2).map((s) => s.shapeId)).toEqual(['20', '21', '22'])
    expect(pageCount(23)).toBe(3)
    expect(pageCount(0)).toBe(1)
  })

  it('formats failure shares and durations without hiding small failures', () => {
    expect(failedShare(shape())).toBe('0.3%')
    expect(failedShare(shape({ failedExecutions: 0 }))).toBe('0%')
    expect(failedShare(shape({ failedExecutions: 1, executions: 5000 }))).toBe('<0.1%')
    expect(failedShare(shape({ failedExecutions: 50, executions: 50 }))).toBe('100%')
    expect(fmtDuration(null)).toBe('—')
    expect(fmtDuration(2.5)).toBe('2.5 ms')
    expect(fmtDuration(1100)).toBe('1100 ms')
    expect(fmtDuration(12500)).toBe('12.5 s')
  })

  it('explains a missing percentile with the minimum it needs', () => {
    const few = shape({ executions: 94, percentiles: { p50: 3000, p95: null, p99: null } })
    expect(percentileHint(few, 'p50')).toBe('')
    expect(percentileHint(few, 'p95')).toBe('p95 needs at least 200 executions with a valid duration (this shape has 94)')
  })

  it('opens a representative execution as a sequence from its root start, with the library as origin', () => {
    const t = exampleTarget(shape(), 'p50')!
    expect(t.traceId).toBe('a'.repeat(32))
    expect(t.view).toBe('sequence')
    expect(t.window).toEqual({ from: String(1000000000000n - 60_000_000_000n), to: String(1000000000000n + 3_600_000_000_000n) })
    expect(t.origin).toEqual({ page: 'executions', label: 'the sequence library (POST /orders · p50 · 121 ms)' })
    expect(exampleTarget(shape(), 'p95')).toBeNull() // no kept example
    expect(exampleTarget(shape(), 'slowest')!.origin!.label).toContain('slowest · 3000 ms')
  })

  it('lists the other services after the entry service', () => {
    expect(servicesText(shape())).toBe('auth · orders · payments')
    expect(servicesText(shape(), 2)).toBe('auth · orders · +1')
  })

  it('lists distinct calls like the server builds shapes: caller -> callee, repeats counted, unknown parents named', () => {
    const spans = [
      span('a', '0000000000000000', 'gateway', 'POST /orders', 100),
      span('b', 'a', 'gateway', 'orders/Create', 110, 'client'),
      span('c', 'b', 'orders', 'Create', 120),
      span('d', 'c', 'orders', 'SELECT', 130, 'client'),
      span('e', 'c', 'orders', 'SELECT', 140, 'client'),
      span('f', 'zzzzzzzzzzzzzzzz', 'worker', 'late job', 105, 'consumer')
    ]
    expect(callsOf(spans)).toEqual([
      { caller: 'unknown caller', callee: 'worker · late job', kind: 'consumer', count: 1 },
      { caller: 'gateway · POST /orders', callee: 'gateway · orders/Create', kind: 'client', count: 1 },
      { caller: 'gateway · orders/Create', callee: 'orders · Create', kind: 'server', count: 1 },
      { caller: 'orders · Create', callee: 'orders · SELECT', kind: 'client', count: 2 }
    ])
    expect(callsOf([spans[0]])).toEqual([])
  })
})
