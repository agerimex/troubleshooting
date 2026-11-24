import { type Span } from '@/types/filter'

// What makes two recorded calls "the same call", beyond service, kind and
// name. Database instrumentation such as otelsql names every span
// "sql:query", so a query is told apart by its statement (db.statement, or
// db.query.text in newer semantic conventions). The statement is normalized
// so that calls of one query with different values are one call: literals
// and placeholders become ?, whitespace collapses, and lists of values
// (IN (…), VALUES rows) of any length become one (?). The quality gate's
// evidence groups statements by whitespace only; values are removed here
// too because a query with inline ids would otherwise be a new call per id.

const STATEMENT_ATTRIBUTES = ['db.statement', 'db.query.text']

// TARGET_ATTRIBUTES name what a call talks to (as in the Sequence view).
const TARGET_ATTRIBUTES = ['db.system', 'peer.service', 'server.address', 'net.peer.name', 'messaging.system', 'rpc.service']

export function attrOf (s: Span, key: string): string {
  return (s.tags ?? []).find((t) => t.key === key)?.value ?? ''
}

export function normalizeStatement (text: string): string {
  return text
    .replace(/'(?:[^']|'')*'/g, '?') // string literals
    .replace(/\$\d+|(?<![:\w]):[A-Za-z_]\w*|(?<!@)@[A-Za-z_]\w*/g, '?') // $1, :name, @name
    .replace(/(?<![\w.$?])-?\d+(?:\.\d+)?(?![\w.])/g, '?') // numbers that are not part of a name
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/\(\s*\?(?:\s*,\s*\?)*\s*\)/g, '(?)') // IN (?, ?, ?) and one VALUES row
    .replace(/\(\?\)(?:\s*,\s*\(\?\))+/g, '(?)') // several VALUES rows
}

// statementOf is the span's normalized statement, '' when none is recorded.
export function statementOf (s: Span): string {
  for (const key of STATEMENT_ATTRIBUTES) {
    const v = attrOf(s, key)
    if (v.trim()) {
      return normalizeStatement(v)
    }
  }
  return ''
}

// tablesOf lists the tables a statement names after FROM, JOIN, INTO or
// UPDATE, lower case, without quotes, sorted. A subquery (FROM (…)) names
// none itself; its own FROM does.
export function tablesOf (statement: string): string[] {
  const out = new Set<string>()
  const re = /\b(?:from|join|into|update)\s+("[^"]+"|`[^`]+`|\[[^\]]+\]|[A-Za-z_][\w$]*(?:\.[A-Za-z_][\w$]*)*)/gi
  for (const m of statement.matchAll(re)) {
    out.add(m[1].replace(/["`[\]]/g, '').toLowerCase())
  }
  return [...out].sort()
}

// targetOf is what the call talks to, '' when not recorded.
export function targetOf (s: Span): string {
  for (const key of TARGET_ATTRIBUTES) {
    const v = attrOf(s, key)
    if (v) {
      return v
    }
  }
  return ''
}

// callIdentity: service, kind and name, and the statement when there is one.
export function callIdentity (s: Span): string {
  const statement = statementOf(s)
  return [s.service, s.spanKind ?? '', s.name, ...(statement ? [statement] : [])].join('|')
}

// shortStatement cuts a statement for a label.
export function shortStatement (statement: string, max = 60): string {
  return statement.length <= max ? statement : statement.slice(0, max - 1) + '…'
}
