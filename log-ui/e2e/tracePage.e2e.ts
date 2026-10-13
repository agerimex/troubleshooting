import { expect, test, type Page } from '@playwright/test'
import { TRACE, mockApi, span } from './mockApi'

// Focused UI checks of the span search (step1.md, section 4): finding a child
// error, opening its trace report and evidence, an unavailable total,
// truncation, retry and stale responses. The combined report is in report.e2e.ts.

// Log Analysis stays mounted (hidden) while an execution is open, so count visible rows only.
const rows = (page: Page) => page.locator('[data-testid=span-name]').filter({ visible: true })
// The spans of the open execution (Timeline).
const timelineSpans = (page: Page) => page.getByTestId('timeline-name')

async function selectStatus (page: Page, status: string) {
  await page.getByTestId('filter-status').click()
  await page.locator('.p-dropdown-item', { hasText: status }).click()
}

const exception = {
  'exception.type': 'AccessDeniedException',
  'exception.message': 'access denied to bucket recordings',
  'exception.stacktrace': 'AccessDeniedException: access denied\n\tat storage.Put(client.go:118)\n\tat upload.Run(worker.go:31)'
}

test('an error search finds a failed child, and its trace shows the exception and link', async ({ page }) => {
  const root = span('POST /recordings', { spanId: '00000000000000a0', status: 'ok', childSpanCount: 1 })
  const pipeline = span('upload pipeline', { spanId: '00000000000000a1', parentSpanId: '00000000000000a0', childSpanCount: 1 })
  const upload = span('UploadToStorage', {
    spanId: '00000000000000a3', parentSpanId: '00000000000000a1', status: 'error', statusMessage: 'upload failed',
    events: [{ timeUnixNano: '1760090000219000000', name: 'exception', attributes: exception }],
    links: [{ traceId: 'ffffffffffffffffffffffffffffff01', spanId: '00000000000000f1', traceState: '', attributes: { 'link.reason': 'retry of upload' } }]
  })
  const api = await mockApi(page, (body) => {
    if (body.trace_id) {
      return { spans: [root, pipeline, upload] }
    }
    if (body.status === 'error') {
      // The backend's answer for scope=all; roots only would return nothing.
      return { spans: body.scope === 'all' ? [upload] : [] }
    }
    return { spans: [root] }
  }, () => ({ count: 1 }))

  await page.goto('/')
  await expect(rows(page)).toHaveText(['POST /recordings'])

  await selectStatus(page, 'error')
  await expect(rows(page)).toHaveText(['UploadToStorage'])
  const search = api.spanRequests.at(-1)!
  expect(search.scope).toBe('all')
  expect(search.status).toBe('error')
  expect(BigInt(search.time_to)).toBeGreaterThan(BigInt(search.time_from))
  // [from, to): the end is the first nanosecond after the selected millisecond.
  expect(BigInt(search.time_to) % 1000000n).toBe(0n)

  await page.getByTestId('open-trace').first().click()
  await expect(page.getByTestId('trace-id')).toHaveText(TRACE)
  await expect(timelineSpans(page)).toHaveText(['POST /recordings', 'upload pipeline', 'UploadToStorage'])
  // The report gets the trace and the window of the search, nothing else.
  const reportRequest = api.reportRequests.at(-1)!
  expect(reportRequest).toEqual({ trace_id: TRACE, time_from: search.time_from, time_to: search.time_to })
  await expect(page.getByTestId('spans-summary')).toContainText('3 spans shown · complete')

  await page.getByTestId('timeline-row').filter({ hasText: 'UploadToStorage' }).click()
  await page.getByTestId('report').getByTestId('evidence').click()
  const dialog = page.getByTestId('evidence-dialog')
  await expect(dialog.getByTestId('status-message')).toHaveText('upload failed')
  await expect(dialog.getByTestId('exception')).toContainText('AccessDeniedException: access denied to bucket recordings')
  await expect(dialog.getByTestId('stacktrace')).toContainText('at upload.Run(worker.go:31)')
  await expect(dialog.getByTestId('link-trace')).toHaveText('ffffffffffffffffffffffffffffff01')
  await expect(dialog.getByTestId('link-span')).toHaveText('00000000000000f1')
  await expect(dialog).toContainText('retry of upload')
  await expect(dialog.getByTestId('link-not-inspected')).toBeVisible()
})

test('a span without recorded events says so, without claiming there was no error', async ({ page }) => {
  await mockApi(page, () => ({ spans: [span('GET /health')] }), () => ({ count: 1 }))
  await page.goto('/')
  await page.getByTestId('evidence').click()
  await expect(page.getByTestId('no-events')).toContainText('does not show that no error happened')
  await expect(page.getByTestId('no-links')).toBeVisible()
})

test('RF11: when the count fails the rows stay and the total is unavailable', async ({ page }) => {
  let countFails = true
  await mockApi(page, () => ({ spans: [span('a'), span('b', { spanId: '00000000000000b0' })], hasMore: true }),
    () => (countFails ? { error: 'count timed out' } : { count: 7 }))
  await page.goto('/')

  await expect(rows(page)).toHaveText(['a', 'b'])
  await expect(page.getByTestId('total')).toContainText('Total unavailable: count timed out')
  await expect(page.getByTestId('total')).not.toContainText('of 0')
  // Paging doesn't depend on the total.
  await expect(page.locator('.p-paginator-next')).toBeEnabled()

  countFails = false
  await page.getByTestId('retry-total').click()
  await expect(page.getByTestId('total')).toContainText('Rows 1–2 of 7')
})

test('a failed count is never replaced by the previous total', async ({ page }) => {
  let call = 0
  await mockApi(page, () => ({ spans: [span('a')] }), () => (++call === 1 ? { count: 5 } : { error: 'boom' }))
  await page.goto('/')
  await expect(page.getByTestId('total')).toContainText('of 5')
  await page.getByTestId('refresh').click()
  await expect(page.getByTestId('total')).toContainText('Total unavailable')
  await expect(page.getByTestId('total')).not.toContainText('of 5')
})

test('RF10: exactly N children are complete; more are marked as truncated under their parent', async ({ page }) => {
  const parents = [
    span('exactly', { traceId: 'a'.repeat(32), childSpanCount: 2 }),
    span('more', { traceId: 'b'.repeat(32), childSpanCount: 1001 })
  ]
  const api = await mockApi(page, (body) => {
    if (body.parent_id) {
      const kids = [span('child-1', { traceId: body.trace_id, spanId: '00000000000000c1', parentSpanId: body.parent_id }),
        span('child-2', { traceId: body.trace_id, spanId: '00000000000000c2', parentSpanId: body.parent_id })]
      return { spans: kids, hasMore: body.trace_id === 'b'.repeat(32) }
    }
    return { spans: parents }
  }, () => ({ count: 2 }))
  await page.goto('/')

  await page.locator('tr', { hasText: 'exactly' }).locator('.p-treetable-toggler').click()
  await expect(rows(page)).toHaveCount(4)
  await expect(page.getByTestId('node-notice')).toHaveCount(0)
  const expand = api.spanRequests.at(-1)!
  // Children are asked for by full identity, within the search window.
  expect(expand).toMatchObject({ trace_id: 'a'.repeat(32), parent_id: '00000000000000a0', rows_per_page: 1000 })
  expect(expand.time_from).toBe(api.spanRequests[0].time_from)
  expect(expand.time_to).toBe(api.spanRequests[0].time_to)

  await page.locator('tr', { hasText: 'more' }).locator('.p-treetable-toggler').click()
  await expect(page.getByTestId('node-notice')).toHaveText(/Only the first 1000 child spans are shown/)
})

test('a child load error stays on its node and can be retried', async ({ page }) => {
  let fail = true
  await mockApi(page, (body) => {
    if (body.parent_id) {
      return fail ? { error: 'connection reset' } : { spans: [span('child', { parentSpanId: body.parent_id, spanId: '00000000000000c1' })] }
    }
    return { spans: [span('parent', { childSpanCount: 1 })] }
  }, () => ({ count: 1 }))
  await page.goto('/')
  await page.locator('.p-treetable-toggler').first().click()
  await expect(page.getByTestId('node-error')).toContainText('Could not load child spans: connection reset')

  fail = false
  await page.getByTestId('retry-children').click()
  await expect(rows(page)).toHaveText(['parent', 'child'])
  await expect(page.getByTestId('node-error')).toHaveCount(0)
})

test('RF13: a failed search is an explicit error, not an empty result, and Retry recovers', async ({ page }) => {
  let fail = true
  await mockApi(page, () => (fail ? { status: 504, error: 'query limit exceeded: the query took longer than 10s' } : { spans: [span('recovered')] }),
    () => ({ count: 1 }))
  await page.goto('/')
  await expect(page.getByTestId('notice')).toContainText('Could not load spans: query limit exceeded')
  await expect(page.getByTestId('empty')).toHaveCount(0)
  await expect(page.getByTestId('total')).not.toContainText('of 0')

  fail = false
  await page.getByTestId('retry').click()
  await expect(rows(page)).toHaveText(['recovered'])
  await expect(page.getByTestId('notice')).toHaveCount(0)
})

test('an empty result says that nothing matched', async ({ page }) => {
  await mockApi(page, () => ({ spans: [] }), () => ({ count: 0 }))
  await page.goto('/')
  await expect(page.getByTestId('empty')).toContainText('No spans match')
  await expect(page.getByTestId('notice')).toHaveCount(0)
})

test('RF14: a slow response for an old filter cannot overwrite the current one', async ({ page }) => {
  await mockApi(page, (body) => (body.method_name
    ? { spans: [span('new filter')] }
    : { spans: [span('old filter')], delayMs: 1500 }), () => ({ count: 1 }))
  await page.goto('/')
  await page.getByTestId('filter-name').fill('new')
  await expect(rows(page)).toHaveText(['new filter'])
  await page.waitForTimeout(2000) // the old response has arrived by now
  await expect(rows(page)).toHaveText(['new filter'])
})

test('RF05/RF06: a missing parent is described within the window; widening attaches it', async ({ page }) => {
  const child = span('orphan child', { spanId: '00000000000000a1', parentSpanId: '00000000000000a0' })
  const parent = span('late parent', { spanId: '00000000000000a0', childSpanCount: 1 })
  const api = await mockApi(page, (body) => {
    if (!body.trace_id) {
      return { spans: [child] }
    }
    const widened = BigInt(body.time_to) - BigInt(body.time_from) > 2n * 3600n * 1000000000n
    return { spans: widened ? [parent, child] : [child] }
  }, () => ({ count: 1 }))
  await page.goto('/')
  await page.getByTestId('open-trace').click()
  await expect(page.getByTestId('timeline-parent-missing')).toBeVisible()

  const before = api.reportRequests.at(-1)!
  await page.getByTestId('widen-window').click()
  await expect(timelineSpans(page)).toHaveText(['late parent', 'orphan child'])
  await expect(page.getByTestId('timeline-parent-missing')).toHaveCount(0)
  const after = api.reportRequests.at(-1)!
  expect(BigInt(before.time_from) - BigInt(after.time_from)).toBe(3600n * 1000000000n)
  expect(BigInt(after.time_to) - BigInt(before.time_to)).toBe(3600n * 1000000000n)
  await expect(page.getByTestId('report-scope')).toBeVisible()
})

test('paging sends the full cursor and keeps the window', async ({ page }) => {
  const first = [span('p1-a', { spanId: '0000000000000001' }), span('p1-b', { spanId: '0000000000000002', traceId: 'c'.repeat(32) })]
  const api = await mockApi(page, (body) => (body.after_time ? { spans: [span('p2')] } : { spans: first, hasMore: true }), () => ({ count: 3 }))
  await page.goto('/')
  await expect(rows(page)).toHaveText(['p1-a', 'p1-b'])
  await page.locator('.p-paginator-next').click()
  await expect(rows(page)).toHaveText(['p2'])
  const [one, two] = [api.spanRequests[0], api.spanRequests.at(-1)!]
  expect(two).toMatchObject({ after_time: first[1].timeStamp, after_trace_id: 'c'.repeat(32), after_span_id: '0000000000000002' })
  expect([two.time_from, two.time_to]).toEqual([one.time_from, one.time_to])
  // The total isn't counted again for the same traversal.
  expect(api.countRequests).toHaveLength(1)
})

test('opening a trace by id validates it', async ({ page }) => {
  const api = await mockApi(page, (body) => ({ spans: body.trace_id ? [span('found', { traceId: body.trace_id })] : [] }))
  await page.goto('/')
  await page.getByTestId('trace-id-input').fill('not-an-id')
  await page.getByTestId('open-trace-by-id').click()
  await expect(page.getByTestId('notice')).toContainText('32 hex digits')

  await page.getByTestId('trace-id-input').fill(TRACE.toUpperCase())
  await page.getByTestId('open-trace-by-id').click()
  await expect(timelineSpans(page)).toHaveText(['found'])
  expect(api.reportRequests.at(-1)!.trace_id).toBe(TRACE)
  await page.getByTestId('return-origin').click()
  await expect(page.getByTestId('trace-id-input')).toBeVisible()
})
