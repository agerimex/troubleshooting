import { type ISpans } from '@/interfaces/spans.interface'
import Security from '@/services/security'
import { requestJSON } from '@/services/request'
import { type SpanFilter } from '@/types/filter'

let defaultFilter: SpanFilter = {
  parentId: "",
  rowsPerPage: 10, 
  timeFrom: "0", 
  status: "",
  serviceName: ""
};

export class Spans implements ISpans {
  public async viewSpans (filter: SpanFilter = defaultFilter): Promise<[any, any]> {
    const payload = {
      parent_id: filter.parentId,
      rows_per_page: filter.rowsPerPage,
      time_from: filter.timeFrom,
      after_span_id: filter.afterSpanId,
      status: filter.status,
      service_name: filter.serviceName,
      method_name: filter.methodName
    }
    return requestJSON(import.meta.env.VITE_LOGS_APP_API_URL + '/api/v1/view-spans', Security.requestOptions(payload), (data) => data.Spans)
  }

  public async countOfSpans (filter: SpanFilter = defaultFilter): Promise<[any, any]> {
    const payload = {
      parent_id: filter.parentId,
      rows_per_page: filter.rowsPerPage,
      time_from: filter.timeFrom,
      status: filter.status,
      service_name: filter.serviceName,
      method_name: filter.methodName
    }
    return requestJSON(import.meta.env.VITE_LOGS_APP_API_URL + '/api/v1/count-spans', Security.requestOptions(payload), (data) => data.Count)
  }
}
