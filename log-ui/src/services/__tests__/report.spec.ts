import { describe, expect, it } from 'vitest'
import {
  NO_LOGS_TEXT, SCOPE_BOUNDARY_TEXT, association, completeness, escapeHtml, exportHtml, exportJson, fromResponse,
  logsOfSpan, mergeSections, reportLinks, sectionSummary, serviceVersion, sortedLogs, spanOrder
} from '@/services/report'
import { ROOT_PARENT } from '@/services/spanTree'
import { type Span } from '@/types/filter'
import { type LogRecord, type Report, type ReportResponse, type ReportSection } from '@/types/report'

const traceA = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
const traceB = 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
const base = 1760000000000000000n

function span (spanId: string, parentSpanId: string, overrides: Partial<Span> = {}): Span {
  return {
    timeStamp: base.toString(), name: 'op-' + spanId, service: 'recording-api', traceId: traceA, spanId, parentSpanId,
    msg: '', tags: [], serviceTags: [], childSpanCount: 0, statusCode: 2, status: 'ok', statusMessage: '',
    duration: 150000, events: [], links: [], ...overrides
  }
}

function log (recordId: string, spanId: string, offsetMs: number, overrides: Partial<LogRecord> = {}): LogRecord {
  return {
    timeUnixNano: (base + BigInt(offsetMs) * 1000000n).toString(), recordId, traceId: traceA, spanId,
    severity: 'INFO', service: 'recording-worker', message: recordId, attributes: {}, ...overrides
  }
}

function section<T> (records: T[], overrides: Partial<ReportSection<T>> = {}): Omit<ReportSection<T>, 'retrievedAt'> {
  return { status: 'ok', records, limit: 1000, shown: records.length, truncated: false, total: records.length, totalStatus: 'ok', ...overrides }
}

const request = span('1000000000000001', ROOT_PARENT, { name: 'start_recording', serviceTags: [{ key: 'service.version', value: '1.4.0' }] })
const upload = span('1000000000000002', '1000000000000001', { name: 'upload_recording', service: 'recording-worker', status: 'error', statusCode: 1 })

// The M1-RPT-007 records, deliberately not in time order.
const logs = [
  log('log-accepted', request.spanId, 10),
  log('log-queued', '', 20),
  log('log-denied', upload.spanId, 1000, { severity: 'ERROR', message: 'Storage returned 403 AccessDenied' }),
  log('log-upload-start', upload.spanId, 200),
  log('log-retry', upload.spanId, 400, { severity: 'WARN' })
]

function response (overrides: Partial<ReportResponse> = {}): ReportResponse {
  return {
    traceId: traceA, timeFrom: (base - 60000000000n).toString(), timeTo: (base + 60000000000n).toString(),
    generatedAt: (base + 120000000000n).toString(), spans: section([request, upload]), logs: section(logs), warnings: [], ...overrides
  }
}

function report (overrides: Partial<ReportResponse> = {}): Report {
  return fromResponse(response(overrides))!
}

describe('log associations (M1-RPT-007, M1-COR-003)', () => {
  it('focuses a span on its own logs in recorded time order', () => {
    expect(logsOfSpan(report(), upload).map((l) => l.recordId)).toEqual(['log-upload-start', 'log-retry', 'log-denied'])
  })

  it('shows all logs in recorded time order', () => {
    expect(sortedLogs(logs).map((l) => l.recordId)).toEqual(['log-accepted', 'log-queued', 'log-upload-start', 'log-retry', 'log-denied'])
  })

  it('keeps a log without a span id at trace level', () => {
    const a = association(logs[1], report())
    expect(a.kind).toBe('no-span-id')
    expect(a.label).toBe('No span identifier recorded')
    expect(logsOfSpan(report(), request).map((l) => l.recordId)).toEqual(['log-accepted'])
  })

  it('never attaches a log to a different span by its time', () => {
    const orphan = log('log-context', 'ffffffffffffffff', 200)
    expect(association(orphan, report()).label).toBe('Referenced span not found within the window')
    expect(association(orphan, report({ spans: section([request, upload], { truncated: true }) })).kind).toBe('span-not-loaded')
    expect(association(orphan, report({ spans: section([], { status: 'unavailable', error: 'down' }) })).kind).toBe('spans-unavailable')
  })

  it('matches the span of the same trace only (M1-COR-001)', () => {
    const other = span(upload.spanId, ROOT_PARENT, { traceId: traceB })
    const r = report({ spans: section([request, other]) })
    expect(association(logs[2], r).kind).toBe('span-not-found')
  })
})

describe('completeness (M1-CMP-001, -003, -008)', () => {
  it('is complete only when both sources returned everything', () => {
    expect(completeness(report()).complete).toBe(true)
    const failed = completeness(report({ logs: section([], { status: 'unavailable', error: 'connection refused', total: null, totalStatus: 'unavailable' }) }))
    expect(failed.complete).toBe(false)
    expect(failed.reasons[0]).toContain('Logs evidence is unavailable')
    expect(completeness(report({ spans: section([request, upload], { truncated: true, limit: 2 }) })).complete).toBe(false)
  })

  it('labels numbers as records shown and never shows a failed source as zero', () => {
    expect(sectionSummary(section([1, 2, 3], { limit: 3 }) as ReportSection<number>, 'logs')).toBe('3 logs shown · complete · 3 matching')
    expect(sectionSummary(section([1, 2, 3], { limit: 3, truncated: true, total: 4 }) as ReportSection<number>, 'spans'))
      .toBe('3 spans shown · truncated: more spans match (limit 3) · 4 matching')
    expect(sectionSummary(section([1, 2, 3], { total: null, totalStatus: 'unavailable' }) as ReportSection<number>, 'logs'))
      .toContain('matching total unavailable')
    const down = sectionSummary(section([], { status: 'unavailable', total: null, totalStatus: 'unavailable' }) as ReportSection<number>, 'logs')
    expect(down).toContain('unavailable')
    expect(down).not.toMatch(/\b0 logs/)
  })
})

describe('retry (M1-REL-005)', () => {
  it('replaces only the retried section and keeps the trace and window', () => {
    const before = report({ logs: section([], { status: 'unavailable', error: 'down' }) })
    const after = mergeSections(before, { ...response({ spans: undefined }), generatedAt: (base + 999n).toString() })
    expect(after.logs.status).toBe('ok')
    expect(after.logs.records).toHaveLength(5)
    expect(after.logs.retrievedAt).toBe((base + 999n).toString())
    expect(after.spans).toBe(before.spans)
    expect([after.traceId, after.timeFrom, after.timeTo, after.generatedAt]).toEqual([before.traceId, before.timeFrom, before.timeTo, before.generatedAt])
  })
})

describe('span metadata', () => {
  it('shows a missing version as unavailable, not invented (M1-RPT-004)', () => {
    expect(serviceVersion(request)).toBe('1.4.0')
    expect(serviceVersion(upload)).toBeNull()
  })

  it('lists links to other traces as not inspected (M1-RPT-008, M1-COR-005)', () => {
    const linked = span('1000000000000003', request.spanId, { links: [{ traceId: traceB, spanId: '2000000000000001', traceState: '', attributes: {} }] })
    const links = reportLinks(report({ spans: section([request, linked]) }))
    expect(links).toEqual([{ fromSpanId: linked.spanId, fromSpanName: linked.name, traceId: traceB, spanId: '2000000000000001', attributes: {}, inspected: false }])
  })

  it('orders spans by parent with depth', () => {
    expect(spanOrder([upload, request]).map((o) => [o.span.spanId, o.depth])).toEqual([[request.spanId, 0], [upload.spanId, 1]])
  })
})

describe('export (M1-EXP-001, -002, -004, -005)', () => {
  const exportedAt = new Date(1760000300000)

  it('JSON carries the full report, its scope and its limitations', () => {
    const json = exportJson(report(), exportedAt)
    expect(json.scope).toMatchObject({ kind: 'single-trace', traceId: traceA, statement: SCOPE_BOUNDARY_TEXT })
    expect(json.generatedAt).toBe(report().generatedAt)
    expect(json.spans.records.map((s) => s.spanId)).toEqual([request.spanId, upload.spanId])
    expect(json.spans.records[0].serviceVersion).toBe('1.4.0')
    expect(json.spans.records[1].serviceVersion).toBeNull()
    expect(json.logs.records.map((l) => l.recordId)).toEqual(['log-accepted', 'log-queued', 'log-upload-start', 'log-retry', 'log-denied'])
    expect(json.logs.records[1]).toMatchObject({ spanId: null, association: 'no-span-id' })
    expect(json.spans.source).toBe('trace')
    expect(json.logs.source).toBe('log')
    expect(json.complete).toBe(true)
  })

  it('keeps an unavailable source distinct from an empty one', () => {
    const json = exportJson(report({
      logs: section([], { status: 'unavailable', error: 'connection refused', total: null, totalStatus: 'unavailable' }),
      spans: section([request, upload], { truncated: true, limit: 2, total: 3 })
    }), exportedAt)
    expect(json.complete).toBe(false)
    expect(json.logs.status).toBe('unavailable')
    expect(json.logs.total).toBeNull()
    expect(json.spans.truncated).toBe(true)
    expect(json.limitations).toHaveLength(2)

    const html = exportHtml(report({ logs: section([], { status: 'unavailable', error: 'connection refused' }) }), exportedAt)
    expect(html).toContain('Incomplete')
    expect(html).toContain('Logs unavailable (source unavailable)')
    expect(html).not.toContain(NO_LOGS_TEXT)
    const empty = exportHtml(report({ logs: section([]) }), exportedAt)
    expect(empty).toContain(NO_LOGS_TEXT)
  })

  it('HTML treats recorded text as text and has no scripts', () => {
    const evil = log('log-evil', upload.spanId, 5, { message: "<script>alert('recorded-log-text')</script>" })
    const html = exportHtml(report({ logs: section([evil]) }), exportedAt)
    expect(html).not.toContain('<script')
    expect(html).toContain('&lt;script&gt;alert(&#39;recorded-log-text&#39;)&lt;/script&gt;')
    expect(html).toContain("Content-Security-Policy\" content=\"default-src 'none'")
    expect(escapeHtml('"&<>\'')).toBe('&quot;&amp;&lt;&gt;&#39;')
  })

  it('HTML shows the scope, span relationships, events apart from logs, and links as not inspected', () => {
    const failed = { ...upload, events: [{ timeUnixNano: base.toString(), name: 'exception', attributes: { 'exception.type': 'AccessDenied', 'exception.stacktrace': 'at upload()' } }],
      links: [{ traceId: traceB, spanId: '2000000000000001', traceState: '', attributes: {} }] }
    const html = exportHtml(report({ spans: section([request, failed]) }), exportedAt)
    expect(html).toContain(traceA)
    expect(html).toContain('Span events (source: traces, not log records)')
    expect(html).toContain('at upload()')
    expect(html).toMatch(new RegExp(traceB + '.*not inspected'))
    expect(html).toContain('padding-left:1.75rem') // upload is a child of request
  })
})
