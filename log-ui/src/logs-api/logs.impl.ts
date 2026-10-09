import { type ILogs } from '@/interfaces/logs.interface'
import Security from '@/services/security'
import { requestJSON } from '@/services/request'

export class Logs implements ILogs {
  public async allLogs (): Promise<[any, any]> {
    return requestJSON(import.meta.env.VITE_LOGS_APP_API_URL + '/api/v1/view-logs', Security.requestOptions('', 'GET'), (data) => data.logs)
  }
}
