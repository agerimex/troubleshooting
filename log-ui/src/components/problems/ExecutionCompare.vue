<template>
  <div class="cmp" data-testid="execution-compare">
    <div class="legend text-sm">
      <span class="key cur"></span><b>{{ currentLabel }}</b> · <span data-testid="compare-current-ms">{{ fmt(c.currentMs) }} ms</span>
      <span class="key ref ml"></span><b>{{ referenceLabel }}</b> · <span data-testid="compare-reference-ms">{{ fmt(c.referenceMs) }} ms</span>
    </div>
    <p v-if="c.differentEntry" class="state warn mt-2" data-testid="compare-different-entry">
      The two executions start with different operations: their calls are matched by path, so few of them line up.
    </p>
    <div class="mt-2 text-sm" data-testid="compare-differences">
      <template v-if="diffs.length">
        <b>Calls that differ</b>
        <ul class="diffs">
          <li v-for="d in diffs.slice(0, 6)" :key="d" data-testid="compare-difference">{{ d }}</li>
        </ul>
        <span v-if="diffs.length > 6" class="text-xs opacity-80">and {{ diffs.length - 6 }} more, marked below</span>
      </template>
      <span v-else>Both executions made the same calls, the same number of times.</span>
    </div>

    <div class="grid mt-3">
      <div class="axis text-xs opacity-80"><span></span><div class="ticks"><span>0 ms</span><span>{{ fmt(c.axisMs / 2) }} ms</span><span>{{ fmt(c.axisMs) }} ms</span></div></div>
      <div v-for="r in c.rows" :key="r.path" class="row" :class="r.change" data-testid="compare-row" :data-change="r.change">
        <div class="label" :style="{ paddingLeft: r.depth * 12 + 'px' }">
          <span class="name" :title="r.service + ' · ' + (r.spanKind || 'kind not recorded')" data-testid="compare-name">{{ r.name }}</span>
          <span class="counts" data-testid="compare-counts">
            <template v-if="r.change === 'new'"><span class="tag new">new</span><template v-if="r.current.count > 1"> × {{ r.current.count }}</template></template>
            <template v-else-if="r.change === 'missing'"><span class="tag missing">not called</span></template>
            <template v-else-if="r.change === 'same'"><template v-if="r.current.count > 1">× {{ r.current.count }}</template></template>
            <template v-else><span class="tag" :class="r.change">× {{ r.reference.count }} → {{ r.current.count }}</span></template>
          </span>
        </div>
        <div class="lanes">
          <span class="lane">
            <template v-for="(s, i) in r.current.segments" :key="i">
              <span v-if="s.endMs !== null" class="bar cur" :class="{ bad: s.error }" :style="bar(s.startMs, s.endMs)" data-testid="compare-current-bar"/>
            </template>
          </span>
          <span class="lane">
            <template v-for="(s, i) in r.reference.segments" :key="i">
              <span v-if="s.endMs !== null" class="bar ref" :class="{ bad: s.error }" :style="bar(s.startMs, s.endMs)" data-testid="compare-reference-bar"/>
            </template>
          </span>
        </div>
      </div>
    </div>
    <div class="text-xs opacity-80 mt-2">
      One time scale from each execution's start: upper bar {{ currentLabel }}, lower bar {{ referenceLabel }}. Calls are matched by their path
      (service, kind and name of the call and of its callers); repeated calls share a row. This shows what each execution recorded differently,
      not which difference caused the change in duration.
    </div>
  </div>
</template>

<script lang="ts">
import { computed, defineComponent, type PropType } from 'vue'
import { compareExecutions, differences } from '@/services/executionCompare'
import { type Span } from '@/types/filter'

// ExecutionCompare draws two executions on one time scale, call by call
// (services/executionCompare.ts).
export default defineComponent({
  name: 'ExecutionCompare',
  props: {
    reference: { type: Array as PropType<Span[]>, required: true },
    current: { type: Array as PropType<Span[]>, required: true },
    referenceLabel: { type: String, required: true },
    currentLabel: { type: String, required: true }
  },
  setup (props) {
    const c = computed(() => compareExecutions(props.reference, props.current))
    const diffs = computed(() => differences(c.value))
    const pct = (v: number) => Math.min(100, Math.max(0, (v / c.value.axisMs) * 100))
    const bar = (start: number, end: number) => ({ left: pct(start) + '%', width: Math.max(0.4, pct(end) - pct(start)) + '%' })
    const fmt = (v: number) => v.toFixed(v >= 10 ? 0 : 1)
    return { c, diffs, bar, fmt }
  }
})
</script>

<style scoped>
.cmp { background: var(--surface-ground); border-radius: 8px; padding: 12px; }
.legend { display: flex; align-items: center; flex-wrap: wrap; gap: 4px; }
.key { display: inline-block; width: 12px; height: 8px; border-radius: 2px; margin-right: 4px; }
.key.cur, .bar.cur { background: var(--primary-color); }
.key.ref, .bar.ref { background: #7f9cc0; }
.ml { margin-left: 14px; }
.diffs { margin: 2px 0 0 18px; list-style: disc; }
.grid { display: grid; gap: 2px; }
.axis, .row { display: grid; grid-template-columns: minmax(150px, 38%) 1fr; gap: 10px; align-items: center; }
.ticks { display: flex; justify-content: space-between; }
.row { padding: 4px 6px; border-radius: 6px; }
.row.new, .row.more { background: rgba(251, 146, 60, 0.08); }
.row.missing, .row.fewer { background: rgba(127, 156, 192, 0.1); }
.label { display: flex; gap: 6px; align-items: baseline; min-width: 0; font-size: 13px; }
.name { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.counts { color: var(--text-color-secondary); white-space: nowrap; font-size: 12px; }
.tag { border-radius: 4px; padding: 0 5px; font-weight: 700; }
.tag.new, .tag.more { color: #fb923c; }
.tag.missing, .tag.fewer { color: #93b4dd; }
.lanes { display: grid; gap: 2px; }
.lane { position: relative; display: block; height: 8px; background: var(--surface-border); border-radius: 3px; }
.bar { position: absolute; top: 0; height: 8px; border-radius: 2px; box-shadow: inset -1px 0 0 var(--surface-ground); }
.bar.bad { outline: 1px solid #f87171; }
.state { padding: 8px; border-radius: 6px; font-size: 13px; }
.state.warn { background: rgba(251, 191, 36, 0.12); color: #fcd34d; }
.mt-2 { margin-top: 8px; }
.mt-3 { margin-top: 12px; }
@media (max-width: 600px) { .axis, .row { grid-template-columns: 120px 1fr; } }
</style>
