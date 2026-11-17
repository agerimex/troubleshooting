import { type Catalogue, type EvaluationSummary, type HistoryFilter, type QualityEvaluation, type QualityProject, type QualityRecord, type QualityRequest } from '@/types/quality'

export interface IQuality {
  evaluate (request: QualityRequest): Promise<[string | null, QualityEvaluation | null]>
  projects (): Promise<[string | null, QualityProject[] | null]>
  evaluations (filter: HistoryFilter): Promise<[string | null, { evaluations: EvaluationSummary[], next: string } | null]>
  record (id: string): Promise<[string | null, QualityRecord | null]>
  catalogue (project: string, ref: string): Promise<[string | null, Catalogue | null]>
}
