import { type LogFilter, type LogPage, type ReportRequest, type ReportResponse } from '@/types/report'

export interface ILogs {
  searchLogs(filter: LogFilter): Promise<[string | null, LogPage | null]>
  traceReport(request: ReportRequest): Promise<[string | null, ReportResponse | null]>
}
