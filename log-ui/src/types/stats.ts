// Types of the milestone 2 API: statistics, error groups, examples and
// comparisons (log-analysis internal/data). Times are Unix nanoseconds as
// strings. Values that could not be determined are null, never 0.

// A population of recorded calls. An optional dimension is undefined for
// "any value" and '' for "not recorded" (an explicit unknown group).
export interface Population {
  service: string
  operation?: string
  spanKind?: string
  environment?: string
  version?: string
  installation?: string
  segment?: string
  timeFrom: string
  timeTo: string
}

export interface Notice {
  code: string
  message: string
}

export interface Percentile {
  valueMs: number | null
  status: 'available' | 'insufficient_sample' | 'unavailable'
  minimum: number
  samples: number
}

export interface CallStats {
  operation: string
  spanKind: string
  version?: string
  calls: number
  errors: number
  ok: number
  unset: number
  errorFraction: number | null
  ratePerMinute: number
  validDurations: number
  invalidDurations: number
  percentiles: Record<string, Percentile>
  conflicts: number
  sampling: string[]
}

export interface PlanPart {
  from: string
  to: string
  source: 'stored' | 'raw' | 'unavailable'
  wholeBucket: boolean
  revision?: number
  state?: 'provisional' | 'final'
  definitions?: string
  generatedAt?: string
  receiptCutoff?: string
  reason?: string
}

export interface BucketPoint {
  from: string
  to: string
  source: string
  state?: string
  revision?: number
  coverage: string
  calls: number | null
  errors: number | null
  available: boolean
}

export interface Coverage {
  status: 'complete' | 'gap' | 'unknown' | 'open'
  coveredMinutes: number
  minutes: number
  reasons: string[]
}

export type StatsStatus = 'ok' | 'unavailable' | 'timeout' | 'error' | 'outside_retention' | 'exact_bounds_unavailable'

export interface PopulationStats {
  population: Record<string, string | null>
  label: string
  durationSeconds: number
  status: StatsStatus
  condition: string
  error?: string
  suggestedFrom?: string
  suggestedTo?: string
  rows: CallStats[]
  series: BucketPoint[]
  coverage: Coverage
  plan: PlanPart[]
  notices: Notice[]
  lateExcluded: number
  unknownReceipt: number
  cutoff: string
  generatedAt: string
  definitions: Record<string, string>
}

export interface ErrorGroup {
  key: string
  sourceKind: 'span_status' | 'exception' | 'log'
  service: string
  operation: string
  environment: string
  method: 'exception' | 'message'
  fingerprint: string
  exceptionType: string
  frame: string
  signature: string
  occurrences: number
  unlinked: number
  affectedTraces: number
  // Distinct recorded (TraceId, SpanId); null when part of the window was
  // summarized before call counts were recorded.
  affectedCalls?: number | null
  firstSeen: string
  lastSeen: string
  versions: { version: string, occurrences: number }[] | null
}

export interface SourceResult {
  status: StatsStatus
  error?: string
  coverage: Coverage
}

export interface ErrorGroups {
  population: Record<string, string | null>
  label: string
  status: StatsStatus
  condition: string
  sources: Record<string, SourceResult>
  groups: ErrorGroup[]
  plan: PlanPart[]
  notices: Notice[]
  cutoff: string
  generatedAt: string
  definitions: Record<string, string>
}

export interface Example {
  sourceKind: string
  traceId: string
  spanId: string
  recordId?: string
  eventIndex?: number
  time: string
  durationMs?: number | null
  status?: string
  service: string
  operation: string
  version: string
  message: string
}

export interface ExamplesResult {
  status: StatsStatus
  condition: string
  examples: Example[]
  limit: number
  shown: number
  total: number
  truncated: boolean
  thresholdMs?: number
  description: string
  cutoff: string
}

export interface GroupRef {
  sourceKind: string
  service: string
  operation: string
  environment: string
  fingerprint: string
}

export interface ExamplesRequest {
  population: Population
  kind: 'slow' | 'error'
  thresholdMs?: number
  group?: GroupRef
  limit?: number
  cutoff?: string
}

export type Classification = 'regression_signal' | 'improvement_signal' | 'within_threshold' | 'insufficient_evidence' | 'incomparable' | 'descriptive'

export interface MetricComparison {
  metric: string
  unit: string
  deltaUnit: string
  baseline: number | null
  candidate: number | null
  absolute: number | null
  relative: number | null
  relativeNote?: string
  classification: Classification
  reasons: string[]
}

export interface ComparisonRow {
  operation: string
  spanKind: string
  baseline: CallStats | null
  candidate: CallStats | null
  metrics: MetricComparison[]
  notes: string[]
}

export interface ScopeOccurrences {
  occurrences: number | null
  affectedTraces: number | null
  unlinked: number | null
  firstSeen?: string
  lastSeen?: string
}

export interface GroupComparison extends ErrorGroup {
  baseline: ScopeOccurrences
  candidate: ScopeOccurrences
  label: string
  reason?: string
  absoluteChange: number | null
  relativeChange: number | null
  relativeChangeNote?: string
}

export interface Mismatch {
  dimension: string
  baseline: string
  candidate: string
  affects: string[]
}

export interface Rules {
  version: string
  latencyAbsoluteMs: number
  latencyRelativePct: number
  percentileMinimum: Record<string, number>
  errorFractionMinimumCalls: number
  errorFractionAbsolutePp: number
  description: string
}

export interface Analysis {
  id: string
  kind: string
  title: string
  regenerates?: string
  generatedAt: string
  contracts: Record<string, string>
  rules: Rules
  baseline: PopulationStats
  candidate: PopulationStats
  baselineErrors: ErrorGroups
  candidateErrors: ErrorGroups
  mismatches: Mismatch[]
  rows: ComparisonRow[]
  errorGroups: GroupComparison[]
  notices: Notice[]
  interpretation: string
}

export interface AnalysisSummary {
  id: string
  kind: string
  title: string
  createdAt: string
}

// Where an investigation report was opened from: an example of a stored
// comparison. The report itself is generated now.
export interface ReportContext {
  analysisId: string
  analysisTitle: string
  analysisGeneratedAt: string
  example: string
  population: string
}

// The effective retention (GET retention): raw spans and logs and
// statistics expire independently.
export interface RetentionPolicy {
  tracesSeconds: number
  logsSeconds: number
  statsSeconds: number
  bucketSeconds: number
  latenessSeconds: number
  marginSeconds: number
  compatible: boolean
  defaultSampling: string
  clockOffsetSeconds: number
  physicalDeletion: string
}
