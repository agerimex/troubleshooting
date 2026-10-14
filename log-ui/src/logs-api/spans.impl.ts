import { type ISpans } from '@/interfaces/spans.interface'
import Security from '@/services/security'
import { requestJSON } from '@/services/request'
import { type SpanFilter, type SpanPage } from '@/types/filter'

function payloadOf (filter: SpanFilter, withCursor: boolean) {
  const payload: Record<string, unknown> = {
    time_from: filter.timeFrom,
    time_to: filter.timeTo,
    scope: filter.scope,
    trace_id: filter.traceId,
    parent_id: filter.parentId,
    rows_per_page: filter.rowsPerPage,
    status: filter.status,
    service_name: filter.serviceName,
    method_name: filter.methodName
  }
  if (withCursor) {
    payload.after_time = filter.afterTime
    payload.after_trace_id = filter.afterTraceId
    payload.after_span_id = filter.afterSpanId
  }
  return payload
}

export class Spans implements ISpans {
  public async viewSpans (filter: SpanFilter): Promise<[string | null, SpanPage | null]> {
    return requestJSON(import.meta.env.VITE_LOGS_APP_API_URL + '/api/v1/view-spans', Security.requestOptions(payloadOf(filter, true)),
      (data) => ({ spans: data.Spans ?? [], hasMore: data.HasMore === true }))
  }

  // The total ignores the cursor: it counts the whole list for the filter.
  public async countOfSpans (filter: SpanFilter): Promise<[string | null, number | null]> {
    return requestJSON(import.meta.env.VITE_LOGS_APP_API_URL + '/api/v1/count-spans', Security.requestOptions(payloadOf(filter, false)),
      (data) => (typeof data.Count === 'number' ? data.Count : null))
  }
}
