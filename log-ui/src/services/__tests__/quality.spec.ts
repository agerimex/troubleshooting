import { describe, expect, it } from 'vitest'
import {
  baselineText, ciRunLink, evidenceTarget, evidenceView, exportHtml, exportJson, gateLabel, limitText, parseResult, presetWindow, savedSpans, snapshotReport, sourceText, statusLabel, valueText
} from '@/services/quality'
import { type QualityCheckResult, type QualityEvaluation } from '@/types/quality'

const TRACE = '9fc2b374080200000000000000000001'

export function candidate (): QualityEvaluation {
  const check = (c: Partial<QualityCheckResult>): QualityCheckResult => ({
    id: 'x', type: 'latency', entry: { service: 'recording-api', operation: 'start_session', span_kind: 'server' }, advisory: false,
    status: 'fail', reason: 'limit_exceeded', summary: '', unit: 'ms', value: null, limit: null, target: null, executions: 200, violations: 200,
    details: [], evidence: [], ...c
  })
  return {
    contract: 'quality-gate-v1', method: 'exact nearest-rank percentile of entry span durations', run: 'pr-42', time_from: '1794859200000000000',
    time_to: '1794861000000000000', baseline: 'main', baseline_time_from: '1794859200000000000', baseline_time_to: '1794861000000000000',
    gate: 'failed', evaluated_at: '2026-11-17T00:00:00Z',
    checks: [
      check({ id: 'startup-p95', value: 910, limit: 800, target: 500, target_status: 'open', summary: 'p95 910 ms, limit 800 ms, 200 executions',
        baseline: { status: 'pass', value: 420, executions: 200, summary: 'p95 420 ms' },
        evidence: [{ trace_id: TRACE, span_id: '00000000000000a0', time_from: '1794859554000000000', time_to: '1794859675010000000', note: 'slowest execution: 1010 ms' }] }),
      check({ id: 'db-calls', type: 'db_calls', unit: 'calls per execution', value: 21, limit: 3, details: ['permission_lookup × 20', 'insert_session × 1'],
        baseline: { status: 'insufficient', value: null, executions: 0, summary: 'no executions' } }),
      check({ id: 'no-directory', type: 'forbidden_call', unit: 'executions with the call', value: 200, limit: 0,
        evidence: [{ trace_id: TRACE, span_id: '00000000000000d1', time_from: '1', time_to: '2', note: 'api → directory-api (GET /users)' }] })
    ]
  }
}

describe('quality page logic', () => {
  it('labels gates, statuses and values', () => {
    expect(gateLabel('unresolved')).toBe('Unresolved: not enough data')
    expect(statusLabel('insufficient')).toBe('No data')
    expect(valueText(909.96, 'ms')).toBe('910 ms')
    expect(valueText(559.94, 'ms')).toBe('559.9 ms')
    expect(valueText(21, 'calls per execution')).toBe('21')
    expect(valueText(null, 'ms')).toBe('-')
    const [lat, db] = candidate().checks
    expect(limitText(lat)).toBe('800 ms (target 500 ms, open)')
    expect(baselineText(lat)).toBe('420 ms')
    expect(baselineText(db)).toBe('no data')
  })

  it('computes the preset window as [now - preset, now) in nanoseconds', () => {
    expect(presetWindow('15m', 1000000)).toEqual({ from: (1000000n * 1000000n - 900000000000n).toString(), to: '1000000000000' })
    expect(presetWindow('nonsense', 3600000).from).toBe('0')
  })

  it('opens evidence in the viewer with a Quality origin; calls between services as a sequence', () => {
    const [lat, , dir] = candidate().checks
    expect(evidenceView(lat)).toBe('timeline')
    expect(evidenceView(dir)).toBe('sequence')
    expect(evidenceTarget(lat, lat.evidence[0])).toEqual({
      traceId: TRACE, spanId: '00000000000000a0', window: { from: '1794859554000000000', to: '1794859675010000000' }, view: 'timeline',
      origin: { page: 'quality', label: 'the quality check startup-p95' }
    })
    expect(evidenceTarget(dir, dir.evidence[0], 'timeline')?.view).toBe('timeline')
    expect(evidenceTarget(lat, { ...lat.evidence[0], trace_id: 'nope' })).toBeNull()
    expect(evidenceTarget(lat, { ...lat.evidence[0], time_to: lat.evidence[0].time_from })).toBeNull()
    expect(evidenceTarget(lat, { ...lat.evidence[0], span_id: 'x' })?.spanId).toBeUndefined()
  })

  it('reads a CLI result file or an API answer, and nothing else', () => {
    const ev = candidate()
    expect(parseResult(JSON.stringify(ev))).toEqual([null, ev])
    expect(parseResult(JSON.stringify({ error: false, data: { Evaluation: ev } }))[1]?.gate).toBe('failed')
    expect(parseResult('not json')[0]).toBe('The file is not JSON.')
    expect(parseResult(JSON.stringify({ ...ev, contract: 'quality-gate-v2' }))[0]).toContain('not a quality-gate-v1 evaluation')
    expect(parseResult(JSON.stringify({ ...ev, checks: [{ id: 1 }] }))[0]).toBe('The evaluation has an invalid check.')
    const loose = JSON.parse(JSON.stringify(ev))
    loose.checks[0].details = null
    loose.checks[0].evidence = [{ note: 'no trace' }]
    const [, fixed] = parseResult(JSON.stringify(loose))
    expect(fixed?.checks[0].details).toEqual([])
    expect(fixed?.checks[0].evidence).toEqual([])
  })

  it('exports HTML without scripts and with recorded text escaped, JSON as shown', () => {
    const ev = candidate()
    ev.checks[1].details.push('<script>alert(1)</script>')
    ev.run = '<img src=x onerror=alert(1)>'
    const html = exportHtml(ev)
    expect(html).not.toMatch(/<script|<img/i)
    expect(html).toContain('&lt;script&gt;alert(1)&lt;/script&gt;')
    expect(html).toContain("default-src 'none'")
    expect(html).toContain('<h1>Quality gate: Failed</h1>')
    expect(html).toContain('<th>Baseline</th>')
    expect(html).toContain('permission_lookup × 20')
    expect(JSON.parse(exportJson(ev))).toEqual(ev)
  })
})

describe('the CLI source of a result file', () => {
  it('names the commit, the rules and their digest; only http(s) CI links', () => {
    const s = { commit: '0123456789abcdef0123', ref: 'feat/x', ci_run: 'https://ci.example/runs/7', rules: '/tmp/q.yaml', rules_ref: 'main@89ab01234567:quality.yaml', rules_sha256: 'd80ff3781a3d0000' }
    expect(sourceText(s)).toBe('commit 0123456789ab (feat/x) · rules main@89ab01234567:quality.yaml, sha256 d80ff3781a3d')
    expect(sourceText({ ...s, pr_head: 'fedcba9876543210' })).toBe('commit 0123456789ab (feat/x), merging head fedcba987654 · rules main@89ab01234567:quality.yaml, sha256 d80ff3781a3d')
    expect(sourceText({ rules: 'quality.yaml' })).toBe('rules quality.yaml')
    expect(sourceText(undefined)).toBe('')
    expect(sourceText({ commit: 5 } as any)).toBe('')
    expect(ciRunLink(s)).toBe('https://ci.example/runs/7')
    expect(ciRunLink({ ci_run: 'javascript:alert(1)' })).toBe('')
    const [err, ev] = parseResult(JSON.stringify({ contract: 'quality-gate-v1', gate: 'failed', checks: [], run: 'r', source: s }))
    expect(err).toBeNull()
    expect(exportHtml(ev!)).toContain('<p>commit 0123456789ab (feat/x) · rules main@89ab01234567:quality.yaml, sha256 d80ff3781a3d</p>')
  })
})

describe('saved snapshots (CI result files)', () => {
  const snap = {
    truncated: true,
    spans: [
      { span_id: '00000000000000b1', parent_id: '0000000000000000', service: 'api', name: 'GET /x', kind: 'server', start: '1000', duration_us: 12000, status: 'error' },
      { span_id: '00000000000000b2', parent_id: '00000000000000b1', service: 'api', name: 'SELECT', kind: 'client', start: '2000', duration_us: 10, status: 'ok', peer: 'postgresql' },
      { span_id: 'bad', parent_id: '', service: 'api', name: 'x', kind: 'client', start: '1', duration_us: 1, status: 'ok' }
    ]
  }

  it('turns the valid spans into spans of the trace', () => {
    const spans = savedSpans(snap, TRACE)
    expect(spans.map((s) => s.spanId)).toEqual(['00000000000000b1', '00000000000000b2'])
    expect(spans[0]).toMatchObject({ traceId: TRACE, parentSpanId: '0000000000000000', statusCode: 1, status: 'error', duration: 12000, spanKind: 'server', events: [], links: [] })
    expect(spans[1].tags).toEqual([{ key: 'peer.service', value: 'postgresql' }])
    expect(savedSpans(undefined, TRACE)).toEqual([])
    expect(savedSpans({ spans: 'x' } as any, TRACE)).toEqual([])
  })

  it('is a report whose logs were not kept, never "no logs"', () => {
    const r = snapshotReport(TRACE, { from: '0', to: '9' }, snap, '5')
    expect(r.spans).toMatchObject({ status: 'ok', shown: 2, truncated: true, total: null })
    expect(r.logs).toMatchObject({ status: 'unavailable', error: 'Logs are not kept in a saved snapshot.', records: [] })
  })

  it('travels with the evidence target only when it has spans', () => {
    const c = candidate().checks[0]
    const e = { ...c.evidence[0], snapshot: snap }
    expect(evidenceTarget(c, e)?.snapshot).toBe(snap)
    expect(evidenceTarget(c, { ...e, snapshot: { spans: [] } })?.snapshot).toBeUndefined()
    expect(evidenceTarget(c, { ...e, snapshot: { error: 'query limit' } })?.snapshot).toBeUndefined()
  })
})
