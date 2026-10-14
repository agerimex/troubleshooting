import { describe, expect, it } from 'vitest'
import {
  PAGES, initialView, investigationHash, investigationLink, logsOfSpanId, parseRoute, traceLogsOf, unattachedLogs
} from '@/services/navigation'
import { type Span } from '@/types/filter'
import { type LogRecord, type Report, type ReportSection } from '@/types/report'

const TRACE = 'abcdefabcdefabcdefabcdefabcdef01'

function span (spanId: string, parentSpanId = '0000000000000000', extra: Partial<Span> = {}): Span {
  return {
    timeStamp: '1000000000', name: 'op-' + spanId.slice(-2), service: 'svc', traceId: TRACE, spanId, parentSpanId, msg: '', tags: [], serviceTags: [],
    childSpanCount: 0, statusCode: 0, status: 'ok', statusMessage: '', duration: 2500, events: [], links: [], ...extra
  } as Span
}
function log (recordId: string, spanId: string, t: number): LogRecord {
  return { timeUnixNano: String(t), recordId, traceId: TRACE, spanId, severity: 'INFO', service: 'svc', message: recordId, attributes: {} }
}
function section<T> (records: T[], extra: Partial<ReportSection<T>> = {}): ReportSection<T> {
  return { status: 'ok', records, limit: 1000, shown: records.length, truncated: false, total: records.length, totalStatus: 'ok', retrievedAt: '1', ...extra }
}
function report (spans: Span[], logs: LogRecord[], extra: { spans?: Partial<ReportSection<Span>>, logs?: Partial<ReportSection<LogRecord>> } = {}): Report {
  return { traceId: TRACE, timeFrom: '0', timeTo: '9', generatedAt: '5', spans: section(spans, extra.spans), logs: section(logs, extra.logs), warnings: [] } as unknown as Report
}

describe('navigation', () => {
  it('has the five destinations in the blueprint order, Log Analysis first', () => {
    expect(PAGES.map((p) => p.label)).toEqual(['Log Analysis', 'Timeline & Sequence', 'Problems', 'Releases', 'Settings'])
  })

  it('opens Timeline unless a view was chosen or saved', () => {
    expect(initialView(undefined, null)).toBe('timeline')
    expect(initialView(undefined, 'garbage')).toBe('timeline')
    expect(initialView(undefined, 'sequence')).toBe('sequence')
    expect(initialView('timeline', 'sequence')).toBe('timeline')
  })

  it('a link carries identifiers only and round-trips', () => {
    const t = { traceId: TRACE, window: { from: '100', to: '200' }, spanId: '00000000000000a1', recordId: 'fx-1:a', view: 'sequence' as const,
      origin: { page: 'logs' as const, label: 'Log Analysis' } }
    const hash = investigationHash(t)
    expect(hash).toBe('#/executions?trace=' + TRACE + '&from=100&to=200&span=00000000000000a1&log=fx-1%3Aa&view=sequence')
    expect(hash).not.toContain('Log Analysis')
    expect(investigationLink('http://host/app/#/logs', t)).toBe('http://host/app/' + hash)
    const back = parseRoute(hash)
    expect(back).toEqual({ page: 'executions', error: '', target: { traceId: TRACE, window: { from: '100', to: '200' }, spanId: '00000000000000a1', recordId: 'fx-1:a', view: 'sequence' } })
  })

  it('a record id that is not a plain identifier is left out of the link', () => {
    expect(investigationHash({ traceId: TRACE, window: { from: '1', to: '2' }, recordId: 'has spaces <b>' })).not.toContain('log=')
  })

  it('keeps the legacy investigate route and the timing view', () => {
    const r = parseRoute('#/investigate?trace=' + TRACE.toUpperCase() + '&from=1&to=2&view=timing')
    expect(r.page).toBe('executions')
    expect(r.target).toMatchObject({ traceId: TRACE, view: 'timeline' })
  })

  it('rejects an invalid link without guessing a trace', () => {
    expect(parseRoute('#/executions?trace=123&from=1&to=2')).toMatchObject({ page: 'executions', target: null })
    expect(parseRoute('#/executions?trace=123&from=1&to=2').error).toContain('32 hex digits')
    expect(parseRoute('#/executions?trace=' + TRACE + '&from=5&to=5').target).toBeNull()
    expect(parseRoute('#/executions?trace=' + TRACE + '&from=1&to=2&span=xyz').target?.spanId).toBeUndefined()
  })

  it('maps pages and unknown hashes', () => {
    expect(parseRoute('').page).toBe('logs')
    expect(parseRoute('#/releases').page).toBe('releases')
    expect(parseRoute('#/architecture').page).toBe('logs')
  })

  it('attaches a log only to the exact span that recorded it', () => {
    const t = traceLogsOf(report([span('00000000000000a0'), span('00000000000000a1', '00000000000000a0')], [
      log('b', '00000000000000a1', 30), log('a', '00000000000000a1', 10), log('root', '00000000000000a0', 5), log('trace', '', 7), log('gone', '00000000000000ee', 8)
    ]))
    expect(logsOfSpanId(t, '00000000000000a1').map((l) => l.recordId)).toEqual(['a', 'b'])
    expect(logsOfSpanId(t, '').length).toBe(0)
    expect(unattachedLogs(t)).toEqual([
      { log: expect.objectContaining({ recordId: 'trace' }), reason: 'trace-level: no span recorded' },
      { log: expect.objectContaining({ recordId: 'gone' }), reason: 'span not found in this trace and window' }
    ])
  })

  it('says "not loaded", not "not found", when the spans were truncated or unavailable', () => {
    const truncated = traceLogsOf(report([span('00000000000000a0')], [log('gone', '00000000000000ee', 8)], { spans: { truncated: true } }))
    expect(unattachedLogs(truncated)[0].reason).toBe('span not among the loaded spans')
    const down = traceLogsOf(report([], [log('x', '00000000000000a0', 1)], { spans: { status: 'unavailable' } }))
    expect(unattachedLogs(down)[0].reason).toBe('span not among the loaded spans')
  })

  it('an unavailable log source is unavailable, not empty', () => {
    const t = traceLogsOf(report([span('00000000000000a0')], [], { logs: { status: 'unavailable', error: 'down' } }))
    expect(t.status).toBe('unavailable')
    expect(t.error).toBe('down')
  })

})
