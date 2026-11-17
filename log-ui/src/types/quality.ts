// The quality gate's evaluation (log-analysis quality.go, contract
// quality-gate-v1; docs/quality-gate.md). Field names are the API's, as the
// CLI's result.json has them. Times are Unix nanoseconds as strings.

export type CheckStatus = 'pass' | 'fail' | 'insufficient'
export type Gate = 'passed' | 'failed' | 'unresolved'

export interface QualityEntry {
  service: string
  operation: string
  span_kind?: string
}

export interface QualityEvidence {
  trace_id: string
  span_id: string
  time_from: string
  time_to: string
  note: string
  // Only in a CLI result file: the viewer link it printed.
  link?: string
}

export interface QualityBaseline {
  status: CheckStatus
  value: number | null
  executions: number
  summary: string
}

export interface QualityCheckResult {
  id: string
  type: 'latency' | 'db_calls' | 'forbidden_call' | string
  entry: QualityEntry
  advisory: boolean
  status: CheckStatus
  reason: string
  summary: string
  unit: string
  value: number | null
  limit: number | null
  target: number | null
  target_status?: 'met' | 'open'
  executions: number
  violations: number
  details: string[]
  evidence: QualityEvidence[]
  baseline?: QualityBaseline
}

export interface QualityEvaluation {
  contract: string
  method: string
  run: string
  time_from: string
  time_to: string
  baseline?: string
  baseline_time_from?: string
  baseline_time_to?: string
  gate: Gate
  checks: QualityCheckResult[]
  evaluated_at: string
  // Added by the CLI: the tested commit and the rules that decided.
  source?: QualitySource
}

export interface QualityRequest {
  rulesYaml: string
  run: string
  timeFrom: string
  timeTo: string
  baseline?: string
  baselineFrom?: string
  baselineTo?: string
}

export interface QualitySource {
  commit?: string
  ref?: string
  ci_run?: string
  rules?: string
  rules_ref?: string
  rules_sha256?: string
}
