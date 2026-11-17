import { describe, expect, it } from 'vitest'
import {
  baselineText, evidenceTarget, evidenceView, exportHtml, exportJson, gateLabel, limitText, parseResult, presetWindow, statusLabel, valueText
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
