import { readFile } from 'node:fs/promises'
import { expect, test, type Page } from '@playwright/test'
import { mockApi, type Body, type MockOptions, type ReportReply } from './mockApi'
import { sequenceCases } from './seqReference'

// UI checks of the Sequence view of Timeline & Sequence (troubleshooting-sequence-agent),
// entered from a problem example.
// Titles start with the scenario tag they check.

const C = sequenceCases
const UPLOAD = '0000000000000002'
const HOUR = 3_600_000_000_000n
const FROM = (BigInt(C.error.spans[0].timeStamp) / HOUR) * HOUR

const rules = { scope: 'installation', revision: 1, errors: { enabled: true, minAffectedCalls: 1, grouping: 'operation_signature' }, slow: { enabled: true, durationMs: 500, minCalls: 5 }, regressions: { enabled: true, p95RelativePct: 20, p95AbsoluteMs: 20 } }

const problem = (type = 'error') => ({
  id: 'PRB-0a1b2c3d4e',
  type,
  title: 'Upload failed: AccessDenied',
  service: 'recording-api',
  operation: 'upload_recording',
  environment: 'demo',
  qualified: true,
  error: type === 'error'
    ? { key: 'k', sourceKind: 'log', service: 'recording-api', operation: 'upload_recording', environment: 'demo', method: 'message', fingerprint: 'f', exceptionType: '', frame: '', signature: 'Upload failed: AccessDenied', occurrences: 240, unlinked: 0, affectedTraces: 100, affectedCalls: 100, firstSeen: '1', lastSeen: '2', versions: [{ version: '1.1.0', occurrences: 240 }] }
    : undefined,
  slow: type === 'slow_calls' ? { thresholdMs: 200, calls: 8, versions: [] } : undefined,
  explanation: { ruleRevision: 1, rule: 'r', observed: 'o', threshold: 't', basis: 'b', scope: 's', coverage: 'c' }
})

const list = (p = problem()) => ({ data: { Problems: { scope: { service: 'recording-api', time_from: FROM.toString(), time_to: (FROM + HOUR).toString() }, rules, status: 'available', condition: 'ok', sources: {}, problems: [p], total: 1, generatedAt: '1', notices: [] } } })

const example = (id: string) => ({ sourceKind: 'log', traceId: C[id].traceId, spanId: UPLOAD, recordId: '', time: C[id].spans[0]?.timeStamp ?? C.error.spans[0].timeStamp, service: 'recording-api', operation: 'upload_recording', version: C[id].release, message: 'Upload failed: AccessDenied' })
const examples = (ids: string[]) => ({ data: { Examples: { status: 'ok', condition: 'ok', examples: ids.map(example), limit: 20, shown: ids.length, total: ids.length, truncated: false, description: '', cutoff: '1' } } })

function reportOf (body: Body): ReportReply {
  const c = Object.values(C).find((x: any) => x.traceId === body.trace_id) as any
  if (c.id === 'logs-unavailable') {
    return { spans: { records: c.spans }, logs: { status: 'unavailable', error: 'the database is unavailable' } }
  }
  return { spans: { records: c.spans }, logs: { records: c.logs } }
}

async function openExample (page: Page, ids: string[], options: MockOptions = {}, p = problem()) {
  const api = await mockApi(page, () => ({ spans: [] }), () => ({ count: 0 }), {
    services: () => ({ data: { Services: ['recording-api'] } }),
    rules: () => ({ data: { Rules: rules } }),
    problems: () => list(p),
    examples: () => examples(ids),
    report: reportOf,
    ...options
  })
  await page.goto('/')
  await page.evaluate(() => window.localStorage.clear())
  await page.getByTestId('main-nav-problems').click()
  await page.getByTestId('problems-service').fill('recording-api')
  await page.getByTestId('problems-load').click()
  await page.getByTestId('problem-investigate').click()
  await page.getByTestId('example-open-sequence').click()
  await expect(page.getByTestId('report-summary')).toBeVisible()
  return api
}

const inspector = (page: Page) => page.getByTestId('log-inspector')

test('SEQ-UI-001 / SEQ-GEN-001: an error example opens as a sequence of its own release and time', async ({ page }) => {
  await openExample(page, ['error'])
  await expect(page.getByTestId('sequence')).toBeVisible()
  await expect(page.getByTestId('view-sequence')).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByTestId('trace-id')).toHaveText(C.error.traceId)
  await expect(page.getByTestId('origin')).toContainText('Upload failed: AccessDenied')
  await expect(page.getByTestId('seq-participant')).toHaveText(['recording-api', 'thumbnail-worker', 'object-storage'])
  await expect(page.getByTestId('seq-counts')).toHaveText('4 operations / 6 spans')
  await expect(page.getByTestId('seq-call')).toHaveCount(2)
  await expect(page.getByTestId('seq-overlap')).toContainText('overlap for 122 ms')
  await expect(page.getByTestId('sequence')).not.toContainText(/200 OK|success|retry/i)
})

test('SEQ-UI-002: an ERROR log marker on an OK span selects that span and opens the log', async ({ page }) => {
  await openExample(page, ['logs-only'])
  await page.getByTestId('seq-log').filter({ hasText: 'Upload failed: AccessDenied' }).click()
  await expect(inspector(page).getByTestId('inspector-span')).toHaveText('upload_recording')
  await expect(inspector(page).getByTestId('inspector-span-meta')).toContainText('OK')
  await expect(page.getByTestId('log-detail-message')).toHaveText('Upload failed: AccessDenied')
})

test('SEQ-UI-003: either endpoint of a CLIENT/SERVER pair is selectable with its own evidence', async ({ page }) => {
  await openExample(page, ['error'])
  await page.getByTestId('seq-call').filter({ hasText: 'PutObject' }).click()
  await expect(page.getByTestId('pair-switch')).toBeVisible()
  await expect(inspector(page).getByTestId('inspector-span')).toHaveText('PutObject')
  await expect(inspector(page).getByTestId('inspector-span-meta')).toContainText('CLIENT')
  await page.getByTestId('pair-server').click()
  await expect(inspector(page).getByTestId('inspector-span')).toHaveText('PUT /recordings')
  await expect(inspector(page).getByTestId('inspector-span-meta')).toContainText('object-storage')
  await page.getByTestId('view-list').click()
  await expect(page.getByTestId('log-list-row')).toHaveText([/AccessDenied: bucket policy rejected the request/])
  await page.getByTestId('pair-client').click()
  await expect(inspector(page).getByTestId('inspector-span')).toHaveText('PutObject')
})

test('SEQ-UI-004: Sequence and Timing share the report and selection without a new request', async ({ page }) => {
  const api = await openExample(page, ['error'])
  await page.getByTestId('seq-call').filter({ hasText: 'GenerateThumbnail' }).click()
  const requests = api.reportRequests.length
  const stats = api.statsRequests.length
  await page.getByTestId('view-timeline').click()
  await expect(page.getByTestId('timeline-row').filter({ hasText: 'GenerateThumbnail' })).toHaveAttribute('aria-pressed', 'true')
  await page.getByTestId('view-sequence').click()
  await expect(page.getByTestId('seq-call').filter({ hasText: 'GenerateThumbnail' })).toHaveAttribute('aria-pressed', 'true')
  expect(api.reportRequests.length).toBe(requests)
  expect(api.statsRequests.length).toBe(stats)
})

test('SEQ-UI-005 / SEQ-GEN-010: trace-level and missing-span logs stay accessible; a missing parent is explicit', async ({ page }) => {
  await openExample(page, ['partial'])
  await expect(page.getByTestId('seq-limitation').filter({ hasText: 'parent that isn' })).toBeVisible()
  await page.getByTestId('scope-trace').click()
  await expect(page.getByTestId('trace-log-row').filter({ hasText: 'Trace-level audit event' }).getByTestId('trace-log-association')).toHaveText('No span identifier recorded')
  await expect(page.getByTestId('trace-log-row').filter({ hasText: 'Upload started' }).getByTestId('trace-log-association')).toHaveText('Referenced span not found within the window')
})

test('SEQ-UI-008: an unavailable log source keeps the sequence usable and says so', async ({ page }) => {
  await openExample(page, ['logs-unavailable'])
  await expect(page.getByTestId('seq-call')).toHaveCount(2)
  await expect(page.getByTestId('seq-limitation').filter({ hasText: 'Log evidence is unavailable' })).toBeVisible()
  await expect(page.getByTestId('inspector-logs-unavailable')).toBeVisible()
  await expect(page.getByTestId('no-span-logs-recorded')).toHaveCount(0)
})

test('SEQ-UI-009: a slow report for example A cannot replace example B', async ({ page }) => {
  await openExample(page, ['error', 'logs-only'], { report: (body) => ({ ...reportOf(body), delayMs: body.trace_id === C.error.traceId ? 1500 : 0 }) })
  await page.getByTestId('return-origin').click()
  await page.getByTestId('example-select').selectOption({ index: 1 })
  await page.getByTestId('example-open-sequence').click()
  await expect(page.getByTestId('trace-id')).toHaveText(C['logs-only'].traceId)
  await expect(page.getByTestId('sequence')).toBeVisible()
  await page.waitForTimeout(1800)
  await expect(page.getByTestId('trace-id')).toHaveText(C['logs-only'].traceId)
  await expect(page.getByTestId('seq-counts')).toBeVisible()
})

test('SEQ-RES-001: a large trace is paged within the mark budget and the selection can be located', async ({ page }) => {
  await openExample(page, ['large'])
  await expect(page.getByTestId('seq-pager')).toContainText('Marks 1–80 of')
  expect(await page.getByTestId('sequence').locator('.row').count()).toBe(80)
  await page.getByTestId('seq-later').click()
  await expect(page.getByTestId('seq-pager')).toContainText('Marks 81–')
  await page.getByTestId('seq-locate').click()
  await expect(page.getByTestId('seq-pager')).toContainText('Marks 1–80 of')
  await expect(page.locator('[data-testid=seq-start][aria-pressed=true]').first()).toBeVisible()
})

test('SEQ-UI-011: a narrow screen shows a labeled event list without page overflow', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 800 })
  await openExample(page, ['error'])
  await expect(page.getByTestId('seq-list')).toBeVisible()
  await expect(page.getByTestId('seq-list-item').filter({ hasText: 'recording-api → object-storage · call PutObject' })).toBeVisible()
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
  expect(overflow).toBeLessThanOrEqual(1)
})

test('SEQ-RES-006 / SEQ-RES-007: exports keep hidden operations, the view and inert text', async ({ page }) => {
  const evil = structuredClone(C.error)
  evil.logs[6].message = '<img src=x onerror=alert(1)>'
  await openExample(page, ['error'], { report: () => ({ spans: { records: evil.spans }, logs: { records: evil.logs } }) })
  await page.getByTestId('seq-internal').uncheck()
  await expect(page.getByTestId('seq-start').filter({ hasText: 'upload_recording' })).toHaveCount(1) // the selected example span stays
  const json = page.waitForEvent('download')
  await page.getByTestId('export-json').click()
  const j = JSON.parse(await readFile(await (await json).path(), 'utf8'))
  expect(j.spans.records).toHaveLength(6)
  expect(j.logs.records).toHaveLength(12)
  expect(j.presentation.renderer).toBe('sequence-semantics-v1')
  expect(j.presentation.operations).toHaveLength(4)
  expect(j.presentation.view.view).toBe('sequence')
  const html = page.waitForEvent('download')
  await page.getByTestId('export-html').click()
  const h = await readFile(await (await html).path(), 'utf8')
  expect(h).not.toContain('<img src=x')
  expect(h).toContain('&lt;img src=x')
  expect(h).not.toMatch(/<script/i)
  expect(h).toContain('recording-api → object-storage · call PutObject')
})
