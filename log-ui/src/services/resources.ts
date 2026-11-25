import { type Span } from '@/types/filter'

// Resources of a call, from attributes recorded by log-sender
// (WithProcessResources: troubleshooting.host.*, troubleshooting.process.*,
// troubleshooting.call.*) and, for a ClickHouse query of Troubleshooting
// itself, by the API (db.server.*; the CPU time comes from ClickHouse's
// query_log, filled in when spans are read). Nothing is estimated here: a
// figure that was not recorded is not shown.

export interface ResourceRow {
  label: string
  value: string
  testid: string
}

export interface SpanResources {
  // 'query': measured by the database server for this query alone.
  // 'call': the whole process's figures for the time of an entry call.
  kind: 'query' | 'call'
  rows: ResourceRow[]
  note: string
}

export interface ProcessInfo {
  host: string
  pid: string
  text: string
}

function tagOf (span: Span, key: string): string {
  for (const list of [span.tags ?? [], span.serviceTags ?? []]) {
    const found = list.find((t) => t.key === key)
    if (found && found.value !== '') {
      return found.value
    }
  }
  return ''
}

function num (span: Span, key: string): number | null {
  const v = tagOf(span, key)
  if (v === '') {
    return null
  }
  const n = Number(v)
  return Number.isFinite(n) ? n : null
}

const UNITS = ['B', 'KB', 'MB', 'GB', 'TB']

export function bytesText (n: number): string {
  let v = n
  let i = 0
  while (Math.abs(v) >= 1024 && i < UNITS.length - 1) {
    v /= 1024
    i++
  }
  const digits = i === 0 || v >= 100 ? 0 : v >= 10 ? 1 : 2
  return `${v.toFixed(digits)} ${UNITS[i]}`
}

export function cpuText (us: number): string {
  if (us < 1000) {
    return `${us} µs`
  }
  const ms = us / 1000
  return `${ms >= 100 ? ms.toFixed(0) : ms.toFixed(1)} ms`
}

function countText (n: number): string {
  return n.toLocaleString('en-US')
}

export function spanResources (span: Span): SpanResources | null {
  const rows: ResourceRow[] = []
  const push = (label: string, value: number | null, fmt: (n: number) => string, testid: string): void => {
    if (value !== null) {
      rows.push({ label, value: fmt(value), testid })
    }
  }

  const memory = num(span, 'db.server.memory_peak_bytes')
  const readRows = num(span, 'db.server.read_rows')
  if (memory !== null || readRows !== null) {
    const fromLog = tagOf(span, 'db.server.source') === 'query_log'
    push('CPU time', num(span, 'db.server.cpu_us'), cpuText, 'resource-db-cpu')
    push('Peak memory', memory, bytesText, 'resource-db-memory')
    push('Rows read', readRows, countText, 'resource-db-read-rows')
    push('Data read', num(span, 'db.server.read_bytes'), bytesText, 'resource-db-read-bytes')
    push('Rows returned', num(span, 'db.server.result_rows'), countText, 'resource-db-result-rows')
    const note = fromLog
      ? 'Measured by the database server for this query alone (its query log).'
      : 'Reported by the database server during the query. CPU time is not shown: it comes from the server\'s query log, which has no entry for this query (not written yet, expired, or another server).'
    return { kind: 'query', rows, note }
  }

  const overlapping = num(span, 'troubleshooting.call.overlapping_calls')
  if (overlapping === null) {
    return null
  }
  push('Process CPU during the call', num(span, 'troubleshooting.call.process_cpu_us'), cpuText, 'resource-call-cpu')
  push('Memory allocated during the call', num(span, 'troubleshooting.call.allocated_bytes'), bytesText, 'resource-call-allocated')
  push('Heap at the end', num(span, 'troubleshooting.call.heap_bytes'), bytesText, 'resource-call-heap')
  push('Process memory (RSS) at the end', num(span, 'troubleshooting.call.rss_bytes'), bytesText, 'resource-call-rss')
  const container = num(span, 'troubleshooting.call.container_memory_bytes')
  const limit = num(span, 'troubleshooting.process.memory_limit_bytes')
  if (container !== null) {
    const share = limit ? ` of ${bytesText(limit)} (${Math.round(container / limit * 100)}%)` : ''
    rows.push({ label: 'Container memory at the end', value: bytesText(container) + share, testid: 'resource-call-container' })
  }
  const note = overlapping === 0
    ? 'No other request or job of this process ran meanwhile, so these are this call\'s own figures (plus the runtime\'s background work).'
    : `${overlapping} other request${overlapping === 1 ? '' : 's'} or job${overlapping === 1 ? '' : 's'} of this process ran at the same time: CPU and allocations are the whole process's, shared between them.`
  return { kind: 'call', rows, note }
}

// processInfo names the machine and process a span ran in, '' parts omitted;
// null when none is recorded.
export function processInfo (span: Span): ProcessInfo | null {
  const host = tagOf(span, 'host.name')
  const pid = tagOf(span, 'process.pid')
  const cpus = num(span, 'troubleshooting.host.cpu_count')
  const hostMemory = num(span, 'troubleshooting.host.memory_bytes')
  const limit = num(span, 'troubleshooting.process.memory_limit_bytes')
  const parts: string[] = []
  if (host) parts.push(`host ${host}`)
  if (pid) parts.push(`process ${pid}`)
  if (cpus !== null) parts.push(`${cpus} CPU${cpus === 1 ? '' : 's'}`)
  if (hostMemory !== null) parts.push(`${bytesText(hostMemory)} memory`)
  if (limit !== null) parts.push(`process limit ${bytesText(limit)}`)
  if (parts.length === 0) {
    return null
  }
  return { host, pid, text: parts.join(' · ') }
}
