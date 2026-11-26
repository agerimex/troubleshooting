import { describe, expect, it } from 'vitest'
import { filterOf, nameGroups, NO_PICKS, pickSummary, servicesOf, toggleName, toggleService, visibleServices } from '@/services/namePicker'
import { type SpanNameEntry } from '@/types/filter'

const n = (service: string, name: string, spans = 1, entry = false): SpanNameEntry => ({ service, name, spans, entry })
const list = [
  n('worker', 'process job', 40),
  n('api', 'GET /users', 10, true),
  n('api', 'SELECT users', 30),
  n('worker', 'GET /users', 5),
  n('api', 'POST /jobs', 7, true)
]

describe('name picker', () => {
  it('lists services with their span counts, names and whether they start executions', () => {
    expect(servicesOf(list)).toEqual([
      { service: 'api', spans: 47, names: 3, entry: true },
      { service: 'worker', spans: 45, names: 2, entry: false }
    ])
    expect(visibleServices(servicesOf(list), 'WORK').map((r) => r.service)).toEqual(['worker'])
  })

  it('shows the names of the picked services, all of them when none is picked, grouped and sorted', () => {
    expect(nameGroups(list, [], '', false).map((g) => [g.service, g.names.map((x) => x.name)])).toEqual([
      ['api', ['GET /users', 'POST /jobs', 'SELECT users']],
      ['worker', ['GET /users', 'process job']]
    ])
    expect(nameGroups(list, ['worker'], '', false).map((g) => g.service)).toEqual(['worker'])
    expect(nameGroups(list, [], 'get', false).flatMap((g) => g.names.map((x) => x.service + ' ' + x.name))).toEqual(['api GET /users', 'worker GET /users'])
    expect(nameGroups(list, [], '', true).flatMap((g) => g.names.map((x) => x.name))).toEqual(['GET /users', 'POST /jobs'])
  })

  it('narrows a picked service to its picked names and keeps the other picked services whole', () => {
    let p = toggleService(NO_PICKS, 'api')
    p = toggleService(p, 'worker')
    p = toggleName(p, 'api', 'GET /users')
    expect(filterOf(p)).toEqual({ serviceNames: ['worker'], spanNames: [['api', 'GET /users']] })
    expect(pickSummary(p)).toBe('2 services · 1 span name')
    // A name of the same text in another service is a different pick.
    p = toggleName(p, 'worker', 'GET /users')
    expect(filterOf(p)).toEqual({ serviceNames: undefined, spanNames: [['api', 'GET /users'], ['worker', 'GET /users']] })
    // Picking it again removes it.
    expect(toggleName(p, 'worker', 'GET /users').names).toEqual([['api', 'GET /users']])
  })

  it('unpicking a service drops its picked names; no picks is no filter', () => {
    let p = toggleService(NO_PICKS, 'api')
    p = toggleName(p, 'api', 'SELECT users')
    p = toggleService(p, 'api')
    expect(p).toEqual({ services: [], names: [] })
    expect(filterOf(p)).toEqual({ serviceNames: undefined, spanNames: undefined })
    expect(pickSummary(p)).toBe('')
  })

  it('names can be picked without picking their service first', () => {
    const p = toggleName(NO_PICKS, 'worker', 'process job')
    expect(filterOf(p)).toEqual({ serviceNames: undefined, spanNames: [['worker', 'process job']] })
    expect(pickSummary(p)).toBe('1 service · 1 span name')
  })
})
