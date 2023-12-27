import { type ISpans } from '@/interfaces/spans.interface'
import { type SpanFilter } from '@/types/filter'

class SpansService implements ISpans {
  private instance: any = null
  async init (options: ISpans) {
    if (this.instance === null) {
      this.instance = options
    }
  }

  public async viewSpans (filter: SpanFilter): Promise<[any, any]> {
    return await this.instance.viewSpans(filter)
  }

  public async countOfSpans(filter: SpanFilter): Promise<[any, any]> {
    return await this.instance.countOfSpans(filter)
  }
}

const spansApi: SpansService = new SpansService()
export default spansApi
