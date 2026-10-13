// Builders of milestone 2 API answers for the browser checks. Values follow
// the reference fixture (features/milestone_2, REFERENCE_FIXTURE.md).

export const HOUR = 3600n * 1000000000n
export const T0 = 1791694800000000000n

export function callStats (calls: number, errors: number, ms: number | null, overrides: Record<string, unknown> = {}) {
  const samples = (overrides.validDurations as number | undefined) ?? calls
  const p = (min: number) => ({
    valueMs: samples === 0 ? null : ms,
    status: samples === 0 ? 'unavailable' : (samples < min ? 'insufficient_sample' : 'available'),
    minimum: min,
    samples
  })
  return {
    operation: 'upload_recording', spanKind: 'consumer', calls, errors, ok: calls - errors, unset: 0,
    errorFraction: calls ? errors / calls : null, ratePerMinute: calls / 60, validDurations: samples, invalidDurations: 0,
    percentiles: { p50: p(20), p95: p(200), p99: p(1000) }, conflicts: 0, sampling: ['unsampled'], ...overrides
  }
}

export function populationStats (version: string, from: bigint, rows: unknown[], overrides: Record<string, unknown> = {}) {
  return {
    population: {
      service: 'recording-worker', operation: 'upload_recording', span_kind: 'consumer', environment: 'fixture', version,
      installation: null, segment: null, time_from: from.toString(), time_to: (from + HOUR).toString()
    },
    label: `recording-worker / upload_recording / consumer / environment fixture / version ${version}`,
    durationSeconds: 3600, status: 'ok', condition: 'ok', rows, series: [],
    coverage: { status: 'complete', coveredMinutes: 60, minutes: 60, reasons: ['declared complete: fixture'] },
    plan: [{ from: from.toString(), to: (from + HOUR).toString(), source: 'stored', wholeBucket: true, revision: 1, state: 'final' }],
    notices: [], lateExcluded: 0, unknownReceipt: 0, cutoff: (from + 6n * HOUR).toString(), generatedAt: (from + 6n * HOUR).toString(),
    definitions: { metrics: 'metrics-v1', canonical: 'canonical-v1', percentile: 'ClickHouse quantilesTDigest' },
    ...overrides
  }
}

export function metric (name: string, b: number | null, c: number | null, classification: string, overrides: Record<string, unknown> = {}) {
  const absolute = b === null || c === null ? null : c - b
  const relative = absolute === null || b === 0 ? null : 100 * absolute / (b as number)
  const unit = name === 'error_fraction' ? 'percent of recorded calls' : (name === 'calls' ? 'calls' : (name === 'rate' ? 'calls per minute' : 'milliseconds'))
  return {
    metric: name, unit, deltaUnit: name === 'error_fraction' ? 'percentage points' : unit, baseline: b, candidate: c, absolute, relative,
    relativeNote: b === 0 ? 'relative change unavailable: the baseline is zero' : undefined, classification, reasons: [], ...overrides
  }
}

export const STORAGE_GROUP = {
  key: 'exception|recording-worker|upload_recording|fixture|f1', sourceKind: 'exception', service: 'recording-worker', operation: 'upload_recording',
  environment: 'fixture', method: 'exception', fingerprint: 'f1', exceptionType: 'StorageError', frame: 'storage.Upload', signature: '',
  occurrences: 50, unlinked: 0, affectedTraces: 50, firstSeen: (T0 + 2n * HOUR).toString(), lastSeen: (T0 + 3n * HOUR - 1n).toString(), versions: null
}

export const DENIED_GROUP = {
  ...STORAGE_GROUP, key: 'log|recording-worker|upload_recording|fixture|f2', sourceKind: 'log', method: 'message', fingerprint: 'f2',
  exceptionType: '', frame: '', signature: 'Storage returned 403 AccessDenied'
}

// regression is 1.1.0 against 1.0.0 of the reference fixture.
export function regression (id = 'a1b2c3d4e5f6a1b2c3d4e5f6', overrides: Record<string, unknown> = {}) {
  const b = callStats(1000, 10, 100)
  const c = callStats(1000, 50, 160)
  return {
    id, kind: 'comparison', title: 'recording-worker 1.1.0 against 1.0.0', generatedAt: (T0 + 6n * HOUR).toString(),
    contracts: { metrics: 'metrics-v1', errors: 'errors-v1', comparison: 'comparison-v1', canonical: 'canonical-v1', percentile: 't-digest' },
    rules: { version: 'comparison-v1', latencyAbsoluteMs: 20, latencyRelativePct: 20, percentileMinimum: { p50: 20, p95: 200, p99: 1000 }, errorFractionMinimumCalls: 1000, errorFractionAbsolutePp: 1, description: 'Latency: 20 ms and 20 %; error fraction: 1 percentage point with 1000 calls.' },
    baseline: populationStats('1.0.0', T0, [b]),
    candidate: populationStats('1.1.0', T0 + 2n * HOUR, [c]),
    baselineErrors: {}, candidateErrors: {},
    mismatches: [],
    rows: [{
      operation: 'upload_recording', spanKind: 'consumer', baseline: b, candidate: c, notes: [],
      metrics: [
        metric('calls', 1000, 1000, 'descriptive'), metric('rate', 16.6667, 16.6667, 'descriptive'),
        metric('error_fraction', 1, 5, 'regression_signal'),
        metric('p50', 100, 160, 'regression_signal'), metric('p95', 100, 160, 'regression_signal'), metric('p99', 100, 160, 'regression_signal')
      ]
    }],
    errorGroups: [
      { ...STORAGE_GROUP, baseline: { occurrences: 10, affectedTraces: 10, unlinked: 0, firstSeen: T0.toString(), lastSeen: (T0 + HOUR - 1n).toString() },
        candidate: { occurrences: 50, affectedTraces: 50, unlinked: 0, firstSeen: (T0 + 2n * HOUR).toString(), lastSeen: (T0 + 3n * HOUR - 1n).toString() },
        label: 'recurring', absoluteChange: 40, relativeChange: 400 },
      { ...DENIED_GROUP, baseline: { occurrences: 10, affectedTraces: 10, unlinked: 0 }, candidate: { occurrences: 50, affectedTraces: 50, unlinked: 0 },
        label: 'recurring', absoluteChange: 40, relativeChange: 400 }
    ],
    notices: [],
    interpretation: 'Change signals follow the recorded comparison rules; they are heuristics that point to what to investigate, not proof that a deployment caused a change.',
    ...overrides
  }
}
