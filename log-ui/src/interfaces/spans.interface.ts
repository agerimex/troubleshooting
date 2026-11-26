import { type SpanFilter, type SpanNameList, type SpanPage } from '@/types/filter'

export interface ISpans {
  viewSpans(filter: SpanFilter): Promise<[string | null, SpanPage | null]>
  countOfSpans(filter: SpanFilter): Promise<[string | null, number | null]>
  spanNames(timeFrom: string, timeTo: string): Promise<[string | null, SpanNameList | null]>
}
