import { readFile } from 'node:fs/promises'
import { expect, test, type Page } from '@playwright/test'
import { mockApi, type Body, type MockOptions, type ReportReply } from './mockApi'
import { referenceReports } from './pwReference'
import { metric, regression } from './m2Data'

// UI checks of the Problems page (troubleshooting-ui-agent features; examples
// open in Timeline & Sequence, rules live in Settings).
// Titles start with the scenario tag they check.

const FAILED = referenceReports.failed
const LOGONLY = referenceReports.logonly
const UPLOAD = '0000000000000002'
const HOUR = 3_600_000_000_000n
const FROM = (BigInt(FAILED.spans[0].timeStamp) / HOUR) * HOUR

const rules = (revision: number, durationMs = 500) => ({
  scope: 'installation',
  revision,
  savedAt: revision ? '1791709200000000000' : undefined,
  effectiveAt: revision ? '1791709200000000000' : undefined,
  errors: { enabled: true, minAffectedCalls: 1, grouping: 'operation_signature' },
  slow: { enabled: true, durationMs, minCalls: 5 },
  regressions: { enabled: true, p95RelativePct: 20, p95AbsoluteMs: 20 }
})

const explanation = { ruleRevision: 1, rule: 'Error signals', observed: '100 affected call(s), 240 occurrence(s)', threshold: 'at least 1 affected call(s)', basis: 'errors-v1', scope: 'recording-worker', coverage: 'complete' }

const deniedProblem = {
  id: 'PRB-0a1b2c3d4e',
  type: 'error',
  title: 'Storage returned 403 AccessDenied',
  service: 'recording-worker',
  operation: 'upload_recording',
  environment: 'production',
  qualified: true,
  firstSeen: FAILED.logs[0].timeUnixNano,
  lastSeen: FAILED.logs[0].timeUnixNano,
  error: {
    key: 'k', sourceKind: 'log', service: 'recording-worker', operation: 'upload_recording', environment: 'production', method: 'message', fingerprint: 'f1',
    exceptionType: '', frame: '', signature: 'Storage returned 403 AccessDenied', occurrences: 240, unlinked: 0, affectedTraces: 100, affectedCalls: 100,
    firstSeen: '1', lastSeen: '2', versions: [{ version: '1.1.0', occurrences: 240 }]
  },
  explanation
}

const regressionProblem = {
  id: 'PRB-ffeeddccbb',
  type: 'release_regression',
  title: 'upload_recording became slower in version 1.1.0 (against version 1.0.0)',
  service: 'recording-worker',
  operation: 'upload_recording',
  spanKind: 'consumer',
  environment: 'fixture',
  qualified: true,
  regression: {
    analysisId: 'a1b2c3d4e5f6a1b2c3d4e5f6',
    generatedAt: '1791709200000000000',
    baseline: { service: 'recording-worker', version: '1.0.0', time_from: '1', time_to: '2' },
    candidate: { service: 'recording-worker', version: '1.1.0', time_from: FROM.toString(), time_to: (FROM + HOUR).toString() },
    baselineLabel: 'recording-worker / version 1.0.0',
    candidateLabel: 'recording-worker / version 1.1.0',
    metric: metric('p95', 100, 160, 'regression_signal'),
    baselineCalls: 1000, candidateCalls: 1000, baselineSamples: 1000, candidateSamples: 1000
  },
  explanation: { ...explanation, observed: 'p95 100.0 ms → 160.0 ms' }
}

function list (problems: unknown[], extra: Record<string, unknown> = {}) {
  return { data: { Problems: { scope: { service: 'recording-worker', time_from: FROM.toString(), time_to: (FROM + HOUR).toString() }, rules: rules(1), status: problems.length ? 'available' : 'empty', condition: 'ok', sources: {}, problems, total: problems.length, generatedAt: '1', notices: [], ...extra } } }
}

const example = (r: typeof FAILED, overrides: Record<string, unknown> = {}) => ({
  sourceKind: 'log', traceId: r.traceId, spanId: UPLOAD, recordId: r.logs.find((l) => l.severity === 'ERROR' && l.spanId === UPLOAD)!.recordId,
  time: r.logs[0].timeUnixNano, service: 'recording-worker', operation: 'upload_recording', version: '1.1.0', message: 'Storage returned 403 AccessDenied', ...overrides
})

const examples = (exs: unknown[]) => ({ data: { Examples: { status: 'ok', condition: 'ok', examples: exs, limit: 20, shown: exs.length, total: exs.length, truncated: false, description: '', cutoff: '1' } } })

function reportOf (body: Body): ReportReply {
  const r = Object.values(referenceReports).find((x) => x.traceId === body.trace_id)!
  return { spans: { records: r.spans }, logs: { records: r.logs } }
}

async function openWorkspace (page: Page, options: MockOptions) {
  const api = await mockApi(page, () => ({ spans: [] }), () => ({ count: 0 }), {
    services: () => ({ data: { Services: ['recording-worker'] } }),
    rules: () => ({ data: { Rules: rules(1) } }),
    report: reportOf,
    ...options
  })
  await page.goto('/')
  await page.getByTestId('main-nav-problems').click()
  await page.getByTestId('problems-service').fill('recording-worker')
  await page.getByTestId('problems-load').click()
  return api
}

// investigateDenied opens the denied problem and its first retained example
// in Timeline & Sequence.
async function investigateDenied (page: Page, options: MockOptions = {}, view: 'timeline' | 'sequence' = 'timeline') {
  const api = await openWorkspace(page, { problems: () => list([deniedProblem, regressionProblem]), examples: () => examples([example(FAILED), example(LOGONLY)]), ...options })
  await page.getByTestId('problem-row').filter({ hasText: 'AccessDenied' }).getByTestId('problem-investigate').click()
  await page.getByTestId('example-open-' + view).click()
  await expect(page.getByTestId('report-summary')).toBeVisible()
  return api
}

const row = (page: Page, name: string) => page.getByTestId('timeline-row').filter({ hasText: name })

test('PW-GRP-001 / NAV-EXP-007: grouped problems lead with the right measure; a chosen occurrence opens in Timeline & Sequence', async ({ page }) => {
  const api = await openWorkspace(page, { problems: () => list([deniedProblem, regressionProblem]), examples: () => examples([example(FAILED)]) })
  const rows = page.getByTestId('problem-row')
  await expect(rows).toHaveCount(2)
  await expect(rows.nth(0).getByTestId('problem-row-measure')).toContainText('100')
  await expect(rows.nth(0).getByTestId('problem-row-measure')).toContainText('Affected calls')
  await expect(rows.nth(0).getByTestId('problem-row-measure')).toContainText('240 matching ERROR logs')
  await expect(rows.nth(1).getByTestId('problem-row-measure')).toContainText('p95 +60%')
  await expect(rows.nth(1).getByTestId('problem-row-measure')).toContainText('1,000 / 1,000 calls evaluated')
  expect(api.problemRequests[0]).toMatchObject({ service: 'recording-worker', limit: 50 })

  await rows.nth(0).getByTestId('problem-investigate').click()
  await expect(page.getByTestId('problem-title')).toHaveText('Storage returned 403 AccessDenied')
  await expect(page.getByTestId('example-select')).toBeHidden()
  await expect(page.getByTestId('example-single')).toContainText('ERROR log')
  // No report is generated until an occurrence is opened.
  expect(api.reportRequests).toHaveLength(0)
  await page.getByTestId('example-open-timeline').click()
  await expect(page.getByTestId('main-nav-executions')).toHaveAttribute('aria-current', 'page')
  await expect(page.getByTestId('origin')).toContainText('Storage returned 403 AccessDenied')
  await expect(row(page, 'upload_recording')).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByTestId('timeline-row')).toHaveCount(4)
  await expect(page.getByTestId('log-detail-message')).toHaveText('Storage returned 403 AccessDenied')
  // Returning restores the problem detail as it was.
  await page.getByTestId('return-origin').click()
  await expect(page.getByTestId('problem-title')).toHaveText('Storage returned 403 AccessDenied')
})

test('PW-INV-001 / PW-INV-002: the report keeps every service and severity; siblings overlap 120 ms on one axis', async ({ page }) => {
  await investigateDenied(page)
  await expect(page.getByTestId('report-summary')).toContainText('4 spans')
  await expect(page.getByTestId('report-summary')).toContainText('12 logs')
  const bar = (name: string) => row(page, name).getByTestId('timeline-bar')
  await expect(bar('upload_recording')).toHaveAttribute('data-start-ms', '25')
  await expect(bar('upload_recording')).toHaveAttribute('data-end-ms', '225')
  await expect(bar('create_thumbnail')).toHaveAttribute('data-start-ms', '30')
  await expect(bar('create_thumbnail')).toHaveAttribute('data-end-ms', '150')
  await page.getByTestId('view-sequence').click()
  await expect(page.getByTestId('seq-overlap').first()).toContainText('overlap for 120 ms')
})

test('PW-INV-004 / PW-INV-005: switching views keeps the span and log selected without a new request; task lanes are recorded', async ({ page }) => {
  const api = await investigateDenied(page)
  await expect(page.getByTestId('task-lane-name')).toHaveText(['checksum', 'transfer', 'Task not recorded'])
  await page.getByTestId('task-lane').filter({ hasText: 'transfer' }).getByTestId('lane-point').last().click()
  await expect(page.getByTestId('log-detail-message')).toHaveText('Storage returned 403 AccessDenied')
  const requests = api.reportRequests.length
  await page.getByTestId('view-sequence').click()
  await expect(page.getByTestId('sequence')).toBeVisible()
  await page.getByTestId('view-list').click()
  await expect(page.getByTestId('log-list-row').filter({ hasText: 'AccessDenied' })).toHaveClass(/picked/)
  await page.getByTestId('view-timeline').click()
  await page.getByTestId('view-lanes').click()
  await expect(row(page, 'upload_recording')).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByTestId('log-detail-message')).toHaveText('Storage returned 403 AccessDenied')
  expect(api.reportRequests.length).toBe(requests)
})

test('PW-INV-006: all trace logs include trace-level and unknown-span logs, labeled; returning keeps the span', async ({ page }) => {
  const extra = [
    { timeUnixNano: (BigInt(FAILED.spans[0].timeStamp) + 100_000_000n).toString(), recordId: 'trace-level', traceId: FAILED.traceId, spanId: '', severity: 'INFO', service: 'recording-api', message: 'Trace-level note', attributes: {} },
    { timeUnixNano: (BigInt(FAILED.spans[0].timeStamp) + 110_000_000n).toString(), recordId: 'gone', traceId: FAILED.traceId, spanId: '00000000000000ee', severity: 'WARN', service: 'recording-worker', message: 'Unknown span note', attributes: {} }
  ]
  await investigateDenied(page, { report: () => ({ spans: { records: FAILED.spans }, logs: { records: [...FAILED.logs, ...extra] } }) })
  await page.getByTestId('scope-trace').click()
  await expect(page.getByTestId('trace-log-row')).toHaveCount(14)
  await expect(page.getByTestId('trace-log-row').filter({ hasText: 'Trace-level note' }).getByTestId('trace-log-association')).toHaveText('No span identifier recorded')
  await expect(page.getByTestId('trace-log-row').filter({ hasText: 'Unknown span note' }).getByTestId('trace-log-association')).toHaveText('Referenced span not found within the window')
  await page.getByTestId('scope-span').click()
  await page.getByTestId('view-list').click()
  await expect(page.getByTestId('log-list-row')).toHaveCount(6)
  await expect(page.getByTestId('log-list-row').filter({ hasText: 'note' })).toHaveCount(0)
  await expect(row(page, 'upload_recording')).toHaveAttribute('aria-pressed', 'true')
})

test('PW-INV-008: an OK span with an ERROR log stays OK and shows its ERROR log', async ({ page }) => {
  await openWorkspace(page, { problems: () => list([deniedProblem]), examples: () => examples([example(FAILED), example(LOGONLY)]) })
  await page.getByTestId('problem-investigate').click()
  await page.getByTestId('example-select').selectOption({ index: 1 })
  await page.getByTestId('example-open-timeline').click()
  await expect(page.getByTestId('trace-id')).toHaveText(LOGONLY.traceId)
  const meta = row(page, 'upload_recording').getByTestId('timeline-meta')
  await expect(meta).toContainText('OK')
  await expect(meta).toContainText('1 ERROR log')
  await expect(page.getByTestId('log-inspector')).not.toContainText('succeeded')
})

test('PW-INV-009: a slow report for example A cannot replace example B', async ({ page }) => {
  await investigateDenied(page, {
    report: (body) => ({ ...reportOf(body), delayMs: body.trace_id === FAILED.traceId ? 1500 : 0 })
  })
  await page.getByTestId('return-origin').click()
  await page.getByTestId('example-select').selectOption({ index: 1 })
  await page.getByTestId('example-open-timeline').click()
  await expect(page.getByTestId('trace-id')).toHaveText(LOGONLY.traceId)
  await expect(page.getByTestId('report-summary')).toBeVisible()
  await page.waitForTimeout(1800)
  await expect(page.getByTestId('trace-id')).toHaveText(LOGONLY.traceId)
  await expect(page.getByTestId('inspector-span')).toHaveText('upload_recording')
})

test('PW-INV-011: a failed log source keeps the timeline and offers retry, never "no logs"', async ({ page }) => {
  let fail = true
  await investigateDenied(page, {
    report: (body) => (fail ? { spans: { records: FAILED.spans }, logs: { status: 'unavailable', error: 'the database is unavailable' } } : reportOf(body))
  })
  await expect(page.getByTestId('timeline-row')).toHaveCount(4)
  await expect(page.getByTestId('inspector-logs-unavailable')).toBeVisible()
  await expect(page.getByTestId('no-span-logs-recorded')).toHaveCount(0)
  fail = false
  await page.getByTestId('inspector-retry-logs').click()
  await expect(page.getByTestId('task-lanes')).toBeVisible()
})

test('PW-INV-012: logs without spans stay inspectable; no tree is reconstructed', async ({ page }) => {
  await investigateDenied(page, { report: () => ({ spans: { records: [] }, logs: { records: FAILED.logs } }) })
  await expect(page.getByTestId('no-spans')).toBeVisible()
  await expect(page.getByTestId('trace-timeline')).toHaveCount(0)
  await expect(page.getByTestId('trace-log-row')).toHaveCount(12)
})

test('PW-GRP-004 / NAV-EXP-006: an unlinked log example is inspectable and opens no trace', async ({ page }) => {
  const unlinked = { ...deniedProblem, id: 'PRB-unlinked00', qualified: false, attribution: 'Call attribution unavailable: the records name no span.', error: { ...deniedProblem.error, affectedCalls: 0, unlinked: 1, signature: 'Cache volume is full', occurrences: 1 } }
  const api = await openWorkspace(page, {
    problems: () => list([unlinked]),
    examples: () => examples([{ sourceKind: 'log', traceId: '', spanId: '', recordId: 'r-unlinked', time: FAILED.logs[0].timeUnixNano, service: 'recording-worker', operation: 'cache_cleanup', version: '1.1.0', message: 'Cache volume is full' }])
  })
  await expect(page.getByTestId('problem-row-measure')).toContainText('Matching records')
  await expect(page.getByTestId('problem-row-measure')).toContainText('Call attribution unavailable')
  await page.getByTestId('problem-investigate').click()
  await expect(page.getByTestId('unlinked-message')).toHaveText('Cache volume is full')
  await expect(page.getByTestId('unlinked-example')).toContainText('No trace can be opened')
  await expect(page.getByTestId('example-actions')).toHaveCount(0)
  expect(api.reportRequests).toHaveLength(0)
})

test('PW-GRP-008: empty, unavailable, expired and partial overviews say different things', async ({ page }) => {
  let reply = list([])
  await openWorkspace(page, { problems: () => reply })
  await expect(page.getByTestId('problems-empty')).toContainText('No problems observed in the covered scope')
  reply = list([], { status: 'unavailable', condition: 'Problem summaries are unavailable: errors.' })
  await page.getByTestId('problems-load').click()
  await expect(page.getByTestId('problems-unavailable')).toBeVisible()
  await expect(page.getByTestId('problems-empty')).toHaveCount(0)
  reply = list([], { status: 'expired', condition: 'Summary history is unavailable for this interval (outside retention).' })
  await page.getByTestId('problems-load').click()
  await expect(page.getByTestId('problems-expired')).toContainText('Summary history is unavailable')
  reply = list([deniedProblem], { status: 'partial', condition: 'Incomplete results: slow_calls could not be evaluated' })
  await page.getByTestId('problems-load').click()
  await expect(page.getByTestId('problems-partial')).toContainText('Incomplete results')
  await expect(page.getByTestId('problem-row')).toHaveCount(1)
})

test('PW-GRP-007: continuation keeps the scope and rules revision', async ({ page }) => {
  const api = await openWorkspace(page, { problems: (body) => body.cursor ? list([regressionProblem]) : list([deniedProblem], { next: '1', total: 2 }) })
  await page.getByTestId('problems-more').click()
  await expect(page.getByTestId('problem-row')).toHaveCount(2)
  expect(api.problemRequests[1]).toMatchObject({ service: 'recording-worker', cursor: '1', rules_revision: 1, time_from: api.problemRequests[0].time_from, time_to: api.problemRequests[0].time_to })
})

test('PW-RUL-001 / PW-RUL-002: a valid save returns the new revision; invalid enabled fields are rejected, disabled ones kept', async ({ page }) => {
  const api = await openWorkspace(page, {
    problems: () => list([]),
    saveRules: (body) => ({ data: { Rules: { ...body.rules, revision: body.expected_revision + 1, savedAt: '1791709300000000000', effectiveAt: '1791709300000000000' } } })
  })
  await page.getByTestId('main-nav-settings').click()
  await expect(page.getByTestId('rules-revision')).toContainText('Revision 1')
  await page.getByTestId('rule-slow-duration').fill('-5')
  await page.getByTestId('rules-save').click()
  await expect(page.getByTestId('rule-slow-duration-error')).toBeVisible()
  expect(api.ruleSaves).toHaveLength(0)
  await page.getByTestId('rule-slow-enabled').uncheck()
  await expect(page.getByTestId('rule-slow-duration')).toBeDisabled()
  await expect(page.getByTestId('rule-slow-duration')).toHaveValue('-5')
  await page.getByTestId('rule-slow-enabled').check()
  await page.getByTestId('rule-slow-duration').fill('700')
  await page.getByTestId('rules-save').click()
  await expect(page.getByTestId('rules-state')).toContainText('Saved as revision 2')
  expect(api.ruleSaves[0]).toMatchObject({ expected_revision: 1, rules: { slow: { durationMs: 700 } } })
  await expect(page.getByTestId('rules-revision')).toContainText('Revision 2')
  // The shown problem list is evaluated again with the new rules.
  await expect.poll(() => api.problemRequests.length).toBe(2)
})

test('PW-RUL-003 / PW-RUL-004: a failed save keeps the edits; a conflict never overwrites', async ({ page }) => {
  let reply: () => { status: number, error: string, data?: Record<string, unknown> } = () => ({ status: 503, error: 'the database is unavailable' })
  await openWorkspace(page, { problems: () => list([]), saveRules: () => reply() })
  await page.getByTestId('main-nav-settings').click()
  await page.getByTestId('rule-slow-duration').fill('700')
  await page.getByTestId('rules-save').click()
  await expect(page.getByTestId('rules-state')).toContainText('Not saved: the database is unavailable')
  await expect(page.getByTestId('rule-slow-duration')).toHaveValue('700')
  page.once('dialog', (d) => d.dismiss())
  await page.getByTestId('settings-retention').click()
  await expect(page.getByTestId('detection-rules')).toBeVisible()

  reply = () => ({ status: 409, error: 'the rules were changed by another save', data: { Rules: rules(2, 900) } })
  await page.getByTestId('rules-save').click()
  await expect(page.getByTestId('rules-state')).toContainText('revision 2 was saved elsewhere')
  await expect(page.getByTestId('rule-slow-duration')).toHaveValue('700')
  await page.getByTestId('rules-reload').click()
  await expect(page.getByTestId('rule-slow-duration')).toHaveValue('900')
  await expect(page.getByTestId('rules-revision')).toContainText('Revision 2')
})

test('PW-CMP-001 / PW-CMP-002: a regression opens its stored comparison; checking the fix compares it with the affected release', async ({ page }) => {
  const api = await openWorkspace(page, {
    problems: () => list([regressionProblem]),
    examples: () => examples([example(FAILED, { sourceKind: 'span', durationMs: 200 })]),
    analysis: () => ({ data: { Analysis: regression() } }),
    stats: () => ({ data: { Stats: { status: 'ok', condition: 'ok', rows: ['1.0.0', '1.1.0', '1.1.1'].map((v) => ({ operation: 'upload_recording', spanKind: 'consumer', version: v, calls: 1000, validDurations: 1000, percentiles: {} })) } } }),
    compare: () => ({ data: { Analysis: regression() } })
  })
  await page.getByTestId('problem-investigate').click()
  await expect(page.getByTestId('problem-compare')).toBeVisible()
  await expect(page.getByTestId('compare-metric').filter({ hasText: 'p95' })).toContainText('Regression signal')
  await expect(page.getByTestId('compare-signal').first()).toBeVisible()
  await page.getByTestId('compare-fixed').selectOption('v:1.1.1')
  await page.getByTestId('compare-check-fix').click()
  await expect.poll(() => api.compareRequests.length).toBe(1)
  expect(api.compareRequests[0].baseline.version).toBe('1.1.0')
  expect(api.compareRequests[0].candidate.version).toBe('1.1.1')
  await page.getByTestId('compare-original').click()
  await expect.poll(() => api.compareRequests.length).toBe(2)
  expect(api.compareRequests[1].baseline.version).toBe('1.0.0')
  expect(api.compareRequests[1].candidate.version).toBe('1.1.1')
})

test('PW-INV-015 / RPT-DIR-008: the export holds the whole report and inert text', async ({ page }) => {
  const evil = { ...FAILED.logs[7], message: '<img src=x onerror=alert(1)>' }
  await investigateDenied(page, { report: () => ({ spans: { records: FAILED.spans }, logs: { records: [...FAILED.logs.slice(0, 7), evil, ...FAILED.logs.slice(8)] } }) })
  for (const [id, check] of [['export-json', (t: string) => {
    const j = JSON.parse(t)
    expect(j.spans.records).toHaveLength(4)
    expect(j.logs.records).toHaveLength(12)
  }], ['export-html', (t: string) => {
    expect(t).not.toContain('<img src=x')
    expect(t).toContain('&lt;img src=x')
    expect(t).not.toMatch(/<script/i)
  }]] as const) {
    const download = page.waitForEvent('download')
    await page.getByTestId(id).click()
    check(await readFile(await (await download).path(), 'utf8'))
  }
})
