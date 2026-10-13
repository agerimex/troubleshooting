import { type ILogs } from '@/interfaces/logs.interface'
import { type LogFilter, type LogPage, type ReportRequest, type ReportResponse } from '@/types/report'

class LogsService implements ILogs {
  private instance: ILogs | null = null
  async init (options: ILogs) {
    if (this.instance === null) {
      this.instance = options
    }
  }

  public async searchLogs (filter: LogFilter): Promise<[string | null, LogPage | null]> {
    return await this.instance!.searchLogs(filter)
  }

  public async traceReport (request: ReportRequest): Promise<[string | null, ReportResponse | null]> {
    return await this.instance!.traceReport(request)
  }
}

const logsApi: LogsService = new LogsService()
export default logsApi
