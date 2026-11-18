import { describe, expect, it } from 'vitest'
import {
  evaluationHash, exportHtml, historyCells, parseRecord, parseResult, recordedText, requirementCounts, requirementLabel, rulesChanged, streakText,
  validEvaluationId
} from '@/services/quality'
import { parseRoute } from '@/services/navigation'
import { type CatalogueEntry, type HistoryPoint, type QualityEvaluation } from '@/types/quality'

// A small failed evaluation (importing quality.spec's would run its tests again).
function candidate (): QualityEvaluation {
  return {
    contract: 'quality-gate-v1', method: 'm', run: 'pr-42', time_from: '1', time_to: '2', gate: 'failed', evaluated_at: '2026-11-17T00:00:00Z',
    checks: [{
      id: 'startup-p95', type: 'latency', entry: { service: 'recording-api', operation: 'start_session' }, advisory: false, status: 'fail',
      reason: 'limit_exceeded', summary: 'p95 910 ms', unit: 'ms', value: 910, limit: 800, target: null, executions: 200, violations: 200, details: [], evidence: []
    }]
  }
}

const point = (id: string, status: string, commit: string, sha = 's1'): HistoryPoint =>
  ({ evaluation_id: id, recorded_at: '2026-11-17T12:0' + id.slice(-1) + ':00.123Z', commit, ref: 'main', rules_sha256: sha, status })

function entry (status: string, history: HistoryPoint[], streak: CatalogueEntry['streak']): CatalogueEntry {
  return { id: 'REQ-START', title: 'Starts fast', checks: ['startup-p95'], status, summary: '', history, streak }
}

describe('requirements and history', () => {
  it('names requirement statuses and counts them worst first', () => {
    expect(requirementLabel('not_evaluated')).toBe('Not evaluated')
    expect(requirementLabel('absent')).toBe('Not in the rules')
    expect(requirementCounts([{ status: 'met' }, { status: 'not_evaluated' }, { status: 'violated' }, { status: 'met' }]))
      .toBe('1 violated, 2 met, 1 not evaluated')
    expect(requirementCounts([])).toBe('')
  })

  it('says since when a status holds', () => {
    const h = [point('e3', 'violated', 'c3333333333333'), point('e2', 'violated', 'c2222222222222'), point('e1', 'met', 'c1')]
    expect(streakText(entry('violated', h, { since: h[1], evaluations: 2, status_before: 'met' })))
      .toBe('Violated since commit c22222222222 (2 evaluations); met before.')
    expect(streakText(entry('met', h, { since: h[0], evaluations: 1, status_before: 'violated' })))
      .toBe('Met since commit c33333333333 (this evaluation only); violated before.')
    expect(streakText(entry('met', h, { since: h[2], evaluations: 3, at_least: true }))).toBe('Met in all 3 recorded evaluations read.')
    expect(streakText(entry('met', h.slice(0, 1), { since: h[0], evaluations: 1, at_least: true }))).toBe('Met in all one recorded evaluation read.')
    const noCommit = { ...h[0], commit: '' }
    expect(streakText(entry('violated', [noCommit, h[2]], { since: noCommit, evaluations: 1, status_before: 'met' })))
      .toBe('Violated since 2026-11-17 12:03:00 UTC (this evaluation only); met before.')
  })

  it('draws the history oldest first and marks changed rules', () => {
    const h = [point('e3', 'met', 'c3', 's2'), point('e2', 'violated', 'c2', 's1'), point('e1', 'absent', 'c1', 's1')]
    const cells = historyCells(h)
    expect(cells.map((c) => c.evaluation_id)).toEqual(['e1', 'e2', 'e3'])
    expect(cells[2].label).toBe('Met · commit c3 (main) · 2026-11-17 12:03:00 UTC')
    expect(rulesChanged(h, 0)).toBe(true)
    expect(rulesChanged(h, 1)).toBe(false)
    expect(rulesChanged(h, 2)).toBe(false)
    expect(recordedText('2026-11-17T12:05:05Z')).toBe('2026-11-17 12:05:05 UTC')
  })

  it('links and reads recorded evaluations', () => {
    expect(validEvaluationId('0123456789abcdef01234567')).toBe(true)
    expect(validEvaluationId('0123')).toBe(false)
    expect(evaluationHash('0123456789abcdef01234567')).toBe('#/quality?evaluation=0123456789abcdef01234567')
    expect(parseRoute('#/quality?evaluation=0123456789abcdef01234567')).toEqual({ page: 'quality', target: null, error: '', evaluation: '0123456789abcdef01234567' })
    expect(parseRoute('#/quality?evaluation=../x').error).toContain('24 hex digits')
    expect(parseRoute('#/quality')).toEqual({ page: 'quality', target: null, error: '' })

    const ev = { ...candidate(), id: '0123456789abcdef01234567', project: 'recording', recorded_at: '2026-11-17T12:00:00Z' }
    const [err, rec] = parseRecord({ evaluation: ev, rules: 'version: 1\n' })
    expect(err).toBeNull()
    expect(rec?.rules).toBe('version: 1\n')
    expect(rec?.evaluation.requirements).toEqual([])
    expect(parseRecord(null)[0]).toBe('The recorded evaluation is missing.')
    expect(parseRecord({ evaluation: { contract: 'other' } })[0]).toContain('not a quality-gate-v1 evaluation')
  })

  it('keeps valid requirements of a result and exports them', () => {
    const ev = {
      ...candidate(),
      requirements: [
        { id: 'REQ-START', title: 'Starts <fast>', owner: 'team', why: 'users', checks: ['startup-p95', 3], status: 'violated', summary: 'failed: startup-p95' },
        { id: 7, status: 'met' },
        { id: 'REQ-OUTAGE', status: 'not_evaluated' }
      ]
    }
    const [, parsed] = parseResult(JSON.stringify(ev))
    expect(parsed?.requirements?.map((r) => r.id)).toEqual(['REQ-START', 'REQ-OUTAGE'])
    expect(parsed?.requirements?.[0].checks).toEqual(['startup-p95'])
    expect(parsed?.requirements?.[1]).toMatchObject({ title: '', summary: '', checks: [] })
    const html = exportHtml(parsed!)
    expect(html).toContain('<h2>Requirements: 1 violated, 1 not evaluated</h2>')
    expect(html).toContain('Starts &lt;fast&gt;')
    expect(exportHtml(candidate())).not.toContain('Requirements')
  })
})

describe('assumptions (stage 6)', () => {
  it('keeps the assumptions of a result and exports them', () => {
    const ev = { ...candidate(), assumptions: ['Verdicts hold for the recorded spans.', 7, 'Lost <child> spans can\'t be detected.'] }
    const [, parsed] = parseResult(JSON.stringify(ev))
    expect(parsed?.assumptions).toEqual(['Verdicts hold for the recorded spans.', 'Lost <child> spans can\'t be detected.'])
    expect(exportHtml(parsed!)).toContain('<li>Lost &lt;child&gt; spans can&#39;t be detected.</li>')
    expect(parseResult(JSON.stringify(candidate()))[1]?.assumptions).toEqual([])
  })
})
