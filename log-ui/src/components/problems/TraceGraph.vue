<template>
  <div data-testid="trace-graph">
    <div class="graph-scroll">
      <div class="graph" :style="{ width: width + 'px', height: height + 'px' }">
        <svg class="edges" :width="width" :height="height" aria-hidden="true">
          <defs>
            <marker id="pw-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
              <path d="M 0 0 L 10 5 L 0 10 z" class="arrow"/>
            </marker>
          </defs>
          <path v-for="e in layout.edges" :key="e.from + '>' + e.to" :d="edgePath(e)" class="edge" marker-end="url(#pw-arrow)" data-testid="graph-edge"/>
        </svg>
        <button v-for="node in layout.nodes" :key="node.key" type="button" class="node"
          :class="{ selected: node.key === selected, detached: node.detached }"
          :style="{ left: x(node) + 'px', top: y(node) + 'px', width: NODE_W + 'px', height: NODE_H + 'px' }"
          :aria-pressed="node.key === selected" :data-span-key="node.key" data-testid="graph-node"
          @click="$emit('select', node.key)">
          <span class="svc">{{ node.span.service }}</span>
          <span class="name" data-testid="graph-node-name">{{ node.span.name }}</span>
          <span class="row">
            <span class="badge" :class="statusClass(node.span)" data-testid="graph-node-status">{{ statusText(node.span) }}</span>
            <span class="dur">{{ durationText(node.span) }}</span>
          </span>
          <span class="logs" data-testid="graph-node-logs">
            {{ countsOf(node).logs }} {{ countsOf(node).logs === 1 ? 'log' : 'logs' }}<template v-if="countsOf(node).errors > 0"> · <span class="err" data-testid="graph-node-error-logs">{{ countsOf(node).errors }} ERROR</span></template>
            <template v-if="node.detached"> · <span class="warn" title="Its recorded parent is not in this report">parent missing</span></template>
          </span>
        </button>
      </div>
    </div>
    <div class="text-xs opacity-80 mt-2 flex flex-wrap gap-4">
      <span>Arrows: recorded parent → child</span>
      <span>Overlap: recorded start and end times</span>
      <span v-if="layout.hidden > 0" data-testid="graph-hidden">
        Showing {{ layout.nodes.length }} of {{ spans.length }} spans in the graph.
        <a href="#" @click.prevent="$emit('show-all')" data-testid="graph-show-all">Show all</a>
        (the timeline, log list and exports always have every span)
      </span>
    </div>
    <ul v-if="overlaps.length" class="overlaps text-sm mt-2" data-testid="overlaps">
      <li v-for="(o, i) in overlaps.slice(0, 5)" :key="i" data-testid="overlap">
        <b>{{ fmt(o.overlapMs) }} ms overlap</b> · {{ o.a.name }} + {{ o.b.name }} (siblings) · +{{ fmt(o.fromMs) }}–{{ fmt(o.toMs) }} ms
      </li>
    </ul>
  </div>
</template>

<script lang="ts">
import { computed, defineComponent, type PropType } from 'vue'
import { graphLayout, siblingOverlaps, type GraphEdge, type GraphNode, type SpanLogCounts } from '@/services/problems'
import { durationMsText } from '@/services/spanTree'
import { type Span } from '@/types/filter'

const NODE_W = 200
const NODE_H = 104
const GAP_X = 20
const GAP_Y = 46
const PAD = 12

// TraceGraph draws one recorded span per node and recorded parent → child
// edges only. Rendering is bounded by renderLimit; hidden spans stay in the
// report (timeline, log list, exports).
export default defineComponent({
  name: 'TraceGraph',
  props: {
    spans: { type: Array as PropType<Span[]>, required: true },
    selected: { type: String as PropType<string | null>, default: null },
    counts: { type: Object as PropType<Map<string, SpanLogCounts>>, required: true },
    renderLimit: { type: Number, default: 60 }
  },
  emits: ['select', 'show-all'],
  setup (props) {
    const layout = computed(() => graphLayout(props.spans, props.renderLimit))
    const overlaps = computed(() => siblingOverlaps(props.spans))
    const x = (n: GraphNode) => PAD + n.column * (NODE_W + GAP_X)
    const y = (n: GraphNode) => PAD + n.depth * (NODE_H + GAP_Y)
    const width = computed(() => PAD * 2 + layout.value.columns * (NODE_W + GAP_X) - GAP_X)
    const height = computed(() => PAD * 2 + layout.value.depth * (NODE_H + GAP_Y) - GAP_Y)
    const byKey = computed(() => new Map(layout.value.nodes.map((n) => [n.key, n])))
    function edgePath (e: GraphEdge): string {
      const a = byKey.value.get(e.from)!
      const b = byKey.value.get(e.to)!
      const x1 = x(a) + NODE_W / 2
      const y1 = y(a) + NODE_H
      const x2 = x(b) + NODE_W / 2
      const y2 = y(b) - 2
      const mid = (y1 + y2) / 2
      return `M ${x1} ${y1} C ${x1} ${mid}, ${x2} ${mid}, ${x2} ${y2}`
    }
    const countsOf = (n: GraphNode) => props.counts.get(n.key) ?? { logs: 0, errors: 0 }
    const statusText = (s: Span) => (s.status || 'unset').toUpperCase()
    const statusClass = (s: Span) => s.status === 'error' ? 'bad' : s.status === 'ok' ? 'good' : 'neutral'
    const durationText = (s: Span) => {
      const t = durationMsText(s.duration)
      return t === 'invalid' ? 'duration invalid' : Number(t).toFixed(0) + ' ms'
    }
    const fmt = (v: number) => v.toFixed(v >= 10 ? 0 : 1)
    return { layout, overlaps, x, y, width, height, edgePath, countsOf, statusText, statusClass, durationText, fmt, NODE_W, NODE_H }
  }
})
</script>

<style scoped>
.graph-scroll { overflow: auto; max-height: 34rem; border-radius: 8px; background: var(--surface-ground); background-image: radial-gradient(var(--surface-border) 1px, transparent 1px); background-size: 14px 14px; }
.graph { position: relative; min-width: 100%; }
.edges { position: absolute; left: 0; top: 0; pointer-events: none; }
.edge { fill: none; stroke: var(--text-color-secondary); stroke-width: 1.5; }
.arrow { fill: var(--text-color-secondary); }
.node { position: absolute; display: flex; flex-direction: column; align-items: stretch; gap: 3px; text-align: left; padding: 8px 10px;
  background: var(--surface-card); border: 1px solid var(--surface-border); border-radius: 8px; color: var(--text-color); cursor: pointer; font: inherit; }
.node:focus-visible { outline: 2px solid var(--primary-color); outline-offset: 2px; }
.node.selected { border: 2px solid var(--primary-color); padding: 7px 9px; }
.node.detached { border-style: dashed; }
.svc { font-size: 12px; color: var(--text-color-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.name { font-weight: 600; font-size: 14px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row { display: flex; justify-content: space-between; align-items: center; font-size: 13px; }
.logs { font-size: 12px; color: var(--text-color-secondary); }
.err { color: #f87171; }
.warn { color: #fbbf24; }
.badge { font-size: 11px; padding: 1px 6px; border-radius: 4px; font-weight: 600; }
.badge.bad { background: rgba(248, 113, 113, 0.18); color: #fca5a5; }
.badge.good { background: rgba(74, 222, 128, 0.15); color: #86efac; }
.badge.neutral { background: rgba(148, 163, 184, 0.18); color: #cbd5e1; }
.overlaps { list-style: none; padding: 0; }
</style>
