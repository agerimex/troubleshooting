import { type Span } from '@/types/filter'
import { type LogRecord, type Report, type ReportResponse, type ReportSection } from '@/types/report'
import { ROOT_PARENT, durationMsText, formatNs } from '@/services/spanTree'

// A report covers one TraceId within one half-open window [from, to). Everything here is
// derived from the generated report only, so the screen and the exports show
// the same evidence; nothing re-queries the API.

export const NO_LOGS_TEXT = 'No correlated logs found within the selected window'
export const NO_SPANS_TEXT = 'No spans were found for this trace within the searched window.'
export const SCOPE_BOUNDARY_TEXT =
  'This report covers one trace only. Records of other traces are not included, even when they share a business ' +
  'identifier; linked traces are listed as references that have not been inspected. It does not establish complete ' +
  'coverage or the final outcome of a business operation.'

// fromResponse turns a full API answer into a report.
export function fromResponse (response: ReportResponse): Report | null {
  if (!response.spans || !response.logs) {
    return null
  }
  return {
    traceId: response.traceId,
    timeFrom: response.timeFrom,
    timeTo: response.timeTo,
    generatedAt: response.generatedAt,
    spans: { ...response.spans, retrievedAt: response.generatedAt },
    logs: { ...response.logs, retrievedAt: response.generatedAt },
    warnings: response.warnings ?? []
  }
}

// mergeSections replaces the sections a retry returned and keeps the rest;
// the trace and the window never change.
export function mergeSections (report: Report, response: ReportResponse): Report {
  const merged: Report = { ...report }
  const retried: string[] = []
  if (response.spans) {
    merged.spans = { ...response.spans, retrievedAt: response.generatedAt }
    retried.push('spans')
  }
  if (response.logs) {
    merged.logs = { ...response.logs, retrievedAt: response.generatedAt }
    retried.push('logs')
  }
  merged.warnings = [
    ...report.warnings.filter((w) => !retried.includes(w.section)),
    ...(response.warnings ?? []).filter((w) => retried.includes(w.section))
  ]
  return merged
}

// --- completeness

function sectionLimitation (section: ReportSection<unknown>, noun: string): string | null {
  if (section.status !== 'ok') {
    return `${capitalize(noun)} evidence is unavailable (${statusText(section.status)}): ${section.error ?? 'no details'}`
  }
  if (section.truncated) {
    return `${capitalize(noun)} are truncated: ${section.shown} shown (limit ${section.limit}); more match.`
  }
  return null
}

export function completeness (report: Report): { complete: boolean, reasons: string[] } {
  const reasons = [sectionLimitation(report.spans, 'spans'), sectionLimitation(report.logs, 'logs')]
    .filter((r): r is string => r !== null)
  return { complete: reasons.length === 0, reasons }
}

export function statusText (status: string): string {
  switch (status) {
    case 'unavailable': return 'source unavailable'
    case 'limit': return 'query limit or timeout'
    case 'error': return 'query failed'
    default: return status
  }
}

// sectionSummary labels the number as records shown and says whether the
// section is complete.
export function sectionSummary (section: ReportSection<unknown>, noun: string): string {
  if (section.status !== 'ok') {
    return `${capitalize(noun)} unavailable (${statusText(section.status)}). This is not a result of zero records.`
  }
  const state = section.truncated ? `truncated: more ${noun} match (limit ${section.limit})` : 'complete'
  const total = section.totalStatus === 'ok' && section.total !== null
    ? `${section.total} matching`
    : 'matching total unavailable'
  return `${section.shown} ${noun} shown · ${state} · ${total}`
}

function capitalize (s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}

// --- associations

export type AssociationKind = 'span' | 'no-span-id' | 'span-not-found' | 'span-not-loaded' | 'spans-unavailable'

export interface Association {
  kind: AssociationKind
  label: string
  span?: Span
}

// association follows the recorded SpanId only; a log is never attached to a
// span by its timestamp. Every log in a report recorded the report's TraceId.
export function association (log: LogRecord, report: Report): Association {
  if (log.spanId === '') {
    return { kind: 'no-span-id', label: 'No span identifier recorded' }
  }
  if (report.spans.status !== 'ok') {
    return { kind: 'spans-unavailable', label: `Span ${log.spanId}: span evidence unavailable` }
  }
  const span = report.spans.records.find((s) => s.traceId === log.traceId && s.spanId === log.spanId)
  if (span) {
    return { kind: 'span', label: `Span ${span.name}`, span }
  }
  if (report.spans.truncated) {
    return { kind: 'span-not-loaded', label: `Referenced span ${log.spanId} not among the loaded spans` }
  }
  return { kind: 'span-not-found', label: 'Referenced span not found within the window' }
}

function compareNs (a: string, b: string): number {
  const x = BigInt(a)
  const y = BigInt(b)
  return x < y ? -1 : x > y ? 1 : 0
}

// sortedLogs orders by recorded timestamp, then record id (stable for ties).
export function sortedLogs (logs: LogRecord[]): LogRecord[] {
  return [...logs].sort((a, b) => compareNs(a.timeUnixNano, b.timeUnixNano) || (a.recordId < b.recordId ? -1 : a.recordId > b.recordId ? 1 : 0))
}

// logsOfSpan are the logs that recorded this span's identity.
export function logsOfSpan (report: Report, span: Span): LogRecord[] {
  return sortedLogs(report.logs.records.filter((l) => l.traceId === span.traceId && l.spanId === span.spanId))
}

// --- span metadata

export function serviceVersion (span: Span): string | null {
  for (const list of [span.serviceTags ?? [], span.tags ?? []]) {
    const found = list.find((t) => t.key === 'service.version')
    if (found && found.value !== '') {
      return found.value
    }
  }
  return null
}

export interface ReportLink {
  fromSpanId: string
  fromSpanName: string
  traceId: string
  spanId: string
  attributes: Record<string, string>
  // Only a link inside the reported trace has been inspected.
  inspected: boolean
}

export function reportLinks (report: Report): ReportLink[] {
  const links: ReportLink[] = []
  for (const span of report.spans.records) {
    for (const link of span.links ?? []) {
      links.push({
        fromSpanId: span.spanId,
        fromSpanName: span.name,
        traceId: link.traceId,
        spanId: link.spanId,
        attributes: link.attributes ?? {},
        inspected: link.traceId === report.traceId
      })
    }
  }
  return links
}

// spanOrder lists spans depth-first by parent, with their depth; spans whose
// parent is absent start at depth 0.
export function spanOrder (spans: Span[]): { span: Span, depth: number, parentMissing: boolean }[] {
  const ids = new Set(spans.map((s) => s.spanId))
  const children = new Map<string, Span[]>()
  const tops: Span[] = []
  const sorted = [...spans].sort((a, b) => compareNs(a.timeStamp, b.timeStamp))
  for (const s of sorted) {
    if (s.parentSpanId === ROOT_PARENT || !ids.has(s.parentSpanId)) {
      tops.push(s)
    } else {
      children.set(s.parentSpanId, [...(children.get(s.parentSpanId) ?? []), s])
    }
  }
  const out: { span: Span, depth: number, parentMissing: boolean }[] = []
  const seen = new Set<string>()
  const walk = (s: Span, depth: number, parentMissing: boolean) => {
    if (seen.has(s.spanId)) {
      return
    }
    seen.add(s.spanId)
    out.push({ span: s, depth, parentMissing })
    for (const c of children.get(s.spanId) ?? []) {
      walk(c, depth + 1, false)
    }
  }
  for (const s of tops) {
    walk(s, 0, s.parentSpanId !== ROOT_PARENT)
  }
  for (const s of sorted) {
    walk(s, 0, true) // loops
  }
  return out
}

// --- export

export const EXPORT_FORMAT = 'troubleshooting-trace-report'
export const EXPORT_VERSION = 1

// exportJson is the full generated report: a focused span view in the UI never
// narrows it.
export function exportJson (report: Report, exportedAt: Date) {
  const c = completeness(report)
  return {
    format: EXPORT_FORMAT,
    version: EXPORT_VERSION,
    scope: {
      kind: 'single-trace',
      traceId: report.traceId,
      timeFrom: report.timeFrom,
      timeTo: report.timeTo,
      timeFromText: formatNs(report.timeFrom),
      timeToText: formatNs(report.timeTo),
      window: '[from, to)',
      statement: SCOPE_BOUNDARY_TEXT
    },
    generatedAt: report.generatedAt,
    exportedAt: (BigInt(exportedAt.getTime()) * BigInt(1000000)).toString(),
    complete: c.complete,
    limitations: c.reasons,
    warnings: report.warnings,
    spans: {
      source: 'trace',
      status: report.spans.status,
      error: report.spans.error ?? null,
      retrievedAt: report.spans.retrievedAt,
      shown: report.spans.shown,
      limit: report.spans.limit,
      truncated: report.spans.truncated,
      total: report.spans.totalStatus === 'ok' ? report.spans.total : null,
      totalStatus: report.spans.totalStatus,
      records: report.spans.records.map((s) => ({
        traceId: s.traceId,
        spanId: s.spanId,
        parentSpanId: s.parentSpanId,
        name: s.name,
        service: s.service,
        serviceVersion: serviceVersion(s),
        startTime: s.timeStamp,
        durationMicros: s.duration,
        status: s.status,
        statusMessage: s.statusMessage,
        attributes: Object.fromEntries((s.tags ?? []).map((t) => [t.key, t.value])),
        resourceAttributes: Object.fromEntries((s.serviceTags ?? []).map((t) => [t.key, t.value])),
        events: (s.events ?? []).map((e) => ({ source: 'span-event', time: e.timeUnixNano, name: e.name, attributes: e.attributes })),
        links: (s.links ?? []).map((l) => ({ traceId: l.traceId, spanId: l.spanId, attributes: l.attributes, inspected: l.traceId === report.traceId }))
      }))
    },
    logs: {
      source: 'log',
      status: report.logs.status,
      error: report.logs.error ?? null,
      retrievedAt: report.logs.retrievedAt,
      shown: report.logs.shown,
      limit: report.logs.limit,
      truncated: report.logs.truncated,
      total: report.logs.totalStatus === 'ok' ? report.logs.total : null,
      totalStatus: report.logs.totalStatus,
      records: sortedLogs(report.logs.records).map((l) => {
        const a = association(l, report)
        return {
          recordId: l.recordId,
          time: l.timeUnixNano,
          severity: l.severity,
          service: l.service,
          message: l.message,
          traceId: l.traceId,
          spanId: l.spanId === '' ? null : l.spanId,
          association: a.kind,
          associationText: a.label,
          attributes: l.attributes
        }
      })
    },
    links: reportLinks(report)
  }
}

const htmlEscapes: Record<string, string> = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }

// escapeHtml makes recorded text inert in the HTML export.
export function escapeHtml (value: unknown): string {
  return String(value ?? '').replace(/[&<>"']/g, (c) => htmlEscapes[c])
}

// reportHtmlBody is the content of a report's HTML export; every recorded
// value is escaped.
export function reportHtmlBody (report: Report, exportedAt: Date): string {
  const data = exportJson(report, exportedAt)
  const e = escapeHtml
  const rows = (cells: unknown[]) => '<tr>' + cells.map((c) => `<td>${e(c)}</td>`).join('') + '</tr>'
  const head = (cells: string[]) => '<tr>' + cells.map((c) => `<th>${e(c)}</th>`).join('') + '</tr>'
  const section = (s: { status: string, error: string | null, shown: number, limit: number, truncated: boolean, total: number | null }, noun: string) =>
    s.status !== 'ok'
      ? `<p class="bad">${e(capitalize(noun))} unavailable (${e(statusText(s.status))}): ${e(s.error ?? '')}. This is not a result of zero records.</p>`
      : `<p>${e(s.shown)} ${e(noun)} shown (limit ${e(s.limit)}) · ${s.truncated ? '<b class="bad">truncated: more match</b>' : 'complete'} · ${s.total === null ? 'matching total unavailable' : e(s.total) + ' matching'}</p>`

  const spanRows = spanOrder(report.spans.records).map(({ span, depth, parentMissing }) =>
    '<tr>' +
    `<td style="padding-left:${0.5 + depth * 1.25}rem">${e(span.name)}${parentMissing ? ' <i>(parent not found within the searched window)</i>' : ''}</td>` +
    [span.service, serviceVersion(span) ?? 'unavailable', span.status, durationMsText(span.duration), formatNs(span.timeStamp), span.spanId, span.parentSpanId]
      .map((c) => `<td>${e(c)}</td>`).join('') +
    '</tr>').join('')

  const eventBlocks = report.spans.records.flatMap((span) => (span.events ?? []).map((ev) =>
    `<div class="event"><b>${e(ev.name)}</b> · span event of ${e(span.name)} (${e(span.spanId)}) · ${e(formatNs(ev.timeUnixNano))}` +
    (ev.attributes['exception.type'] || ev.attributes['exception.message']
      ? `<div class="bad">${e(ev.attributes['exception.type'] ?? '')} ${e(ev.attributes['exception.message'] ?? '')}</div>` : '') +
    (ev.attributes['exception.stacktrace'] ? `<pre>${e(ev.attributes['exception.stacktrace'])}</pre>` : '') +
    '</div>')).join('')

  const logRows = data.logs.records.map((l) =>
    rows([formatNs(l.time), l.severity || 'not recorded', l.service, l.associationText, l.message, l.recordId])).join('')

  const linkRows = data.links.map((l) =>
    rows([l.fromSpanName, l.traceId, l.spanId, l.inspected ? 'in this trace' : 'not inspected'])).join('')

  return `<h1>Trace investigation report</h1>
<p>Scope: one trace <code>${e(report.traceId)}</code> within ${e(formatNs(report.timeFrom))} – ${e(formatNs(report.timeTo))} ([from, to); Unix ns ${e(report.timeFrom)} – ${e(report.timeTo)}).</p>
<p>Generated at ${e(formatNs(report.generatedAt))}; exported at ${e(formatNs(data.exportedAt))}. This file is a copy of the generated report and does not query the service.</p>
<p><b class="${data.complete ? '' : 'bad'}">${data.complete ? 'Complete' : 'Incomplete'}</b>${data.complete ? ': every source returned all matching records of this trace within the window. That does not prove every event was instrumented or delivered.' : ''}</p>
${data.limitations.length ? '<ul>' + data.limitations.map((r) => `<li class="bad">${e(r)}</li>`).join('') + '</ul>' : ''}
${report.warnings.length ? '<ul>' + report.warnings.map((w) => `<li>${e(w.message)}</li>`).join('') + '</ul>' : ''}
<p><i>${e(SCOPE_BOUNDARY_TEXT)}</i></p>
<h2>Spans (source: traces)</h2>
${section(data.spans, 'spans')}
${data.spans.status === 'ok' && data.spans.records.length === 0 ? `<p>${e(NO_SPANS_TEXT)}</p>` : ''}
<table>${head(['operation', 'service', 'version', 'span status', 'duration, ms', 'start', 'span id', 'parent span id'])}${spanRows}</table>
<h2>Span events (source: traces, not log records)</h2>
${eventBlocks || '<p>No span events were recorded.</p>'}
<h2>Linked traces</h2>
${linkRows ? `<table>${head(['from span', 'trace id', 'span id', 'status'])}${linkRows}</table>` : '<p>No links were recorded.</p>'}
<h2>Log records (source: logs)</h2>
${section(data.logs, 'logs')}
${data.logs.status === 'ok' && data.logs.records.length === 0 ? `<p>${e(NO_LOGS_TEXT)}</p>` : ''}
<table>${head(['time', 'log severity', 'service', 'association', 'message', 'record id'])}${logRows}</table>
`
}

// HTML_STYLE is the stylesheet of exported pages.
export const HTML_STYLE = `body { font-family: system-ui, sans-serif; margin: 1.5rem; color: #1b1b1b; background: #fff; }
table { border-collapse: collapse; width: 100%; margin: 0.5rem 0 1.5rem; font-size: 0.85rem; }
th, td { border: 1px solid #ccc; padding: 0.25rem 0.5rem; text-align: left; vertical-align: top; }
td:nth-last-child(2) { white-space: pre-wrap; }
pre { white-space: pre-wrap; font-size: 0.8rem; background: #f5f5f5; padding: 0.5rem; }
.bad { color: #b00020; }
.event { border-top: 1px solid #ddd; padding: 0.5rem 0; }
section.embedded { border: 2px solid #888; padding: 0 1rem; margin: 1rem 0; }`

// htmlPage wraps a body in a self-contained page without scripts. The
// Content-Security-Policy forbids scripts and remote content anyway.
export function htmlPage (title: string, body: string): string {
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'">
<title>${escapeHtml(title)}</title>
<style>
${HTML_STYLE}
</style>
</head>
<body>
${body}
</body>
</html>
`
}

// exportHtml is a self-contained page without scripts. Every recorded value is
// escaped, and the Content-Security-Policy forbids scripts anyway.
export function exportHtml (report: Report, exportedAt: Date): string {
  return htmlPage('Trace report ' + report.traceId, reportHtmlBody(report, exportedAt))
}

// download saves text as a file in the browser.
export function download (fileName: string, mime: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: mime }))
  const a = document.createElement('a')
  a.href = url
  a.download = fileName
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
