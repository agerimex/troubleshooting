import { type ErrorGroup, type MetricComparison, type Notice, type Population } from '@/types/stats'

// The problem workspace's view of the API (log-analysis problems.go and
// rules.go). Times are Unix nanoseconds as strings.

export type ProblemType = 'error' | 'slow_calls' | 'release_regression'

export interface VersionCount {
  version: string
  occurrences: number
}

export interface SlowCalls {
  thresholdMs: number
  calls: number
  versions: VersionCount[]
}

export interface Regression {
  analysisId: string
  generatedAt: string
  baseline: Record<string, string | null>
  candidate: Record<string, string | null>
  baselineLabel: string
  candidateLabel: string
  metric: MetricComparison
  baselineCalls: number
  candidateCalls: number
  baselineSamples: number
  candidateSamples: number
}

export interface Explanation {
  ruleRevision: number
  rule: string
  observed: string
  threshold: string
  basis: string
  scope: string
  coverage: string
}

export interface Problem {
  id: string
  type: ProblemType
  title: string
  service: string
  operation: string
  spanKind?: string
  environment: string
  qualified: boolean
  attribution?: string
  firstSeen?: string
  lastSeen?: string
  error?: ErrorGroup
  slow?: SlowCalls
  regression?: Regression
  explanation: Explanation
}

export interface ProblemSource {
  status: string
  condition: string
}

export type ListStatus = 'available' | 'empty' | 'partial' | 'unavailable' | 'expired'

export interface ProblemList {
  scope: Record<string, string | null>
  rules: DetectionRules
  status: ListStatus
  condition: string
  sources: Record<string, ProblemSource>
  problems: Problem[]
  total: number
  next?: string
  waiting?: string
  generatedAt: string
  notices: Notice[]
}

export interface ProblemScope {
  service: string
  environment?: string
  timeFrom: string
  timeTo: string
}

export interface ProblemsRequest extends ProblemScope {
  limit?: number
  cursor?: string
  rulesRevision?: number
}

export interface DetectionRules {
  scope: string
  revision: number
  savedAt?: string
  effectiveAt?: string
  errors: { enabled: boolean, minAffectedCalls: number, grouping: string }
  slow: { enabled: boolean, durationMs: number, minCalls: number }
  regressions: { enabled: boolean, p95RelativePct: number, p95AbsoluteMs: number }
}

export interface FieldError {
  field: string
  message: string
}

// The outcome of a save: only 'saved' means a new revision is stored.
export type SaveResult =
  | { kind: 'saved', rules: DetectionRules }
  | { kind: 'invalid', fieldErrors: FieldError[], message: string }
  | { kind: 'conflict', current: DetectionRules | null, message: string }
  | { kind: 'failed', message: string }

export function scopePopulation (scope: ProblemScope): Population {
  return { service: scope.service, environment: scope.environment, timeFrom: scope.timeFrom, timeTo: scope.timeTo }
}
