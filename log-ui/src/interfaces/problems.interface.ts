import { type DetectionRules, type ProblemList, type ProblemsRequest, type SaveResult } from '@/types/problems'

export interface IProblems {
  problems (request: ProblemsRequest): Promise<[string | null, ProblemList | null]>
  services (timeFrom: string, timeTo: string): Promise<[string | null, string[] | null]>
  rules (): Promise<[string | null, DetectionRules | null]>
  saveRules (rules: DetectionRules, expectedRevision: number): Promise<SaveResult>
}
