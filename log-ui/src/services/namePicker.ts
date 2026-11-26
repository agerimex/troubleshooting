import { type SpanNameEntry } from '@/types/filter'

// The pure part of the service and span-name picker in Log Analysis. Picks are
// services (left pane) and (service, span name) pairs (right pane). A service
// with picked names is narrowed to them; a picked service without picked names
// is taken whole; picked names of a service that isn't picked count too.

export interface Picks {
  services: string[]
  names: [string, string][] // [service, span name]
}

export const NO_PICKS: Picks = { services: [], names: [] }

export interface ServiceRow {
  service: string
  spans: number
  names: number
  entry: boolean // one of its span names started an execution
}

export interface NameGroup {
  service: string
  names: SpanNameEntry[]
}

export const pairKey = (service: string, name: string) => service + '\u0000' + name

// servicesOf lists the services of a name list, by name.
export function servicesOf (names: SpanNameEntry[]): ServiceRow[] {
  const by = new Map<string, ServiceRow>()
  for (const n of names) {
    const row = by.get(n.service) ?? { service: n.service, spans: 0, names: 0, entry: false }
    row.spans += n.spans
    row.names++
    row.entry = row.entry || n.entry
    by.set(n.service, row)
  }
  return [...by.values()].sort((a, b) => a.service.localeCompare(b.service))
}

const matches = (text: string, search: string) => !search || text.toLowerCase().includes(search.toLowerCase())

// visibleServices filters the left pane by its search box.
export function visibleServices (rows: ServiceRow[], search: string): ServiceRow[] {
  return rows.filter((r) => matches(r.service, search.trim()))
}

// nameGroups is the right pane: the span names of the picked services (all
// services when none is picked), grouped by service, filtered by the search
// box and, with entryOnly, to names that started an execution.
export function nameGroups (names: SpanNameEntry[], services: string[], search: string, entryOnly: boolean): NameGroup[] {
  const picked = new Set(services)
  const groups = new Map<string, SpanNameEntry[]>()
  for (const n of names) {
    if (picked.size && !picked.has(n.service)) continue
    if (entryOnly && !n.entry) continue
    if (!matches(n.name, search.trim())) continue
    groups.set(n.service, [...(groups.get(n.service) ?? []), n])
  }
  return [...groups.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([service, list]) => ({ service, names: [...list].sort((a, b) => a.name.localeCompare(b.name)) }))
}

// toggleService picks or unpicks a service; unpicking drops its picked names,
// so nothing is filtered that the dialog no longer shows.
export function toggleService (p: Picks, service: string): Picks {
  if (p.services.includes(service)) {
    return { services: p.services.filter((s) => s !== service), names: p.names.filter(([s]) => s !== service) }
  }
  return { services: [...p.services, service], names: p.names }
}

export function toggleName (p: Picks, service: string, name: string): Picks {
  const key = pairKey(service, name)
  const has = p.names.some(([s, n]) => pairKey(s, n) === key)
  return { services: p.services, names: has ? p.names.filter(([s, n]) => pairKey(s, n) !== key) : [...p.names, [service, name]] }
}

export const isNamePicked = (p: Picks, service: string, name: string) => p.names.some(([s, n]) => s === service && n === name)

// filterOf turns picks into the API filter: services with picked names are
// narrowed to those names (pairs only), the other picked services go whole.
export function filterOf (p: Picks): { serviceNames?: string[], spanNames?: [string, string][] } {
  const narrowed = new Set(p.names.map(([s]) => s))
  const whole = p.services.filter((s) => !narrowed.has(s))
  return {
    serviceNames: whole.length ? whole : undefined,
    spanNames: p.names.length ? p.names : undefined
  }
}

export const hasPicks = (p: Picks) => p.services.length > 0 || p.names.length > 0

// pickSummary is the short label next to the picker button.
export function pickSummary (p: Picks): string {
  const services = new Set([...p.services, ...p.names.map(([s]) => s)]).size
  const parts: string[] = []
  if (services) parts.push(`${services} service${services === 1 ? '' : 's'}`)
  if (p.names.length) parts.push(`${p.names.length} span name${p.names.length === 1 ? '' : 's'}`)
  return parts.join(' · ')
}
