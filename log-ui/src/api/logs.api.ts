import { type ILogs } from '@/interfaces/logs.interface'

class LogsService implements ILogs {
  private instance: any
  async init (options: ILogs) {
    this.instance = options
    console.log('init')
  }

  public async allLogs (): Promise<[any, any]> {
    console.log('call')
    return await this.instance.allLogs()
  }
}

const logsApi: LogsService = new LogsService()
export default logsApi
