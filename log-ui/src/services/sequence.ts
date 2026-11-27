import { ROOT_PARENT } from '@/services/spanTree'
import { association, escapeHtml, htmlPage, reportHtmlBody, serviceVersion, sortedLogs } from '@/services/report'
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

// visibleEvents applies the presentation filters: internal operations, and
// hidden participants (an operation that starts or ends on a hidden
// participant, such as every call to a database, is left out with its marks,
// and so are the hidden participants' log markers). The selected operation is
// never filtered out.
export function visibleEvents (model: SequenceModel, showInternal: boolean, selectedKey: string | null, hidden: ReadonlySet<string> = new Set()): SeqEvent[] {
  const out = new Set<string>() // operations left out
  const outByParticipant = new Set<string>() // ... because they touch a hidden participant
  for (const o of model.operations) {
    if (selectedKey && o.spanKeys.includes(selectedKey)) continue
    const touchesHidden = hidden.has(o.source) || hidden.has(o.target)
    if (touchesHidden) outByParticipant.add(o.id)
    if (touchesHidden || (!showInternal && o.kind === 'local')) out.add(o.id)
  }
  return model.events.filter((e) => {
    if (selectedKey && (e.spanKey === selectedKey || e.operationId === selectedKey)) return true
    // Log markers stay with hidden internal operations (as before), and leave
    // with operations of a hidden participant (the logs of those calls).
    if (e.kind === 'log') return !hidden.has(e.participant) && !outByParticipant.has(e.operationId)
    return !out.has(e.operationId)
  })
}

// hiddenOperations counts the operations a set of hidden participants leaves
// out (for the note under the participant switches).
export function hiddenOperations (model: SequenceModel, hidden: ReadonlySet<string>): number {
  return model.operations.filter((o) => hidden.has(o.source) || hidden.has(o.target)).length
}

// visibleOverlaps keeps the overlaps whose two spans both belong to an
// operation the view shows: an overlap of hidden calls is not named.
export function visibleOverlaps (model: SequenceModel, events: SeqEvent[]): SeqOverlap[] {
  const ops = new Set(events.map((e) => e.operationId))
  const keys = new Set(model.operations.filter((o) => ops.has(o.id)).flatMap((o) => o.spanKeys))
  return model.overlaps.filter((o) => keys.has(o.a) && keys.has(o.b))
}

const PROMPT_MAX_SPANS = 300
const PROMPT_MAX_ATTRIBUTES = 30
const PROMPT_MAX_VALUE = 300

function promptValue (v: string): string {
  const one = v.replace(/\s+/g, ' ').trim()
  return one.length > PROMPT_MAX_VALUE ? one.slice(0, PROMPT_MAX_VALUE) + '…' : one
}

// Attributes that locate code come first (the list is capped and the
// receiver stores maps in random order); client network details are noise for
// reading code and are left out.
const PROMPT_FIRST = ['code.', 'http.route', 'http.method', 'http.request.method', 'url.path', 'http.target', 'rpc.', 'messaging.', 'db.system', 'db.operation', 'db.statement', 'db.query.text', 'peer.service', 'server.address', 'exception.']
const PROMPT_SKIP = ['http.user_agent', 'user_agent.original', 'http.client_ip', 'client.address', 'net.peer.ip', 'net.peer.port', 'net.sock.peer.addr', 'net.sock.peer.port', 'network.peer.address', 'network.peer.port', 'client.port', 'net.transport', 'http.flavor', 'network.protocol.version', 'http.scheme']

function promptRank (key: string): number {
  const i = PROMPT_FIRST.findIndex((p) => (p.endsWith('.') ? key.startsWith(p) : key === p))
  return i < 0 ? PROMPT_FIRST.length : i
}

function promptAttributes (list: Record<string, string>[] | undefined): string {
  const all = (list ?? []).filter((t) => t.key && t.value !== '' && !PROMPT_SKIP.includes(t.key))
    .sort((a, b) => promptRank(a.key) - promptRank(b.key) || a.key.localeCompare(b.key))
  const shown = all.slice(0, PROMPT_MAX_ATTRIBUTES).map((t) => `${t.key}=${promptValue(t.value)}`)
  return shown.join('; ') + (all.length > shown.length ? `; … ${all.length - shown.length} more` : '')
}

// promptSpanTree lists the spans by recorded parentage, in start order, each
// with its timing, kind, status and attributes (what an agent searches the
// code for: span names, db.statement, http.route, code.* …). Spans that keep
// rejects are left out; their children move up a level.
function promptSpanTree (spans: Span[], keep: (s: Span) => boolean): string[] {
  const origin = traceStart(spans)
  const ids = new Set(spans.map((x) => x.spanId))
  const children = new Map<string, Span[]>()
  const tops: Span[] = []
  const byStart = (a: Span, b: Span) => { const x = BigInt(a.timeStamp); const y = BigInt(b.timeStamp); return x < y ? -1 : x > y ? 1 : 0 }
  for (const x of spans) {
    if (!x.parentSpanId || x.parentSpanId === ROOT_PARENT || !ids.has(x.parentSpanId)) tops.push(x)
    else children.set(x.parentSpanId, [...(children.get(x.parentSpanId) ?? []), x])
  }
  const out: string[] = []
  let listed = 0
  let left = 0
  const seen = new Set<string>()
  const walk = (x: Span, depth: number) => {
    if (seen.has(x.spanId)) return
    seen.add(x.spanId)
    let next = depth
    if (keep(x)) {
      next = depth + 1
      if (listed >= PROMPT_MAX_SPANS) {
        left++
      } else {
        listed++
        const at = origin !== null ? `+${fmtMs(offsetMs(x.timeStamp, origin))} ms` : 'time unknown'
        const took = x.duration < 0 ? 'no valid duration' : `${fmtMs(x.duration / 1000)} ms`
        const pad = '  '.repeat(depth)
        const status = (x.status || 'unset').toUpperCase() + (x.statusMessage ? `: ${promptValue(x.statusMessage)}` : '')
        out.push(`${pad}- ${at}, ${took} · ${(x.spanKind || 'kind unknown').toUpperCase()} · ${x.service} · "${x.name}" · ${status}`)
        const attrs = promptAttributes(x.tags)
        if (attrs) out.push(`${pad}  attributes: ${attrs}`)
        if (x.parentSpanId && x.parentSpanId !== ROOT_PARENT && !ids.has(x.parentSpanId)) out.push(`${pad}  (its parent span is not in the report)`)
      }
    }
    for (const c of (children.get(x.spanId) ?? []).sort(byStart)) walk(c, next)
  }
  for (const t of tops.sort(byStart)) walk(t, 0)
  if (left) out.push(`- … ${left} more spans not listed`)
  return out
}

// agentPrompt is a prompt for an AI coding agent that works in the source
// code of the services: explain this execution in terms of the code and why
// it is designed that way. It carries what the view shows (after its
// filters) as the span tree with attributes, the services' versions and
// resource attributes, concurrent calls, the failed spans with their status
// messages and the trace's logs. It holds trace ids, service and span names,
// attributes and log messages: what the report holds.
export function agentPrompt (model: SequenceModel, report: Report, view: { events: SeqEvent[], hidden: string[], showInternal: boolean, selectedKey: string | null }): string {
  const spans = report.spans.records
  const root = spans.find((x) => !x.parentSpanId || x.parentSpanId === ROOT_PARENT)
  const selected = view.selectedKey ? spans.find((x) => spanKey(x) === view.selectedKey) : undefined
  const lines: string[] = []
  lines.push('You are working in the source code of the services that produced one execution (one trace), recorded by Troubleshooting from OpenTelemetry spans and logs. The trace below is what the code actually did at run time.')
  lines.push('')
  lines.push('Task: explain this execution in terms of the code and its design.')
  lines.push('1. Find the code behind each recorded span: start at the entry operation, then follow the code path that makes each recorded call, in the recorded order. Search for span names, routes, statements and code.* attributes; name each place as file:line.')
  lines.push('2. Explain what the code does at each step and why it is designed that way: what each call is for, why calls run one after another or concurrently (compare with the concurrent calls below), and what the design trades off. Take the reasons from the code, its comments, the docs and the git history; when a reason is not written down, say so instead of guessing.')
  lines.push('3. Say where the time goes and whether the code explains it, and where the code and the trace disagree: a call the code makes that is not recorded, or a recorded call you cannot find. Name possible reasons (another version, a branch not taken, missing instrumentation) without picking one the evidence does not support.')
  lines.push('Do not change any code; this is an explanation. Use only the evidence below and the repository; do not invent calls, timings, errors or causes. Times are milliseconds from the trace start; durations are recorded span durations.')
  if (selected) {
    lines.push(`Focus on the selected operation "${selected.name}" of ${selected.service} and how it relates to the rest.`)
  }
  lines.push('')
  lines.push('## Execution')
  lines.push(`- ${root ? `Entry "${root.name}" of ${root.service}` : 'Entry span not in the report'}, ${fmtMs(model.totalMs)} ms from first start to last end, ${model.spanCount} spans, ${report.logs.records.length} logs.`)
  const partial = report.spans.status !== 'ok' || report.logs.status !== 'ok' || report.spans.truncated || report.logs.truncated
  lines.push(`- Report: ${partial ? 'partial (see the limitations)' : 'complete within its window'}.`)
  for (const l of model.limitations) lines.push(`- Limitation: ${l}`)
  const observed = model.participants.filter((p) => !model.unobserved.includes(p))
  lines.push(`- Services that recorded spans: ${observed.join(', ') || 'none'}.`)
  if (model.unobserved.length) lines.push(`- Peers seen only from their callers (no spans of their own): ${model.unobserved.join(', ')}.`)
  if (view.hidden.length) lines.push(`- Left out of this view by the user: ${view.hidden.join(', ')}, and every call to or from them.`)
  if (!view.showInternal) lines.push('- Internal operations are left out of this view.')
  // Only what the view shows: spans of shown operations, their services and
  // logs (a log of a hidden span is left out; one without a span stays).
  const ops = new Set(view.events.map((e) => e.operationId))
  const shown = new Set(model.operations.filter((o) => ops.has(o.id)).flatMap((o) => o.spanKeys))
  const shownSpans = spans.filter((x) => shown.has(spanKey(x)))
  lines.push('')
  lines.push('## Services')
  lines.push('Check that the code you read is the recorded version.')
  for (const svc of [...new Set(shownSpans.map((x) => x.service))]) {
    const first = shownSpans.find((x) => x.service === svc)!
    const version = serviceVersion(first)
    const res = promptAttributes((first.serviceTags ?? []).filter((t) => t.key !== 'service.name' && t.key !== 'service.version'))
    lines.push(`- ${svc}: ${version ? 'version ' + version : 'version not recorded'}${res ? `; resource: ${res}` : ''}`)
  }
  lines.push('')
  lines.push('## Spans')
  lines.push('By recorded parentage (indented under the span that started them), in start order: start, duration, kind, service, name, status, then the span attributes.')
  lines.push(...promptSpanTree(spans, (x) => shown.has(spanKey(x))))
  const concurrent = visibleOverlaps(model, view.events)
  if (concurrent.length) {
    lines.push('')
    lines.push('## Concurrent calls')
    lines.push('Sibling spans whose recorded intervals overlap (both in flight; not a dependency).')
    for (const o of concurrent.slice(0, 20)) lines.push(`- "${o.aName}" and "${o.bName}" overlap for ${fmtMs(o.overlapMs)} ms, from +${fmtMs(o.fromMs)} ms`)
    if (concurrent.length > 20) lines.push(`- … ${concurrent.length - 20} more overlaps not listed`)
  }
  const failed = shownSpans.filter((x) => x.status === 'error')
  if (failed.length) {
    lines.push('')
    lines.push('## Failed spans')
    for (const x of failed) lines.push(`- "${x.name}" of ${x.service}${x.statusMessage ? `: ${promptValue(x.statusMessage)}` : ' (no status message recorded)'}`)
  }
  const spanOf = (l: LogRecord) => (l.spanId ? spans.find((x) => x.spanId === l.spanId && x.traceId === l.traceId) : undefined)
  const logs = sortedLogs(report.logs.records).filter((l) => { const s = spanOf(l); return !s || shown.has(spanKey(s)) })
  if (logs.length) {
    lines.push('')
    lines.push('## Logs')
    const origin = traceStart(spans)
    for (const l of logs.slice(0, 200)) {
      const at = origin !== null ? `+${fmtMs(offsetMs(l.timeUnixNano, origin))} ms ` : ''
      const span = spanOf(l)
      lines.push(`- ${at}${l.severity || 'no level'}${l.service ? ' ' + l.service : ''}${span ? ` in "${span.name}"` : ''}: ${l.message}`)
    }
    if (logs.length > 200) lines.push(`- … ${logs.length - 200} more logs not listed`)
  }
  return lines.join('\n')
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
