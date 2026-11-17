import { type IQuality } from '@/interfaces/quality.interface'
import Security from '@/services/security'
import { requestJSON } from '@/services/request'
import { type QualityEvaluation, type QualityRequest } from '@/types/quality'

const base = () => import.meta.env.VITE_LOGS_APP_API_URL + '/api/v1'

export class Quality implements IQuality {
  // The rules go as typed (rules_yaml): the API parses them as strictly as
  // the CLI, so the browser needs no YAML parser of its own.
  public async evaluate (request: QualityRequest): Promise<[string | null, QualityEvaluation | null]> {
    const payload: Record<string, string> = {
      rules_yaml: request.rulesYaml,
      run: request.run,
      time_from: request.timeFrom,
      time_to: request.timeTo
    }
    if (request.baseline) {
      payload.baseline = request.baseline
      if (request.baselineFrom && request.baselineTo) {
        payload.baseline_time_from = request.baselineFrom
        payload.baseline_time_to = request.baselineTo
      }
    }
    return requestJSON(base() + '/quality/evaluate', Security.requestOptions(payload), (data) => data.Evaluation ?? null)
  }
}
