import { expect, test, type Page } from '@playwright/test'
import { TRACE, logRecord, mockApi, span, type ReportReply } from './mockApi'

// UI checks of the product blueprint's navigation (product-blueprint-v1.3):
// five destinations, Log Analysis with inline logs and row actions, the
// execution list of Timeline & Sequence, and investigation links.
// Titles start with the scenario tag they check.

const TRACE_B = 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
const root = span('publish_recording', { spanId: '00000000000000a0', service: 'recording-api', status: 'ok', statusCode: 2, duration: 240000, childSpanCount: 1,
  serviceTags: [{ key: 'service.version', value: '1.1.0' }] })
const child = span('upload_recording', { spanId: '00000000000000a1', parentSpanId: root.spanId, service: 'recording-api', status: 'ok', statusCode: 2, duration: 200000 })
// A second trace with the same shape: a separate execution.
const rootB = span('publish_recording', { traceId: TRACE_B, spanId: '00000000000000a0', service: 'recording-api', status: 'error', statusCode: 1, duration: 300000 })

const logs = [
  logRecord('accepted', 3, { spanId: root.spanId, service: 'recording-api', message: 'Recording request accepted' }),
  logRecord('denied', 215, { spanId: child.spanId, service: 'recording-api', severity: 'ERROR', message: 'Upload failed: AccessDenied' }),
  logRecord('started', 26, { spanId: child.spanId, service: 'recording-api', message: 'Upload started' }),
  logRecord('audit', 239, { service: 'recording-api', severity: 'WARN', message: 'Trace-level audit event' }),
  logRecord('ghost', 100, { spanId: '00000000000000ee', service: 'recording-api', message: 'From an unknown span' })
]
const report = (): ReportReply => ({ spans: { records: [root, child] }, logs: { records: logs } })

async function openLogAnalysis (page: Page) {
  const api = await mockApi(page, (body) => {
    if (body.parent_id) {
      return { spans: body.parent_id === root.spanId ? [child] : [] }
    }
    if (body.trace_id) {
      return { spans: [root, child] }
    }
    return { spans: body.status === 'ok' ? [root] : [root, rootB] }
  }, () => ({ count: 2 }), { report })
  await page.goto('/')
  return api
}

const visibleRows = (page: Page, testId: string) => page.getByTestId(testId).filter({ visible: true })

test('NAV-EXP-001 / NAV-EXP-002 / RPT-DIR-001: five destinations in order; Log Analysis is the default', async ({ page }) => {
  await openLogAnalysis(page)
  await expect(page.getByRole('navigation', { name: 'Main' }).getByRole('button')).toHaveText(['Log Analysis', 'Timeline & Sequence', 'Problems', 'Releases', 'Settings'])
  await expect(page.getByTestId('main-nav-logs')).toHaveAttribute('aria-current', 'page')
  await expect(page.getByTestId('log-analysis')).toBeVisible()
  await expect(visibleRows(page, 'span-name')).toHaveText(['publish_recording', 'publish_recording'])
  await expect(page.getByText('Report', { exact: true })).toHaveCount(0)
  await expect(page.getByText('Architecture', { exact: true })).toHaveCount(0)
})

test('NAV-EXP-011 / NAV-EXP-012: expanding a span shows child spans and its own logs inline, in time order', async ({ page }) => {
  const api = await openLogAnalysis(page)
  await page.locator('tr', { hasText: 'publish_recording' }).first().locator('.p-treetable-toggler').click()
  // Children and logs of the root: its own log, the child span, then the
  // group of trace-level and unmatched logs (never under a guessed span).
  await expect(visibleRows(page, 'log-row-message')).toHaveText(['Recording request accepted'])
  await expect(visibleRows(page, 'span-name')).toHaveText(['publish_recording', 'upload_recording', 'publish_recording'])
  await expect(page.getByTestId('log-group')).toHaveText('Trace-level / unmatched-span logs (2)')
  await page.locator('tr', { hasText: 'Trace-level / unmatched' }).locator('.p-treetable-toggler').click()
  await expect(visibleRows(page, 'log-row-association')).toHaveText(['recorded this span', 'span not found in this trace and window', 'trace-level: no span recorded'])

  await page.locator('tr', { hasText: 'upload_recording' }).locator('.p-treetable-toggler').click()
  const childLogs = page.locator('tr', { hasText: 'Upload' }).getByTestId('log-row-message')
  await expect(childLogs).toHaveText(['Upload started', 'Upload failed: AccessDenied'])
  // A log is a point event: no duration, its own severity.
  await expect(page.locator('tr', { hasText: 'Upload failed: AccessDenied' }).getByTestId('log-row-severity')).toHaveText('ERROR')
  await expect(page.locator('tr', { hasText: 'Upload failed: AccessDenied' })).toContainText('—')
  // The logs of the trace were read once, by trace id and the searched window.
  expect(api.reportRequests).toHaveLength(1)
  expect(api.reportRequests[0]).toEqual({ trace_id: TRACE, time_from: api.spanRequests[0].time_from, time_to: api.spanRequests[0].time_to })
  expect(api.logRequests).toHaveLength(0)
})

test('NAV-EXP-011: only spans with child spans or logs of their own can be expanded, so the arrows show the depth', async ({ page }) => {
  const parent = span('handle_request', { spanId: '00000000000000b0', childSpanCount: 3 })
  const deeper = span('call_storage', { spanId: '00000000000000b1', parentSpanId: parent.spanId, childSpanCount: 1 })
  const withLog = span('validate', { spanId: '00000000000000b2', parentSpanId: parent.spanId })
  const bare = span('serialize', { spanId: '00000000000000b3', parentSpanId: parent.spanId })
  const lone = span('heartbeat', { traceId: 'd'.repeat(32), spanId: '00000000000000d0' })
  await mockApi(page, (body) => {
    if (body.parent_id) {
      return { spans: body.parent_id === parent.spanId ? [deeper, withLog, bare] : [] }
    }
    return { spans: [parent, lone] }
  }, () => ({ count: 2 }), {
    report: (body) => body.trace_id === TRACE
      ? { spans: { records: [parent, deeper, withLog, bare] }, logs: { records: [logRecord('checked', 5, { spanId: withLog.spanId, message: 'Input checked' })] } }
      : { spans: { records: [lone] }, logs: { records: [] } }
  })
  await page.goto('/')
  const toggler = (name: string) => page.locator('tr', { hasText: name }).first().locator('.p-treetable-toggler')

  await toggler('handle_request').click()
  await expect(page.locator('tr', { hasText: 'serialize' })).toBeVisible()
  await expect(toggler('call_storage')).toBeVisible()
  await expect(toggler('validate')).toBeVisible()
  await expect(toggler('serialize')).toBeHidden()
  await toggler('validate').click()
  await expect(page.locator('tr', { hasText: 'Input checked' })).toBeVisible()

  // A row of the list whose logs aren't known yet keeps its arrow until it
  // is expanded; with nothing under it the arrow goes, without a notice row.
  await expect(toggler('heartbeat')).toBeVisible()
  await toggler('heartbeat').click()
  await expect(toggler('heartbeat')).toBeHidden()
  await expect(page.getByText('No child spans or logs')).toHaveCount(0)
})

test('NAV-EXP-013 / NAV-EXP-014: an OK span filter keeps the span OK and its ERROR log visible', async ({ page }) => {
  const api = await openLogAnalysis(page)
  await page.getByTestId('filter-status').click()
  await page.locator('.p-dropdown-item', { hasText: 'ok' }).click()
  await expect(visibleRows(page, 'span-name')).toHaveText(['publish_recording'])
  expect(api.spanRequests.at(-1)).toMatchObject({ status: 'ok', scope: 'all' })
  await page.locator('tr', { hasText: 'publish_recording' }).locator('.p-treetable-toggler').click()
  await page.locator('tr', { hasText: 'upload_recording' }).locator('.p-treetable-toggler').click()
  await expect(page.locator('tr', { hasText: 'Upload failed: AccessDenied' }).getByTestId('log-row-severity')).toHaveText('ERROR')
  await expect(page.getByTestId('filter-scope')).toContainText('never search log text')
  expect(api.logRequests).toHaveLength(0)
})

test('NAV-EXP-004: the row button, right-click and keyboard open the same actions; Escape restores focus', async ({ page }) => {
  await openLogAnalysis(page)
  const button = page.getByTestId('row-actions').first()
  await button.click()
  const labels = ['Open timeline', 'Open sequence', 'Copy investigation link', 'Events and links']
  await expect(page.getByTestId('action-menu').getByRole('menuitem')).toHaveText(labels)
  await expect(page.getByTestId('action-open-timeline')).toBeFocused()
  await page.keyboard.press('ArrowDown')
  await expect(page.getByTestId('action-open-sequence')).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(page.getByTestId('action-menu')).toHaveCount(0)
  await expect(button).toBeFocused()

  await page.locator('tr', { hasText: 'publish_recording' }).first().getByTestId('span-name').click({ button: 'right' })
  await expect(page.getByTestId('action-menu').getByRole('menuitem')).toHaveText(labels)
  await page.keyboard.press('Escape')

  await button.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByTestId('action-menu').getByRole('menuitem')).toHaveText(labels)
})

test('NAV-EXP-003 / NAV-EXP-005 / NAV-EXP-015: open a log in Sequence and return to the same exploration state', async ({ page }) => {
  const api = await openLogAnalysis(page)
  // State: a filter, a hidden column choice kept, an expanded span.
  await page.getByTestId('filter-name').fill('publish')
  await expect(visibleRows(page, 'span-name')).toHaveCount(2)
  const requestsBefore = api.spanRequests.length
  await page.locator('tr', { hasText: 'publish_recording' }).first().locator('.p-treetable-toggler').click()
  await page.locator('tr', { hasText: 'upload_recording' }).locator('.p-treetable-toggler').click()
  const logRow = page.locator('tr', { hasText: 'Upload failed: AccessDenied' })
  await logRow.getByTestId('row-actions').click()
  await page.getByTestId('action-open-sequence').click()

  await expect(page.getByTestId('main-nav-executions')).toHaveAttribute('aria-current', 'page')
  await expect(page.getByTestId('sequence')).toBeVisible()
  await expect(page.getByTestId('inspector-span')).toHaveText('upload_recording')
  await expect(page.getByTestId('log-detail-message')).toHaveText('Upload failed: AccessDenied')
  // The origin's filter did not narrow the report: every log of the trace is in it.
  await expect(page.getByTestId('report-summary')).toContainText('5 logs')
  await expect(page).toHaveURL(/#\/executions\?trace=abcdefabcdefabcdefabcdefabcdef01&.*span=00000000000000a1.*view=sequence/)

  await page.getByTestId('return-origin').click()
  await expect(page.getByTestId('main-nav-logs')).toHaveAttribute('aria-current', 'page')
  await expect(page.getByTestId('filter-name')).toHaveValue('publish')
  await expect(page.locator('tr', { hasText: 'Upload failed: AccessDenied' })).toBeVisible()
  await expect(page.locator('tr', { hasText: 'Upload failed: AccessDenied' }).locator('[data-selected=true]')).toBeVisible()
  // Returning did not query again.
  expect(api.spanRequests.length).toBe(requestsBefore + 2)

  // The browser's Back works the same way.
  await logRow.getByTestId('row-actions').click()
  await page.getByTestId('action-open-timeline').click()
  await expect(page.getByTestId('trace-timeline')).toBeVisible()
  await page.goBack()
  await expect(page.getByTestId('main-nav-logs')).toHaveAttribute('aria-current', 'page')
  await expect(page.getByTestId('filter-name')).toHaveValue('publish')
})

test('NAV-EXP-008 / RPT-DIR-006: a copied link holds identifiers only and opens without the origin', async ({ page, context }) => {
  await openLogAnalysis(page)
  await page.locator('tr', { hasText: 'publish_recording' }).first().locator('.p-treetable-toggler').click()
  await page.locator('tr', { hasText: 'Recording request accepted' }).getByTestId('row-actions').click()
  await page.getByTestId('action-copy-link').click()
  // Without clipboard permission the link is shown for selection, never claimed copied.
  const link = await page.getByTestId('link-text').inputValue()
  await expect(page.getByTestId('link-fallback')).not.toContainText('Link copied')
  expect(link).toContain('#/executions?trace=' + TRACE)
  expect(link).toContain('log=accepted')
  expect(link).not.toContain('Recording')

  const fresh = await context.newPage()
  await mockApi(fresh, () => ({ spans: [] }), () => ({ count: 0 }), { report })
  await fresh.goto(link)
  await expect(fresh.getByTestId('trace-id')).toHaveText(TRACE)
  await expect(fresh.getByTestId('origin')).toHaveCount(0)
  await expect(fresh.getByTestId('log-detail-message')).toHaveText('Recording request accepted')
})

test('NAV-EXP-009: an old investigate link with view=timing opens Timeline', async ({ page }) => {
  await mockApi(page, () => ({ spans: [] }), () => ({ count: 0 }), { report })
  await page.goto(`/#/investigate?trace=${TRACE}&from=1760090000000000000&to=1760093600000000000&span=${child.spanId}&view=timing`)
  await expect(page.getByTestId('trace-timeline')).toBeVisible()
  await expect(page.getByTestId('view-timeline')).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByTestId('inspector-span')).toHaveText('upload_recording')
})

test('an invalid link opens the page with an explanation instead of guessing a trace', async ({ page }) => {
  await mockApi(page, () => ({ spans: [] }), () => ({ count: 0 }), { report })
  await page.goto('/#/executions?trace=123&from=1&to=2')
  await expect(page.getByTestId('route-error')).toContainText('32 hex digits')
  await expect(page.getByTestId('sequence-library')).toBeVisible()
  await expect(page.getByTestId('report')).toHaveCount(0)
})

// --- Timeline & Sequence: the sequence library and exact lookup

// libShape is one shape of the mocked library; root times are in ns.
const libShape = (i: number, extra: Record<string, any> = {}) => ({
  shapeId: String(1000 + i), entryService: 'recording-api', entryOperation: `op-${i}`, services: ['object-storage', 'recording-api'],
  distinctCalls: 3, executions: 5000 - i, failedExecutions: i === 0 ? 50 : 0, invalidDurations: 0,
  percentiles: { p50: 120, p95: 410, p99: 900 },
  examples: {
    p50: { traceId: TRACE, time: root.timeStamp, durationUs: 121000 },
    p95: { traceId: TRACE_B, time: rootB.timeStamp, durationUs: 409000 },
    p99: { traceId: TRACE_B, time: rootB.timeStamp, durationUs: 899000 },
    slowest: { traceId: TRACE_B, time: rootB.timeStamp, durationUs: 3000000 }
  },
  ...extra
})
const library = (shapes: any[], extra: Record<string, any> = {}) => ({ data: { Library: { status: 'ok', shapes, truncated: false, notices: [], plan: [], contract: 'shapes-v1', generatedAt: '1', ...extra } } })

async function openLibrary (page: Page, shapes: (body: Record<string, any>) => any, spans: (body: Record<string, any>) => { spans: any[] } = () => ({ spans: [root, child] })) {
  const api = await mockApi(page, spans, () => ({ count: 0 }), { report, shapes })
  await page.goto('/')
  await page.evaluate(() => window.localStorage.clear())
  await page.getByTestId('main-nav-executions').click()
  return api
}

test('RPT-DIR-012 / LIB-001: an empty library says so, offers the lookup and invents no evidence', async ({ page }) => {
  const api = await openLibrary(page, () => library([]))
  await expect(page.getByTestId('library-empty')).toContainText('No execution started')
  await expect(page.getByTestId('report')).toHaveCount(0)
  await page.getByTestId('trace-lookup').locator('summary').click()
  await expect(page.getByTestId('lookup-trace-id')).toBeVisible()
  expect(api.reportRequests).toHaveLength(0)
  // The library asks for one window, the last 24 hours by default.
  const req = api.shapeRequests[0]
  expect(BigInt(req.time_to) - BigInt(req.time_from)).toBe(24n * 3600n * 1000000000n + 60n * 1000000000n)
  expect(req).toMatchObject({ service: '', operation: '', errors_only: false })
})

test('LIB-002: one row per shape, ten per page, with executions, failures and percentiles', async ({ page }) => {
  const shapes = Array.from({ length: 23 }, (_, i) => libShape(i))
  shapes[1] = libShape(1, { executions: 94, percentiles: { p50: 3000, p95: null, p99: null }, examples: { p50: shapes[0].examples.p50, slowest: shapes[0].examples.slowest } })
  await openLibrary(page, () => library(shapes))
  const rows = page.getByTestId('library-row')
  await expect(rows).toHaveCount(10)
  await expect(page.getByTestId('library-page')).toHaveText('page 1 of 3')
  await expect(rows.nth(0).getByTestId('library-entry')).toHaveText('op-0')
  await expect(rows.nth(0)).toContainText('recording-api → object-storage')
  await expect(rows.nth(0).getByTestId('library-executions')).toHaveText('5,000')
  await expect(rows.nth(0).getByTestId('library-failed')).toHaveText('1.0%')
  await expect(rows.nth(0).getByTestId('library-p95')).toHaveText('410 ms')
  // Too few executions: no p95/p99 and no example for them, never a guess.
  await expect(rows.nth(1).getByTestId('library-p95')).toHaveText('—')
  await expect(rows.nth(1).getByTestId('library-p95')).toHaveAttribute('title', /needs at least 200/)
  await expect(rows.nth(1).getByTestId('library-open-p95')).toBeDisabled()
  await expect(rows.nth(1).getByTestId('library-open-slowest')).toBeEnabled()
  await page.getByTestId('library-next').click()
  await expect(page.getByTestId('library-page')).toHaveText('page 2 of 3')
  await expect(rows.nth(0).getByTestId('library-entry')).toHaveText('op-10')
  await page.getByTestId('library-next').click()
  await expect(rows).toHaveCount(3)
  await expect(page.getByTestId('library-next')).toBeDisabled()
})

test('LIB-003: filters go to the server, and a failed library is an error, not an empty one', async ({ page }) => {
  let fail = false
  const api = await openLibrary(page, () => fail ? { status: 504, error: 'query limit exceeded' } : library([libShape(0)]))
  await expect(page.getByTestId('library-row')).toHaveCount(1)
  await page.getByTestId('library-service').fill('storage')
  await page.getByTestId('library-operation').fill('publish')
  await page.getByTestId('library-errors').check()
  fail = true
  await page.getByTestId('library-search').click()
  await expect(page.getByTestId('library-error')).toContainText('query limit exceeded')
  await expect(page.getByTestId('library-empty')).toHaveCount(0)
  expect(api.shapeRequests.at(-1)).toMatchObject({ service: 'storage', operation: 'publish', errors_only: true })
})

test('LIB-004: a percentile opens its representative execution as a sequence; Back returns to the same library page', async ({ page }) => {
  const shapes = Array.from({ length: 12 }, (_, i) => libShape(i))
  const api = await openLibrary(page, () => library(shapes))
  await page.getByTestId('library-next').click()
  await page.getByTestId('library-row').first().getByTestId('library-open-p95').click()
  await expect(page.getByTestId('trace-id')).toHaveText(TRACE_B)
  await expect(page.getByTestId('view-sequence')).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByTestId('origin')).toContainText('the sequence library (op-10 · p95 · 409 ms)')
  const req = api.reportRequests.at(-1)!
  expect(BigInt(req.time_from)).toBe(BigInt(rootB.timeStamp) - 60n * 1000000000n)
  expect(BigInt(req.time_to)).toBe(BigInt(rootB.timeStamp) + 3600n * 1000000000n)
  await page.getByTestId('return-origin').click()
  await expect(page.getByTestId('sequence-library')).toBeVisible()
  await expect(page.getByTestId('library-page')).toHaveText('page 2 of 2')
  expect(api.shapeRequests).toHaveLength(1) // returning doesn't reload the library
  // The open execution stays one click away.
  await page.getByTestId('library-close').click()
  await expect(page.getByTestId('trace-id')).toHaveText(TRACE_B)
})

test('LIB-005: a row lists its distinct calls, read from one of its executions', async ({ page }) => {
  const api = await openLibrary(page, () => library([libShape(0)]))
  await page.getByTestId('library-calls').click()
  await expect(page.getByTestId('library-call')).toHaveText(['recording-api · publish_recording → recording-api · upload_recording'])
  expect(api.reportRequests.at(-1)).toMatchObject({ trace_id: TRACE, sections: ['spans'] })
  await page.getByTestId('library-calls').click()
  await expect(page.getByTestId('library-calls-row')).toHaveCount(0)
})

test('NAV-CAT-007 / RPT-DIR-002 / RPT-DIR-003 / RPT-DIR-004: exact lookup opens a no-error trace as Timeline, without a problem', async ({ page }) => {
  const api = await openLibrary(page, () => library([]))
  await page.getByTestId('trace-lookup').locator('summary').click()
  await page.getByTestId('lookup-trace-id').fill('not-an-id')
  await page.getByTestId('lookup-open').click()
  await expect(page.getByTestId('lookup-error')).toContainText('32 hex digits')
  await page.getByTestId('lookup-trace-id').fill(TRACE.toUpperCase())
  await page.getByTestId('lookup-open').click()
  await expect(page.getByTestId('trace-id')).toHaveText(TRACE)
  // Without a saved preference the familiar Timeline is shown, beside the logs.
  await expect(page.getByTestId('view-timeline')).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByTestId('log-inspector')).toBeVisible()
  await expect(page.getByTestId('execution-heading')).toHaveText('publish_recording · recording-api')
  await expect(page.getByTestId('report')).not.toContainText(/problem|success/i)
  expect(api.reportRequests[0].trace_id).toBe(TRACE)
})

test('RPT-DIR-007: switching Timeline and Sequence keeps the revision and selection; the library stays behind its button', async ({ page }) => {
  const api = await openLibrary(page, () => library([libShape(0)]))
  await page.getByTestId('trace-lookup').locator('summary').click()
  await page.getByTestId('lookup-trace-id').fill(TRACE)
  await page.getByTestId('lookup-open').click()
  await expect(page.getByTestId('trace-timeline')).toBeVisible()
  await page.getByTestId('timeline-row').filter({ hasText: 'upload_recording' }).click()
  const generated = await page.getByTestId('generated-at').textContent()
  await page.getByTestId('view-sequence').click()
  await expect(page.getByTestId('sequence')).toBeVisible()
  await expect(page.getByTestId('inspector-span')).toHaveText('upload_recording')
  await page.getByTestId('scope-trace').click()
  await page.getByTestId('view-timeline').click()
  await expect(page.getByTestId('generated-at')).toHaveText(generated!)
  expect(api.reportRequests).toHaveLength(1)
  await page.getByTestId('open-library').click()
  await expect(page.getByTestId('library-row')).toHaveCount(1)
})

test('the last chosen view is kept for executions opened without an explicit view', async ({ page }) => {
  await openLibrary(page, () => library([]))
  await page.getByTestId('trace-lookup').locator('summary').click()
  await page.getByTestId('lookup-trace-id').fill(TRACE)
  await page.getByTestId('lookup-open').click()
  await page.getByTestId('view-sequence').click()
  await page.getByTestId('open-library').click()
  await page.getByTestId('lookup-trace-id').fill(TRACE_B)
  await page.getByTestId('lookup-open').click()
  await expect(page.getByTestId('trace-id')).toHaveText(TRACE_B)
  await expect(page.getByTestId('view-sequence')).toHaveAttribute('aria-selected', 'true')
})

test('NAV-EXP-017: a narrow screen keeps every page usable without horizontal page scroll', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 800 })
  await openLogAnalysis(page)
  for (const p of ['logs', 'executions', 'problems', 'releases', 'settings']) {
    await page.getByTestId('main-nav-' + p).click()
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
    expect(overflow, p).toBeLessThanOrEqual(1)
  }
  await page.getByTestId('main-nav-logs').click()
  await expect(page.getByTestId('row-actions').first()).toBeVisible()
})

test('Settings shows the effective retention read from the server', async ({ page }) => {
  await mockApi(page, () => ({ spans: [] }), () => ({ count: 0 }), {
    rules: () => ({ data: { Rules: { scope: 'installation', revision: 0, errors: { enabled: true, minAffectedCalls: 1, grouping: 'operation_signature' }, slow: { enabled: true, durationMs: 500, minCalls: 5 }, regressions: { enabled: true, p95RelativePct: 20, p95AbsoluteMs: 20 } } } })
  })
  await page.route('**/api/v1/retention', (route) => route.fulfill({ json: { error: false, message: 'success', data: { Policy: { tracesSeconds: 259200, logsSeconds: 172800, statsSeconds: 2592000, bucketSeconds: 3600, latenessSeconds: 86400, marginSeconds: 3600, compatible: true, defaultSampling: '', clockOffsetSeconds: 0, physicalDeletion: 'Expired records are removed in the background.' } } } }))
  await page.goto('/')
  await page.getByTestId('main-nav-settings').click()
  await expect(page.getByTestId('detection-rules')).toBeVisible()
  await page.getByTestId('settings-retention').click()
  await expect(page.getByTestId('retention-traces')).toHaveText('3 days')
  await expect(page.getByTestId('retention-logs')).toHaveText('2 days')
  await expect(page.getByTestId('retention-stats')).toHaveText('30 days')
})

