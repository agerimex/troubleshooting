// Body of view-spans and count-spans. Times are Unix nanoseconds as strings
// (they don't fit in a JS number). The window [timeFrom, timeTo) is half-open
// and required; a span is identified by (traceId, spanId), so parentId always
// comes with its traceId.
export interface SpanFilter {
  timeFrom: string
  timeTo: string
  scope?: 'root' | 'all'
  traceId?: string
  parentId?: string
  rowsPerPage?: number
  status?: string
  serviceName?: string
  methodName?: string
  // Cursor: the last row of the previous page.
  afterTime?: string
  afterTraceId?: string
  afterSpanId?: string
}

export interface SpanEvent {
  timeUnixNano: string
  name: string
  attributes: Record<string, string>
}

export interface SpanLink {
  traceId: string
  spanId: string
  traceState: string
  attributes: Record<string, string>
}

export interface Span {
  timeStamp: string
  name: string
  // Recorded span kind (server, client, producer, consumer, internal); absent
  // from older APIs.
  spanKind?: string
  service: string
  traceId: string
  spanId: string
  parentSpanId: string
  msg: string
  tags: Record<string, string>[]
  serviceTags: Record<string, string>[]
  childSpanCount: number
  statusCode: number
  status: string
  statusMessage: string
  duration: number // microseconds
  events: SpanEvent[]
  links: SpanLink[]
}

// One page of spans; hasMore says whether more rows exist after it.
export interface SpanPage {
  spans: Span[]
  hasMore: boolean
}
