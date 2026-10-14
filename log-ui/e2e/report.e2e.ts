import { readFile } from 'node:fs/promises'
import { expect, test, type Page } from '@playwright/test'
import { ROOT, TRACE, logRecord, mockApi, span, type MockLog, type MockSpan, type ReportReply } from './mockApi'

// UI checks of the combined trace and log report (features/milestone_1), as
// shown by the Timeline & Sequence viewer (product blueprint v1.3).
// Test titles start with the scenario tag they check.

const OTHER_TRACE = 'ffffffffffffffffffffffffffffff01'

const request = span('start_recording', {
  spanId: '00000000000000a0', service: 'recording-api', status: 'ok', statusCode: 2, duration: 150000,
  serviceTags: [{ key: 'service.version', value: '1.4.0' }], childSpanCount: 1
})
const upload = span('upload_recording', {
  spanId: '00000000000000a1', parentSpanId: '00000000000000a0', service: 'recording-worker', status: 'ok', statusCode: 2, duration: 900000
})

// M1-RPT-007: offsets out of order; log-queued has no span id.
const logs: MockLog[] = [
  logRecord('log-accepted', 10, { spanId: request.spanId, service: 'recording-api', message: 'Recording request accepted' }),
  logRecord('log-queued', 20, { service: 'recording-api', message: 'Recording queued for processing' }),
  logRecord('log-denied', 1000, { spanId: upload.spanId, service: 'recording-worker', severity: 'ERROR', message: 'Storage returned 403 AccessDenied' }),
  logRecord('log-upload-start', 200, { spanId: upload.spanId, service: 'recording-worker', message: 'Upload started' }),
  logRecord('log-retry', 400, { spanId: upload.spanId, service: 'recording-worker', severity: 'WARN', message: 'Upload retry 1 of 3' })
]

const full = (): ReportReply => ({ spans: { records: [request, upload] }, logs: { records: logs } })

// Every log of the trace, in the inspector's "All trace logs" scope.
const traceRows = (page: Page) => page.getByTestId('trace-log-row')
const traceMessages = (page: Page) => page.getByTestId('trace-log-message')
const spanNames = (page: Page) => page.getByTestId('timeline-name')
async function allLogs (page: Page) {
  await page.getByTestId('scope-trace').click()
}
async function selectSpan (page: Page, name: string) {
  await page.getByTestId('timeline-row').filter({ hasText: name }).click()
}

async function openReportFromLogAnalysis (page: Page, report: (body: Record<string, any>) => ReportReply, spans: MockSpan[] = [request]) {
  const api = await mockApi(page, () => ({ spans }), () => ({ count: spans.length }), { report })
  await page.goto('/')
  await page.getByTestId('open-trace').first().click()
  await expect(page.getByTestId('report')).toBeVisible()
  return api
}

test('M1-RPT-005 / NAV-EXP-003: an ERROR log under its span opens the execution with that log selected and the full context', async ({ page }) => {
  const api = await mockApi(page, () => ({ spans: [request] }), () => ({ count: 1 }), { report: full })
  await page.goto('/')
  // The log rows of a span appear inline when it is expanded (exact trace and span id).
  await page.locator('.p-treetable-toggler').first().click()
  await expect(page.getByTestId('log-row-message').filter({ visible: true })).toHaveText(['Recording request accepted'])
  await expect(page.getByTestId('log-group')).toContainText('Trace-level / unmatched-span logs (1)')
  const expandReport = api.reportRequests.at(-1)!
  expect(expandReport).toEqual({ trace_id: TRACE, time_from: api.spanRequests[0].time_from, time_to: api.spanRequests[0].time_to })

  const logRow = page.locator('tr', { hasText: 'Recording request accepted' })
  await logRow.getByTestId('row-actions').click()
  await page.getByTestId('action-open-timeline').click()
  await expect(page.getByTestId('trace-id')).toHaveText(TRACE)
  // Only the trace and the searched window go to the report: no filter carries over.
  expect(api.reportRequests.at(-1)).toEqual(expandReport)
  await expect(page.getByTestId('inspector-span')).toHaveText('start_recording')
  await expect(page.getByTestId('log-detail-message')).toHaveText('Recording request accepted')
  await allLogs(page)
  await expect(traceMessages(page)).toHaveCount(5)
})

test('M1-RPT-007: selecting a span shows its logs in time order; all trace logs keep the rest', async ({ page }) => {
  const api = await openReportFromLogAnalysis(page, full)
  await allLogs(page)
  await expect(traceMessages(page)).toHaveText([
    'Recording request accepted', 'Recording queued for processing', 'Upload started', 'Upload retry 1 of 3', 'Storage returned 403 AccessDenied'
  ])

  await selectSpan(page, 'upload_recording')
  await page.getByTestId('view-list').click()
  await expect(page.getByTestId('span-log-message')).toHaveText(['Upload started', 'Upload retry 1 of 3', 'Storage returned 403 AccessDenied'])
  const meta = page.getByTestId('inspector-span-meta')
  await expect(page.getByTestId('inspector-span')).toHaveText('upload_recording')
  await expect(meta).toContainText('recording-worker')
  await expect(meta).toContainText('900.000 ms')
  await expect(meta).toContainText('OK')
  // The parent-child context stays in the same report.
  await expect(spanNames(page)).toHaveText(['start_recording', 'upload_recording'])

  await allLogs(page)
  await expect(traceRows(page)).toHaveCount(5)
  await expect(traceRows(page).nth(1).getByTestId('trace-log-association')).toHaveText('No span identifier recorded')
  await expect(traceRows(page).nth(0).getByTestId('trace-log-association')).toHaveText('Span start_recording')
  // Selecting never re-queried: same trace, same window.
  expect(api.reportRequests).toHaveLength(1)
})

test('M1-RPT-001/003/004: span status and log severity are distinct; a missing version is unavailable', async ({ page }) => {
  await openReportFromLogAnalysis(page, full)
  await expect(page.getByTestId('timeline-meta')).toContainText(['OK', 'OK'])
  await expect(page.getByTestId('inspector-span-version')).toHaveText('version 1.4.0')
  await selectSpan(page, 'upload_recording')
  await expect(page.getByTestId('inspector-span-version')).toHaveText('version unavailable')
  await allLogs(page)
  await expect(page.getByTestId('trace-log-severity')).toHaveText(['INFO', 'INFO', 'INFO', 'WARN', 'ERROR'])
  await expect(page.getByTestId('completeness')).toContainText('Complete')
  await expect(page.getByTestId('completeness')).toContainText('does not prove every event was instrumented')
})

test('M1-CMP-001 / M1-REL-005: an unavailable log source keeps the spans, is not zero, and Retry recovers', async ({ page }) => {
  let logsDown = true
  const api = await openReportFromLogAnalysis(page, () => ({
    spans: { records: [request, upload] },
    logs: logsDown ? { status: 'unavailable', error: 'the database is unavailable: dial tcp: connection refused' } : { records: logs }
  }))
  await expect(spanNames(page)).toHaveText(['start_recording', 'upload_recording'])
  await expect(page.getByTestId('logs-summary')).toContainText('Logs unavailable')
  await expect(page.getByTestId('logs-summary')).not.toContainText('0 logs')
  await expect(page.getByTestId('no-logs')).toHaveCount(0)
  await expect(page.getByTestId('inspector-logs-unavailable')).toBeVisible()
  await expect(page.getByTestId('completeness')).toContainText('Incomplete')

  logsDown = false
  await page.getByTestId('retry-logs').click()
  await expect(page.getByTestId('logs-summary')).toContainText('5 logs shown · complete')
  await allLogs(page)
  await expect(traceRows(page)).toHaveCount(5)
  await expect(page.getByTestId('completeness')).toContainText('Complete')
  const [first, retry] = [api.reportRequests[0], api.reportRequests.at(-1)!]
  expect(retry).toEqual({ trace_id: first.trace_id, time_from: first.time_from, time_to: first.time_to, sections: ['logs'] })
})

test('M1-CMP-001: an unavailable trace source keeps the logs', async ({ page }) => {
  await openReportFromLogAnalysis(page, () => ({ spans: { status: 'unavailable', error: 'connection refused' }, logs: { records: logs } }))
  await expect(page.getByTestId('spans-summary')).toContainText('Spans unavailable')
  await expect(page.getByTestId('spans-unavailable')).toBeVisible()
  await expect(page.getByTestId('no-spans')).toHaveCount(0)
  await expect(traceRows(page)).toHaveCount(5)
  await expect(traceRows(page).nth(0).getByTestId('trace-log-association')).toContainText('span evidence unavailable')
  await expect(page.getByTestId('origin-missing')).toContainText('span source is unavailable')
  await expect(page.getByTestId('completeness')).toContainText('Incomplete')
})

test('M1-CMP-002: a successful empty log search says so', async ({ page }) => {
  await openReportFromLogAnalysis(page, () => ({ spans: { records: [request] }, logs: { records: [] } }))
  await expect(page.getByTestId('no-logs')).toHaveText('No correlated logs found within the selected window')
  await expect(page.getByTestId('logs-summary')).not.toContainText('unavailable')
})

test('M1-CMP-003: a limit shows the records shown and whether the section is truncated', async ({ page }) => {
  const three = [logs[0], logs[1], logs[3]]
  await openReportFromLogAnalysis(page, () => ({ spans: { records: [request, upload], limit: 3 }, logs: { records: three, limit: 3, truncated: true, total: 4 } }))
  await expect(page.getByTestId('logs-summary')).toContainText('3 logs shown · truncated: more logs match (limit 3) · 4 matching')
  await expect(page.getByTestId('spans-summary')).toContainText('2 spans shown · complete')
  await expect(page.getByTestId('limitation')).toContainText(['Logs are truncated: 3 shown (limit 3); more match.'])
})

test('M1-CMP-006: a window older than the log retention warns that logs may have expired', async ({ page }) => {
  await openReportFromLogAnalysis(page, () => ({
    spans: { records: [request] }, logs: { records: [] },
    warnings: [{ code: 'retention', section: 'logs', message: 'related logs may have expired under the configured retention policy (72 h). Their absence is not proof that no error was logged.' }]
  }))
  await expect(page.getByTestId('report-warning')).toContainText('may have expired')
  await expect(spanNames(page)).toHaveText(['start_recording'])
})

test('M1-CMP-007: a timed-out section offers retry and the report is incomplete', async ({ page }) => {
  await openReportFromLogAnalysis(page, () => ({
    spans: { status: 'limit', error: 'query limit exceeded: the query took longer than 10s; narrow the time window or filters' },
    logs: { records: logs }
  }))
  await expect(page.getByTestId('spans-error')).toContainText('took longer than 10s')
  await expect(page.getByTestId('retry-spans')).toBeVisible()
  await expect(page.getByTestId('completeness')).toContainText('Incomplete')
})

test('M1-CMP-008: a failed total is unavailable, never the previous one', async ({ page }) => {
  let call = 0
  await openReportFromLogAnalysis(page, () => (++call === 1
    ? { spans: { records: [request] }, logs: { records: logs, total: 12, truncated: true, limit: 5 } }
    : { spans: { records: [request] }, logs: { records: logs.slice(0, 3), total: null } }))
  await expect(page.getByTestId('logs-summary')).toContainText('12 matching')
  await page.getByTestId('regenerate').click()
  await expect(page.getByTestId('logs-summary')).toContainText('3 logs shown')
  await expect(page.getByTestId('logs-summary')).toContainText('matching total unavailable')
  await expect(page.getByTestId('logs-summary')).not.toContainText('12')
})

test('M1-COR-006: logs without spans stay available and no tree is invented', async ({ page }) => {
  await openReportFromLogAnalysis(page, () => ({ spans: { records: [] }, logs: { records: logs } }))
  await expect(page.getByTestId('no-spans')).toContainText('No spans were found for this trace within the searched window.')
  await expect(page.getByTestId('trace-timeline')).toHaveCount(0)
  await expect(traceRows(page)).toHaveCount(5)
  await expect(traceRows(page).nth(0).getByTestId('trace-log-association')).toHaveText('Referenced span not found within the window')
})

test('M1-RPT-008 / M1-COR-005: the scope is one trace; a linked trace is a reference, not inspected', async ({ page }) => {
  const linked = { ...upload, events: [{ timeUnixNano: upload.timeStamp, name: 'exception', attributes: { 'exception.type': 'AccessDenied', 'exception.stacktrace': 'at upload()' } }],
    links: [{ traceId: OTHER_TRACE, spanId: '00000000000000f1', traceState: '', attributes: {} }] }
  const api = await openReportFromLogAnalysis(page, () => ({ spans: { records: [request, linked] }, logs: { records: [] } }))
  await expect(page.getByTestId('scope-boundary')).toContainText('does not establish complete coverage or the final outcome of a business operation')
  await expect(page.getByTestId('report-scope')).toContainText('one trace')
  await expect(page.getByTestId('linked-trace')).toContainText(OTHER_TRACE)
  await expect(page.getByTestId('linked-trace')).toContainText('not inspected')
  // The span event is not an ingested log record.
  await expect(page.getByTestId('no-logs')).toBeVisible()
  expect(api.reportRequests.map((r) => r.trace_id)).toEqual([TRACE])

  await page.getByTestId('open-linked-report').click()
  await expect(page.getByTestId('trace-id')).toHaveText(OTHER_TRACE)
})

test('M1-REL-004: a slow report for trace A cannot replace the report for trace B', async ({ page }) => {
  const traceB = 'b'.repeat(32)
  const rootA = span('root A', { traceId: TRACE })
  const rootB = span('root B', { traceId: traceB, spanId: '00000000000000b0' })
  await mockApi(page, () => ({ spans: [rootA, rootB] }), () => ({ count: 2 }), {
    report: (body) => body.trace_id === TRACE
      ? { delayMs: 1500, spans: { records: [rootA] }, logs: { records: [logRecord('log-A', 1, { message: 'from A' })] } }
      : { spans: { records: [rootB] }, logs: { records: [logRecord('log-B', 1, { traceId: traceB, message: 'from B' })] } }
  })
  await page.goto('/')
  await page.getByTestId('open-trace').first().click()
  await page.getByTestId('return-origin').click()
  await page.getByTestId('open-trace').nth(1).click()
  await expect(page.getByTestId('trace-id')).toHaveText(traceB)
  await allLogs(page)
  await expect(traceMessages(page)).toHaveText(['from B'])
  await page.waitForTimeout(2000) // A's response has arrived by now
  await expect(page.getByTestId('trace-id')).toHaveText(traceB)
  await expect(traceMessages(page)).toHaveText(['from B'])
  await expect(spanNames(page)).toHaveText(['root B'])
})

async function exported (page: Page, testId: string): Promise<string> {
  const [download] = await Promise.all([page.waitForEvent('download'), page.getByTestId(testId).click()])
  return readFile((await download.path())!, 'utf8')
}

test('M1-EXP-001 / M1-EXP-005: a JSON export from a focused view keeps the whole generated report', async ({ page }) => {
  const linked = { ...upload, links: [{ traceId: OTHER_TRACE, spanId: '00000000000000f1', traceState: '', attributes: {} }] }
  await openReportFromLogAnalysis(page, () => ({ spans: { records: [request, linked] }, logs: { records: logs } }))
  await selectSpan(page, 'upload_recording')
  await page.getByTestId('view-list').click()
  await expect(page.getByTestId('log-list-row')).toHaveCount(3)

  const json = JSON.parse(await exported(page, 'export-json'))
  expect(json.scope).toMatchObject({ kind: 'single-trace', traceId: TRACE })
  expect(json.spans.records.map((s: any) => s.name)).toEqual(['start_recording', 'upload_recording'])
  expect(json.logs.records.map((l: any) => l.recordId)).toEqual(['log-accepted', 'log-queued', 'log-upload-start', 'log-retry', 'log-denied'])
  expect(json.logs.records[1]).toMatchObject({ spanId: null, association: 'no-span-id' })
  expect(json.links).toEqual([expect.objectContaining({ traceId: OTHER_TRACE, inspected: false })])
  expect(json.generatedAt).toMatch(/^\d+$/)
  expect(json.complete).toBe(true)
  // The view on screen is recorded, not applied to the evidence.
  expect(json.presentation.view).toMatchObject({ view: 'timeline', selected: TRACE + ':' + upload.spanId })
})

test('M1-EXP-002: an export keeps an unavailable source and a truncated one', async ({ page }) => {
  await openReportFromLogAnalysis(page, () => ({ spans: { records: [request], truncated: true, limit: 1, total: 2 }, logs: { status: 'unavailable', error: 'down' } }))
  const json = JSON.parse(await exported(page, 'export-json'))
  expect(json.complete).toBe(false)
  expect(json.spans.truncated).toBe(true)
  expect(json.logs.status).toBe('unavailable')
  expect(json.logs.total).toBeNull()
  const html = await exported(page, 'export-html')
  expect(html).toContain('Incomplete')
  expect(html).toContain('Logs unavailable')
  expect(html).not.toContain('No correlated logs found')
})

test('M1-EXP-003 / RPT-DIR-009: export uses the displayed revision; only regeneration adds late evidence', async ({ page }) => {
  let late = false
  await openReportFromLogAnalysis(page, () => ({
    spans: { records: [request] },
    logs: { records: late ? [logs[0], logRecord('log-late', 500, { message: 'arrived later' })] : [logs[0]] }
  }))
  await allLogs(page)
  await expect(traceRows(page)).toHaveCount(1)
  late = true
  const before = JSON.parse(await exported(page, 'export-json'))
  expect(before.logs.records.map((l: any) => l.recordId)).toEqual(['log-accepted'])
  await page.getByTestId('regenerate').click()
  await allLogs(page)
  await expect(traceRows(page)).toHaveCount(2)
  const after = JSON.parse(await exported(page, 'export-json'))
  expect(after.logs.records.map((l: any) => l.recordId)).toEqual(['log-accepted', 'log-late'])
  expect(after.generatedAt).not.toBe(before.generatedAt)
})

test('M1-EXP-004: recorded message text stays text in the HTML export', async ({ page, context }) => {
  const evil = logRecord('log-evil', 5, { spanId: request.spanId, message: "<script>alert('recorded-log-text')</script>" })
  await openReportFromLogAnalysis(page, () => ({ spans: { records: [request] }, logs: { records: [evil] } }))
  await allLogs(page)
  await expect(traceMessages(page)).toHaveText("<script>alert('recorded-log-text')</script>")
  const html = await exported(page, 'export-html')
  expect(html).not.toContain('<script')

  const viewer = await context.newPage()
  let dialogs = 0
  viewer.on('dialog', async (d) => { dialogs++; await d.dismiss() })
  await viewer.setContent(html)
  await expect(viewer.getByText("<script>alert('recorded-log-text')</script>").first()).toBeVisible()
  await expect(viewer.getByText(TRACE).first()).toBeVisible()
  expect(dialogs).toBe(0)
})

test('M1-CMP-004/005: the report uses the window; widening regenerates with the parent', async ({ page }) => {
  const child = span('async child', { spanId: '00000000000000c1', parentSpanId: '00000000000000c0' })
  const parent = span('parent', { spanId: '00000000000000c0', parentSpanId: ROOT })
  const api = await openReportFromLogAnalysis(page, (body) => {
    const widened = BigInt(body.time_to) - BigInt(body.time_from) > 2n * 3600n * 1000000000n
    return { spans: { records: widened ? [parent, child] : [child] }, logs: { records: [logRecord('after-parent', 50, { spanId: child.spanId })] } }
  }, [child])
  await expect(page.getByTestId('timeline-parent-missing')).toBeVisible()
  await page.getByTestId('widen-window').click()
  await expect(page.getByTestId('timeline-parent-missing')).toHaveCount(0)
  await expect(spanNames(page)).toHaveText(['parent', 'async child'])
  // The selection survives the new revision.
  await expect(page.getByTestId('inspector-span')).toHaveText('async child')
  await allLogs(page)
  await expect(traceRows(page)).toHaveCount(1)
  expect(api.reportRequests).toHaveLength(2)
})
