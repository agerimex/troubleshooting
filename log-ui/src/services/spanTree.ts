import { format } from 'date-fns'
import { type Span } from '@/types/filter'
import { type LogRecord } from '@/types/report'

// ParentSpanId of root spans.
export const ROOT_PARENT = '0000000000000000'

// INVALID_DURATION is shown for a span without a valid recorded duration
// (stored as a negative value): it is never shown as 0.
export const INVALID_DURATION = 'invalid'

// durationMsText is a duration in microseconds as milliseconds with three
// decimals, or INVALID_DURATION.
export function durationMsText (micros: number): string {
  return micros < 0 ? INVALID_DURATION : (micros / 1000).toFixed(3)
}

// --- time: the API uses Unix nanoseconds as strings, beyond JS number precision

export function toNs (date: Date): string {
  return (BigInt(date.getTime()) * BigInt(1000000)).toString()
}

// toNsExclusiveEnd is the first nanosecond after the millisecond: windows are
// [from, to), so an end taken from a Date still includes everything shown with
// that time.
export function toNsExclusiveEnd (date: Date): string {
  return ((BigInt(date.getTime()) + BigInt(1)) * BigInt(1000000)).toString()
}

export function addMs (ns: string, ms: number): string {
  return (BigInt(ns) + BigInt(ms) * BigInt(1000000)).toString()
}

export function nsToDate (ns: string): Date {
  return new Date(Number(BigInt(ns) / BigInt(1000000)))
}

// formatNs shows a time with microseconds, e.g. 10.10.26 14:03:07.123456.
export function formatNs (ns: string): string {
  const micros = (BigInt(ns) / BigInt(1000)) % BigInt(1000000)
  return format(nsToDate(ns), 'dd.MM.yy HH:mm:ss') + '.' + micros.toString().padStart(6, '0')
}

// --- tree nodes

export interface SpanNodeData extends Omit<Span, 'duration'> {
  kind?: undefined
  humanTime: string
  duration: number | string // milliseconds, or "invalid"
  durationMicros: number
  // Why the span is shown at the top of a trace although it has a parent.
  parentMissing?: string
}

// A row that is not a span: a truncation notice or a load error under the
// node it belongs to, so it stays next to the affected result, or a group
// heading (Log Analysis: trace-level and unmatched logs).
export interface MessageNodeData {
  kind: 'notice' | 'error' | 'group'
  text: string
  parentKey: string
}

// A log row in Log Analysis: a point event under the span that recorded it
// (no duration); msg and service let the shared columns show it.
export interface LogNodeData {
  kind: 'log'
  log: LogRecord
  humanTime: string
  association: string
  msg: string
  service: string
  traceId: string
  spanId: string
  duration: string
}

export interface TreeNode {
  key: string
  data: SpanNodeData | MessageNodeData | LogNodeData
  leaf?: boolean
  children?: TreeNode[]
}

// spanKey is the full identity of a span: span ids are unique only within a trace.
export function spanKey (traceId: string, spanId: string): string {
  return traceId + ':' + spanId
}

export function createItem (row: Span): TreeNode {
  return {
    key: spanKey(row.traceId, row.spanId),
    data: {
      ...row,
      events: row.events ?? [],
      links: row.links ?? [],
      humanTime: formatNs(row.timeStamp),
      duration: row.duration < 0 ? INVALID_DURATION : Math.round(row.duration / 1000),
      durationMicros: row.duration
    },
    leaf: row.childSpanCount == 0
  }
}

export function messageNode (parentKey: string, kind: 'notice' | 'error', text: string): TreeNode {
  return { key: parentKey + ':' + kind, data: { kind, text, parentKey }, leaf: true }
}

export const PARENT_NOT_IN_WINDOW = 'Parent not found within the searched window'
export const PARENT_NOT_LOADED = 'Parent not among the loaded spans'
export const PARENT_LOOP = 'Parent references form a loop'

// buildTraceTree arranges the spans of one trace by their parent ids. Spans
// whose parent is not in the list are shown at the top with the reason: when
// the list was truncated the parent may simply not have been loaded.
export function buildTraceTree (spans: Span[], truncated: boolean): TreeNode[] {
  const byId = new Map<string, Span>()
  for (const span of spans) {
    byId.set(span.spanId, span)
  }
  const childrenOf = new Map<string, Span[]>()
  const tops: Span[] = []
  for (const span of spans) {
    if (span.parentSpanId === ROOT_PARENT || !byId.has(span.parentSpanId)) {
      tops.push(span)
    } else {
      const siblings = childrenOf.get(span.parentSpanId) ?? []
      siblings.push(span)
      childrenOf.set(span.parentSpanId, siblings)
    }
  }

  const visited = new Set<string>()
  function build (span: Span): TreeNode {
    visited.add(span.spanId)
    const node = createItem(span)
    const children = (childrenOf.get(span.spanId) ?? []).filter((c) => !visited.has(c.spanId))
    node.children = children.map(build)
    node.leaf = node.children.length === 0
    return node
  }

  const roots: TreeNode[] = []
  for (const span of tops) {
    const node = build(span)
    if (span.parentSpanId !== ROOT_PARENT) {
      (node.data as SpanNodeData).parentMissing = truncated ? PARENT_NOT_LOADED : PARENT_NOT_IN_WINDOW
    }
    roots.push(node)
  }
  // Spans reachable from no top span have parents that point at each other.
  for (const span of spans) {
    if (!visited.has(span.spanId)) {
      const node = build(span)
      ;(node.data as SpanNodeData).parentMissing = PARENT_LOOP
      roots.push(node)
    }
  }
  return roots
}

// expandAll returns expandedKeys for every node that has children.
export function expandAll (tree: TreeNode[], keys: Record<string, boolean> = {}): Record<string, boolean> {
  for (const node of tree) {
    if (node.children && node.children.length > 0) {
      keys[node.key] = true
      expandAll(node.children, keys)
    }
  }
  return keys
}

// replaceNode returns a copy of the tree with the node `key` replaced at any
// depth. Only replacing top-level nodes left expanded children without
// their loaded grandchildren.
export function replaceNode (tree: TreeNode[], key: string, replacement: TreeNode): TreeNode[] {
  return tree.map((n) => {
    if (n.key === key) {
      return replacement
    }
    if (n.children) {
      return { ...n, children: replaceNode(n.children, key, replacement) }
    }
    return n
  })
}

export function findNode (tree: TreeNode[], key: string): TreeNode | null {
  for (const n of tree) {
    if (n.key === key) {
      return n
    }
    const found = n.children ? findNode(n.children, key) : null
    if (found) {
      return found
    }
  }
  return null
}
