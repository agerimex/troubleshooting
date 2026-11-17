import { type IQuality } from '@/interfaces/quality.interface'
import Security from '@/services/security'
import { requestJSON } from '@/services/request'
import { type Catalogue, type EvaluationSummary, type HistoryFilter, type QualityEvaluation, type QualityProject, type QualityRecord, type QualityRequest } from '@/types/quality'

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

  public async projects (): Promise<[string | null, QualityProject[] | null]> {
    return requestJSON(base() + '/quality/projects', Security.requestOptions(null, 'GET'), (data) => data.Projects ?? [])
  }

  public async evaluations (filter: HistoryFilter): Promise<[string | null, { evaluations: EvaluationSummary[], next: string } | null]> {
    const q = new URLSearchParams({ project: filter.project })
    if (filter.ref) {
      q.set('ref', filter.ref)
    }
    if (filter.before) {
      q.set('before', filter.before)
    }
    return requestJSON(base() + '/quality/evaluations?' + q.toString(), Security.requestOptions(null, 'GET'),
      (data) => ({ evaluations: data.Evaluations ?? [], next: data.Next ?? '' }))
  }

  public async record (id: string): Promise<[string | null, QualityRecord | null]> {
    return requestJSON(base() + '/quality/evaluations/' + encodeURIComponent(id), Security.requestOptions(null, 'GET'), (data) => data.Record ?? null)
  }

  public async catalogue (project: string, ref: string): Promise<[string | null, Catalogue | null]> {
    const q = new URLSearchParams({ project })
    if (ref) {
      q.set('ref', ref)
    }
    return requestJSON(base() + '/quality/requirements?' + q.toString(), Security.requestOptions(null, 'GET'), (data) => data.Catalogue ?? null)
  }
}
