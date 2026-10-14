import { describe, expect, it } from 'vitest'
import {
  ROOT_PARENT, PARENT_LOOP, PARENT_NOT_IN_WINDOW, PARENT_NOT_LOADED, addMs, buildTraceTree, createItem, durationMsText, expandAll,
  findNode, formatNs, replaceNode, toNs, toNsExclusiveEnd, type SpanNodeData, type TreeNode
} from '@/services/spanTree'
import { type Span } from '@/types/filter'

const trace = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'

function span (spanId: string, parentSpanId: string, traceId = trace): Span {
  return {
    timeStamp: '1760000000000000000', name: 'op-' + spanId, service: 'svc', traceId, spanId, parentSpanId,
    msg: '', tags: [], serviceTags: [], childSpanCount: 0, statusCode: 0, status: 'unset', statusMessage: '',
    duration: 1500, events: [], links: []
  }
}

const names = (nodes: TreeNode[] = []) => nodes.map((n) => (n.data as SpanNodeData).name)

describe('nanosecond times', () => {
  it('keeps full precision beyond JS numbers', () => {
    const d = new Date(1760000000123)
    expect(toNs(d)).toBe('1760000000123000000')
    expect(toNsExclusiveEnd(d)).toBe('1760000000124000000')
    expect(addMs('1760000000123000001', -3600000)).toBe('1759996400123000001')
  })

  it('formats microseconds', () => {
    expect(formatNs('1760000000123456789')).toMatch(/\.123456$/)
  })
})

describe('createItem', () => {
  it('keys a node by trace and span id', () => {
    const a = createItem(span('1111111111111111', ROOT_PARENT, 'a'.repeat(32)))
    const b = createItem(span('1111111111111111', ROOT_PARENT, 'b'.repeat(32)))
    expect(a.key).not.toBe(b.key)
  })

  it('shows the expand arrow from the child count', () => {
    expect(createItem({ ...span('1', ROOT_PARENT), childSpanCount: 0 }).leaf).toBe(true)
    expect(createItem({ ...span('1', ROOT_PARENT), childSpanCount: 2 }).leaf).toBe(false)
  })
})

describe('buildTraceTree', () => {
  it('nests four levels', () => {
    const tree = buildTraceTree([span('4', '3'), span('1', ROOT_PARENT), span('3', '2'), span('2', '1')], false)
    expect(names(tree)).toEqual(['op-1'])
    expect(names(tree[0].children)).toEqual(['op-2'])
    expect(names(tree[0].children![0].children)).toEqual(['op-3'])
    expect(names(tree[0].children![0].children![0].children)).toEqual(['op-4'])
    expect(tree[0].children![0].children![0].children![0].leaf).toBe(true)
    expect(Object.keys(expandAll(tree))).toHaveLength(3)
  })

  it('says a missing parent was not found within the window', () => {
    const tree = buildTraceTree([span('2', '1')], false)
    expect((tree[0].data as SpanNodeData).parentMissing).toBe(PARENT_NOT_IN_WINDOW)
  })

  it('says only that the parent was not loaded when the list is truncated', () => {
    const tree = buildTraceTree([span('2', '1')], true)
    expect((tree[0].data as SpanNodeData).parentMissing).toBe(PARENT_NOT_LOADED)
  })

  it('attaches a parent that arrived later', () => {
    const tree = buildTraceTree([span('2', '1'), span('1', ROOT_PARENT)], false)
    expect(names(tree)).toEqual(['op-1'])
    expect((tree[0].data as SpanNodeData).parentMissing).toBeUndefined()
  })

  it('shows every span of a parent loop once', () => {
    const tree = buildTraceTree([span('1', '2'), span('2', '1')], false)
    expect(tree).toHaveLength(1)
    expect((tree[0].data as SpanNodeData).parentMissing).toBe(PARENT_LOOP)
    expect(names(tree[0].children)).toEqual(['op-2'])
  })
})

describe('replaceNode', () => {
  it('replaces a node at any depth', () => {
    const tree = buildTraceTree([span('1', ROOT_PARENT), span('2', '1'), span('3', '2')], false)
    const key = tree[0].children![0].children![0].key
    const replaced = replaceNode(tree, key, { key, data: { kind: 'notice', text: 'x', parentKey: key } })
    expect(findNode(replaced, key)?.data).toMatchObject({ kind: 'notice' })
    expect(findNode(tree, key)?.data).toMatchObject({ name: 'op-3' })
  })
})

describe('durations', () => {
  it('M2-LAT-005: an invalid duration is shown as invalid, never as zero', () => {
    expect(durationMsText(-1)).toBe('invalid')
    expect(durationMsText(0)).toBe('0.000')
    expect(durationMsText(150000)).toBe('150.000')
  })
})
