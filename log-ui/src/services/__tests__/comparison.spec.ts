import { describe, expect, it } from 'vitest'
import {
  COMPARISON_FORMAT, defaultWindow, exportComparisonHtml, exportComparisonJson, formatAbsolute, formatMetric, formatRelative,
  groupLabelText, groupTitle, percentileText, planSummary, populationText, slowThreshold, type IncludedReport
} from '@/services/comparison'
import { type Report } from '@/types/report'
import { type Analysis, type CallStats, type ErrorGroup, type MetricComparison, type PopulationStats } from '@/types/stats'

const hour = 3600n * 1000000000n
const t0 = 1791694800000000000n

function callStats (calls: number, errors: number, p95: number | null, samples = calls): CallStats {
  const p = (v: number | null, min: number) => ({ valueMs: v, status: samples === 0 ? 'unavailable' as const : (samples < min ? 'insufficient_sample' as const : 'available' as const), minimum: min, samples })
  return {
    operation: 'upload_recording', spanKind: 'consumer', calls, errors, ok: calls - errors, unset: 0,
    errorFraction: calls ? errors / calls : null, ratePerMinute: calls / 60, validDurations: samples, invalidDurations: 0,
    percentiles: { p50: p(p95, 20), p95: p(p95, 200), p99: p(p95, 1000) }, conflicts: 0, sampling: ['unsampled']
  }
}

function stats (label: string, from: bigint, rows: CallStats[]): PopulationStats {
  return {
    population: { service: 'recording-worker', operation: 'upload_recording', span_kind: 'consumer', environment: null, version: label, installation: null, segment: null, time_from: from.toString(), time_to: (from + hour).toString() },
    label: 'recording-worker / upload_recording / consumer / version ' + label, durationSeconds: 3600, status: 'ok', condition: 'ok',
    rows, series: [], coverage: { status: 'complete', coveredMinutes: 60, minutes: 60, reasons: [] },
    plan: [{ from: from.toString(), to: (from + hour).toString(), source: 'stored', wholeBucket: true, revision: 7, state: 'final' }],
    notices: [], lateExcluded: 0, unknownReceipt: 0, cutoff: (from + 2n * hour).toString(), generatedAt: (from + 2n * hour).toString(),
    definitions: { metrics: 'metrics-v1', percentile: 't-digest' }
  }
}

function metric (overrides: Partial<MetricComparison>): MetricComparison {
  return { metric: 'p95', unit: 'milliseconds', deltaUnit: 'milliseconds', baseline: 100, candidate: 160, absolute: 60, relative: 60, classification: 'regression_signal', reasons: [], ...overrides }
}

const group: ErrorGroup = {
  key: 'k', sourceKind: 'exception', service: 'recording-worker', operation: 'upload_recording', environment: 'fixture', method: 'exception',
  fingerprint: 'f', exceptionType: 'StorageError', frame: 'storage.Upload', signature: '', occurrences: 50, unlinked: 0, affectedTraces: 50,
  firstSeen: '1', lastSeen: '2', versions: null
}

function analysis (): Analysis {
  const b = callStats(1000, 10, 100)
  const c = callStats(1000, 50, 160)
  return {
    id: 'a1b2c3d4e5f6a1b2c3d4e5f6', kind: 'comparison', title: '1.1.0 against 1.0.0 <script>alert(1)</script>', generatedAt: (t0 + 6n * hour).toString(),
    contracts: { metrics: 'metrics-v1', errors: 'errors-v1', comparison: 'comparison-v1' },
    rules: { version: 'comparison-v1', latencyAbsoluteMs: 20, latencyRelativePct: 20, percentileMinimum: { p50: 20, p95: 200, p99: 1000 }, errorFractionMinimumCalls: 1000, errorFractionAbsolutePp: 1, description: 'rules' },
    baseline: stats('1.0.0', t0, [b]), candidate: stats('1.1.0', t0 + 2n * hour, [c]),
    baselineErrors: {} as any, candidateErrors: {} as any, mismatches: [],
    rows: [{ operation: 'upload_recording', spanKind: 'consumer', baseline: b, candidate: c, notes: [], metrics: [
      metric({}),
      metric({ metric: 'error_fraction', unit: 'percent', deltaUnit: 'percentage points', baseline: 1, candidate: 5, absolute: 4, relative: 400 })
    ] }],
    errorGroups: [{ ...group, baseline: { occurrences: 10, affectedTraces: 10, unlinked: 0 }, candidate: { occurrences: 50, affectedTraces: 50, unlinked: 0 }, label: 'recurring', absoluteChange: 40, relativeChange: 400 }],
    notices: [], interpretation: 'Change signals are not proof of a cause.'
  }
}

function report (): Report {
  const section = { status: 'ok' as const, limit: 1000, shown: 1, truncated: false, total: 1, totalStatus: 'ok' as const, retrievedAt: '1' }
  return {
    traceId: 'abcdefabcdefabcdefabcdefabcdef01', timeFrom: '1', timeTo: '2', generatedAt: '3', warnings: [],
    spans: { ...section, records: [] , shown: 0, total: 0 },
    logs: { ...section, records: [{ timeUnixNano: '1', recordId: 'r1', traceId: 'abcdefabcdefabcdefabcdefabcdef01', spanId: '', severity: 'ERROR', service: 'w', message: '<img src=x onerror=alert(1)> Storage returned 403 AccessDenied', attributes: {} }] }
  }
}

const included = (): IncludedReport[] => [{
  report: report(), includedAt: '5',
  context: { analysisId: 'a1b2c3d4e5f6a1b2c3d4e5f6', analysisTitle: 't', analysisGeneratedAt: '1', example: 'candidate occurrence of StorageError', population: 'candidate: 1.1.0' }
}]

describe('formatting', () => {
  it('M2-CMP-001: shows units, percentage points and signed changes', () => {
    expect(formatAbsolute(metric({}))).toBe('+60.0 ms')
    expect(formatRelative(metric({}))).toBe('+60.00 %')
    const ef = metric({ metric: 'error_fraction', absolute: 4, relative: 400 })
    expect(formatAbsolute(ef)).toBe('+4.00 pp')
    expect(formatMetric('error_fraction', 5)).toBe('5.00 %')
    expect(formatMetric('rate', 16.6667)).toBe('16.67 calls/min')
  })

  it('M2-CMP-002: rounds only for display', () => {
    expect(formatRelative(metric({ absolute: -55, relative: -34.375 }))).toBe('-34.38 %')
  })

  it('M2-CMP-003: a zero baseline shows no relative change, never infinity', () => {
    const m = metric({ baseline: 0, candidate: 100, absolute: 100, relative: null, relativeNote: 'relative change unavailable: the baseline is zero' })
    expect(formatRelative(m)).toBe('unavailable (baseline is zero)')
    expect(formatRelative(m)).not.toMatch(/Infinity|NaN/)
  })

  it('M2-CAL-005 / M2-CMP-008: unavailable is never shown as zero', () => {
    expect(formatMetric('p95', null)).toBe('unavailable')
    expect(formatAbsolute(metric({ absolute: null }))).toBe('unavailable')
  })

  it('M2-LAT-004: an insufficient sample shows its count and minimum, not a headline value', () => {
    expect(percentileText({ valueMs: 160, status: 'insufficient_sample', minimum: 200, samples: 50 })).toBe('insufficient sample (50 of 200)')
    expect(percentileText({ valueMs: 160, status: 'available', minimum: 200, samples: 1000 })).toBe('160.0 ms')
  })

  it('names groups by what they were grouped on', () => {
    expect(groupTitle(group)).toBe('StorageError at storage.Upload')
    expect(groupTitle({ ...group, method: 'message', signature: 'Request request-<id> failed with status 403' })).toBe('Request request-<id> failed with status 403')
    expect(groupLabelText('not_observed_in_candidate')).toBe('Not observed in the candidate scope')
  })

  it('describes populations with explicit unknown groups', () => {
    expect(populationText({ service: 's', version: '', environment: 'production', timeFrom: '0', timeTo: '1' })).toBe('s · environment production · version not recorded')
  })

  it('summarizes where values come from', () => {
    expect(planSummary([
      { from: '0', to: '1', source: 'stored', wholeBucket: true, state: 'final' },
      { from: '1', to: '2', source: 'stored', wholeBucket: true, state: 'provisional' },
      { from: '2', to: '3', source: 'raw', wholeBucket: false }
    ])).toBe('2 stored buckets (1 provisional), 1 interval computed from raw records')
  })

  it('M2-INV-001: the slow-example threshold is the displayed candidate p95', () => {
    expect(slowThreshold(analysis().rows[0])).toBe(160)
    expect(slowThreshold({ candidate: null })).toBeNull()
  })

  it('default windows are whole UTC hours', () => {
    const w = defaultWindow(Date.UTC(2026, 9, 11, 14, 37))
    expect(BigInt(w.timeTo) % hour).toBe(0n)
    expect(BigInt(w.timeTo) - BigInt(w.timeFrom)).toBe(6n * hour)
  })
})

describe('comparison export', () => {
  it('M2-REP-001 / M2-REP-002: JSON keeps the reviewed analysis exactly and embeds the selected reports', () => {
    const a = analysis()
    const data = exportComparisonJson(a, included(), new Date(0))
    expect(data.format).toBe(COMPARISON_FORMAT)
    expect(data.analysis).toEqual(a)
    expect(data.embeddedReports).toHaveLength(1)
    expect(data.embeddedReports[0].role).toBe('selected example')
    expect(data.embeddedReports[0].report.logs.records[0].message).toContain('403 AccessDenied')
    expect(data.liveLinks).toEqual([])
  })

  it('M2-REP-001: HTML shows definitions, values and rules, and recorded text stays inert', () => {
    const html = exportComparisonHtml(analysis(), included(), new Date(0))
    expect(html).toContain("default-src 'none'")
    expect(html).not.toContain('<script>')
    expect(html).not.toContain('<img src=x')
    expect(html).toContain('&lt;script&gt;alert(1)&lt;/script&gt;')
    expect(html).toContain('&lt;img src=x onerror=alert(1)&gt; Storage returned 403 AccessDenied')
    for (const text of ['+60.0 ms', '+60.00 %', '+4.00 pp', '+400.00 %', 'Regression signal', 'comparison-v1', 'Selected example 1 of 1', 'not the whole population',
      'a1b2c3d4e5f6a1b2c3d4e5f6', 'StorageError at storage.Upload', '1000 calls, 1000 valid durations']) {
      expect(html).toContain(text)
    }
  })

  it('M2-REP-003: an unavailable source stays unavailable in the export', () => {
    const a = analysis()
    a.errorGroups[0].baseline = { occurrences: null, affectedTraces: null, unlinked: null }
    a.errorGroups[0].label = 'undetermined'
    const html = exportComparisonHtml(a, [], new Date(0))
    expect(html).toContain('<td>unavailable</td>')
    expect(html).toContain('Undetermined')
    expect(html).toContain('No combined report was included.')
  })
})
