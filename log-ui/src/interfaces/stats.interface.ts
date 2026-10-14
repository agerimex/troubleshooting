import { type ShapeFilter, type ShapeLibrary } from '@/types/shapes'
import { type Analysis, type AnalysisSummary, type ErrorGroups, type ExamplesRequest, type ExamplesResult, type Population, type PopulationStats, type RetentionPolicy } from '@/types/stats'

export interface IStats {
  callStats(population: Population, options: { groupByVersion?: boolean, series?: boolean }): Promise<[string | null, PopulationStats | null]>
  errorGroups(population: Population): Promise<[string | null, ErrorGroups | null]>
  examples(request: ExamplesRequest): Promise<[string | null, ExamplesResult | null]>
  compare(baseline: Population, candidate: Population, regenerates?: string): Promise<[string | null, Analysis | null]>
  analyses(): Promise<[string | null, AnalysisSummary[] | null]>
  analysis(id: string): Promise<[string | null, Analysis | null]>
  retention(): Promise<[string | null, RetentionPolicy | null]>
  shapes(filter: ShapeFilter): Promise<[string | null, ShapeLibrary | null]>
}
