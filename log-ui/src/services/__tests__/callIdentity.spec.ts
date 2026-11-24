import { describe, expect, it } from 'vitest'
import { callIdentity, normalizeStatement, statementOf, tablesOf, targetOf } from '@/services/callIdentity'
import { compareExecutions, differences, toneOf } from '@/services/executionCompare'
import { groupRepeats, timeline } from '@/services/problems'
import { type Span } from '@/types/filter'

const ROOT = '0000000000000000'
const MS = 1_000_000
const base = 1_700_000_000_000 * MS

const span = (trace: string, spanId: string, parentSpanId: string, name: string, atMs: number, ms: number, tags: Record<string, string> = {}, kind = 'client'): Span => ({
  timeStamp: String(BigInt(base) + BigInt(Math.round(atMs * MS))), name, spanKind: kind, service: 'rooms-api', traceId: trace.repeat(32), spanId, parentSpanId,
  msg: '', tags: Object.entries(tags).map(([key, value]) => ({ key, value })), serviceTags: [], childSpanCount: 0, statusCode: 0, status: 'ok', statusMessage: '',
  duration: Math.round(ms * 1000), events: [], links: []
})

describe('call identity', () => {
  it('writes values, placeholders and lists of values as ?', () => {
    expect(normalizeStatement("SELECT *\n\t FROM devices WHERE room_id = 42 AND name = 'it''s' AND x > -1.5")).toBe('SELECT * FROM devices WHERE room_id = ? AND name = ? AND x > ?')
    expect(normalizeStatement('select * from t where id = $1 and a = :name and b = @p1 and c = ?')).toBe('select * from t where id = ? and a = ? and b = ? and c = ?')
    expect(normalizeStatement('SELECT a FROM t WHERE id IN (1, 2, 3)')).toBe(normalizeStatement('SELECT a FROM t WHERE id IN (?)'))
    expect(normalizeStatement('INSERT INTO t (a, b) VALUES (?, ?), (?, ?), (?, ?)')).toBe('INSERT INTO t (a, b) VALUES (?)')
    // names with digits and casts stay
    expect(normalizeStatement('SELECT t1.c2 FROM t1 WHERE v::int = 3 LIMIT 20')).toBe('SELECT t1.c2 FROM t1 WHERE v::int = ? LIMIT ?')
  })

  it('names the tables after FROM, JOIN, INTO and UPDATE', () => {
    expect(tablesOf('SELECT * FROM (SELECT r.id FROM rooms r LEFT JOIN "Devices" d ON d.room = r.id) x')).toEqual(['devices', 'rooms'])
    expect(tablesOf('INSERT INTO main.sessions (a) VALUES (?)')).toEqual(['main.sessions'])
    expect(tablesOf('UPDATE `users` SET a = ?')).toEqual(['users'])
    expect(tablesOf('SELECT 1')).toEqual([])
  })

  it('tells database calls apart by statement, other calls by name', () => {
    const a = span('a', '0000000000000001', ROOT, 'sql:query', 0, 1, { 'db.system': 'sqlite', 'db.statement': 'SELECT * FROM rooms WHERE id = 1' })
    const b = span('a', '0000000000000002', ROOT, 'sql:query', 0, 1, { 'db.system': 'sqlite', 'db.statement': 'SELECT * FROM rooms WHERE id = 2' })
    const c = span('a', '0000000000000003', ROOT, 'sql:query', 0, 1, { 'db.system': 'sqlite', 'db.query.text': 'SELECT * FROM devices' })
    expect(callIdentity(a)).toBe(callIdentity(b))
    expect(callIdentity(a)).not.toBe(callIdentity(c))
    expect(statementOf(c)).toBe('SELECT * FROM devices')
    expect(callIdentity(span('a', '0000000000000004', ROOT, 'GET /users', 0, 1))).toBe('rooms-api|client|GET /users')
    expect(targetOf(a)).toBe('sqlite')
  })
})

// A room list: four authentication queries, then the rooms. The old version
// reads rooms and devices in one query; the new one lists the room names and
// then queries each room (an N+1).
const AUTH = ['SELECT id, user_id FROM tokens WHERE token_hash = ?', 'SELECT id, login FROM users WHERE id = ?', 'SELECT g.id FROM groups g JOIN user_groups ug ON ug.group_id = g.id WHERE ug.user_id = ?', 'SELECT key, value FROM user_config WHERE user_id = ?']
const JOIN = 'SELECT r.id, r.name, d.id FROM rooms r LEFT JOIN devices d ON d.room_id = r.id'
const NAMES = 'SELECT DISTINCT name FROM rooms ORDER BY 1'
const ROOM = (id: number) => `SELECT * FROM (SELECT r.id, r.name FROM rooms r WHERE r.id = ${id}) x LEFT JOIN devices d ON d.room_id = x.id`

function roomList (trace: string, perRoom: boolean): Span[] {
  const db = (i: number, at: number, statement: string) => span(trace, (i + 1).toString(16).padStart(16, '0'), 'f000000000000000', 'sql:query', at, 0.4, { 'db.system': 'sqlite', 'db.statement': statement })
  const spans = [span(trace, 'f000000000000000', ROOT, '/rooms-with-devices', 0, perRoom ? 10 : 2.5, {}, 'server')]
  AUTH.forEach((q, i) => spans.push(db(i, 0.1 + i * 0.4, q)))
  if (perRoom) {
    spans.push(db(10, 1.8, NAMES))
    for (let r = 0; r < 20; r++) {
      spans.push(db(20 + r, 2.2 + r * 0.4, ROOM(r + 1)))
    }
  } else {
    spans.push(db(10, 1.8, JOIN))
  }
  return spans
}

describe('a changed query, compared', () => {
  const c = compareExecutions(roomList('a', false), roomList('b', true))
  const row = (statement: string) => c.rows.find((r) => r.statement === statement)!

  it('keeps unchanged queries together and splits the others by statement', () => {
    for (const q of AUTH) {
      expect(row(q).change).toBe('same')
    }
    expect(row(JOIN).change).toBe('missing')
    expect(row(normalizeStatement(NAMES)).change).toBe('new')
    expect(row(normalizeStatement(ROOM(1))).current.count).toBe(20)
  })

  it('infers that the new queries likely replace the old one, and says why', () => {
    expect(c.replacements).toHaveLength(1)
    const rp = c.replacements[0]
    expect(rp.reference.map((r) => r.statement)).toEqual([JOIN])
    expect(rp.current.map((r) => r.statement)).toEqual([normalizeStatement(NAMES), normalizeStatement(ROOM(1))])
    expect(rp.reasons).toEqual(['same place: after sql:query SELECT key, value FROM user_config WHER…', 'same target: sqlite', 'same tables: devices, rooms'])
    expect(toneOf(row(JOIN))).toBe('replaced')
    // the replaced query is listed just before the queries in its place
    const statements = c.rows.map((r) => r.statement)
    expect(statements.indexOf(JOIN) + 1).toBe(statements.indexOf(normalizeStatement(NAMES)))
    expect(toneOf(row(AUTH[0]))).toBe('same')
    const d = differences(c)
    expect(d).toHaveLength(1)
    expect(d[0]).toMatch(/^likely in place of each other: sql:query SELECT r\.id, .*… \(1 call\) → sql:query SELECT DISTINCT name .*… \(1 call\), sql:query SELECT \* FROM \(SELECT r\.id, .*… \(20 calls\); same place: after /)
  })

  it('does not pair calls to another target or to other tables', () => {
    const other = roomList('b', true).map((s) => s.tags.some((t) => t.value === NAMES) ? { ...s, tags: [{ key: 'db.system', value: 'postgresql' }, { key: 'db.statement', value: NAMES }] } : s)
    const rp = compareExecutions(roomList('a', false), other).replacements
    expect(rp).toHaveLength(1)
    expect(rp[0].current.map((r) => r.statement)).toEqual([normalizeStatement(ROOM(1))]) // the postgresql call stays new
    const audit = roomList('b', false).map((s) => s.tags.some((t) => t.value === JOIN) ? { ...s, tags: [{ key: 'db.system', value: 'sqlite' }, { key: 'db.statement', value: 'INSERT INTO audit (a) VALUES (?)' }] } : s)
    expect(compareExecutions(roomList('a', false), audit).replacements).toHaveLength(0) // no common table
  })

  it('leaves a new query of another table out of the pair', () => {
    const settings = span('b', '00000000000000ff', 'f000000000000000', 'sql:query', 1.7, 0.05, { 'db.system': 'sqlite', 'db.statement': 'SELECT value FROM settings WHERE key = ?' })
    const c2 = compareExecutions(roomList('a', false), [...roomList('b', true), settings])
    expect(c2.replacements).toHaveLength(1)
    expect(c2.replacements[0].current.map((r) => r.statement)).toEqual([normalizeStatement(NAMES), normalizeStatement(ROOM(1))])
    expect(c2.rows.find((r) => r.statement.includes('settings'))!.replacement).toBeNull()
  })

  it('folds the per-room queries in the timeline, not the authentication ones', () => {
    const items = groupRepeats(timeline(roomList('b', true)).rows, new Set(), null)
    const groups = items.filter((i) => i.type === 'group')
    expect(groups).toHaveLength(1)
    expect(groups[0].type === 'group' && groups[0].group.rows.length).toBe(20)
    expect(groups[0].type === 'group' && groups[0].group.statement).toBe(normalizeStatement(ROOM(1)))
  })
})
