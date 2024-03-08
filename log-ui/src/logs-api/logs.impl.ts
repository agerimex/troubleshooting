import { type ILogs } from '@/interfaces/logs.interface'
import Security from '@/services/security'

export class Logs implements ILogs {
  public async allLogs (): Promise<[any, any]> {
    const [error, res] = await fetch(import.meta.env.VITE_LOGS_APP_API_URL + '/api/v1/view-logs', Security.requestOptions('', 'GET'))
      .then((response) => response.json())
      .then((data) => {
        if (data.error) {
          return data.error
        } else {
          return [null, data.data.logs]
        }
      })

    return [error, res]
  }
}
