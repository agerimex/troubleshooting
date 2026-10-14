import { type ISpans } from '@/interfaces/spans.interface'
import { type SpanFilter, type SpanPage } from '@/types/filter'

class SpansService implements ISpans {
  private instance: ISpans | null = null
  async init (options: ISpans) {
    if (this.instance === null) {
      this.instance = options
    }
  }

  public async viewSpans (filter: SpanFilter): Promise<[string | null, SpanPage | null]> {
    return await this.instance!.viewSpans(filter)
  }

  public async countOfSpans(filter: SpanFilter): Promise<[string | null, number | null]> {
    return await this.instance!.countOfSpans(filter)
  }
}

const spansApi: SpansService = new SpansService()
export default spansApi
