import { type IProblems } from '@/interfaces/problems.interface'
import Security from '@/services/security'
import { requestJSON } from '@/services/request'
import { type DetectionRules, type ProblemList, type ProblemsRequest, type SaveResult } from '@/types/problems'

const base = () => import.meta.env.VITE_LOGS_APP_API_URL + '/api/v1'

export class Problems implements IProblems {
  public async problems (request: ProblemsRequest): Promise<[string | null, ProblemList | null]> {
    const payload = {
      service: request.service,
      environment: request.environment,
      time_from: request.timeFrom,
      time_to: request.timeTo,
      limit: request.limit,
      cursor: request.cursor,
      rules_revision: request.rulesRevision
    }
    return requestJSON(base() + '/problems', Security.requestOptions(payload), (data) => data.Problems ?? null)
  }

  public async services (timeFrom: string, timeTo: string): Promise<[string | null, string[] | null]> {
    const q = new URLSearchParams({ time_from: timeFrom, time_to: timeTo })
    return requestJSON(base() + '/problems/services?' + q.toString(), Security.requestOptions(null, 'GET'), (data) => data.Services ?? [])
  }

  public async rules (): Promise<[string | null, DetectionRules | null]> {
    return requestJSON(base() + '/detection-rules', Security.requestOptions(null, 'GET'), (data) => data.Rules ?? null)
  }

  // saveRules tells a stored revision apart from validation errors, a
  // conflict with another save and a failure; it never throws.
  public async saveRules (rules: DetectionRules, expectedRevision: number): Promise<SaveResult> {
    try {
      const response = await fetch(base() + '/detection-rules', Security.requestOptions({ rules, expected_revision: expectedRevision }))
      let body: any = null
      try {
        body = await response.json()
      } catch {
        // reported below with the status
      }
      if (response.status === 409) {
        return { kind: 'conflict', current: body?.data?.Rules ?? null, message: body?.message ?? 'conflict' }
      }
      if (response.status === 400 && body?.data?.FieldErrors) {
        return { kind: 'invalid', fieldErrors: body.data.FieldErrors, message: body.message }
      }
      if (!response.ok || body === null || body.error || !body.data?.Rules) {
        return { kind: 'failed', message: body?.message || `request failed with status ${response.status}` }
      }
      return { kind: 'saved', rules: body.data.Rules }
    } catch (e) {
      return { kind: 'failed', message: e instanceof Error ? e.message : String(e) }
    }
  }
}
