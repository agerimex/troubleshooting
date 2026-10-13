import { type ILogs } from '@/interfaces/logs.interface'
import Security from '@/services/security'
import { requestJSON } from '@/services/request'
import { type LogFilter, type LogPage, type ReportRequest, type ReportResponse } from '@/types/report'

export class Logs implements ILogs {
  public async searchLogs (filter: LogFilter): Promise<[string | null, LogPage | null]> {
    const payload = {
      time_from: filter.timeFrom,
      time_to: filter.timeTo,
      trace_id: filter.traceId,
      service_name: filter.serviceName,
      severity: filter.severity,
      message: filter.message,
      rows_per_page: filter.rowsPerPage,
      after_time: filter.afterTime,
      after_record_id: filter.afterRecordId
    }
    return requestJSON(import.meta.env.VITE_LOGS_APP_API_URL + '/api/v1/search-logs', Security.requestOptions(payload),
      (data) => ({ logs: data.Logs ?? [], hasMore: data.HasMore === true }))
  }

  public async traceReport (request: ReportRequest): Promise<[string | null, ReportResponse | null]> {
    const payload = {
      trace_id: request.traceId,
      time_from: request.timeFrom,
      time_to: request.timeTo,
      span_limit: request.spanLimit,
      log_limit: request.logLimit,
      sections: request.sections
    }
    return requestJSON(import.meta.env.VITE_LOGS_APP_API_URL + '/api/v1/trace-report', Security.requestOptions(payload),
      (data) => (data.Report ?? null))
  }
}
