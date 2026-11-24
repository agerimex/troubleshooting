import { expect, test } from '@playwright/test'
import { TRACE, mockApi, span, type MockSpan, type ReportReply } from './mockApi'

// Repeated calls folded in the Timeline, and Compare: another execution on
// one time scale with this one (site/guide/timeline-sequence.md).

const BASELINE = 'b'.repeat(32)
const FROM = '1760090000000000000'
const TO = '1760093600000000000'

// start_session with n permission lookups, and a directory call if asked.
function session (traceId: string, lookups: number, directory: boolean): MockSpan[] {
  const root = span('start_session', { traceId, spanId: '00000000000000a0', service: 'api', spanKind: 'server', status: 'ok', duration: 900000 } as Partial<MockSpan>)
  const check = span('check_permissions', { traceId, spanId: '00000000000000b0', parentSpanId: root.spanId, service: 'api', spanKind: 'internal', status: 'ok', duration: 100000 } as Partial<MockSpan>)
  const out = [root, check]
  for (let i = 0; i < lookups; i++) {
    out.push(span('permission_lookup', { traceId, spanId: '00000000000001' + i.toString(16).padStart(2, '0'), parentSpanId: check.spanId, service: 'api', spanKind: 'client', status: 'ok', duration: 20000 } as Partial<MockSpan>))
  }
  if (directory) {
    out.push(span('GET /users', { traceId, spanId: '00000000000000d1', parentSpanId: root.spanId, service: 'api', spanKind: 'client', status: 'ok', duration: 40000 } as Partial<MockSpan>))
  }
  return out
}

const current = session(TRACE, 5, true)
const baseline = session(BASELINE, 1, false)
const report = (body: Record<string, any>): ReportReply => ({
  spans: { records: body.trace_id === BASELINE ? baseline : body.trace_id === TRACE ? current : [] },
  logs: { records: [] }
})

test('EXC-001: five repeated lookups are one timeline row that opens into five', async ({ page }) => {
  await mockApi(page, () => ({ spans: [] }), () => ({ count: 0 }), { report })
  await page.goto(`/#/executions?trace=${TRACE}&from=${FROM}&to=${TO}&view=timeline`)
  const group = page.getByTestId('timeline-group')
  await expect(group).toHaveCount(1)
  await expect(group.getByTestId('timeline-group-count')).toHaveText('× 5')
  await expect(group.getByTestId('timeline-group-segment')).toHaveCount(5)
  await expect(group.getByTestId('timeline-group-meta')).toContainText('20 ms each')
  await expect(page.getByTestId('timeline-row')).toHaveCount(3) // root, check_permissions, GET /users
  await group.click()
  await expect(group).toHaveAttribute('aria-expanded', 'true')
  await expect(page.getByTestId('timeline-row').filter({ hasText: 'permission_lookup' })).toHaveCount(5)
  await group.click()
  await expect(page.getByTestId('timeline-row')).toHaveCount(3)
})

test('EXC-002: an execution opened on a repeated call shows that call', async ({ page }) => {
  await mockApi(page, () => ({ spans: [] }), () => ({ count: 0 }), { report })
  await page.goto(`/#/executions?trace=${TRACE}&from=${FROM}&to=${TO}&view=timeline&span=0000000000000102`)
  await expect(page.getByTestId('timeline-group')).toHaveAttribute('aria-expanded', 'true')
  await expect(page.locator('[data-span-key="' + TRACE + ':0000000000000102"]')).toHaveAttribute('aria-pressed', 'true')
})

test('EXC-003: Compare reads another trace in the same window and lines up their calls', async ({ page }) => {
  const api = await mockApi(page, () => ({ spans: [] }), () => ({ count: 0 }), { report })
  await page.goto(`/#/executions?trace=${TRACE}&from=${FROM}&to=${TO}&view=timeline`)
  await page.getByTestId('view-compare').click()
  await page.getByTestId('compare-trace-input').fill('not-a-trace')
  await page.getByTestId('compare-trace-submit').click()
  await expect(page.getByTestId('compare-error')).toHaveText('A trace id is 32 hex characters.')
  await page.getByTestId('compare-trace-input').fill(BASELINE.toUpperCase())
  await page.getByTestId('compare-trace-submit').click()
  await expect(page.getByTestId('execution-compare')).toBeVisible()
  expect(api.reportRequests.at(-1)).toMatchObject({ trace_id: BASELINE, time_from: FROM, time_to: TO })
  // Stacked by default: one block per execution, the changed call colored and described.
  await expect(page.getByTestId('execution-compare')).toHaveAttribute('data-layout', 'stacked')
  const reference = page.getByTestId('compare-block-reference')
  const current = page.getByTestId('compare-block-current')
  await expect(reference.getByTestId('compare-stacked-name')).toHaveText(['start_session', 'check_permissions', 'permission_lookup'])
  await expect(current.getByTestId('compare-stacked-name')).toHaveText(['start_session', 'check_permissions', 'permission_lookup', 'GET /users'])
  const lookups = current.getByTestId('compare-stacked-row').filter({ hasText: 'permission_lookup' })
  await expect(lookups).toHaveAttribute('data-change', 'more')
  await expect(lookups.getByTestId('compare-stacked-count')).toHaveText('× 5')
  await expect(lookups.getByTestId('compare-stacked-bar')).toHaveCount(5)
  await expect(current.getByTestId('compare-run-note')).toContainText('5 calls, overlapping')
  await expect(current.getByTestId('compare-stacked-new')).toHaveText('new')
  await expect(page.getByTestId('compare-totals')).toContainText('+')
  await page.getByTestId('compare-differences').locator('summary').click()
  await expect(page.getByTestId('compare-difference')).toHaveText(['permission_lookup: 1 → 5 calls', 'GET /users: a new call'])

  // Side by side: one row per call with a bar for each execution; the choice is kept.
  await page.getByTestId('compare-layout-side').click()
  const rows = page.getByTestId('compare-row')
  await expect(rows.getByTestId('compare-name')).toHaveText(['start_session', 'check_permissions', 'permission_lookup', 'GET /users'])
  await expect(rows.filter({ hasText: 'permission_lookup' })).toHaveAttribute('data-change', 'more')
  await expect(rows.filter({ hasText: 'permission_lookup' }).getByTestId('compare-counts')).toHaveText('× 1 → 5')
  await expect(rows.filter({ hasText: 'permission_lookup' }).getByTestId('compare-current-bar')).toHaveCount(5)
  await expect(rows.filter({ hasText: 'GET /users' })).toHaveAttribute('data-change', 'new')
  await expect(rows.filter({ hasText: 'GET /users' }).getByTestId('compare-counts')).toHaveText('new')
  // Back to the timeline and the compared trace stays loaded, in the chosen layout.
  await page.getByTestId('view-timeline').click()
  await expect(page.getByTestId('trace-timeline')).toBeVisible()
  await page.getByTestId('view-compare').click()
  await expect(page.getByTestId('execution-compare')).toHaveAttribute('data-layout', 'side')
})

test('EXC-004: a trace without spans in the window is said, not drawn', async ({ page }) => {
  await mockApi(page, () => ({ spans: [] }), () => ({ count: 0 }), { report })
  await page.goto(`/#/executions?trace=${TRACE}&from=${FROM}&to=${TO}&view=timeline`)
  await page.getByTestId('view-compare').click()
  await page.getByTestId('compare-trace-input').fill('c'.repeat(32))
  await page.getByTestId('compare-trace-submit').click()
  await expect(page.getByTestId('compare-error')).toContainText('No spans of trace ' + 'c'.repeat(32))
  await expect(page.getByTestId('execution-compare')).toHaveCount(0)
  await page.getByTestId('compare-trace-input').fill(TRACE)
  await page.getByTestId('compare-trace-submit').click()
  await expect(page.getByTestId('compare-error')).toHaveText('That is this execution: enter another trace id.')
})

// Database calls are told apart by statement; a query replaced by others is
// paired as an inference, with its reasons.
const OLD = 'e'.repeat(32)
const NEW = 'f'.repeat(32)
function roomList (traceId: string, perRoom: boolean): MockSpan[] {
  const root = span('/rooms', { traceId, spanId: '00000000000000a0', service: 'api', spanKind: 'server', status: 'ok', duration: 9000 } as Partial<MockSpan>)
  let n = 0
  const db = (statement: string) => span('sql:query', {
    traceId, spanId: '00000000000001' + (n++).toString(16).padStart(2, '0'), parentSpanId: root.spanId, service: 'api', spanKind: 'client', status: 'ok', duration: 300,
    tags: [{ key: 'db.system', value: 'sqlite' }, { key: 'db.statement', value: statement }]
  } as Partial<MockSpan>)
  const out = [root, db('SELECT id FROM tokens WHERE hash = ?')]
  if (perRoom) {
    out.push(db('SELECT DISTINCT name FROM rooms'))
    for (let r = 1; r <= 4; r++) {
      out.push(db(`SELECT * FROM rooms r LEFT JOIN devices d ON d.room_id = r.id WHERE r.id = ${r}`))
    }
  } else {
    out.push(db('SELECT r.id, d.id FROM rooms r LEFT JOIN devices d ON d.room_id = r.id'))
  }
  return out
}
const oldRooms = roomList(OLD, false)
const newRooms = roomList(NEW, true)
const roomReport = (body: Record<string, any>): ReportReply => ({
  spans: { records: body.trace_id === OLD ? oldRooms : body.trace_id === NEW ? newRooms : [] },
  logs: { records: [] }
})

test('EXC-005: a replaced query is paired with the queries in its place, as an inference', async ({ page }) => {
  await mockApi(page, () => ({ spans: [] }), () => ({ count: 0 }), { report: roomReport })
  await page.goto(`/#/executions?trace=${NEW}&from=${FROM}&to=${TO}&view=timeline`)
  // The timeline folds the per-room query only, with its statement.
  const group = page.getByTestId('timeline-group')
  await expect(group).toHaveCount(1)
  await expect(group.getByTestId('timeline-group-count')).toHaveText('× 4')
  await expect(group.getByTestId('timeline-group-statement')).toHaveText('SELECT * FROM rooms r LEFT JOIN devices d ON d.room_id = r.id WHERE r.id = ?')

  await page.getByTestId('view-compare').click()
  await page.getByTestId('compare-trace-input').fill(OLD)
  await page.getByTestId('compare-trace-submit').click()
  const note = page.getByTestId('compare-replacement')
  await expect(note).toHaveCount(1)
  await expect(note).toContainText('Likely in place of each other')
  await expect(note).toContainText('inferred')
  await expect(note.getByTestId('compare-replacement-reasons')).toHaveText('same place: after sql:query SELECT id FROM tokens WHERE hash = ?; same target: sqlite; same tables: devices, rooms.')
  const current = page.getByTestId('compare-block-current')
  await expect(current.getByTestId('compare-stacked-row').filter({ hasText: 'FROM tokens' })).toHaveAttribute('data-change', 'same')
  await expect(current.getByTestId('compare-stacked-new')).toHaveText(['replaces', 'replaces'])
  await expect(page.getByTestId('compare-block-reference').getByTestId('compare-stacked-missing')).toHaveText('replaced')
  await expect(current.getByTestId('compare-stacked-row').filter({ hasText: 'WHERE r.id = ?' }).getByTestId('compare-stacked-count')).toHaveText('× 4')
  await page.getByTestId('compare-differences').locator('summary').click()
  await expect(page.getByTestId('compare-difference')).toHaveCount(1)
})
