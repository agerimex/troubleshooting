import { type ShapeFilter, type ShapeLibrary } from '@/types/shapes'
import { type IStats } from '@/interfaces/stats.interface'
import { type Analysis, type AnalysisSummary, type ErrorGroups, type ExamplesRequest, type ExamplesResult, type Population, type PopulationStats, type RetentionPolicy } from '@/types/stats'

class StatsService implements IStats {
  private instance: IStats | null = null
  async init (options: IStats) {
    if (this.instance === null) {
      this.instance = options
    }
  }

  public async callStats (population: Population, options: { groupByVersion?: boolean, series?: boolean }): Promise<[string | null, PopulationStats | null]> {
    return await this.instance!.callStats(population, options)
  }

  public async errorGroups (population: Population): Promise<[string | null, ErrorGroups | null]> {
    return await this.instance!.errorGroups(population)
  }

  public async examples (request: ExamplesRequest): Promise<[string | null, ExamplesResult | null]> {
    return await this.instance!.examples(request)
  }

  public async compare (baseline: Population, candidate: Population, regenerates?: string): Promise<[string | null, Analysis | null]> {
    return await this.instance!.compare(baseline, candidate, regenerates)
  }

  public async analyses (): Promise<[string | null, AnalysisSummary[] | null]> {
    return await this.instance!.analyses()
  }

  public async analysis (id: string): Promise<[string | null, Analysis | null]> {
    return await this.instance!.analysis(id)
  }

  public async retention (): Promise<[string | null, RetentionPolicy | null]> {
    return await this.instance!.retention()
  }

  public async shapes (filter: ShapeFilter): Promise<[string | null, ShapeLibrary | null]> {
    return await this.instance!.shapes(filter)
  }
}

const statsApi: StatsService = new StatsService()
export default statsApi
