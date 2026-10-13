<template>
  <div class="timeline" data-testid="trace-timeline">
    <div class="axis text-xs opacity-80">
      <span>0 ms</span><span>{{ fmt(tl.totalMs / 2) }} ms</span><span>{{ fmt(tl.totalMs) }} ms</span>
    </div>
    <button v-for="row in tl.rows" :key="row.key" type="button" class="tl-row" :class="{ selected: row.key === selected }"
      :aria-pressed="row.key === selected" :data-span-key="row.key" data-testid="timeline-row" @click="$emit('select', row.key)">
      <span class="head">
        <span class="name" :style="{ paddingLeft: row.depth * 12 + 'px' }">
          <span data-testid="timeline-name">{{ row.span.name }}</span><span v-if="row.detached" class="warn" data-testid="timeline-parent-missing"> · parent missing</span>
        </span>
        <span class="meta" data-testid="timeline-meta">
          {{ (row.span.status || 'unset').toUpperCase() }} · {{ row.endMs === null ? 'duration invalid' : fmt(row.endMs - row.startMs) + ' ms' }}<template v-if="errorsOf(row.key) > 0"> · {{ errorsOf(row.key) }} ERROR log{{ errorsOf(row.key) === 1 ? '' : 's' }}</template>
        </span>
      </span>
      <span class="track">
        <span v-if="row.endMs !== null" class="bar" :class="{ bad: row.span.status === 'error' }" data-testid="timeline-bar"
          :style="{ left: pct(row.startMs) + '%', width: Math.max(0.4, pct(row.endMs) - pct(row.startMs)) + '%' }"
          :data-start-ms="row.startMs" :data-end-ms="row.endMs"/>
        <span v-else class="start-only" :style="{ left: pct(row.startMs) + '%' }" title="Start recorded; no valid end">▏ start only</span>
      </span>
    </button>
    <div class="text-xs opacity-80 mt-2">Time from trace start · bars are recorded intervals; overlapping children are not added up</div>
  </div>
</template>

<script lang="ts">
import { computed, defineComponent, type PropType } from 'vue'
import { timeline, type SpanLogCounts } from '@/services/problems'
import { type Span } from '@/types/filter'

// TraceTimeline shows every span of the loaded report on one axis relative to
// the trace start, in parent order; the same selection as the graph.
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
    const pct = (v: number) => Math.min(100, Math.max(0, (v / tl.value.totalMs) * 100))
    const fmt = (v: number) => v.toFixed(v >= 10 ? 0 : 1)
    const errorsOf = (key: string) => props.counts.get(key)?.errors ?? 0
    return { tl, pct, fmt, errorsOf }
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
.track { position: relative; display: block; height: 10px; margin-top: 5px; background: var(--surface-border); border-radius: 3px; }
.bar { position: absolute; top: 0; height: 10px; border-radius: 3px; background: var(--text-color-secondary); }
.selected .bar { background: var(--primary-color); }
.bar.bad { outline: 1px solid #f87171; }
.start-only { position: absolute; top: -3px; font-size: 11px; color: #fbbf24; white-space: nowrap; }
</style>
