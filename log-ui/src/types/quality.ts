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
  // Only in a CLI result file: the evidence trace's spans as they were when
  // it was checked (troubleshooting-check snapshot.go).
  snapshot?: SavedSnapshot
}

// SavedSnapshot keeps an evidence trace's spans (names, services, kinds,
// times, status), so it can be shown after the raw spans expired or the
// Troubleshooting that recorded them is gone. No logs, attributes or events.
export interface SavedSnapshot {
  spans?: SavedSpan[]
  truncated?: boolean
  error?: string
}

export interface SavedSpan {
  span_id: string
  parent_id: string
  service: string
  name: string
  kind: string
  start: string // Unix ns
  duration_us: number // -1: invalid
  status: string
  peer?: string // peer.service or db.system of a CLIENT span
}

export interface QualityBaseline {
  status: CheckStatus
  value: number | null
  executions: number
  summary: string
}

export interface QualityCheckResult {
  id: string
  type: 'latency' | 'db_calls' | 'forbidden_call' | 'errors' | string
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
  // The tested commit and the rules that decided (the CLI's, or sent with a
  // recorded evaluation).
  source?: QualitySource
  // The rules' project and requirements (stage 5); absent in older results.
  project?: string
  requirements?: RequirementResult[]
  // Set when the evaluation was recorded.
  id?: string
  recorded_at?: string
  record_error?: string
  // Sent to the history under id, but not read back: the verdict is the local one.
  record_unconfirmed?: boolean
  // In how many checked executions lost child spans could be seen.
  child_counts?: { executions: number, counted: number, status: 'all' | 'partial' | 'none' | string }
  recorded_before?: boolean
  reevaluated?: boolean
  // What the verdicts rest on: which lost spans the recorded data can show.
  assumptions?: string[]
}

export type RequirementStatus = 'met' | 'violated' | 'insufficient' | 'not_evaluated'

export interface RequirementResult {
  id: string
  title: string
  owner?: string
  why?: string
  checks: string[]
  status: RequirementStatus | string
  summary: string
  // Guardrails support the requirement: they decide the gate, never its status.
  guardrails?: string[]
  guardrail_summary?: string
  // What decides the status (its checks' definitions): a streak never spans two.
  criteria_sha256?: string
}

// A recorded evaluation in the history list.
export interface EvaluationSummary {
  id: string
  project: string
  recorded_at: string
  run: string
  gate: Gate | string
  source: QualitySource
  checks: { id: string, status: string }[]
  requirements: { id: string, status: string, criteria?: string }[]
}

export interface QualityProject {
  project: string
  evaluations: number
  last_recorded_at: string
  last_gate: string
}

// A requirement's status in one recorded evaluation; 'absent' when that
// evaluation's rules didn't have it.
export interface HistoryPoint {
  evaluation_id: string
  recorded_at: string
  commit?: string
  ref?: string
  rules_sha256?: string
  criteria_sha256?: string
  status: string
}

export interface Streak {
  since: HistoryPoint
  evaluations: number
  at_least?: boolean
  status_before?: string
  // The streak ended because the requirement's criteria changed.
  criteria_changed?: boolean
}

export interface CatalogueEntry extends RequirementResult {
  streak: Streak
  history: HistoryPoint[] // newest first
}

export interface Catalogue {
  project: string
  ref?: string
  latest: EvaluationSummary | null
  requirements: CatalogueEntry[]
  checks: QualityCheckResult[]
  unlinked: string[]
}

// A recorded evaluation exactly as stored, with the rules text it used.
export interface QualityRecord {
  evaluation: QualityEvaluation
  rules: string
}

export interface HistoryFilter {
  project: string
  ref?: string
  before?: string
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
  pr_head?: string
  ref?: string
  ci_run?: string
  rules?: string
  rules_ref?: string
  rules_sha256?: string
}
