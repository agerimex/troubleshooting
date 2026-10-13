import { type Span } from '@/types/filter'

// Body of search-logs. The window [timeFrom, timeTo) is half-open and
// required (Unix nanoseconds as strings). service and message are
// case-insensitive substrings; severity is exact (e.g. ERROR).
export interface LogFilter {
  timeFrom: string
  timeTo: string
  traceId?: string
  serviceName?: string
  severity?: string
  message?: string
  rowsPerPage?: number
  // Cursor: the last row of the previous page.
  afterTime?: string
  afterRecordId?: string
}

// One logical log record. traceId and spanId are exactly what was recorded;
// an empty string means "not recorded" and is never filled in by the UI.
export interface LogRecord {
  timeUnixNano: string
  recordId: string
  traceId: string
  spanId: string
  severity: string
  service: string
  message: string
  attributes: Record<string, string>
}

export interface LogPage {
  logs: LogRecord[]
  hasMore: boolean
}

export type SectionStatus = 'ok' | 'unavailable' | 'limit' | 'error'

// One source's evidence in a report. shown is the number of records
// returned; truncated says more matched. total is null when counting failed.
export interface ReportSection<T> {
  status: SectionStatus
  error?: string
  records: T[]
  limit: number
  shown: number
  truncated: boolean
  total: number | null
  totalStatus: 'ok' | 'unavailable'
  totalError?: string
  // When this section was retrieved (Unix ns); a retried section is newer
  // than the report.
  retrievedAt: string
}

export interface ReportWarning {
  code: string
  section: 'spans' | 'logs'
  message: string
}

// Report is what the API returns for one trace within one window, plus the
// time each section was retrieved. Both sections are always present in a
// generated report.
export interface Report {
  traceId: string
  timeFrom: string
  timeTo: string
  generatedAt: string
  spans: ReportSection<Span>
  logs: ReportSection<LogRecord>
  warnings: ReportWarning[]
}

export interface ReportRequest {
  traceId: string
  timeFrom: string
  timeTo: string
  spanLimit?: number
  logLimit?: number
  sections?: ('spans' | 'logs')[]
}

// The API's answer to a report request: only the requested sections.
export interface ReportResponse {
  traceId: string
  timeFrom: string
  timeTo: string
  generatedAt: string
  spans?: Omit<ReportSection<Span>, 'retrievedAt'>
  logs?: Omit<ReportSection<LogRecord>, 'retrievedAt'>
  warnings: ReportWarning[]
}
