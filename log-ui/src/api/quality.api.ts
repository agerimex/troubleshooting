import { type IQuality } from '@/interfaces/quality.interface'
import { type QualityEvaluation, type QualityRequest } from '@/types/quality'

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
}

const qualityApi: QualityService = new QualityService()
export default qualityApi
