// Sequence library (shapes-v1): execution shapes of a window. A shape is an
// entry operation and the set of distinct caller -> callee calls of its
// executions (repeats and order don't split shapes).

export interface ShapeFilter {
  timeFrom: string // Unix ns, half-open window
  timeTo: string
  service?: string // any service of the shape (substring)
  operation?: string // entry operation (substring)
  errorsOnly?: boolean
}

export type ShapePercentile = 'p50' | 'p95' | 'p99'
export type ShapeExampleKind = ShapePercentile | 'slowest'

export interface ShapeExample {
  traceId: string
  time: string // root start, Unix ns
  durationUs: number
}

export interface Shape {
  shapeId: string
  entryService: string
  entryOperation: string
  services: string[]
  distinctCalls: number
  executions: number
  failedExecutions: number
  invalidDurations: number
  // Root duration percentiles in ms; null with too few valid durations.
  percentiles: Record<ShapePercentile, number | null>
  examples: Partial<Record<ShapeExampleKind, ShapeExample | null>>
}

export interface ShapeNotice {
  code: string
  message: string
}

export interface ShapeLibrary {
  status: string // ok, partial, or an error status
  error?: string
  shapes: Shape[]
  truncated: boolean
  notices: ShapeNotice[]
  contract: string
  generatedAt: string
}
