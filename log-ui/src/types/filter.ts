export interface SpanFilter {
    parentId?: string, 
    rowsPerPage?: number, 
    timeFrom?: string, 
    status?: string,
    serviceName?: string,
    methodName?: string
  }
