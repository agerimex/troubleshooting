import { expect, test } from '@playwright/test'
import { mockApi, span, type Body } from './mockApi'

// The Quality page (docs/quality-gate.md) against a mocked API: the rules go
// as typed, the result shows the gate and every check, and evidence opens in
// Timeline & Sequence with a way back.

const TRACE = '9fc2b374080200000000000000000001'

function evaluation (gate = 'failed') {
  return {
    contract: 'quality-gate-v1', method: 'exact nearest-rank percentile of entry span durations', run: 'pr-42',
    time_from: '1794859200000000000', time_to: '1794861000000000000', baseline: 'main',
    baseline_time_from: '1794859200000000000', baseline_time_to: '1794861000000000000', gate, evaluated_at: '2026-11-17T00:00:00Z',
    checks: [
      {
        id: 'db-calls', type: 'db_calls', entry: { service: 'recording-api', operation: 'start_session', span_kind: 'server' }, advisory: false,
        status: 'fail', reason: 'limit_exceeded', summary: 'up to 21 database calls per execution, limit 3, 200 executions', unit: 'calls per execution',
        value: 21, limit: 3, target: 2, target_status: 'open', executions: 200, violations: 200, details: ['permission_lookup × 20', 'insert_session × 1'],
        evidence: [{ trace_id: TRACE, span_id: '00000000000000a0', time_from: '1794859440000000000', time_to: '1794859560850000000', note: 'execution with the most database calls: 21' }],
        baseline: { status: 'pass', value: 2, executions: 200, summary: '' }
      },
      {
        id: 'startup-p95', type: 'latency', entry: { service: 'recording-api', operation: 'start_session' }, advisory: false,
        status: 'insufficient', reason: 'too_few_executions', summary: '19 executions, at least 20 needed', unit: 'ms',
        value: 518, limit: 800, target: null, executions: 19, violations: 0, details: [], evidence: []
      }
    ]
  }
}

async function setup (page: import('@playwright/test').Page, reply: (body: Body) => { status: number, body: unknown }) {
  const requests: Body[] = []
  await mockApi(page, (body) => ({ spans: body.trace_id === TRACE ? [span('start_session', { traceId: TRACE, service: 'recording-api' })] : [] }))
  await page.route('**/api/v1/quality/evaluate', async (route) => {
    const body = route.request().postDataJSON()
    requests.push(body)
    const r = reply(body)
    await route.fulfill({ status: r.status, contentType: 'application/json', body: JSON.stringify(r.body) })
  })
  await page.goto('/')
  await page.getByTestId('main-nav-quality').click()
  return requests
}

test('QG-UI-001: a run is checked with the rules as typed; failures name their evidence', async ({ page }) => {
  const requests = await setup(page, () => ({ status: 200, body: { error: false, message: 'success', data: { Evaluation: evaluation() } } }))
  await page.getByTestId('quality-example').click()
  await page.getByTestId('quality-run').fill('pr-42')
  await page.getByTestId('quality-baseline').fill('main')
  await page.getByTestId('quality-evaluate').click()

  await expect(page.getByTestId('quality-gate')).toHaveText('Quality gate: Failed')
  expect(requests).toHaveLength(1)
  expect(requests[0].rules_yaml).toContain('max_per_execution: 3')
  expect(requests[0].run).toBe('pr-42')
  expect(requests[0].baseline).toBe('main')
  expect(BigInt(requests[0].time_to) - BigInt(requests[0].time_from)).toBe(3600n * 1000000000n)
  expect(requests[0].baseline_time_from).toBeUndefined()

  await expect(page.getByTestId('quality-status-db-calls')).toHaveText('Fail')
  await expect(page.getByTestId('quality-value-db-calls')).toHaveText('21')
  await expect(page.getByTestId('quality-baseline-db-calls')).toHaveText('2')
  await expect(page.getByTestId('quality-check-db-calls')).toContainText('permission_lookup × 20')
  await expect(page.getByTestId('quality-check-db-calls')).toContainText('3 (target 2, open)')
  await expect(page.getByTestId('quality-status-startup-p95')).toHaveText('No data')

  await page.getByTestId('quality-open-timeline-db-calls').click()
  await expect(page.getByTestId('report')).toBeVisible()
  await expect(page).toHaveURL(new RegExp('#/executions\\?trace=' + TRACE + '.*span=00000000000000a0'))
  await expect(page.getByTestId('origin')).toContainText('the quality check db-calls')
  await page.getByTestId('return-origin').click()
  await expect(page.getByTestId('quality-gate')).toHaveText('Quality gate: Failed')
})

test('QG-UI-002: invalid rules show the API message and no result', async ({ page }) => {
  await setup(page, () => ({ status: 400, body: { error: true, message: 'rules: yaml: unmarshal errors:\n  line 9: field targt_ms not found in type data.QualityCheck', data: null } }))
  await page.getByTestId('quality-rules').fill('version: 1\nchecks: []\n')
  await page.getByTestId('quality-run').fill('r')
  await page.getByTestId('quality-evaluate').click()
  await expect(page.getByTestId('quality-error')).toContainText('field targt_ms not found')
  await expect(page.getByTestId('quality-result')).toHaveCount(0)
})

test('QG-UI-003: a CI result file opens with its evidence; other files are refused', async ({ page }) => {
  await setup(page, () => ({ status: 500, body: {} }))
  const ev = { ...evaluation('unresolved'), source: { commit: '0123456789abcdef', ref: 'feat/x', ci_run: 'https://ci.example/runs/7', rules: '/tmp/q.yaml', rules_ref: 'main@89ab01234567:quality.yaml', rules_sha256: 'd80ff3781a3d00' } }
  await page.getByTestId('quality-file').setInputFiles({ name: 'result.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(ev)) })
  await expect(page.getByTestId('quality-gate')).toHaveText('Quality gate: Unresolved: not enough data')
  await expect(page.getByTestId('quality-gate-box')).toContainText('From the file result.json')
  await expect(page.getByTestId('quality-source')).toHaveText('commit 0123456789ab (feat/x) · rules main@89ab01234567:quality.yaml, sha256 d80ff3781a3d · CI run')
  await expect(page.getByTestId('quality-source').getByRole('link', { name: 'CI run' })).toHaveAttribute('href', 'https://ci.example/runs/7')
  const bad = { ...ev, source: { ...ev.source, ci_run: 'javascript:alert(1)' } }
  await page.getByTestId('quality-file').setInputFiles({ name: 'bad.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(bad)) })
  await expect(page.getByTestId('quality-source')).not.toContainText('CI run')
  await expect(page.getByTestId('quality-source').getByRole('link')).toHaveCount(0)
  await page.getByTestId('quality-file').setInputFiles({ name: 'other.json', mimeType: 'application/json', buffer: Buffer.from('{"contract":"x"}') })
  await expect(page.getByTestId('quality-error')).toContainText('other.json: The file is not a quality-gate-v1 evaluation')
})

test('QG-UI-004: the rules and run stay in this browser between visits', async ({ page }) => {
  await setup(page, () => ({ status: 500, body: {} }))
  await page.getByTestId('quality-rules').fill('version: 1\n# mine\n')
  await page.getByTestId('quality-run').fill('pr-7')
  await page.reload()
  await page.getByTestId('main-nav-quality').click()
  await expect(page.getByTestId('quality-rules')).toHaveValue('version: 1\n# mine\n')
  await expect(page.getByTestId('quality-run')).toHaveValue('pr-7')
})

test('QG-UI-005: a CI result\'s saved snapshot opens when the live spans are gone; live data wins', async ({ page }) => {
  await setup(page, () => ({ status: 500, body: {} }))
  const GONE = '0aa7dedc4bb0a64d39881e733a91f6d0'
  const t0 = 1794917244779080744n
  const snap = (id: string, parent: string, name: string, kind: string, offUs: number, durUs: number, peer?: string) =>
    ({ span_id: id, parent_id: parent, service: 'example-orders', name, kind, start: (t0 + BigInt(offUs) * 1000n).toString(), duration_us: durUs, status: 'unset', peer })
  const spans = [
    snap('00000000000000b1', '0000000000000000', 'GET /orders/{id}', 'server', 0, 12000),
    snap('00000000000000b2', '00000000000000b1', 'SELECT order_items', 'client', 10, 1100, 'postgresql'),
    snap('00000000000000b3', '00000000000000b1', 'SELECT prices WHERE item_id = ?', 'client', 1200, 1100, 'postgresql'),
    snap('00000000000000b4', '00000000000000b1', 'GET /discounts', 'client', 9700, 2100, 'legacy-pricing'),
    { span_id: 'not-a-span-id', start: 'x' } // a hand-edited file: left out
  ]
  const ev = evaluation()
  ev.checks[0].evidence = [
    { trace_id: GONE, span_id: '00000000000000b2', time_from: (t0 - 60000000000n).toString(), time_to: (t0 + 60000000000n).toString(), note: 'execution with the most database calls: 9', snapshot: { spans } } as any,
    { ...ev.checks[0].evidence[0], snapshot: { spans } } as any
  ]
  await page.getByTestId('quality-file').setInputFiles({ name: 'result.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(ev)) })
  await expect(page.getByTestId('quality-snapshot-db-calls').first()).toHaveText('saved snapshot: 4 spans')

  await page.getByTestId('quality-open-timeline-db-calls').first().click()
  await expect(page.getByTestId('saved-snapshot')).toContainText('Saved snapshot from the CI result')
  await expect(page.getByTestId('completeness')).toHaveCount(0)
  await expect(page.getByTestId('report-summary')).toContainText('4 spans')
  await expect(page.getByTestId('logs-summary')).toContainText('Logs are not kept in a saved snapshot.')
  await expect(page.getByTestId('retry-logs')).toHaveCount(0)
  await expect(page.getByTestId('inspector-logs-not-kept')).toHaveText('Logs are not kept in a saved snapshot.')
  await expect(page.getByTestId('inspector-retry-logs')).toHaveCount(0)
  await expect(page.getByTestId('generated-at')).toHaveCount(0)
  await expect(page.getByTestId('report')).toContainText('SELECT prices WHERE item_id = ?')
  await page.getByTestId('view-sequence').click()
  await expect(page.getByTestId('report')).toContainText('legacy-pricing')

  // The other evidence's trace is still on the server: the live report wins.
  await page.getByTestId('return-origin').click()
  await page.getByTestId('quality-open-timeline-db-calls').nth(1).click()
  await expect(page.getByTestId('report-summary')).toContainText('1 spans')
  await expect(page.getByTestId('saved-snapshot')).toHaveCount(0)
  await expect(page.getByTestId('completeness')).toBeVisible()
})
