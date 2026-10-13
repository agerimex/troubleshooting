import { type Page, type Route } from '@playwright/test'

// A tiny in-test stand-in for log-analysis. Each test decides how view-spans,
// count-spans, search-logs and trace-report answer, and every request body is
// recorded.

export const ROOT = '0000000000000000'
export const TRACE = 'abcdefabcdefabcdefabcdefabcdef01'

export interface MockSpan {
  timeStamp: string
  name: string
  service: string
  traceId: string
  spanId: string
  parentSpanId: string
  msg: string
  tags: Record<string, string>[]
  serviceTags: Record<string, string>[]
  childSpanCount: number
  statusCode: number
  status: string
  statusMessage: string
  duration: number
  events: { timeUnixNano: string, name: string, attributes: Record<string, string> }[]
  links: { traceId: string, spanId: string, traceState: string, attributes: Record<string, string> }[]
}

export interface MockLog {
  timeUnixNano: string
  recordId: string
  traceId: string
  spanId: string
  severity: string
  service: string
  message: string
  attributes: Record<string, string>
}

let clock = 1760090000000000000n

export function span (name: string, overrides: Partial<MockSpan> = {}): MockSpan {
  clock += 1000000n
  return {
    timeStamp: clock.toString(), name, service: 'svc', traceId: TRACE, spanId: '00000000000000a0', parentSpanId: ROOT,
    msg: '', tags: [], serviceTags: [], childSpanCount: 0, statusCode: 0, status: 'unset', statusMessage: '',
    duration: 1000, events: [], links: [], ...overrides
  }
}

export const LOG_BASE = 1760090100000000000n

// logRecord's offsetMs is relative to LOG_BASE, so tests can list records out of time order.
export function logRecord (recordId: string, offsetMs: number, overrides: Partial<MockLog> = {}): MockLog {
  return {
    timeUnixNano: (LOG_BASE + BigInt(offsetMs) * 1000000n).toString(), recordId, traceId: TRACE, spanId: '',
    severity: 'INFO', service: 'svc', message: recordId, attributes: {}, ...overrides
  }
}

export type Body = Record<string, any>
export interface Reply { status?: number, spans?: MockSpan[], hasMore?: boolean, count?: number, error?: string, delayMs?: number }
export interface LogReply { status?: number, logs?: MockLog[], hasMore?: boolean, error?: string, delayMs?: number }

export interface SectionReply<T> {
  status?: 'ok' | 'unavailable' | 'limit' | 'error'
  error?: string
  records?: T[]
  truncated?: boolean
  limit?: number
  total?: number | null
}
export interface ReportReply {
  status?: number
  error?: string
  delayMs?: number
  spans?: SectionReply<MockSpan>
  logs?: SectionReply<MockLog>
  warnings?: { code: string, section: string, message: string }[]
}

export interface MockApi {
  shapeRequests: Body[]
  spanRequests: Body[]
  countRequests: Body[]
  logRequests: Body[]
  reportRequests: Body[]
  statsRequests: Body[]
  groupRequests: Body[]
  exampleRequests: Body[]
  compareRequests: Body[]
  problemRequests: Body[]
  ruleSaves: Body[]
}

async function fulfil (route: Route, reply: { status?: number, error?: string, delayMs?: number }, data: Record<string, unknown>) {
  if (reply.delayMs) {
    await new Promise((resolve) => setTimeout(resolve, reply.delayMs))
  }
  const status = reply.status ?? (reply.error ? 500 : 200)
  const body = reply.error ? { error: true, message: reply.error } : { error: false, message: 'success', data }
  await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
}

function section<T> (reply: SectionReply<T> | undefined) {
  const records = reply?.records ?? []
  const status = reply?.status ?? 'ok'
  const ok = status === 'ok'
  const total = reply?.total === undefined ? records.length : reply.total
  return {
    status,
    error: reply?.error,
    records: ok ? records : [],
    limit: reply?.limit ?? 1000,
    shown: ok ? records.length : 0,
    truncated: ok && (reply?.truncated ?? false),
    total: ok ? total : null,
    totalStatus: ok && total !== null ? 'ok' : 'unavailable'
  }
}

let generated = 1760090500000000000n

// A milestone 2 handler answers with data (the object inside the envelope's
// data, e.g. { Stats: ... }) or an error.
export interface DataReply { status?: number, error?: string, delayMs?: number, data?: Record<string, unknown> }

export interface MockOptions {
  stats?: (body: Body) => DataReply
  errorGroups?: (body: Body) => DataReply
  examples?: (body: Body) => DataReply
  // Sequence library; without it the library is empty.
  shapes?: (body: Body) => DataReply
  compare?: (body: Body) => DataReply
  analyses?: () => DataReply
  analysis?: (id: string) => DataReply
  problems?: (body: Body) => DataReply
  services?: () => DataReply
  rules?: () => DataReply
  saveRules?: (body: Body) => DataReply
  logs?: (body: Body) => LogReply
  // Without it a report gets its spans from the spans handler (as a trace
  // request) and no logs.
  report?: (body: Body) => ReportReply
}

export async function mockApi (page: Page, spans: (body: Body) => Reply, count: (body: Body) => Reply = () => ({ count: 0 }),
  options: MockOptions = {}): Promise<MockApi> {
  const api: MockApi = { shapeRequests: [], spanRequests: [], countRequests: [], logRequests: [], reportRequests: [], statsRequests: [], groupRequests: [], exampleRequests: [], compareRequests: [], problemRequests: [], ruleSaves: [] }
  const dataRoute = async (path: string, requests: Body[], handler: ((body: Body) => DataReply) | undefined) => {
    await page.route(path, async (route) => {
      const body = route.request().postDataJSON()
      requests.push(body)
      const reply = handler ? handler(body) : {}
      await fulfil(route, reply, reply.data ?? {})
    })
  }
  await dataRoute('**/api/v1/stats/calls', api.statsRequests, options.stats)
  await dataRoute('**/api/v1/stats/error-groups', api.groupRequests, options.errorGroups)
  await dataRoute('**/api/v1/stats/examples', api.exampleRequests, options.examples)
  await dataRoute('**/api/v1/stats/shapes', api.shapeRequests, options.shapes ?? (() => ({ data: { Library: { status: 'ok', shapes: [], truncated: false, notices: [], plan: [], contract: 'shapes-v1', generatedAt: '1' } } })))
  await dataRoute('**/api/v1/compare', api.compareRequests, options.compare)
  await dataRoute('**/api/v1/problems', api.problemRequests, options.problems)
  await page.route('**/api/v1/problems/services*', async (route) => {
    const reply = options.services ? options.services() : { data: { Services: [] } }
    await fulfil(route, reply, reply.data ?? {})
  })
  await page.route('**/api/v1/detection-rules', async (route) => {
    if (route.request().method() === 'GET') {
      const reply = options.rules ? options.rules() : { status: 404, error: 'no rules mocked' }
      await fulfil(route, reply, reply.data ?? {})
      return
    }
    const body = route.request().postDataJSON()
    api.ruleSaves.push(body)
    const reply = options.saveRules ? options.saveRules(body) : { status: 503, error: 'no save mocked' }
    // A rejected save may still carry data (field errors, the current revision).
    const status = reply.status ?? (reply.error ? 500 : 200)
    await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify({ error: !!reply.error, message: reply.error ?? 'success', data: reply.data ?? {} }) })
  })
  await page.route('**/api/v1/analyses', async (route) => {
    const reply = options.analyses ? options.analyses() : { data: { Analyses: [] } }
    await fulfil(route, reply, reply.data ?? {})
  })
  await page.route('**/api/v1/analyses/*', async (route) => {
    const id = route.request().url().split('/').pop() ?? ''
    const reply = options.analysis ? options.analysis(id) : { status: 404, error: 'analysis not found' }
    await fulfil(route, reply, reply.data ?? {})
  })
  await page.route('**/api/v1/view-spans', async (route) => {
    const body = route.request().postDataJSON()
    api.spanRequests.push(body)
    const reply = spans(body)
    await fulfil(route, reply, { Spans: reply.spans ?? [], HasMore: reply.hasMore ?? false })
  })
  await page.route('**/api/v1/count-spans', async (route) => {
    const body = route.request().postDataJSON()
    api.countRequests.push(body)
    const reply = count(body)
    await fulfil(route, reply, { Count: reply.count ?? 0 })
  })
  await page.route('**/api/v1/search-logs', async (route) => {
    const body = route.request().postDataJSON()
    api.logRequests.push(body)
    const reply = options.logs ? options.logs(body) : {}
    await fulfil(route, reply, { Logs: reply.logs ?? [], HasMore: reply.hasMore ?? false })
  })
  await page.route('**/api/v1/trace-report', async (route) => {
    const body = route.request().postDataJSON()
    api.reportRequests.push(body)
    const reply: ReportReply = options.report
      ? options.report(body)
      : (() => {
          const r = spans({ trace_id: body.trace_id, scope: 'all', time_from: body.time_from, time_to: body.time_to, rows_per_page: 1000 })
          return r.error
            ? { spans: { status: 'error' as const, error: r.error }, delayMs: r.delayMs }
            : { spans: { records: r.spans, truncated: r.hasMore }, delayMs: r.delayMs }
        })()
    const wanted: string[] = body.sections ?? ['spans', 'logs']
    generated += 1000000000n
    await fulfil(route, reply, {
      Report: {
        traceId: body.trace_id,
        timeFrom: body.time_from,
        timeTo: body.time_to,
        generatedAt: generated.toString(),
        spans: wanted.includes('spans') ? section(reply.spans) : undefined,
        logs: wanted.includes('logs') ? section(reply.logs) : undefined,
        warnings: (reply.warnings ?? []).filter((w) => wanted.includes(w.section))
      }
    })
  })
  return api
}
