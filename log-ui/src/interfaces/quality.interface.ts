import { type QualityEvaluation, type QualityRequest } from '@/types/quality'

export interface IQuality {
  evaluate (request: QualityRequest): Promise<[string | null, QualityEvaluation | null]>
}
