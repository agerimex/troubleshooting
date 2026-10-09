export interface SpanFilter {
    parentId?: string, 
    rowsPerPage?: number, 
    timeFrom?: string,
    afterSpanId?: string,
    status?: string,
    serviceName?: string,
    methodName?: string
  }
