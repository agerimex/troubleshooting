<template>
  <div ref="root" class="seq" data-testid="sequence">
    <div class="bar">
      <label class="chk"><input type="checkbox" v-model="showInternal" data-testid="seq-internal"/> Internal operations</label>
      <span class="text-xs opacity-80" data-testid="seq-counts">{{ model.operations.length }} operations / {{ model.spanCount }} spans</span>
    </div>
    <p v-if="model.overlaps.length" class="note" data-testid="seq-overlap">
      {{ model.overlaps[0].aName }} and {{ model.overlaps[0].bName }} overlap for {{ fmtMs(model.overlaps[0].overlapMs) }} ms (recorded intervals, both in flight; not a dependency or a join).
      <template v-if="model.overlaps.length > 1"> {{ model.overlaps.length - 1 }} more overlap(s).</template>
      <button type="button" class="link" @click="$emit('show-timing')" data-testid="seq-overlap-timing">See Timing</button>
    </p>
    <p v-for="(l, i) in model.limitations" :key="i" class="note warn" data-testid="seq-limitation">{{ l }}</p>

    <div v-if="page.total > maxMarks" class="pager" data-testid="seq-pager">
      <span>Marks {{ page.start + 1 }}–{{ page.start + page.events.length }} of {{ page.total }} (render limit {{ maxMarks }}; the report keeps every span)</span>
      <button type="button" :disabled="page.start === 0" @click="start = page.start - maxMarks" data-testid="seq-earlier">Earlier</button>
      <button type="button" :disabled="page.start + maxMarks >= page.total" @click="start = page.start + maxMarks" data-testid="seq-later">Later</button>
      <button type="button" :disabled="!selected" @click="locateSelection" data-testid="seq-locate">Locate selection</button>
      <span v-if="hiddenErrors > 0" class="err" data-testid="seq-hidden-errors">{{ hiddenErrors }} ERROR log marker(s) on other pages</span>
    </div>

    <!-- Narrow screens: a chronological list with explicit source and target. -->
    <ol v-if="narrow" class="list" data-testid="seq-list">
      <li v-for="e in page.events" :key="e.id">
        <button type="button" class="item" :class="[e.kind, { sel: isSelected(e) }]" @click="pick(e)" data-testid="seq-list-item">{{ eventText(e, model) }}</button>
      </li>
    </ol>

    <div v-else class="diagram" :style="{ '--cols': lanesShown.length }">
      <div class="heads">
        <div v-for="p in lanesShown" :key="p" class="head" :class="{ unobserved: model.unobserved.includes(p) || p === OTHER_LANE }" data-testid="seq-participant">
          {{ p }}<span v-if="p === OTHER_LANE" class="text-xs"> ({{ laneSet.other.length }})</span>
        </div>
      </div>
      <div class="rows">
        <div v-for="c in lanesShown.length" :key="'l' + c" class="lifeline" :style="{ left: `calc(${(c - 0.5) / lanesShown.length * 100}% )` }"/>
        <div v-if="activation" class="activation" :class="{ 'open-top': !activation.startsHere, 'open-bottom': !activation.endsHere }"
          :style="activationStyle" aria-hidden="true" data-testid="seq-activation"/>
        <div v-for="e in page.events" :key="e.id" class="row">
          <button v-if="e.kind === 'call' || e.kind === 'message'" type="button" class="arrow" :class="{ sel: isSelected(e), rtl: col(e.target!) < col(e.participant), msg: e.kind === 'message' }"
            :style="arrowStyle(e)" :aria-pressed="isSelected(e)" @click="pick(e)" :data-testid="e.kind === 'message' ? 'seq-message' : 'seq-call'" :data-span-key="e.spanKey">
            <span class="lbl">+{{ fmtMs(e.timeMs ?? 0) }} ms · {{ e.kind === 'message' ? 'Message' : 'Call' }}<template v-if="repeatOf(e)"> {{ repeatOf(e) }}</template><br><b>{{ e.label }}</b><template v-if="!targetObserved(e)"> <i>(peer not instrumented)</i></template></span>
            <span class="line"/>
          </button>
          <button v-else type="button" class="mark" :class="[e.kind, statusClass(e), { sel: isSelected(e) }]" :style="markStyle(e)"
            :aria-pressed="isSelected(e)" @click="pick(e)" :data-testid="'seq-' + e.kind" :data-span-key="e.spanKey">
            <span class="t">{{ e.timeMs === null ? 'time unknown' : '+' + fmtMs(e.timeMs) + ' ms' }} · {{ kindWord(e) }}<template v-if="laneOf(laneSet, e.participant) === OTHER_LANE"> · {{ e.participant }}</template><template v-if="e.kind === 'end' && isSelected(e)"> · end of the selection</template><template v-if="e.note"> · <span class="n">{{ e.note }}</span></template></span>
            <span class="d">{{ e.label }}<template v-if="e.kind === 'end' && e.durationMs !== undefined"> · {{ fmtMs(e.durationMs) }} ms</template></span>
          </button>
        </div>
      </div>
    </div>
    <p class="legend text-xs opacity-80">
      Arrows = correlated client/server spans (or a client's recorded peer); dashed arrows = messages, a producer span and its one consumer span, one-way, with the consumer's processing in its own column. End markers = span completion, not a response; the selected operation's bar runs on its lifeline from its start to its end. Vertical spacing shows order, not elapsed time; equal times are ordered by id, which is not causality.
      <template v-if="model.links.length"> {{ model.links.length }} recorded link(s) are associations, not calls: see Span details.</template>
    </p>
  </div>
</template>

<script lang="ts">
import { computed, defineComponent, onBeforeUnmount, onMounted, ref, watch, type PropType } from 'vue'
import { activationOf, eventText, fmtMs, laneOf, lanes, locate, OTHER_LANE, pageOf, visibleEvents, type SeqEvent, type SequenceModel } from '@/services/sequence'

// SequenceView draws the sequence model: one column per participant (at most
// maxLanes), one row per mark in chronological order (at most maxMarks per
// page). Hiding internal operations and paging are presentation only.
export default defineComponent({
  name: 'SequenceView',
  props: {
    model: { type: Object as PropType<SequenceModel>, required: true },
    selected: { type: String as PropType<string | null>, default: null },
    maxMarks: { type: Number, default: 80 },
    maxLanes: { type: Number, default: 6 }
  },
  emits: ['select', 'select-log', 'show-timing'],
  setup (props, { emit }) {
    const showInternal = ref(true)
    const start = ref(0)
    const root = ref<HTMLElement | null>(null)
    const width = ref(1000)
    let observer: ResizeObserver | null = null
    onMounted(() => {
      if (root.value && typeof ResizeObserver !== 'undefined') {
        observer = new ResizeObserver((entries) => { width.value = entries[0].contentRect.width })
        observer.observe(root.value)
        width.value = root.value.clientWidth
      }
    })
    onBeforeUnmount(() => observer?.disconnect())
    const narrow = computed(() => width.value > 0 && width.value < 430)

    const events = computed(() => visibleEvents(props.model, showInternal.value, props.selected))
    const page = computed(() => pageOf(events.value, start.value, props.maxMarks))
    const laneSet = computed(() => lanes(props.model, props.maxLanes))
    const lanesShown = computed(() => laneSet.value.other.length ? [...laneSet.value.shown, OTHER_LANE] : laneSet.value.shown)
    const col = (participant: string) => lanesShown.value.indexOf(laneOf(laneSet.value, participant))
    const hiddenErrors = computed(() => {
      const shown = new Set(page.value.events.map((e) => e.id))
      return events.value.filter((e) => e.kind === 'log' && !shown.has(e.id)).length
    })

    // The selected span stays reachable: when the selection changes to a span
    // whose marks are on another page, move there.
    watch(() => props.selected, (key) => {
      if (key && !page.value.events.some((e) => e.spanKey === key || e.operationId === key)) {
        locateSelection()
      }
    })
    function locateSelection () {
      if (!props.selected) {
        return
      }
      const at = locate(events.value, props.selected, props.maxMarks)
      if (at !== null) {
        start.value = at
      }
    }

    const opOf = (e: SeqEvent) => props.model.operations.find((o) => o.id === e.operationId)
    const isSelected = (e: SeqEvent) => !!props.selected && (e.spanKey === props.selected || (e.kind !== 'log' && (opOf(e)?.spanKeys.includes(props.selected) ?? false)))
    function pick (e: SeqEvent) {
      if (e.kind === 'log') {
        emit('select-log', { spanKey: e.spanKey, recordId: e.logId })
      } else {
        emit('select', e.spanKey)
      }
    }
    // The activation bar of the selected operation: on its lifeline, from its
    // call arrow (or start mark) down to its end mark, so the two read as one.
    const activation = computed(() => {
      const key = props.selected
      const op = key ? props.model.operations.find((o) => o.spanKeys.includes(key)) : undefined
      return op ? activationOf(events.value, page.value, op.id) : null
    })
    const ROW = 54
    const PAD = 6
    const MARK_TOP = 6 // .mark top within its row
    const activationStyle = computed(() => {
      const a = activation.value
      if (!a) {
        return {}
      }
      const startsWithArrow = page.value.events[a.from]?.kind === 'call'
      const top = a.startsHere ? PAD + a.from * ROW + (startsWithArrow ? ROW - 10 : ROW / 2) : 0
      const style: Record<string, string> = { left: `calc(${pct(col(a.participant))}% - 4px)`, top: top + 'px' }
      if (a.endsHere) {
        style.height = Math.max(PAD + a.to * ROW + MARK_TOP - top, 4) + 'px' // to the end mark's top edge
      } else {
        style.bottom = '0'
      }
      return style
    })
    const pct = (i: number) => ((i + 0.5) / lanesShown.value.length) * 100
    function arrowStyle (e: SeqEvent) {
      const a = col(e.participant)
      const b = col(e.target!)
      const left = Math.min(pct(a), pct(b))
      const w = Math.max(Math.abs(pct(a) - pct(b)), 100 / lanesShown.value.length / 2)
      return { left: left + '%', width: w + '%' }
    }
    function markStyle (e: SeqEvent) {
      const w = 100 / lanesShown.value.length
      const c = col(e.participant)
      return { left: `calc(${c * w}% + 4px)`, width: `calc(${w}% - 8px)` }
    }
    const kindWord = (e: SeqEvent) => e.kind === 'start' ? 'Started' : e.kind === 'end' ? 'Ended' : e.kind === 'log' ? `${e.severity} log` : e.kind === 'message' ? 'Message' : 'Call'
    // An end mark shows the status of the span it ends (a message's: the consumer's).
    const statusClass = (e: SeqEvent) => {
      if (e.kind === 'log') {
        return 'bad'
      }
      const o = opOf(e)
      const status = o?.kind === 'message' && e.spanKey !== o.id ? o.serverStatus : o?.status
      return status === 'error' && e.kind === 'end' ? 'errspan' : ''
    }
    const repeatOf = (e: SeqEvent) => { const r = opOf(e)?.repeat; return r ? `(repeat ${r.index} of ${r.count})` : '' }
    const targetObserved = (e: SeqEvent) => opOf(e)?.targetObserved ?? true
    return {
      root, narrow, showInternal, start, page, laneSet, lanesShown, col, hiddenErrors, locateSelection, isSelected, pick, arrowStyle, markStyle, activation, activationStyle,
      kindWord, statusClass, repeatOf, targetObserved, eventText, fmtMs, laneOf, OTHER_LANE
    }
  }
})
</script>

<style scoped>
.seq { background: var(--surface-ground); border-radius: 8px; padding: 12px; }
.bar { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.chk { display: flex; align-items: center; gap: 6px; font-size: 13px; cursor: pointer; }
.chk input { accent-color: var(--primary-color); }
.note { background: var(--surface-card); border-radius: 6px; padding: 7px 10px; font-size: 13px; margin: 8px 0 0; }
.note.warn { color: #fcd34d; }
.pager { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; font-size: 12px; margin-top: 8px; }
.pager button, .link { background: none; border: 1px solid var(--surface-border); color: var(--text-color); border-radius: 6px; padding: 3px 8px; cursor: pointer; font: inherit; }
.pager button:disabled { opacity: 0.4; cursor: default; }
.link { border: 0; color: var(--primary-color); padding: 0 4px; }
.err { color: #fca5a5; }
.diagram { margin-top: 10px; }
.heads { display: grid; grid-template-columns: repeat(var(--cols), minmax(0, 1fr)); gap: 6px; }
.head { background: var(--surface-card); border-radius: 6px; text-align: center; font-size: 13px; padding: 7px 4px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.head.unobserved { border: 1px dashed var(--surface-border); background: transparent; font-style: italic; }
.rows { position: relative; padding: 6px 0; }
.lifeline { position: absolute; top: 0; bottom: 0; width: 1px; background: var(--surface-border); }
.row { position: relative; height: 54px; }
.activation { position: absolute; width: 8px; border-radius: 2px; background: color-mix(in srgb, var(--primary-color) 35%, transparent);
  border: 1px solid var(--primary-color); box-sizing: border-box; pointer-events: none; }
.activation.open-top { border-top: 0; mask-image: linear-gradient(to bottom, transparent, #000 24px); }
.activation.open-bottom { border-bottom: 0; mask-image: linear-gradient(to top, transparent, #000 24px); }
.activation.open-top.open-bottom { mask-image: linear-gradient(to bottom, transparent, #000 24px, #000 calc(100% - 24px), transparent); }
.mark { position: absolute; top: 6px; height: 42px; text-align: left; background: var(--surface-card); border: 1px solid var(--surface-border); border-radius: 6px;
  color: var(--text-color); padding: 3px 8px; font: inherit; font-size: 12px; cursor: pointer; overflow: hidden; display: flex; flex-direction: column; justify-content: center; }
.mark.end { border-style: dashed; background: transparent; }
.mark.log, .mark.bad { background: rgba(248, 113, 113, 0.12); border-color: rgba(248, 113, 113, 0.5); color: #fca5a5; }
.mark.sel, .arrow.sel .lbl { outline: 2px solid var(--primary-color); outline-offset: 1px; }
.mark:focus-visible, .arrow:focus-visible, .item:focus-visible, .chk input:focus-visible { outline: 2px solid var(--primary-color); }
.t { color: var(--text-color-secondary); font-size: 11px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.mark.log .t { color: #fca5a5; }
.d { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.n { color: #fcd34d; }
.arrow { position: absolute; top: 0; height: 54px; background: none; border: 0; padding: 0; cursor: pointer; color: var(--text-color); font: inherit; }
.lbl { display: block; text-align: center; font-size: 12px; line-height: 1.3; border-radius: 4px; padding: 0 4px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.line { position: absolute; left: 0; right: 0; bottom: 10px; height: 2px; background: #60a5fa; }
.line::after { content: ''; position: absolute; right: -1px; top: -4px; border: 5px solid transparent; border-left: 7px solid #60a5fa; border-right: 0; }
.arrow.rtl .line::after { right: auto; left: -1px; border-left: 0; border-right: 7px solid #60a5fa; }
.arrow.sel .line { background: var(--primary-color); }
.arrow.msg .line { background: repeating-linear-gradient(to right, #60a5fa 0 6px, transparent 6px 10px); }
.arrow.msg.sel .line { background: repeating-linear-gradient(to right, var(--primary-color) 0 6px, transparent 6px 10px); }
.list { list-style: none; padding: 0; margin: 10px 0 0; }
.item { display: block; width: 100%; text-align: left; background: var(--surface-card); border: 1px solid var(--surface-border); border-radius: 6px; color: var(--text-color);
  padding: 8px; margin-bottom: 6px; font: inherit; font-size: 13px; cursor: pointer; min-height: 44px; }
.item.log { color: #fca5a5; }
.item.sel { border-color: var(--primary-color); }
.legend { margin: 8px 0 0; }
@media (prefers-reduced-motion: reduce) { * { transition: none !important; } }
</style>
