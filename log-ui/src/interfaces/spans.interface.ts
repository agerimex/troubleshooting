import { type SpanFilter, type SpanPage } from '@/types/filter'

export interface ISpans {
  viewSpans(filter: SpanFilter): Promise<[string | null, SpanPage | null]>
  countOfSpans(filter: SpanFilter): Promise<[string | null, number | null]>
}
