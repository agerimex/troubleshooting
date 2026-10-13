import { readFile } from 'node:fs/promises'
import { expect, test, type Page } from '@playwright/test'
import { logRecord, mockApi, span, type MockOptions } from './mockApi'
import { DENIED_GROUP, HOUR, STORAGE_GROUP, T0, callStats, metric, populationStats, regression } from './m2Data'

// UI checks of statistics and release comparison (features/milestone_2).
// Test titles start with the scenario tag they check.

const FAILED_TRACE = 'abcdefabcdefabcdefabcdefabcdef02'
const UPLOAD = '00000000000000a1'

const apiSpan = span('start_recording', { traceId: FAILED_TRACE, spanId: '00000000000000a0', service: 'recording-api', status: 'ok', statusCode: 2 })
const uploadSpan = span('upload_recording', { traceId: FAILED_TRACE, spanId: UPLOAD, parentSpanId: '00000000000000a0', service: 'recording-worker', status: 'error', statusCode: 1, duration: 160000 })
const denied = logRecord('denied', 150, { traceId: FAILED_TRACE, spanId: UPLOAD, service: 'recording-worker', severity: 'ERROR', message: 'Storage returned 403 AccessDenied' })
const retry = logRecord('retry', 60, { traceId: FAILED_TRACE, spanId: UPLOAD, service: 'recording-worker', severity: 'WARN', message: 'Upload retry 1 of 3' })
const accepted = logRecord('accepted', 5, { traceId: FAILED_TRACE, spanId: '00000000000000a0', service: 'recording-api', message: 'Recording request accepted' })

async function openCompare (page: Page, options: MockOptions) {
  const api = await mockApi(page, () => ({ spans: [] }), () => ({ count: 0 }), {
    report: () => ({ spans: { records: [apiSpan, uploadSpan] }, logs: { records: [accepted, retry, denied] } }),
    ...options
  })
  await page.goto('/')
  await page.getByTestId('main-nav-releases').click()
  await page.getByTestId('baseline-service').fill('recording-worker')
  await page.getByTestId('candidate-service').fill('recording-worker')
  return api
}

async function runComparison (page: Page) {
  await page.getByTestId('compare-run').click()
  await expect(page.getByTestId('analysis')).toBeVisible()
}

const metricRow = (page: Page, name: string) => page.getByTestId('metric-' + name).first()

test('M2-CMP-001: a regression shows interpretable differences, both populations and the rules', async ({ page }) => {
  const api = await openCompare(page, { compare: () => ({ data: { Analysis: regression() } }) })
  await runComparison(page)

  expect(api.compareRequests[0].baseline.service).toBe('recording-worker')
  expect(api.compareRequests[0].candidate.service).toBe('recording-worker')
  const p95 = metricRow(page, 'p95')
  await expect(p95.getByTestId('metric-baseline')).toHaveText('100.0 ms')
  await expect(p95.getByTestId('metric-candidate')).toHaveText('160.0 ms')
  await expect(p95.getByTestId('metric-absolute')).toHaveText('+60.0 ms')
  await expect(p95.getByTestId('metric-relative')).toHaveText('+60.00 %')
  await expect(p95.getByTestId('metric-class')).toHaveText('Regression signal')
  const ef = metricRow(page, 'error_fraction')
  await expect(ef.getByTestId('metric-baseline')).toHaveText('1.00 %')
  await expect(ef.getByTestId('metric-candidate')).toHaveText('5.00 %')
  await expect(ef.getByTestId('metric-absolute')).toHaveText('+4.00 pp')
  await expect(ef.getByTestId('metric-relative')).toHaveText('+400.00 %')
  await expect(ef.getByTestId('metric-class')).toHaveText('Regression signal')
  await expect(page.getByTestId('population-baseline')).toContainText('version 1.0.0')
  await expect(page.getByTestId('population-candidate')).toContainText('version 1.1.0')
  await expect(page.getByTestId('row-samples')).toContainText('Baseline: 1000 calls, 1000 valid durations · Candidate: 1000 calls')
  await expect(page.getByTestId('analysis-revision')).toContainText('comparison-v1')
  await expect(page.getByTestId('analysis-interpretation')).toContainText('not proof that a deployment caused a change')
  await expect(page.getByTestId('analysis-group-title').first()).toHaveText('StorageError at storage.Upload')
  await expect(page.getByTestId('group-label').first()).toContainText('Recurring in both scopes')
})

test('M2-CMP-003: a zero baseline has no relative percentage', async ({ page }) => {
  const a = regression()
  a.rows[0].metrics = [metric('p95', 0, 100, 'regression_signal'), metric('error_fraction', 0, 0, 'within_threshold')]
  await openCompare(page, { compare: () => ({ data: { Analysis: a } }) })
  await runComparison(page)
  await expect(metricRow(page, 'p95').getByTestId('metric-relative')).toHaveText('unavailable (baseline is zero)')
  await expect(metricRow(page, 'error_fraction').getByTestId('metric-relative')).toHaveText('unavailable (baseline is zero)')
  await expect(metricRow(page, 'error_fraction').getByTestId('metric-class')).toHaveText('Within threshold')
  await expect(page.getByTestId('analysis')).not.toContainText('Infinity')
})

test('M2-CMP-006 / M2-CMP-007: mismatches and missing evidence withhold conclusions but keep the statistics', async ({ page }) => {
  const a = regression()
  a.mismatches = [{ dimension: 'environment', baseline: 'fixture', candidate: 'test', affects: ['metrics', 'error_groups'] }]
  a.rows[0].metrics = [
    metric('error_fraction', 1, 5, 'incomparable', { reasons: ['the populations differ in environment'] }),
    metric('p95', 100, 160, 'insufficient_evidence', { reasons: ['only 50 candidate duration samples for p95 (the rule needs 200)'] })
  ]
  await openCompare(page, { compare: () => ({ data: { Analysis: a } }) })
  await runComparison(page)
  await expect(page.getByTestId('analysis-mismatch')).toContainText('differ in environment (baseline fixture, candidate test)')
  await expect(metricRow(page, 'error_fraction').getByTestId('metric-class')).toHaveText('Not comparable')
  await expect(metricRow(page, 'error_fraction').getByTestId('metric-candidate')).toHaveText('5.00 %')
  await expect(metricRow(page, 'p95').getByTestId('metric-class')).toHaveText('Insufficient evidence')
  await expect(metricRow(page, 'p95')).toContainText('only 50 candidate duration samples')
})

test('M2-CMP-008: an operation without candidate calls stays visible and unavailable', async ({ page }) => {
  const a = regression()
  a.rows = [{
    operation: 'publish_recording', spanKind: 'server', baseline: callStats(1000, 10, 40), candidate: null as any,
    notes: ['No candidate calls were observed for this operation in the candidate scope. This does not mean it was fixed, became faster or was removed from the application.'],
    metrics: [metric('calls', 1000, 0, 'descriptive'), metric('error_fraction', 1, null, 'insufficient_evidence'), metric('p95', 40, null, 'insufficient_evidence')]
  }]
  await openCompare(page, { compare: () => ({ data: { Analysis: a } }) })
  await runComparison(page)
  await expect(page.getByTestId('row-note')).toContainText('does not mean it was fixed')
  await expect(metricRow(page, 'calls').getByTestId('metric-candidate')).toHaveText('0')
  await expect(metricRow(page, 'p95').getByTestId('metric-candidate')).toHaveText('unavailable')
  await expect(metricRow(page, 'error_fraction').getByTestId('metric-candidate')).toHaveText('unavailable')
})

test('M2-INV-001 / M2-REF-001: a slow example opens its trace in the population window, keeps the context and can be exported', async ({ page }) => {
  const api = await openCompare(page, {
    compare: () => ({ data: { Analysis: regression() } }),
    examples: () => ({ data: { Examples: {
      status: 'ok', condition: 'ok', limit: 10, shown: 1, total: 1000, truncated: true, thresholdMs: 160, cutoff: '1', description: 'Observed slow calls at or above the threshold, slowest first.',
      examples: [{ sourceKind: 'span', traceId: FAILED_TRACE, spanId: UPLOAD, time: (T0 + 2n * HOUR + 1000n).toString(), durationMs: 160, service: 'recording-worker', operation: 'upload_recording', version: '1.1.0', message: 'Upload failed' }]
    } } })
  })
  await runComparison(page)
  await page.getByTestId('row-slow-examples').click()
  await expect(page.getByTestId('examples-summary')).toContainText('1 of 1000 shown · truncated')
  const ex = api.exampleRequests[0]
  expect(ex).toMatchObject({ kind: 'slow', threshold_ms: 160, population: { version: '1.1.0', operation: 'upload_recording', span_kind: 'consumer' } })
  expect(ex.cutoff).toBe((T0 + 8n * HOUR).toString())

  await page.getByTestId('open-example-report').click()
  await expect(page.getByTestId('report')).toBeVisible()
  const report = api.reportRequests[0]
  expect(report).toMatchObject({ trace_id: FAILED_TRACE, time_from: (T0 + 2n * HOUR).toString(), time_to: (T0 + 3n * HOUR).toString() })
  await expect(page.getByTestId('report-context')).toContainText('revision a1b2c3d4e5f6a1b2c3d4e5f6')
  await expect(page.getByTestId('report-context')).toContainText('observed slow call')
  await expect(page.getByTestId('inspector-span')).toHaveText('upload_recording')
  // INFO and WARN context of the trace is not hidden by the example filters.
  await page.getByTestId('scope-trace').click()
  await expect(page.getByTestId('trace-log-message')).toHaveText(['Recording request accepted', 'Upload retry 1 of 3', 'Storage returned 403 AccessDenied'])

  // Widening the report does not touch the comparison.
  await page.getByTestId('widen-window').click()
  await expect.poll(() => api.reportRequests.length).toBe(2)
  expect(api.compareRequests).toHaveLength(1)

  await page.getByTestId('include-report').click()
  await expect(page.getByTestId('report-included')).toBeVisible()
  await page.getByTestId('return-origin').click()
  await expect(page.getByTestId('included-report')).toHaveCount(1)

  const downloadHtml = page.waitForEvent('download')
  await page.getByTestId('analysis-export-html').click()
  const html = await readFile((await (await downloadHtml).path())!, 'utf8')
  expect(html).toContain('Selected example 1 of 1')
  expect(html).toContain('Storage returned 403 AccessDenied')
  expect(html).toContain('+60.0 ms')
  expect(html).toContain('comparison-v1')

  const downloadJson = page.waitForEvent('download')
  await page.getByTestId('analysis-export-json').click()
  const data = JSON.parse(await readFile((await (await downloadJson).path())!, 'utf8'))
  expect(data.analysis.id).toBe('a1b2c3d4e5f6a1b2c3d4e5f6')
  expect(data.embeddedReports).toHaveLength(1)
  expect(data.embeddedReports[0].role).toBe('selected example')
  expect(data.embeddedReports[0].report.logs.records.map((l: { message: string }) => l.message)).toContain('Storage returned 403 AccessDenied')
})

test('M2-INV-002: an error log example opens its report with the log highlighted', async ({ page }) => {
  await openCompare(page, {
    compare: () => ({ data: { Analysis: regression() } }),
    examples: () => ({ data: { Examples: {
      status: 'ok', condition: 'ok', limit: 10, shown: 1, total: 50, truncated: true, cutoff: '1', description: 'Recorded occurrences of this error group.',
      examples: [{ sourceKind: 'log', traceId: FAILED_TRACE, spanId: UPLOAD, recordId: denied.recordId, time: denied.timeUnixNano, service: 'recording-worker', operation: 'upload_recording', version: '1.1.0', message: denied.message }]
    } } })
  })
  await runComparison(page)
  await page.getByTestId('group-candidate-examples').nth(1).click()
  await expect(page.getByTestId('example-row')).toContainText('Storage returned 403 AccessDenied')
  await page.getByTestId('open-example-report').click()
  await expect(page.getByTestId('log-detail-message')).toHaveText('Storage returned 403 AccessDenied')
  await expect(page.getByTestId('inspector-span')).toHaveText('upload_recording')
  await expect(page.getByTestId('report-context')).toContainText('Storage returned 403 AccessDenied')
})

test('M2-INV-003: bounded examples are labeled as examples, not the population', async ({ page }) => {
  await openCompare(page, {
    compare: () => ({ data: { Analysis: regression() } }),
    examples: () => ({ data: { Examples: {
      status: 'ok', condition: 'ok', limit: 5, shown: 5, total: 40, truncated: true, cutoff: '1',
      description: 'Observed slow calls at or above the threshold, slowest first. They are examples of the population, not the full set of contributing calls, and not a single "p95 trace".',
      examples: Array.from({ length: 5 }, (_, i) => ({ sourceKind: 'span', traceId: FAILED_TRACE, spanId: UPLOAD, time: String(i), durationMs: 900, service: 's', operation: 'o', version: 'v', message: '' }))
    } } })
  })
  await runComparison(page)
  await page.getByTestId('row-slow-examples').click()
  await expect(page.getByTestId('examples-summary')).toContainText('5 of 40 shown · truncated: more examples match (limit 5)')
  await expect(page.getByTestId('examples-summary')).toContainText('not a single "p95 trace"')
  await expect(page.getByTestId('example-row')).toHaveCount(5)
})

test('M2-REP-002 / M2-INV-004: regenerating creates a new revision and the earlier one stays available', async ({ page }) => {
  const first = regression('a1b2c3d4e5f6a1b2c3d4e5f6')
  const second = regression('bbbbbbbbbbbbbbbbbbbbbbbb', { regenerates: first.id })
  second.rows[0].candidate = callStats(1010, 50, 160)
  let n = 0
  const api = await openCompare(page, {
    compare: () => ({ data: { Analysis: n++ === 0 ? first : second } }),
    analyses: () => ({ data: { Analyses: [{ id: second.id, kind: 'comparison', title: 'second', createdAt: '2' }, { id: first.id, kind: 'comparison', title: 'first', createdAt: '1' }] } }),
    analysis: (id) => ({ data: { Analysis: id === first.id ? first : second } })
  })
  await runComparison(page)
  await expect(page.getByTestId('row-samples')).toContainText('Candidate: 1000 calls')
  await page.getByTestId('analysis-regenerate').click()
  await expect(page.getByTestId('analysis-revision')).toContainText('regenerates a1b2c3d4e5f6a1b2c3d4e5f6')
  expect(api.compareRequests[1].regenerates).toBe(first.id)
  await expect(page.getByTestId('row-samples')).toContainText('Candidate: 1010 calls')

  await page.getByTestId('saved-analyses').click()
  await page.locator('.p-dropdown-item', { hasText: 'first' }).click()
  await expect(page.getByTestId('analysis-revision')).toContainText('Revision a1b2c3d4e5f6a1b2c3d4e5f6')
  await expect(page.getByTestId('row-samples')).toContainText('Candidate: 1000 calls')
})

test('M2-ERR-005 / M2-REP-003: unavailable error evidence is undetermined, never zero', async ({ page }) => {
  const a = regression()
  a.errorGroups = [{ ...DENIED_GROUP, baseline: { occurrences: null, affectedTraces: null, unlinked: null }, candidate: { occurrences: 2, affectedTraces: 2, unlinked: 0 },
    label: 'undetermined', reason: 'novelty undetermined: baseline logs evidence is unavailable', absoluteChange: null, relativeChange: null } as any]
  await openCompare(page, { compare: () => ({ data: { Analysis: a } }) })
  await runComparison(page)
  await expect(page.getByTestId('group-baseline')).toHaveText('unavailable')
  await expect(page.getByTestId('group-label')).toContainText('Undetermined')
  await expect(page.getByTestId('group-label')).toContainText('baseline logs evidence is unavailable')
})

// --- statistics of one population

async function openStats (page: Page, options: MockOptions) {
  const api = await mockApi(page, () => ({ spans: [] }), () => ({ count: 0 }), options)
  await page.goto('/')
  await page.getByTestId('main-nav-releases').click()
  await page.getByTestId('releases-stats').click()
  await page.getByTestId('stats-pop-service').fill('recording-worker')
  await page.getByTestId('stats-load').click()
  return api
}

const noGroups = () => ({ data: { ErrorGroups: { status: 'ok', condition: 'no recorded error evidence in this scope', groups: [], notices: [], plan: [], cutoff: '1', generatedAt: '1',
  sources: { spans: { status: 'ok', coverage: { status: 'complete', reasons: [] } }, logs: { status: 'ok', coverage: { status: 'complete', reasons: [] } } },
  definitions: { errors: 'errors-v1', grouping: 'g', sourceKind: 's' } } } })

test('M2-CAL-005: a timed-out query is unavailable, an empty result is a confirmed zero', async ({ page }) => {
  let n = 0
  await openStats(page, {
    stats: () => (n++ === 0
      ? { data: { Stats: populationStats('1.0.0', T0, [], { status: 'timeout', condition: 'statistics query timed out', error: 'the query took longer than 10s' }) } }
      : { data: { Stats: populationStats('1.0.0', T0, [callStats(0, 0, null)], { condition: 'no observed calls in this scope' }) } }),
    errorGroups: noGroups
  })
  await expect(page.getByTestId('stats-status')).toContainText('statistics query timed out')
  await expect(page.getByTestId('stats-rows')).toHaveCount(0)
  await page.getByTestId('stats-load').click()
  await expect(page.getByTestId('stats-condition')).toHaveText('no observed calls in this scope')
  await expect(page.getByTestId('stats-calls')).toHaveText('0')
  await expect(page.getByTestId('stats-error-fraction')).toHaveText('unavailable')
  await expect(page.getByTestId('stats-p95')).toHaveText('unavailable')
})

test('M2-CAL-008 / M2-LAT-004: a coverage gap is not a zero; small samples are insufficient', async ({ page }) => {
  const b = (i: bigint) => (T0 + i * HOUR).toString()
  await openStats(page, {
    stats: () => ({ data: { Stats: populationStats('1.0.0', T0, [callStats(100, 1, 12, { validDurations: 100 })], {
      coverage: { status: 'gap', coveredMinutes: 120, minutes: 180, reasons: ['declared collection gap: outage'] },
      series: [
        { from: b(0n), to: b(1n), source: 'stored', state: 'final', coverage: 'complete', calls: 100, errors: 1, available: true },
        { from: b(1n), to: b(2n), source: 'stored', state: 'final', coverage: 'complete', calls: 0, errors: 0, available: true },
        { from: b(2n), to: b(3n), source: 'raw', coverage: 'gap', calls: null, errors: null, available: false }
      ]
    }) } }),
    errorGroups: noGroups
  })
  await expect(page.getByTestId('series-calls')).toHaveText(['100', '0', 'coverage gap — not a confirmed zero'])
  await expect(page.getByTestId('stats-coverage')).toContainText('gap')
  await expect(page.getByTestId('stats-p50')).toHaveText('12.0 ms')
  await expect(page.getByTestId('stats-p95')).toHaveText('insufficient sample (100 of 200)')
  await expect(page.getByTestId('stats-p99')).toHaveText('insufficient sample (100 of 1000)')
})

test('M2-RET-009: sub-hour bounds without raw evidence offer explicit whole-hour bounds', async ({ page }) => {
  const api = await openStats(page, {
    stats: (body) => (body.population.time_from === T0.toString()
      ? { data: { Stats: populationStats('1.0.0', T0, [callStats(1000, 10, 100)], { durationSeconds: 7200 }) } }
      : { data: { Stats: populationStats('1.0.0', T0, [], { status: 'exact_bounds_unavailable', condition: 'exact statistics for these bounds are unavailable', suggestedFrom: T0.toString(), suggestedTo: (T0 + 2n * HOUR).toString() }) } }),
    errorGroups: noGroups
  })
  await expect(page.getByTestId('stats-status')).toContainText('exact statistics for these bounds are unavailable')
  await page.getByTestId('stats-use-whole-buckets').click()
  await expect(page.getByTestId('stats-calls')).toHaveText('1000')
  expect(api.statsRequests.at(-1)!.population).toMatchObject({ time_from: T0.toString(), time_to: (T0 + 2n * HOUR).toString() })
  await expect(page.getByTestId('stats-header')).toContainText('2 h')
})

test('M2-RET-001: expired raw evidence is explained, never reconstructed', async ({ page }) => {
  await openStats(page, {
    stats: () => ({ data: { Stats: populationStats('1.0.0', T0, [callStats(1000, 10, 100)]) } }),
    errorGroups: () => ({ data: { ErrorGroups: { ...noGroups().data.ErrorGroups, condition: 'ok', groups: [{ ...STORAGE_GROUP, occurrences: 10, affectedTraces: 10, versions: [{ version: '1.0.0', occurrences: 10 }] }] } } }),
    examples: () => ({ data: { Examples: { status: 'outside_retention', condition: 'The raw spans and logs of this window are outside retention: statistics remain, but individual traces, log messages and stack traces can no longer be shown.', examples: [], limit: 10, shown: 0, total: 0, truncated: false, cutoff: '1', description: '' } } })
  })
  await expect(page.getByTestId('group-occurrences')).toHaveText('10')
  await page.getByTestId('group-examples').click()
  await expect(page.getByTestId('examples-status')).toContainText('outside retention')
  await expect(page.getByTestId('example-row')).toHaveCount(0)
})

test('M2-REP-003: an unavailable error-log source is marked unavailable next to available span statistics', async ({ page }) => {
  await openStats(page, {
    stats: () => ({ data: { Stats: populationStats('1.0.0', T0, [callStats(1000, 10, 100)]) } }),
    errorGroups: () => ({ data: { ErrorGroups: { ...noGroups().data.ErrorGroups, condition: 'partial: logs unavailable',
      sources: { spans: { status: 'ok', coverage: { status: 'complete', reasons: [] } }, logs: { status: 'unavailable', error: 'the database is unavailable', coverage: { status: 'unknown', reasons: [] } } } } } })
  })
  await expect(page.getByTestId('stats-calls')).toHaveText('1000')
  await expect(page.getByTestId('groups-source').filter({ hasText: 'logs' })).toContainText('unavailable — its groups are missing, not zero')
})
