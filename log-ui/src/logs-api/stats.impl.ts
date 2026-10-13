import { type ShapeFilter, type ShapeLibrary } from '@/types/shapes'
import { type IStats } from '@/interfaces/stats.interface'
import Security from '@/services/security'
import { requestJSON } from '@/services/request'
import { type Analysis, type AnalysisSummary, type ErrorGroups, type ExamplesRequest, type ExamplesResult, type Population, type PopulationStats, type RetentionPolicy } from '@/types/stats'

const base = () => import.meta.env.VITE_LOGS_APP_API_URL + '/api/v1'

// populationPayload keeps the difference between "any" (omitted) and "not
// recorded" ('').
export function populationPayload (p: Population) {
  return {
    service: p.service,
    operation: p.operation,
    span_kind: p.spanKind,
    environment: p.environment,
    version: p.version,
    installation: p.installation,
    segment: p.segment,
    time_from: p.timeFrom,
    time_to: p.timeTo
  }
}

export class Stats implements IStats {
  public async callStats (population: Population, options: { groupByVersion?: boolean, series?: boolean }): Promise<[string | null, PopulationStats | null]> {
    const payload = { population: populationPayload(population), group_by_version: options.groupByVersion ?? false, series: options.series ?? false }
    return requestJSON(base() + '/stats/calls', Security.requestOptions(payload), (data) => data.Stats ?? null)
  }

  public async errorGroups (population: Population): Promise<[string | null, ErrorGroups | null]> {
    return requestJSON(base() + '/stats/error-groups', Security.requestOptions({ population: populationPayload(population) }), (data) => data.ErrorGroups ?? null)
  }

  public async examples (request: ExamplesRequest): Promise<[string | null, ExamplesResult | null]> {
    const payload = {
      population: populationPayload(request.population),
      kind: request.kind,
      threshold_ms: request.thresholdMs,
      group: request.group
        ? { source_kind: request.group.sourceKind, service: request.group.service, operation: request.group.operation, environment: request.group.environment, fingerprint: request.group.fingerprint }
        : undefined,
      limit: request.limit,
      cutoff: request.cutoff
    }
    return requestJSON(base() + '/stats/examples', Security.requestOptions(payload), (data) => data.Examples ?? null)
  }

  public async compare (baseline: Population, candidate: Population, regenerates?: string): Promise<[string | null, Analysis | null]> {
    const payload = { baseline: populationPayload(baseline), candidate: populationPayload(candidate), regenerates }
    return requestJSON(base() + '/compare', Security.requestOptions(payload), (data) => data.Analysis ?? null)
  }

  public async analyses (): Promise<[string | null, AnalysisSummary[] | null]> {
    return requestJSON(base() + '/analyses', Security.requestOptions(null, 'GET'), (data) => data.Analyses ?? [])
  }

  public async analysis (id: string): Promise<[string | null, Analysis | null]> {
    return requestJSON(base() + '/analyses/' + encodeURIComponent(id), Security.requestOptions(null, 'GET'), (data) => data.Analysis ?? null)
  }

  public async shapes (f: ShapeFilter): Promise<[string | null, ShapeLibrary | null]> {
    const payload = { time_from: f.timeFrom, time_to: f.timeTo, service: f.service ?? '', operation: f.operation ?? '', errors_only: f.errorsOnly ?? false }
    return requestJSON(base() + '/stats/shapes', Security.requestOptions(payload), (data) => data.Library ?? null)
  }

  public async retention (): Promise<[string | null, RetentionPolicy | null]> {
    return requestJSON(base() + '/retention', Security.requestOptions(null, 'GET'), (data) => data.Policy ?? null)
  }
}
