import { type ISpans } from '@/interfaces/spans.interface'
import Security from '@/services/security'
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
      status: filter.status,
      service_name: filter.serviceName
    }
    const [error, res] = await fetch(import.meta.env.VITE_LOGS_APP_API_URL + '/api/v1/view-spans', Security.requestOptions(payload))
      .then((response) => response.json())
      .then((data) => {
        if (data.error) {
          return data.error
        } else {
          return [null, data.data.Spans]
        }
      })

    return [error, res]
  }

  public async countOfSpans (filter: SpanFilter = defaultFilter): Promise<[any, any]> {
    const payload = {
      parent_id: filter.parentId,
      rows_per_page: filter.rowsPerPage,
      time_from: filter.timeFrom,
      status: filter.status,
      service_name: filter.serviceName
    }
    const [error, res] = await fetch(import.meta.env.VITE_LOGS_APP_API_URL + '/api/v1/count-spans', Security.requestOptions(payload))
      .then((response) => response.json())
      .then((data) => {
        if (data.error) {
          return data.error
        } else {
          return [null, data.data.Count]
        }
      })

    return [error, res]
  }
}
