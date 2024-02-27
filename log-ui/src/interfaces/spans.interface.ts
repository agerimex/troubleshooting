import { type SpanFilter } from '@/types/filter'

export interface ISpans {
  viewSpans(filter: SpanFilter): Promise<[any, any]>
  countOfSpans(filter: SpanFilter): Promise<[any, any]>
}
