import { type IQuality } from '@/interfaces/quality.interface'
import { type Catalogue, type EvaluationSummary, type HistoryFilter, type QualityEvaluation, type QualityProject, type QualityRecord, type QualityRequest } from '@/types/quality'

class QualityService implements IQuality {
  private instance: IQuality | null = null
  async init (options: IQuality) {
    if (this.instance === null) {
      this.instance = options
    }
  }

  public async evaluate (request: QualityRequest): Promise<[string | null, QualityEvaluation | null]> {
    return await this.instance!.evaluate(request)
  }

  public async projects (): Promise<[string | null, QualityProject[] | null]> {
    return await this.instance!.projects()
  }

  public async evaluations (filter: HistoryFilter): Promise<[string | null, { evaluations: EvaluationSummary[], next: string } | null]> {
    return await this.instance!.evaluations(filter)
  }

  public async record (id: string): Promise<[string | null, QualityRecord | null]> {
    return await this.instance!.record(id)
  }

  public async catalogue (project: string, ref: string): Promise<[string | null, Catalogue | null]> {
    return await this.instance!.catalogue(project, ref)
  }
}

const qualityApi: QualityService = new QualityService()
export default qualityApi
