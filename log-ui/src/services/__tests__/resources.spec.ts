import { describe, expect, it } from 'vitest'
import { bytesText, cpuText, processInfo, spanResources } from '@/services/resources'
import { type Span } from '@/types/filter'

function span (tags: Record<string, string>, serviceTags: Record<string, string> = {}): Span {
  const list = (m: Record<string, string>) => Object.entries(m).map(([key, value]) => ({ key, value }))
  return {
    timeStamp: '1', name: 'x', service: 'LogAnalysis', traceId: 't', spanId: 's', parentSpanId: '0', msg: '',
    tags: list(tags), serviceTags: list(serviceTags), childSpanCount: 0, statusCode: 0, status: 'unset',
    statusMessage: '', duration: 10, events: [], links: []
  }
}

const PROCESS = {
  'host.name': 'api-1', 'process.pid': '1', 'troubleshooting.host.cpu_count': '1',
  'troubleshooting.host.memory_bytes': String(1007177728), 'troubleshooting.process.memory_limit_bytes': String(160 * 1024 * 1024)
}

describe('resources', () => {
  it('formats bytes and CPU time', () => {
    expect(bytesText(512)).toBe('512 B')
    expect(bytesText(3036072)).toBe('2.90 MB')
    expect(bytesText(160 * 1024 * 1024)).toBe('160 MB')
    expect(cpuText(845)).toBe('845 µs')
    expect(cpuText(118229)).toBe('118 ms')
    expect(cpuText(9348)).toBe('9.3 ms')
  })

  it('shows a query measured from the query log, CPU included', () => {
    const r = spanResources(span({
      'db.system': 'clickhouse', 'db.server.cpu_us': '118229', 'db.server.memory_peak_bytes': '3036072',
      'db.server.read_rows': '156', 'db.server.read_bytes': '40960', 'db.server.result_rows': '3', 'db.server.source': 'query_log'
    }, PROCESS))!
    expect(r.kind).toBe('query')
    expect(r.rows.map((x) => [x.label, x.value])).toEqual([
      ['CPU time', '118 ms'], ['Peak memory', '2.90 MB'], ['Rows read', '156'], ['Data read', '40.0 KB'], ['Rows returned', '3']
    ])
    expect(r.note).toContain('for this query alone')
  })

  it('says why a query recorded without the query log has no CPU time', () => {
    const r = spanResources(span({ 'db.server.memory_peak_bytes': '1000', 'db.server.read_rows': '0', 'db.server.read_bytes': '0' }))!
    expect(r.rows.map((x) => x.label)).toEqual(['Peak memory', 'Rows read', 'Data read'])
    expect(r.note).toContain('CPU time is not shown')
  })

  it('shows an entry call alone as its own figures, with the container share', () => {
    const r = spanResources(span({
      'troubleshooting.call.overlapping_calls': '0', 'troubleshooting.call.process_cpu_us': '9348',
      'troubleshooting.call.allocated_bytes': '1321072', 'troubleshooting.call.heap_bytes': '2373400',
      'troubleshooting.call.rss_bytes': '16592896', 'troubleshooting.call.container_memory_bytes': String(8 * 1024 * 1024)
    }, PROCESS))!
    expect(r.kind).toBe('call')
    expect(r.rows.find((x) => x.testid === 'resource-call-container')!.value).toBe('8.00 MB of 160 MB (5%)')
    expect(r.note).toContain('this call\'s own figures')
  })

  it('marks figures shared with overlapping calls', () => {
    const r = spanResources(span({ 'troubleshooting.call.overlapping_calls': '2', 'troubleshooting.call.process_cpu_us': '10' }))!
    expect(r.note).toContain('2 other requests or jobs')
    expect(r.note).toContain('shared')
  })

  it('shows nothing for a span without recorded resources', () => {
    expect(spanResources(span({ 'http.method': 'GET' }))).toBeNull()
    expect(processInfo(span({}))).toBeNull()
  })

  it('names the machine and process', () => {
    expect(processInfo(span({}, PROCESS))!.text).toBe('host api-1 · process 1 · 1 CPU · 961 MB memory · process limit 160 MB')
  })
})
