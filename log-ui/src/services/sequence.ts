import { ROOT_PARENT } from '@/services/spanTree'
import { association, escapeHtml, htmlPage, reportHtmlBody, sortedLogs } from '@/services/report'
import { isErrorSeverity, offsetMs, spanKey, traceStart } from '@/services/problems'
import { type Span } from '@/types/filter'
import { type LogRecord, type Report } from '@/types/report'

// A sequence view of one generated report (sequence-semantics-v1 of the
// troubleshooting-sequence-agent package). It is derived from recorded
// evidence only: participants are recorded services, a CLIENT span and its
// single directly parented SERVER span of another service are one correlated
// operation (a call), a PRODUCER span and its single directly parented
// CONSUMER span of another service are one message (one-way: an arrow at the
// send, then the consumer's processing in its own column, no end of the send);
// both spans of a pair stay selectable, every other span is its own
// operation. Completion markers are span ends, never responses. Nothing is
// inferred from names, arrival order or timestamps alone.

export const SEQUENCE_RENDERER = 'sequence-semantics-v1'

export const NO_SERVICE = 'service not recorded'

// Attributes that name the peer of a CLIENT span whose server was not
// captured; the peer is shown as unobserved, never as an instrumented service.
const PEER_ATTRIBUTES = ['peer.service', 'server.address', 'net.peer.name', 'db.system', 'messaging.system', 'rpc.service']

export type OperationKind = 'call' | 'message' | 'client' | 'server' | 'local' | 'producer' | 'consumer'

export interface SeqOperation {
  id: string // the key of its first span (the CLIENT or PRODUCER of a pair)
  name: string
  kind: OperationKind
  spanKind: string // recorded kind of the first span ('' when not recorded)
  source: string // participant where it starts
  target: string // participant it reaches (= source for local operations)
  targetObserved: boolean // false: a peer recorded only by the client's attributes, or unknown
  spanKeys: string[] // CLIENT then SERVER of a call, PRODUCER then CONSUMER of a message
  startMs: number
  endMs: number | null // null: no valid end (never a zero duration); a message: the consumer's end
  serverName?: string // the SERVER of a call, or the CONSUMER of a message
  status: string
  serverStatus?: string
  consumerStartMs?: number // a message: the consumer's processing, in the target's column
  missingParent: boolean
  repeat?: { index: number, count: number } // same name, source and target: repeated calls, not retries
}

export type EventKind = 'call' | 'message' | 'start' | 'end' | 'log'

export interface SeqEvent {
  id: string
  kind: EventKind
  timeMs: number | null // null: no usable time (listed separately, never placed by guess)
  operationId: string
  spanKey: string
  participant: string
  target?: string // for calls and messages
  label: string
  durationMs?: number // an end mark: the duration of the span it ends
  logId?: string
  severity?: string
  note?: string // a timing limitation, e.g. a log outside its span's interval
}

export interface SeqOverlap {
  a: string // span keys
  b: string
  aName: string
  bName: string
  fromMs: number
  toMs: number
  overlapMs: number
}

export interface SeqLink {
  fromKey: string
  fromName: string
  traceId: string
  spanId: string
  inReport: boolean // a link to a span of this report; otherwise not loaded
}

export interface SequenceModel {
  renderer: string
  traceId: string
  generatedAt: string
  participants: string[] // order of first appearance
  unobserved: string[] // participants seen only as a client's recorded peer
  operations: SeqOperation[]
  events: SeqEvent[]
  overlaps: SeqOverlap[]
  links: SeqLink[]
  traceLevelLogs: number // logs of the trace without a SpanId
  missingSpanLogs: number // logs whose SpanId isn't among the report's spans
  clockUncertain: string[] // calls whose server interval lies outside its client's; messages consumed before they were produced
  limitations: string[]
  totalMs: number
  spanCount: number
}

const kindOf = (s: Span) => (s.spanKind ?? '').toLowerCase()
const serviceOf = (s: Span) => s.service || NO_SERVICE
const attr = (s: Span, key: string) => [...(s.tags ?? []), ...(s.serviceTags ?? [])].find((t) => t.key === key)?.value ?? ''

function peerOf (s: Span): string {
  for (const key of PEER_ATTRIBUTES) {
    const v = attr(s, key)
    if (v) {
      return v
    }
  }
  return ''
}

function compareKey (a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0
}

// buildSequence derives the sequence model of a report. It is linear in spans
// and logs apart from sorting; overlaps are found per parent with a sweep, so
// an all-overlapping trace stays bounded by maxOverlaps.
export function buildSequence (report: Report, maxOverlaps = 50): SequenceModel {
  const spans = report.spans.records
  const origin = traceStart(spans) ?? (report.logs.records.length ? BigInt(sortedLogs(report.logs.records)[0].timeUnixNano) : 0n)
  const byKey = new Map(spans.map((s) => [spanKey(s), s]))
  const children = new Map<string, Span[]>()
  for (const s of spans) {
    if (s.parentSpanId && s.parentSpanId !== ROOT_PARENT) {
      const parent = s.traceId + ':' + s.parentSpanId
      children.set(parent, [...(children.get(parent) ?? []), s])
    }
  }
  const startOf = (s: Span) => offsetMs(s.timeStamp, origin)
  const endOf = (s: Span) => (s.duration >= 0 ? startOf(s) + s.duration / 1000 : null)

  // Pairs: a CLIENT with exactly one directly parented SERVER child of
  // another service. Several candidates stay separate (no guessing).
  // Messages: a PRODUCER with exactly one directly parented CONSUMER child of
  // another service, by the same rule (fan-out or none: no pairing).
  const pairOf = new Map<string, Span>() // client or producer key -> server or consumer
  const paired = new Set<string>() // server and consumer keys consumed by a pair
  const peerKind: Record<string, string> = { client: 'server', producer: 'consumer' }
  for (const s of spans) {
    const want = peerKind[kindOf(s)]
    if (!want) {
      continue
    }
    const peers = (children.get(spanKey(s)) ?? []).filter((c) => kindOf(c) === want && serviceOf(c) !== serviceOf(s))
    if (peers.length === 1) {
      pairOf.set(spanKey(s), peers[0])
      paired.add(spanKey(peers[0]))
    }
  }

  const participants: string[] = []
  const unobserved: string[] = []
  const addParticipant = (p: string, observed = true) => {
    if (!participants.includes(p)) {
      participants.push(p)
      if (!observed) {
        unobserved.push(p)
      }
    } else if (observed && unobserved.includes(p)) {
      unobserved.splice(unobserved.indexOf(p), 1)
    }
  }

  const ordered = [...spans].sort((a, b) => compareKey(a.timeStamp.padStart(20, '0'), b.timeStamp.padStart(20, '0')) || compareKey(spanKey(a), spanKey(b)))
  const operations: SeqOperation[] = []
  const operationOfSpan = new Map<string, SeqOperation>()
  const clockUncertain: string[] = []
  for (const s of ordered) {
    const key = spanKey(s)
    if (paired.has(key)) {
      continue
    }
    const kind = kindOf(s)
    const parentMissing = !!s.parentSpanId && s.parentSpanId !== ROOT_PARENT && !byKey.has(s.traceId + ':' + s.parentSpanId)
    const source = serviceOf(s)
    addParticipant(source)
    const op: SeqOperation = {
      id: key, name: s.name, kind: 'local', spanKind: kind, source, target: source, targetObserved: true, spanKeys: [key],
      startMs: startOf(s), endMs: endOf(s), status: s.status || 'unset', missingParent: parentMissing
    }
    const server = pairOf.get(key)
    if (server && kind === 'producer') {
      // One-way: the arrow is the send; what follows is the consumer's own
      // processing. Delivery may come after the send ended (async), so only a
      // consumer starting before its producer is a clock doubt.
      op.kind = 'message'
      op.target = serviceOf(server)
      op.spanKeys.push(spanKey(server))
      op.serverName = server.name
      op.serverStatus = server.status || 'unset'
      op.consumerStartMs = startOf(server)
      op.endMs = endOf(server)
      addParticipant(op.target)
      if (op.consumerStartMs < op.startMs) {
        clockUncertain.push(key)
      }
    } else if (server) {
      op.kind = 'call'
      op.target = serviceOf(server)
      op.spanKeys.push(spanKey(server))
      op.serverName = server.name
      op.serverStatus = server.status || 'unset'
      addParticipant(op.target)
      const ss = startOf(server)
      const se = endOf(server)
      if (ss < op.startMs || (se !== null && op.endMs !== null && se > op.endMs)) {
        clockUncertain.push(key)
      }
    } else if (kind === 'client') {
      const peer = peerOf(s)
      op.kind = 'client'
      op.target = peer ? `${peer} (not instrumented)` : 'unknown peer'
      op.targetObserved = false
      addParticipant(op.target, false)
    } else if (kind === 'server') {
      op.kind = 'server'
    } else if (kind === 'producer' || kind === 'consumer') {
      op.kind = kind
    }
    operations.push(op)
    for (const k of op.spanKeys) {
      operationOfSpan.set(k, op)
    }
  }

  // Repeated calls: same name, source and target. Described as repeats only.
  const groups = new Map<string, SeqOperation[]>()
  for (const op of operations) {
    const g = op.name + '\u0000' + op.source + '\u0000' + op.target
    groups.set(g, [...(groups.get(g) ?? []), op])
  }
  for (const list of groups.values()) {
    if (list.length > 1) {
      list.forEach((op, i) => { op.repeat = { index: i + 1, count: list.length } })
    }
  }

  const events: SeqEvent[] = []
  for (const op of operations) {
    if (op.kind === 'message') {
      // The send (no end of its own: a message has no response), then the
      // consumer's processing in its column. :start/:end are the consumer's,
      // so the activation bar spans the processing.
      const consumer = op.spanKeys[1]
      events.push({ id: op.id + ':send', kind: 'message', timeMs: op.startMs, operationId: op.id, spanKey: op.id, participant: op.source, target: op.target, label: op.name })
      events.push({ id: op.id + ':start', kind: 'start', timeMs: op.consumerStartMs!, operationId: op.id, spanKey: consumer, participant: op.target, label: op.serverName! })
      if (op.endMs !== null) {
        events.push({ id: op.id + ':end', kind: 'end', timeMs: op.endMs, operationId: op.id, spanKey: consumer, participant: op.target, label: op.serverName!, durationMs: op.endMs - op.consumerStartMs! })
      }
      continue
    }
    const startKind: EventKind = op.kind === 'call' || op.kind === 'client' ? 'call' : 'start'
    events.push({ id: op.id + ':start', kind: startKind, timeMs: op.startMs, operationId: op.id, spanKey: op.id, participant: op.source, target: startKind === 'call' ? op.target : undefined, label: op.name })
    if (op.endMs !== null) {
      events.push({ id: op.id + ':end', kind: 'end', timeMs: op.endMs, operationId: op.id, spanKey: op.id, participant: op.source, label: op.name, durationMs: op.endMs - op.startMs })
    }
  }

  // ERROR/FATAL logs are markers on the span that recorded them; on a paired
  // operation they sit on the participant of their own span.
  let traceLevelLogs = 0
  let missingSpanLogs = 0
  for (const log of sortedLogs(report.logs.records)) {
    if (!log.spanId) {
      traceLevelLogs++
      continue
    }
    const key = log.traceId + ':' + log.spanId
    const span = byKey.get(key)
    if (!span) {
      missingSpanLogs++
      continue
    }
    if (!isErrorSeverity(log.severity)) {
      continue
    }
    const op = operationOfSpan.get(key)!
    const t = offsetMs(log.timeUnixNano, origin)
    const start = startOf(span)
    const end = endOf(span)
    const outside = t < start || (end !== null && t > end)
    events.push({
      id: 'log:' + log.recordId, kind: 'log', timeMs: t, operationId: op.id, spanKey: key, participant: serviceOf(span), label: log.message,
      logId: log.recordId, severity: log.severity, note: outside ? 'recorded outside its span\'s interval' : undefined
    })
  }
  // Chronological, ties broken by a stable id (not a causal order).
  events.sort((a, b) => (a.timeMs ?? Infinity) - (b.timeMs ?? Infinity) || compareKey(a.id, b.id))

  const overlaps = siblingOverlapsBounded(spans, children, startOf, endOf, new Set(clockUncertain.length ? spans.map(serviceOf) : []), maxOverlaps)

  const links: SeqLink[] = []
  for (const s of spans) {
    for (const l of s.links ?? []) {
      links.push({ fromKey: spanKey(s), fromName: s.name, traceId: l.traceId, spanId: l.spanId, inReport: byKey.has(l.traceId + ':' + l.spanId) })
    }
  }

  const limitations: string[] = []
  if (report.spans.status !== 'ok') {
    limitations.push(`Span evidence is unavailable (${report.spans.error || report.spans.status}): no sequence can be drawn.`)
  } else if (report.spans.truncated) {
    limitations.push(`The server returned ${report.spans.shown} of ${report.spans.total ?? 'more'} spans (limit ${report.spans.limit}): the sequence is partial.`)
  }
  if (report.logs.status !== 'ok') {
    limitations.push(`Log evidence is unavailable (${report.logs.error || report.logs.status}): no log markers are shown, which doesn't mean there were none.`)
  } else if (report.logs.truncated) {
    limitations.push(`Logs are truncated: ${report.logs.shown} of ${report.logs.total ?? 'more'} loaded.`)
  }
  const uncertainCalls = clockUncertain.filter((k) => operationOfSpan.get(k)?.kind === 'call').length
  const uncertainMessages = clockUncertain.length - uncertainCalls
  if (clockUncertain.length) {
    const what = [
      uncertainCalls ? `${uncertainCalls} server span(s) start before or end after their client span` : '',
      uncertainMessages ? `${uncertainMessages} consumer span(s) start before their producer span` : ''
    ].filter(Boolean).join(' and ')
    limitations.push(`${what}: clocks of those services may differ, so cross-service overlaps are not claimed.`)
  }
  const missingParents = operations.filter((o) => o.missingParent).length
  if (missingParents) {
    limitations.push(`${missingParents} operation(s) reference a parent that isn't in this report; no parent or root was invented.`)
  }

  const ends = operations.flatMap((o) => [o.startMs, o.endMs ?? o.startMs])
  return {
    renderer: SEQUENCE_RENDERER,
    traceId: report.traceId,
    generatedAt: report.generatedAt,
    participants,
    unobserved,
    operations,
    events,
    overlaps,
    links,
    traceLevelLogs,
    missingSpanLogs,
    clockUncertain,
    limitations,
    totalMs: ends.length ? Math.max(...ends) : 0,
    spanCount: spans.length
  }
}

// siblingOverlapsBounded: overlaps of children of the same parent, found per
// parent by a sweep over start-sorted intervals; at most max pairs. Siblings
// of different services are skipped when clocks are uncertain.
function siblingOverlapsBounded (spans: Span[], children: Map<string, Span[]>, startOf: (s: Span) => number, endOf: (s: Span) => number | null,
  uncertainServices: Set<string>, max: number): SeqOverlap[] {
  const keys = new Set(spans.map(spanKey))
  const out: SeqOverlap[] = []
  for (const [parent, kids] of children) {
    if (!keys.has(parent)) {
      continue
    }
    const iv = kids
      .map((s) => ({ s, start: startOf(s), end: endOf(s) }))
      .filter((x): x is { s: Span, start: number, end: number } => x.end !== null)
      .sort((a, b) => a.start - b.start)
    const active: typeof iv = []
    for (const cur of iv) {
      for (let i = active.length - 1; i >= 0; i--) {
        if (active[i].end <= cur.start) {
          active.splice(i, 1)
        }
      }
      for (const a of active) {
        if (uncertainServices.size && serviceOf(a.s) !== serviceOf(cur.s)) {
          continue
        }
        const from = Math.max(a.start, cur.start)
        const to = Math.min(a.end, cur.end)
        if (to > from) {
          out.push({ a: spanKey(a.s), b: spanKey(cur.s), aName: a.s.name, bName: cur.s.name, fromMs: from, toMs: to, overlapMs: to - from })
          if (out.length >= max) {
            return out.sort((x, y) => y.overlapMs - x.overlapMs)
          }
        }
      }
      active.push(cur)
    }
  }
  return out.sort((x, y) => y.overlapMs - x.overlapMs)
}

// --- presentation helpers

export interface SequencePage {
  events: SeqEvent[]
  start: number // index of the first event shown
  total: number
}

// visibleEvents applies the internal-operation filter (presentation only),
// then pages by maxMarks. Events of the selected span are never filtered out.
export function visibleEvents (model: SequenceModel, showInternal: boolean, selectedKey: string | null): SeqEvent[] {
  if (showInternal) {
    return model.events
  }
  const local = new Set(model.operations.filter((o) => o.kind === 'local').map((o) => o.id))
  return model.events.filter((e) => !local.has(e.operationId) || e.kind === 'log' || e.operationId === selectedKey || e.spanKey === selectedKey)
}

export function pageOf (events: SeqEvent[], start: number, maxMarks: number): SequencePage {
  const from = Math.max(0, Math.min(start, Math.max(0, events.length - maxMarks)))
  return { events: events.slice(from, from + maxMarks), start: from, total: events.length }
}

// activationOf places the activation bar of one operation on a page: rows
// (indexes into page.events) from its start mark to its end mark, clipped to
// the page; startsHere/endsHere say whether the ends are on this page. null
// when the operation has no start or no valid end, or lies on other pages.
export interface Activation {
  participant: string
  from: number
  to: number
  startsHere: boolean
  endsHere: boolean
}

export function activationOf (events: SeqEvent[], page: SequencePage, operationId: string): Activation | null {
  const s = events.findIndex((e) => e.id === operationId + ':start')
  const en = events.findIndex((e) => e.id === operationId + ':end')
  if (s < 0 || en < 0 || page.events.length === 0) {
    return null
  }
  const first = page.start
  const last = page.start + page.events.length - 1
  if (en < first || s > last) {
    return null
  }
  return {
    participant: events[s].participant,
    from: Math.max(s, first) - first,
    to: Math.min(en, last) - first,
    startsHere: s >= first,
    endsHere: en <= last
  }
}

// locate returns the page start that shows the first event of a span.
export function locate (events: SeqEvent[], key: string, maxMarks: number): number | null {
  const i = events.findIndex((e) => e.spanKey === key || e.operationId === key)
  return i < 0 ? null : Math.floor(i / maxMarks) * maxMarks
}

export interface Lanes {
  shown: string[] // participants with their own lane
  other: string[] // participants folded into "other services"
}

// lanes keeps at most maxLanes; the rest share one "other services" lane,
// with their names shown on every mark.
export function lanes (model: SequenceModel, maxLanes: number): Lanes {
  if (model.participants.length <= maxLanes) {
    return { shown: model.participants, other: [] }
  }
  return { shown: model.participants.slice(0, maxLanes - 1), other: model.participants.slice(maxLanes - 1) }
}

export const OTHER_LANE = 'other services'

export function laneOf (l: Lanes, participant: string): string {
  return l.shown.includes(participant) ? participant : OTHER_LANE
}

// eventText is the one-line description of a mark, used in the list view and
// the export: source → target for calls.
export function eventText (e: SeqEvent, model: SequenceModel): string {
  const at = e.timeMs === null ? 'time unknown' : `+${fmtMs(e.timeMs)} ms`
  const op = model.operations.find((o) => o.id === e.operationId)
  switch (e.kind) {
    case 'call':
      return `${at} · ${e.participant} → ${e.target} · call ${e.label}${op?.serverName && op.serverName !== e.label ? ` (server span ${op.serverName})` : ''}`
    case 'message':
      return `${at} · ${e.participant} → ${e.target} · message ${e.label}${op?.serverName && op.serverName !== e.label ? ` (consumer span ${op.serverName})` : ''}`
    case 'start':
      return `${at} · ${e.participant} · started ${e.label}`
    case 'end':
      return `${at} · ${e.participant} · ended ${e.label}${e.durationMs !== undefined ? ` · ${fmtMs(e.durationMs)} ms` : ''}`
    case 'log':
      return `${at} · ${e.participant} · ${e.severity} log: ${e.label}${e.note ? ` (${e.note})` : ''}`
  }
  return at
}

export function fmtMs (v: number): string {
  return Math.abs(v) >= 10 ? v.toFixed(0) : v.toFixed(1)
}

// pairEndpoints are the selectable endpoints of the operation holding key: a
// call's CLIENT and SERVER, or a message's PRODUCER and CONSUMER (client and
// server hold its first and second span; endpointLabels names their kinds).
export function pairEndpoints (model: SequenceModel, key: string | null): { client: string, server: string } | null {
  if (!key) {
    return null
  }
  const op = model.operations.find((o) => o.spanKeys.includes(key))
  return op && (op.kind === 'call' || op.kind === 'message') ? { client: op.spanKeys[0], server: op.spanKeys[1] } : null
}

export function endpointLabels (model: SequenceModel, key: string | null): [string, string] {
  const op = key ? model.operations.find((o) => o.spanKeys.includes(key)) : undefined
  return op?.kind === 'message' ? ['PRODUCER', 'CONSUMER'] : ['CLIENT', 'SERVER']
}

// logsWithAssociation is every log of the report with its association, for
// the export's event list.
export function sequenceExport (model: SequenceModel, report: Report, view: { view: string, selected: string | null, showInternal: boolean, logScope: string }) {
  return {
    renderer: model.renderer,
    view,
    participants: model.participants,
    unobservedParticipants: model.unobserved,
    operations: model.operations,
    events: model.events.map((e) => ({ ...e, text: eventText(e, model) })),
    overlaps: model.overlaps,
    links: model.links,
    limitations: model.limitations,
    logAssociations: report.logs.records.map((l: LogRecord) => ({ recordId: l.recordId, association: association(l, report).label }))
  }
}

// exportSequenceHtml is the report's existing HTML export (every span and log,
// escaped, no scripts) followed by the sequence as an ordered, labeled event
// list with the view that was on screen.
export function exportSequenceHtml (report: Report, model: SequenceModel, view: { view: string, selected: string | null, showInternal: boolean, logScope: string }, exportedAt: Date): string {
  const e = escapeHtml
  const items = model.events.map((ev) => `<li>${e(eventText(ev, model))}</li>`).join('')
  const ops = model.operations.map((o) => `<tr><td>${e(o.name)}</td><td>${e(o.kind)}</td><td>${e(o.source)}</td><td>${e(o.target)}${o.targetObserved ? '' : ' (not observed)'}</td><td>${e(o.spanKeys.join(', '))}</td><td>${e(o.status)}${o.serverStatus ? ' / server ' + e(o.serverStatus) : ''}</td></tr>`).join('')
  const section = `<h2>Sequence (${e(model.renderer)})</h2>
<p>A presentation of the recorded spans and logs above, not a recovered execution. Arrows are correlated CLIENT/SERVER spans (calls) or a PRODUCER and its one CONSUMER (messages, one-way); end markers are span completions, not responses; order within equal times is by id, not causality.
View on screen: ${e(view.view)}, selected span ${e(view.selected ?? 'none')}, internal operations ${view.showInternal ? 'shown' : 'hidden'}, log scope ${e(view.logScope)}. Hidden operations are still listed here.</p>
${model.limitations.length ? '<ul>' + model.limitations.map((l) => `<li>${e(l)}</li>`).join('') + '</ul>' : ''}
<p>Participants: ${e(model.participants.join(', '))}</p>
<table><tr><th>operation</th><th>kind</th><th>from</th><th>to</th><th>spans</th><th>status</th></tr>${ops}</table>
<ol>${items}</ol>`
  return htmlPage('Trace report ' + report.traceId, reportHtmlBody(report, exportedAt) + section)
}
