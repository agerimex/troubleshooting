<template>
  <div class="timeline" data-testid="trace-timeline">
    <div class="axis text-xs opacity-80">
      <span>0 ms</span><span>{{ fmt(tl.totalMs / 2) }} ms</span><span>{{ fmt(tl.totalMs) }} ms</span>
    </div>
    <template v-for="item in items" :key="item.type === 'row' ? item.row.key : item.group.key">
      <button v-if="item.type === 'group'" type="button" class="tl-row group" :aria-expanded="item.expanded" data-testid="timeline-group"
        :title="item.expanded ? 'Fold the repeated calls into one row' : 'Show each call'" @click="toggle(item.group.key)">
        <span class="head">
          <span class="name" :style="{ paddingLeft: item.group.depth * 12 + 'px' }">
            <span class="caret" aria-hidden="true">{{ item.expanded ? '▾' : '▸' }}</span>
            <span data-testid="timeline-name">{{ item.group.name }}</span> <span class="count" data-testid="timeline-group-count">× {{ item.group.rows.length }}</span>
          </span>
          <span class="meta" data-testid="timeline-group-meta">{{ groupMeta(item.group) }}</span>
        </span>
        <span class="track">
          <template v-for="r in item.group.rows" :key="r.key">
            <span v-if="r.endMs !== null" class="bar seg" :class="{ bad: r.span.status === 'error' }" data-testid="timeline-group-segment"
              :style="{ left: pct(r.startMs) + '%', width: Math.max(0.4, pct(r.endMs) - pct(r.startMs)) + '%' }"/>
          </template>
        </span>
      </button>
      <button v-else type="button" class="tl-row" :class="{ selected: item.row.key === selected }"
        :aria-pressed="item.row.key === selected" :data-span-key="item.row.key" data-testid="timeline-row" @click="$emit('select', item.row.key)">
        <span class="head">
          <span class="name" :style="{ paddingLeft: item.row.depth * 12 + 'px' }">
            <span data-testid="timeline-name">{{ item.row.span.name }}</span><span v-if="item.row.detached" class="warn" data-testid="timeline-parent-missing"> · parent missing</span>
          </span>
          <span class="meta" data-testid="timeline-meta">
            {{ (item.row.span.status || 'unset').toUpperCase() }} · {{ item.row.endMs === null ? 'duration invalid' : fmt(item.row.endMs - item.row.startMs) + ' ms' }}<template v-if="errorsOf(item.row.key) > 0"> · {{ errorsOf(item.row.key) }} ERROR log{{ errorsOf(item.row.key) === 1 ? '' : 's' }}</template>
          </span>
        </span>
        <span class="track">
          <span v-if="item.row.endMs !== null" class="bar" :class="{ bad: item.row.span.status === 'error' }" data-testid="timeline-bar"
            :style="{ left: pct(item.row.startMs) + '%', width: Math.max(0.4, pct(item.row.endMs) - pct(item.row.startMs)) + '%' }"
            :data-start-ms="item.row.startMs" :data-end-ms="item.row.endMs"/>
          <span v-else class="start-only" :style="{ left: pct(item.row.startMs) + '%' }" title="Start recorded; no valid end">▏ start only</span>
        </span>
      </button>
    </template>
    <div class="text-xs opacity-80 mt-2">Time from trace start · bars are recorded intervals; overlapping children are not added up<template v-if="hasGroups"> · repeated calls are folded into one row: open it to see each call</template></div>
  </div>
</template>

<script lang="ts">
import { computed, defineComponent, ref, type PropType } from 'vue'
import { groupRepeats, timeline, type SpanLogCounts, type TimelineGroup } from '@/services/problems'
import { type Span } from '@/types/filter'

// TraceTimeline shows every span of the loaded report on one axis relative to
// the trace start, in parent order; the same selection as the graph. Runs of
// repeated calls (groupRepeats) are one row until opened.
export default defineComponent({
  name: 'TraceTimeline',
  props: {
    spans: { type: Array as PropType<Span[]>, required: true },
    selected: { type: String as PropType<string | null>, default: null },
    counts: { type: Object as PropType<Map<string, SpanLogCounts>>, required: true }
  },
  emits: ['select'],
  setup (props) {
    const tl = computed(() => timeline(props.spans))
    const expanded = ref(new Set<string>())
    const items = computed(() => groupRepeats(tl.value.rows, expanded.value, props.selected))
    const hasGroups = computed(() => items.value.some((i) => i.type === 'group'))
    function toggle (key: string) {
      const next = new Set(expanded.value)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      expanded.value = next
    }
    const pct = (v: number) => Math.min(100, Math.max(0, (v / tl.value.totalMs) * 100))
    const fmt = (v: number) => v.toFixed(v >= 10 ? 0 : 1)
    const errorsOf = (key: string) => props.counts.get(key)?.errors ?? 0
    function groupMeta (g: TimelineGroup): string {
      const status = g.errors > 0 ? `${g.errors} ERROR` : (g.rows[0].span.status || 'unset').toUpperCase()
      const each = g.minMs === null ? 'durations invalid' : g.minMs === g.maxMs ? `${fmt(g.minMs)} ms each` : `${fmt(g.minMs)}–${fmt(g.maxMs!)} ms each`
      const span = g.endMs === null ? '' : ` · ${fmt(g.endMs - g.startMs)} ms first start to last end`
      return `${status} · ${each}${span}`
    }
    return { tl, items, hasGroups, toggle, pct, fmt, errorsOf, groupMeta }
  }
})
</script>

<style scoped>
.timeline { background: var(--surface-ground); border-radius: 8px; padding: 12px; }
.axis { display: flex; justify-content: space-between; margin-bottom: 6px; }
.tl-row { display: block; width: 100%; text-align: left; background: transparent; border: 0; border-left: 3px solid transparent; color: var(--text-color);
  padding: 6px 8px; border-radius: 6px; cursor: pointer; font: inherit; }
.tl-row:focus-visible { outline: 2px solid var(--primary-color); }
.tl-row.selected { background: var(--surface-card); border-left-color: var(--primary-color); }
.head { display: flex; justify-content: space-between; gap: 4px 8px; font-size: 13px; flex-wrap: wrap; }
.name { font-weight: 600; overflow-wrap: anywhere; }
.meta { color: var(--text-color-secondary); }
.warn { color: #fbbf24; font-weight: 400; }
.caret { display: inline-block; width: 12px; color: var(--text-color-secondary); }
.count { color: #fb923c; font-weight: 700; }
.track { position: relative; display: block; height: 10px; margin-top: 5px; background: var(--surface-border); border-radius: 3px; }
.bar { position: absolute; top: 0; height: 10px; border-radius: 3px; background: var(--text-color-secondary); }
.bar.seg { background: #fb923c; box-shadow: inset -1px 0 0 var(--surface-ground); border-radius: 2px; }
.selected .bar { background: var(--primary-color); }
.bar.bad { outline: 1px solid #f87171; }
.start-only { position: absolute; top: -3px; font-size: 11px; color: #fbbf24; white-space: nowrap; }
</style>
