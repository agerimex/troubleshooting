<template>
  <div class="cmp" data-testid="execution-compare" :data-layout="layout">
    <div class="top">
      <div class="totals" data-testid="compare-totals">
        <span class="who">{{ referenceLabel }}</span> <b data-testid="compare-reference-ms">{{ fmt(c.referenceMs) }} ms</b>
        <span class="arrow" aria-hidden="true">→</span>
        <span class="who">{{ currentLabel }}</span> <b data-testid="compare-current-ms">{{ fmt(c.currentMs) }} ms</b>
        <span class="delta" :class="{ up: c.currentMs > c.referenceMs }" data-testid="compare-delta">{{ deltaText(c) }}</span>
      </div>
      <div class="seg" role="group" aria-label="Compare layout">
        <button type="button" :aria-pressed="layout === 'stacked'" :class="{ on: layout === 'stacked' }" @click="setLayout('stacked')" data-testid="compare-layout-stacked">Stacked</button>
        <button type="button" :aria-pressed="layout === 'side'" :class="{ on: layout === 'side' }" @click="setLayout('side')" data-testid="compare-layout-side">Side by side</button>
      </div>
    </div>
    <p v-if="c.differentEntry" class="state warn" data-testid="compare-different-entry">
      The two executions start with different operations: their calls are matched by path, so few of them line up.
    </p>
    <details v-if="diffs.length" class="diffs" data-testid="compare-differences">
      <summary>{{ diffs.length }} {{ diffs.length === 1 ? 'call differs' : 'calls differ' }}</summary>
      <ul><li v-for="d in diffs" :key="d" data-testid="compare-difference">{{ d }}</li></ul>
    </details>
    <p v-else class="diffs" data-testid="compare-differences">Both executions made the same calls, the same number of times.</p>

    <div class="axis"><span></span><div class="ticks"><span>0 ms</span><span>{{ fmt(c.axisMs / 2) }} ms</span><span>{{ fmt(c.axisMs) }} ms</span></div></div>

    <template v-if="layout === 'stacked'">
      <section v-for="b in blocks" :key="b.side" class="block" :data-testid="'compare-block-' + b.side">
        <h4><span>{{ b.label }}</span><b>{{ fmt(b.ms) }} ms</b></h4>
        <template v-for="r in b.rows" :key="r.path">
          <div class="row" :class="toneOf(r)" data-testid="compare-stacked-row" :data-change="r.change">
            <span class="label" :style="{ paddingLeft: r.depth * 12 + 'px' }">
              <span class="name" :title="r.service + ' · ' + (r.spanKind || 'kind not recorded')" data-testid="compare-stacked-name">{{ r.name }}</span>
              <em v-if="r[b.side].count > 1" class="note" data-testid="compare-stacked-count">× {{ r[b.side].count }}</em>
              <em v-if="b.side === 'current' && r.change === 'new'" class="note" data-testid="compare-stacked-new">new</em>
              <em v-if="b.side === 'reference' && r.change === 'missing'" class="note" data-testid="compare-stacked-missing">not called now</em>
            </span>
            <span class="track">
              <template v-for="(s, i) in r[b.side].segments" :key="i">
                <span v-if="s.endMs !== null" class="bar" :class="{ bad: s.error }" :style="bar(s.startMs, s.endMs)" data-testid="compare-stacked-bar"/>
              </template>
            </span>
          </div>
          <div v-if="noteFor(r, b.side)" class="row run" :class="toneOf(r)" data-testid="compare-run-note">
            <span></span>
            <span class="track open"><span class="brace" :style="bar(noteFor(r, b.side)!.startMs, noteFor(r, b.side)!.endMs)">{{ noteFor(r, b.side)!.text }}</span></span>
          </div>
        </template>
      </section>
    </template>

    <template v-else>
      <div v-for="r in c.rows" :key="r.path" class="row pair" :class="toneOf(r)" data-testid="compare-row" :data-change="r.change">
        <span class="label" :style="{ paddingLeft: r.depth * 12 + 'px' }">
          <span class="name" :title="r.service + ' · ' + (r.spanKind || 'kind not recorded')" data-testid="compare-name">{{ r.name }}</span>
          <em class="note" data-testid="compare-counts">
            <template v-if="r.change === 'new'">new<template v-if="r.current.count > 1"> × {{ r.current.count }}</template></template>
            <template v-else-if="r.change === 'missing'">not called now</template>
            <template v-else-if="r.change === 'same'"><template v-if="r.current.count > 1">× {{ r.current.count }}</template></template>
            <template v-else>× {{ r.reference.count }} → {{ r.current.count }}</template>
          </em>
        </span>
        <span class="lanes">
          <span class="track">
            <template v-for="(s, i) in r.current.segments" :key="i">
              <span v-if="s.endMs !== null" class="bar" :class="{ bad: s.error }" :style="bar(s.startMs, s.endMs)" data-testid="compare-current-bar"/>
            </template>
          </span>
          <span class="track">
            <template v-for="(s, i) in r.reference.segments" :key="i">
              <span v-if="s.endMs !== null" class="bar ref" :class="{ bad: s.error }" :style="bar(s.startMs, s.endMs)" data-testid="compare-reference-bar"/>
            </template>
          </span>
        </span>
      </div>
    </template>

    <p class="foot">
      Each execution is timed from its own start<template v-if="layout === 'side'">; upper bar {{ currentLabel }}, lower bar {{ referenceLabel }}</template>.
      This shows what the two executions recorded differently, not which difference caused the change in duration.
      <span class="legend"><i class="k more"></i>more calls <i class="k new"></i>new <i class="k fewer"></i>fewer or not called <i class="k same"></i>unchanged</span>
    </p>
    <details class="how">
      <summary>How calls are matched</summary>
      Calls are matched by their path: the service, kind and name of the call and of each caller up to the entry. Calls with the same path share a row,
      so a call made once in one execution and twenty times in the other is one row.
    </details>
  </div>
</template>

<script lang="ts">
import { computed, defineComponent, ref, type PropType } from 'vue'
import { compareExecutions, deltaText, differences, runNote, sideRows, toneOf, type CompareRow, type RunNote } from '@/services/executionCompare'
import { type Span } from '@/types/filter'

export const COMPARE_LAYOUT_KEY = 'troubleshooting.compare.layout'
type Layout = 'stacked' | 'side'

// ExecutionCompare draws two executions on one time scale
// (services/executionCompare.ts): stacked (one block per execution) or side
// by side (one row per call with a bar for each execution). Colors say what
// changed: more calls, new, fewer or not called, unchanged.
export default defineComponent({
  name: 'ExecutionCompare',
  props: {
    reference: { type: Array as PropType<Span[]>, required: true },
    current: { type: Array as PropType<Span[]>, required: true },
    referenceLabel: { type: String, required: true },
    currentLabel: { type: String, required: true }
  },
  setup (props) {
    const saved = (): string | null => { try { return window.localStorage.getItem(COMPARE_LAYOUT_KEY) } catch { return null } }
    const layout = ref<Layout>(saved() === 'side' ? 'side' : 'stacked')
    function setLayout (l: Layout) {
      layout.value = l
      try { window.localStorage.setItem(COMPARE_LAYOUT_KEY, l) } catch { /* a convenience only */ }
    }
    const c = computed(() => compareExecutions(props.reference, props.current))
    const diffs = computed(() => differences(c.value))
    const blocks = computed(() => ([
      { side: 'reference' as const, label: props.referenceLabel, ms: c.value.referenceMs, rows: sideRows(c.value, 'reference') },
      { side: 'current' as const, label: props.currentLabel, ms: c.value.currentMs, rows: sideRows(c.value, 'current') }
    ]))
    // A run note goes under the side that made more of a changed call.
    function noteFor (r: CompareRow, side: 'reference' | 'current'): RunNote | null {
      const larger = r.change === 'more' || r.change === 'new' ? 'current' : r.change === 'fewer' || r.change === 'missing' ? 'reference' : null
      return larger === side ? runNote(r[side]) : null
    }
    const pct = (v: number) => Math.min(100, Math.max(0, (v / c.value.axisMs) * 100))
    const bar = (start: number, end: number) => ({ left: pct(start) + '%', width: Math.max(0.4, pct(end) - pct(start)) + '%' })
    const fmt = (v: number) => v.toFixed(v >= 10 ? 0 : 1)
    return { layout, setLayout, c, diffs, blocks, noteFor, toneOf, deltaText, bar, fmt }
  }
})
</script>

<style scoped>
.cmp { --entry: var(--primary-color); --same: #8b98a8; --more: #fb923c; --new: #60a5fa; --fewer: #a5b4c8;
  background: var(--surface-ground); border-radius: 8px; padding: 14px; font-size: 13px; display: grid; gap: 6px; }
.top { display: flex; justify-content: space-between; align-items: center; gap: 8px; flex-wrap: wrap; }
.totals { font-size: 14px; display: flex; align-items: baseline; gap: 6px; flex-wrap: wrap; }
.totals .who { color: var(--text-color-secondary); }
.totals b { font-variant-numeric: tabular-nums; }
.arrow { color: var(--text-color-secondary); }
.delta { font-weight: 700; color: var(--text-color-secondary); margin-left: 4px; }
.delta.up { color: var(--more); }
.seg { display: inline-flex; border: 1px solid var(--surface-border); border-radius: 8px; padding: 2px; gap: 2px; }
.seg button { background: none; border: 0; color: var(--text-color-secondary); padding: 4px 10px; border-radius: 6px; cursor: pointer; font: inherit; font-size: 12px; }
.seg button.on { background: var(--surface-card); color: var(--text-color); font-weight: 600; }
.seg button:focus-visible { outline: 2px solid var(--primary-color); }
.diffs { margin: 0; color: var(--text-color-secondary); font-size: 12px; }
.diffs summary { cursor: pointer; }
.diffs ul { margin: 4px 0 0 18px; list-style: disc; }
.axis, .row { display: grid; grid-template-columns: minmax(150px, 36%) 1fr; gap: 10px; align-items: center; }
.axis { margin-top: 4px; font-size: 11px; color: var(--text-color-secondary); }
.ticks { display: flex; justify-content: space-between; border-bottom: 1px solid var(--surface-border); padding-bottom: 2px; }
.block { display: grid; gap: 3px; margin-top: 8px; }
.block h4 { display: flex; justify-content: space-between; margin: 0 0 2px; font-size: 13px; font-weight: 600; }
.block h4 b { color: var(--entry); font-variant-numeric: tabular-nums; }
.label { display: flex; gap: 6px; align-items: baseline; min-width: 0; font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, 'Liberation Mono', monospace; font-size: 12px; }
.name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.note { font-style: normal; font-weight: 700; white-space: nowrap; color: var(--tone); }
.row { --tone: var(--same); }
.row.entry { --tone: var(--entry); }
.row.more { --tone: var(--more); }
.row.new { --tone: var(--new); }
.row.fewer, .row.missing { --tone: var(--fewer); }
.row.same .note { color: var(--text-color-secondary); font-weight: 400; }
.track { position: relative; display: block; height: 12px; background: var(--surface-border); border-radius: 3px; }
.track.open { background: none; height: auto; min-height: 18px; }
.bar { position: absolute; top: 1px; bottom: 1px; min-width: 2px; border-radius: 2px; background: var(--tone); box-shadow: inset -1px 0 0 var(--surface-ground); }
.row.missing .bar, .row.fewer .bar { background: transparent; border: 1px dashed var(--tone); }
.bar.ref { opacity: 0.45; }
.bar.bad { outline: 1px solid #f87171; }
.pair { padding: 3px 0; }
.lanes { display: grid; gap: 2px; }
.pair .track { height: 8px; }
.run .brace { position: absolute; top: 0; border-top: 2px solid var(--tone); padding-top: 3px; color: var(--tone); font-size: 11px; white-space: nowrap; }
.foot { margin: 8px 0 0; font-size: 11px; color: var(--text-color-secondary); }
.legend { display: inline-flex; gap: 4px 10px; flex-wrap: wrap; align-items: center; margin-left: 6px; }
.k { display: inline-block; width: 10px; height: 8px; border-radius: 2px; margin-right: -6px; }
.k.more { background: var(--more); } .k.new { background: var(--new); } .k.fewer { border: 1px dashed var(--fewer); } .k.same { background: var(--same); }
.how { font-size: 11px; color: var(--text-color-secondary); }
.how summary { cursor: pointer; }
.state.warn { margin: 0; padding: 8px; border-radius: 6px; background: rgba(251, 191, 36, 0.12); color: #fcd34d; }
@media (max-width: 600px) { .axis, .row { grid-template-columns: 120px 1fr; } .run .brace { white-space: normal; } }
</style>
