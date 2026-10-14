// Tag colors. A span status (recorded by tracing) and a log severity
// (recorded by logging) are different facts and are always labeled apart.

export function spanStatusSeverity (status: string) {
  switch (status) {
    case 'error': return 'danger'
    case 'ok': return 'success'
    default: return 'info'
  }
}

export function logSeverity (severity: string) {
  switch (severity) {
    case 'FATAL':
    case 'ERROR': return 'danger'
    case 'WARN': return 'warning'
    case 'INFO': return 'info'
    default: return 'secondary'
  }
}
