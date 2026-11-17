import { expect, test, type Page } from '@playwright/test'
import { mockApi } from './mockApi'

// Requirements and history on the Quality page (stage 5, docs/quality-gate.md)
// against a mocked API: the catalogue of a project's requirements with their
// history, the recorded evaluations, and links that reopen one.

const IDS = ['0000000000000000000000e1', '0000000000000000000000e2', '0000000000000000000000e3']
const point = (i: number, status: string, sha = 's1') =>
  ({ evaluation_id: IDS[i], recorded_at: `2026-11-17T12:0${i}:00Z`, commit: 'c' + (i + 1) + '0000000000000000', ref: 'main', rules_sha256: sha, status })

function catalogue () {
  const latest = { id: IDS[2], project: 'recording', recorded_at: '2026-11-17T12:02:00Z', run: 'ci-3', gate: 'passed', source: { commit: 'c30000000000000000', ref: 'main' }, checks: [], requirements: [] }
  return {
    project: 'recording', ref: 'main', latest, unlinked: ['spare-check'],
    checks: [
      { id: 'startup-p95', type: 'latency', entry: { service: 'recording-api', operation: 'start_session' }, advisory: false, status: 'pass', reason: 'within_limit', summary: 'p95 560 ms', unit: 'ms', value: 560, limit: 800, target: 500, executions: 200, violations: 0, details: [], evidence: [] },
      { id: 'db-calls', type: 'db_calls', entry: { service: 'recording-api', operation: 'start_session' }, advisory: false, status: 'pass', reason: 'within_limit', summary: 'up to 2', unit: 'calls per execution', value: 2, limit: 3, target: null, executions: 200, violations: 0, details: [], evidence: [] }
    ],
    requirements: [
      {
        id: 'REQ-START', title: 'A session starts within 800 ms', owner: 'recording-team', why: 'Participants leave slow sessions.', checks: ['startup-p95', 'db-calls'],
        status: 'met', summary: 'all 2 of its checks passed',
        history: [point(2, 'met', 's2'), point(1, 'violated'), point(0, 'met')],
        streak: { since: point(2, 'met', 's2'), evaluations: 1, status_before: 'violated' }
      },
      {
        id: 'REQ-OUTAGE', title: 'Recording continues during an analytics outage', checks: [], status: 'not_evaluated', summary: 'no check enforces it yet',
        history: [point(2, 'not_evaluated'), point(1, 'not_evaluated'), point(0, 'not_evaluated')],
        streak: { since: point(0, 'not_evaluated'), evaluations: 3, at_least: true }
      }
    ]
  }
}

function recorded (id: string, gate: string) {
  return {
    contract: 'quality-gate-v1', method: 'exact nearest-rank percentile of entry span durations', run: 'ci-2', time_from: '1794859200000000000',
    time_to: '1794861000000000000', gate, evaluated_at: '2026-11-17T12:01:00Z', project: 'recording', id, recorded_at: '2026-11-17T12:01:00Z',
    source: { commit: 'c20000000000000000', ref: 'main', rules_sha256: 'ab'.repeat(32) },
    requirements: [
      { id: 'REQ-START', title: 'A session starts within 800 ms', owner: 'recording-team', checks: ['startup-p95', 'db-calls'], status: 'violated', summary: 'failed: db-calls' },
      { id: 'REQ-OUTAGE', title: 'Recording continues during an analytics outage', checks: [], status: 'not_evaluated', summary: 'no check enforces it yet' }
    ],
    checks: [{
      id: 'db-calls', type: 'db_calls', entry: { service: 'recording-api', operation: 'start_session' }, advisory: false, status: 'fail', reason: 'limit_exceeded',
      summary: 'up to 21 database calls per execution, limit 3, 200 executions', unit: 'calls per execution', value: 21, limit: 3, target: null,
      executions: 200, violations: 200, details: ['permission_lookup × 20'], evidence: []
    }]
  }
}

const ok = (data: unknown) => ({ error: false, message: 'success', data })

async function setup (page: Page, projects: unknown[] = [{ project: 'recording', evaluations: 3, last_recorded_at: '2026-11-17T12:02:00Z', last_gate: 'passed' }]) {
  const calls: string[] = []
  await mockApi(page, () => ({ spans: [] }))
  await page.route('**/api/v1/quality/**', async (route) => {
    const url = new URL(route.request().url())
    calls.push(url.pathname + url.search)
    let body: unknown = { error: true, message: 'not found' }
    let status = 404
    if (url.pathname.endsWith('/quality/projects')) {
      body = ok({ Projects: projects }); status = 200
    } else if (url.pathname.endsWith('/quality/requirements')) {
      body = ok({ Catalogue: catalogue() }); status = 200
    } else if (url.pathname.endsWith('/quality/evaluations')) {
      const before = url.searchParams.get('before')
      const rows = [2, 1, 0].map((i) => ({ id: IDS[i], project: 'recording', recorded_at: `2026-11-17T12:0${i}:00Z`, run: 'ci-' + (i + 1), gate: i === 1 ? 'failed' : 'passed', source: { commit: 'c' + (i + 1) + '0000000000000000', ref: 'main' }, checks: [], requirements: [{ id: 'REQ-START', status: i === 1 ? 'violated' : 'met' }] }))
      body = ok(before ? { Evaluations: rows.slice(2), Next: '' } : { Evaluations: rows.slice(0, 2), Next: '1794859200000000000' }); status = 200
    } else if (url.pathname.endsWith('/quality/evaluations/' + IDS[1])) {
      body = ok({ Record: { evaluation: recorded(IDS[1], 'failed'), rules: 'version: 1\nproject: recording\n' } }); status = 200
    }
    await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
  })
  return calls
}

test('QG-UI-006: the requirements catalogue shows each requirement with its history; a cell opens that evaluation', async ({ page }) => {
  const calls = await setup(page)
  await page.goto('/')
  await page.getByTestId('main-nav-quality').click()
  // Something is recorded: the page opens on Requirements.
  await expect(page.getByTestId('quality-tab-requirements')).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByTestId('quality-catalogue-counts')).toHaveText('2 requirements: 1 met, 1 not evaluated')
  await expect(page.getByTestId('quality-catalogue-status-REQ-START')).toHaveText('Met')
  await expect(page.getByTestId('quality-catalogue-req-REQ-START')).toContainText('Owner: recording-team · Participants leave slow sessions.')
  await expect(page.getByTestId('quality-catalogue-streak-REQ-START')).toHaveText('Met since commit c30000000000 (this evaluation only); violated before.')
  await expect(page.getByTestId('quality-catalogue-status-REQ-OUTAGE')).toHaveText('Not evaluated')
  await expect(page.getByTestId('quality-catalogue-unlinked')).toContainText('spare-check')
  const cells = page.getByTestId('quality-catalogue-req-REQ-START').getByRole('button')
  await expect(cells).toHaveCount(3)
  // Oldest first: the middle cell is the violated evaluation.
  await expect(cells.nth(1)).toHaveAttribute('aria-label', 'Violated · commit c20000000000 (main) · 2026-11-17 12:01:00 UTC')

  // The branch filter goes to the API.
  await page.getByTestId('quality-ref').fill('main')
  await page.getByTestId('quality-ref').press('Enter')
  await expect.poll(() => calls.some((c) => c.includes('/quality/requirements?project=recording&ref=main'))).toBe(true)

  await cells.nth(1).click()
  await expect(page.getByTestId('quality-gate')).toHaveText('Quality gate: Failed')
  await expect(page).toHaveURL(new RegExp('#/quality\\?evaluation=' + IDS[1] + '$'))
  await expect(page.getByTestId('quality-recorded')).toContainText('Recorded 2026-11-17 12:01:00 UTC as ' + IDS[1] + ' in project recording')
  await expect(page.getByTestId('quality-req-status-REQ-START')).toHaveText('Violated')
  await expect(page.getByTestId('quality-requirements-result')).toContainText('Requirements: 1 violated, 1 not evaluated')
  await page.getByTestId('quality-record-rules').locator('summary').click()
  await expect(page.getByTestId('quality-record-rules')).toContainText('project: recording')

  await page.getByTestId('quality-saved-back').click()
  await expect(page).toHaveURL(/#\/quality$/)
  await expect(page.getByTestId('quality-catalogue-counts')).toBeVisible()
})

test('QG-UI-007: the history lists recorded evaluations page by page; an evaluation link reopens it', async ({ page }) => {
  await setup(page)
  await page.goto('/')
  await page.getByTestId('main-nav-quality').click()
  await page.getByTestId('quality-tab-history').click()
  await expect(page.getByTestId('quality-history-row-' + IDS[2])).toContainText('Passed')
  await expect(page.getByTestId('quality-history-row-' + IDS[1])).toContainText('1 violated')
  await expect(page.getByTestId('quality-history-row-' + IDS[0])).toHaveCount(0)
  await page.getByTestId('quality-history-more').click()
  await expect(page.getByTestId('quality-history-row-' + IDS[0])).toContainText('c10000000000')
  await expect(page.getByTestId('quality-history-more')).toHaveCount(0)

  // A copied link opens the evaluation without the list.
  await page.goto('/#/quality?evaluation=' + IDS[1])
  await expect(page.getByTestId('quality-gate')).toHaveText('Quality gate: Failed')
  await expect(page.getByTestId('quality-check-db-calls')).toContainText('permission_lookup × 20')
  // An unknown evaluation is an error, not an empty page.
  await page.goto('/#/quality?evaluation=' + IDS[0])
  await expect(page.getByTestId('quality-saved-error')).toContainText('could not be opened')
})

test('QG-UI-008: with nothing recorded, the page explains recording and opens on Check a run', async ({ page }) => {
  await setup(page, [])
  await page.goto('/')
  await page.getByTestId('main-nav-quality').click()
  await expect(page.getByTestId('quality-tab-check')).toHaveAttribute('aria-selected', 'true')
  await page.getByTestId('quality-tab-requirements').click()
  await expect(page.getByTestId('quality-projects-empty')).toContainText('Nothing is recorded yet.')
  await expect(page.getByTestId('quality-projects-empty')).toContainText('troubleshooting-check --record')
})
