<template>
  <div class="library" data-testid="sequence-library">
    <div class="flex justify-between items-start flex-wrap gap-2">
      <div>
        <h2 class="title">Sequence library</h2>
        <p class="text-sm opacity-80">
          One row per distinct execution shape: the entry operation and the set of calls it makes. Repeats and order don't split a shape;
          an execution that makes other calls (an early failure, say) is another shape. Open a row's typical (p50), slow (p95/p99) or slowest execution as a sequence.
        </p>
      </div>
      <PrimeButton v-if="hasOpenExecution" type="button" size="small" text label="Back to the open execution" @click="$emit('close')" data-testid="library-close"/>
    </div>

    <form class="row mt-2" @submit.prevent="load" data-testid="library-form">
      <label class="fld">From
        <Calendar v-model="from" showTime hourFormat="24" dateFormat="dd.mm.yy" inputId="lib-from" data-testid="library-from"/>
      </label>
      <label class="fld">To (exclusive)
        <Calendar v-model="to" showTime hourFormat="24" dateFormat="dd.mm.yy" inputId="lib-to" data-testid="library-to"/>
      </label>
      <label class="fld">Service
        <input v-model="service" placeholder="any service in the shape" class="w-48" data-testid="library-service"/>
      </label>
      <label class="fld">Entry operation
        <input v-model="operation" placeholder="any" class="w-48" data-testid="library-operation"/>
      </label>
      <label class="chk"><input type="checkbox" v-model="errorsOnly" data-testid="library-errors"/> with failed executions</label>
      <PrimeButton type="submit" size="small" label="Show" :loading="loading" data-testid="library-search"/>
    </form>

    <p v-if="formError" class="state bad mt-2" role="alert" data-testid="library-form-error">{{ formError }}</p>
    <p v-else-if="error" class="state bad mt-2" role="alert" data-testid="library-error">The sequence library is unavailable: {{ error }} <button type="button" class="link" @click="load">Retry</button></p>
    <template v-else-if="library">
      <div class="text-xs opacity-80 mt-2" data-testid="library-scope">
        {{ windowText(shownWindow.from, shownWindow.to) }} · {{ library.shapes.length }} {{ library.shapes.length === 1 ? 'shape' : 'shapes' }}<template v-if="library.truncated"> (the {{ library.shapes.length }} with the most executions)</template>
        · executions = traces whose root span started in the window · durations of the root span
      </div>
      <p v-for="n in library.notices" :key="n.code + n.message" class="state mt-1" data-testid="library-notice">{{ n.message }}</p>
      <p v-if="library.status === 'partial'" class="state mt-1" data-testid="library-partial">Some hours of this window have no execution shapes (see above); the table covers the others.</p>
      <p v-if="library.shapes.length === 0" class="state mt-2" data-testid="library-empty">No execution started in this window with these filters.</p>
      <template v-else>
        <div class="tablewrap">
          <table class="ltable" data-testid="library-table">
            <thead>
              <tr>
                <th>Entry</th><th>Calls</th><th class="num">Executions</th><th class="num">Failed</th>
                <th class="num">p50</th><th class="num">p95</th><th class="num">p99</th><th>Open sequence</th>
              </tr>
            </thead>
            <tbody>
              <template v-for="s in rows" :key="s.shapeId">
              <tr data-testid="library-row" :data-shape-id="s.shapeId">
                <td>
                  <div class="entry" data-testid="library-entry">{{ s.entryOperation }}</div>
                  <div class="text-xs opacity-80">{{ s.entryService }}<template v-if="s.services.length > 1"> → {{ servicesText(s) }}</template></div>
                </td>
                <td data-label="Calls" :title="s.distinctCalls + ' distinct caller → callee calls, ' + s.services.length + ' services'">
                  <button type="button" class="link" :aria-expanded="!!expanded[s.shapeId]" @click="toggleCalls(s)" data-testid="library-calls">{{ s.distinctCalls }} {{ expanded[s.shapeId] ? '▴' : '▾' }}</button>
                </td>
                <td class="num" data-label="Executions" data-testid="library-executions">{{ s.executions.toLocaleString() }}</td>
                <td class="num" data-label="Failed" :class="{ bad: s.failedExecutions > 0 }" data-testid="library-failed">{{ failedShare(s) }}</td>
                <td v-for="p in percentiles" :key="p" class="num" :data-label="p" :data-testid="'library-' + p" :title="percentileHint(s, p)">{{ fmtDuration(s.percentiles[p]) }}</td>
                <td>
                  <div class="opens">
                    <button v-for="k in kinds" :key="k" type="button" class="open" :disabled="!s.examples[k]" @click="open(s, k)"
                      :title="s.examples[k] ? 'Open the ' + k + ' execution (' + fmtDuration(s.examples[k]!.durationUs / 1000) + ')' : (k === 'slowest' ? 'no execution kept' : percentileHint(s, k))"
                      :data-testid="'library-open-' + k">{{ k }}</button>
                  </div>
                </td>
              </tr>
              <tr v-if="expanded[s.shapeId]" class="calls" data-testid="library-calls-row">
                <td colspan="8">
                  <p v-if="callState[s.shapeId]?.loading" class="text-xs opacity-80">Reading the calls of one execution of this shape…</p>
                  <p v-else-if="callState[s.shapeId]?.error" class="state bad" data-testid="library-calls-error">The calls are unavailable: {{ callState[s.shapeId]!.error }}</p>
                  <template v-else-if="callState[s.shapeId]">
                    <p class="text-xs opacity-80">Distinct calls of this shape, as recorded in its {{ callState[s.shapeId]!.example }} execution (× how often in that execution):</p>
                    <ol class="calllist" data-testid="library-call-list">
                      <li v-for="c in callState[s.shapeId]!.calls" :key="c.caller + c.callee + c.kind" data-testid="library-call">
                        <span class="opacity-80">{{ c.caller }}</span> → <b>{{ c.callee }}</b><span v-if="c.kind" class="text-xs opacity-80"> ({{ c.kind }})</span><span v-if="c.count > 1" class="text-xs"> ×{{ c.count }}</span>
                      </li>
                    </ol>
                    <p v-if="callState[s.shapeId]!.calls.length === 0" class="text-xs opacity-80">This shape makes no calls: its executions are a single span.</p>
                  </template>
                </td>
              </tr>
              </template>
            </tbody>
          </table>
        </div>
        <div class="flex items-center gap-2 mt-2" data-testid="library-pager">
          <PrimeButton type="button" size="small" text label="Previous" :disabled="page === 0" @click="page--" data-testid="library-previous"/>
          <span class="text-xs opacity-80" data-testid="library-page">page {{ page + 1 }} of {{ pages }}</span>
          <PrimeButton type="button" size="small" text label="Next" :disabled="page + 1 >= pages" @click="page++" data-testid="library-next"/>
        </div>
      </template>
    </template>

    <details class="mt-3" data-testid="trace-lookup">
      <summary class="text-sm">Open one execution by trace id</summary>
      <form class="row mt-2" @submit.prevent="lookup">
        <label class="fld grow">Trace ID
          <input v-model="traceIdInput" placeholder="32 hex digits" class="mono" spellcheck="false" data-testid="lookup-trace-id"/>
        </label>
        <PrimeButton type="submit" size="small" label="Open" data-testid="lookup-open"/>
      </form>
      <p class="text-xs opacity-80 mt-1">Read within the From/To window above.</p>
      <p v-if="lookupError" class="state bad mt-1" role="alert" data-testid="lookup-error">{{ lookupError }}</p>
    </details>
  </div>
</template>

<script lang="ts">
import { computed, defineComponent, ref } from 'vue'
import PrimeButton from 'primevue/button'
import Calendar from 'primevue/calendar'
import logsApi from '@/api/logs.api'
import statsApi from '@/api/stats.api'
import { Logs } from '@/logs-api/logs.impl'
import { Stats } from '@/logs-api/stats.impl'
import { windowText } from '@/services/comparison'
import { toNs } from '@/services/spanTree'
import { type ExecutionTarget } from '@/services/navigation'
import { callsOf, EXAMPLE_KINDS, exampleTarget, failedShare, fmtDuration, pageCount, pageOf, percentileHint, servicesText } from '@/services/shapes'
import { type ShapeCall } from '@/services/shapes'
import { type Shape, type ShapeExampleKind, type ShapeLibrary } from '@/types/shapes'

// SequenceLibrary lists the distinct execution shapes of a window (computed
// by ClickHouse, stored hourly) with their executions, failures and duration
// percentiles, ten per page; a row opens a representative execution as a
// sequence. It keeps its window, filters and page while an execution is open.
export default defineComponent({
  name: 'SequenceLibrary',
  components: { PrimeButton, Calendar },
  props: {
    hasOpenExecution: { type: Boolean, default: false }
  },
  emits: ['open-execution', 'close'],
  setup (_props, { emit }) {
    statsApi.init(new Stats())
    logsApi.init(new Logs())
    const day = 24 * 3600_000
    const now = Date.now()
    const from = ref<Date>(new Date(now - day))
    const to = ref<Date>(new Date(now + 60_000))
    const service = ref('')
    const operation = ref('')
    const errorsOnly = ref(false)
    const loading = ref(false)
    const error = ref('')
    const formError = ref('')
    const library = ref<ShapeLibrary | null>(null)
    const shownWindow = ref({ from: '', to: '' })
    const page = ref(0)
    const traceIdInput = ref('')
    const lookupError = ref('')
    let seq = 0

    function chosenWindow (): { from: string, to: string } | null {
      const f = toNs(from.value)
      const t = toNs(to.value)
      return BigInt(t) > BigInt(f) ? { from: f, to: t } : null
    }

    async function load () {
      const w = chosenWindow()
      if (!w) {
        formError.value = 'The window end must be after its start.'
        return
      }
      formError.value = ''
      const mine = ++seq
      loading.value = true
      error.value = ''
      const [err, lib] = await statsApi.shapes({
        timeFrom: w.from, timeTo: w.to, service: service.value.trim(), operation: operation.value.trim(), errorsOnly: errorsOnly.value
      })
      if (mine !== seq) {
        return
      }
      loading.value = false
      shownWindow.value = w
      page.value = 0
      if (err !== null || lib === null) {
        error.value = err ?? 'no answer'
        library.value = null
        return
      }
      if (lib.status !== 'ok' && lib.status !== 'partial') {
        error.value = lib.error || lib.status
        library.value = null
        return
      }
      library.value = lib
    }

    const rows = computed(() => library.value ? pageOf(library.value.shapes, page.value) : [])
    const pages = computed(() => pageCount(library.value?.shapes.length ?? 0))

    function open (s: Shape, kind: ShapeExampleKind) {
      const t = exampleTarget(s, kind)
      if (t) {
        emit('open-execution', t)
      }
    }

    // The calls of a shape are read on demand from one of its executions (one
    // trace-anchored report), since the stored shape keeps only their hashes.
    const expanded = ref<Record<string, boolean>>({})
    const callState = ref<Record<string, { loading: boolean, error?: string, example?: string, calls: ShapeCall[] }>>({})
    async function toggleCalls (s: Shape) {
      expanded.value = { ...expanded.value, [s.shapeId]: !expanded.value[s.shapeId] }
      if (!expanded.value[s.shapeId] || callState.value[s.shapeId]) {
        return
      }
      const kind = (["p50", "slowest", "p95", "p99"] as ShapeExampleKind[]).find((k) => s.examples[k])
      const t = kind ? exampleTarget(s, kind) : null
      if (!kind || !t) {
        callState.value = { ...callState.value, [s.shapeId]: { loading: false, error: "no execution of this shape was kept", calls: [] } }
        return
      }
      callState.value = { ...callState.value, [s.shapeId]: { loading: true, calls: [] } }
      const [err, response] = await logsApi.traceReport({ traceId: t.traceId, timeFrom: t.window.from, timeTo: t.window.to, sections: ["spans"] })
      const spans = response?.spans?.records
      callState.value = {
        ...callState.value,
        [s.shapeId]: err !== null || !spans ? { loading: false, error: err ?? "no answer", calls: [] } : { loading: false, example: kind, calls: callsOf(spans) }
      }
    }

    function lookup () {
      const id = traceIdInput.value.trim().toLowerCase()
      if (!/^[0-9a-f]{32}$/.test(id)) {
        lookupError.value = 'A trace id has 32 hex digits.'
        return
      }
      const w = chosenWindow()
      if (!w) {
        lookupError.value = 'The window end must be after its start.'
        return
      }
      lookupError.value = ''
      emit('open-execution', { traceId: id, window: w } as ExecutionTarget)
    }

    load()
    return {
      from, to, service, operation, errorsOnly, loading, error, formError, library, shownWindow, page, rows, pages,
      percentiles: ['p50', 'p95', 'p99'] as const, kinds: EXAMPLE_KINDS, traceIdInput, lookupError,
      expanded, callState, toggleCalls, load, open, lookup, windowText, failedShare, fmtDuration, percentileHint, servicesText
    }
  }
})
</script>

<style scoped>
.title { font-size: 25px; font-weight: 500; margin: 0; }
.row { display: flex; flex-wrap: wrap; align-items: flex-end; gap: 10px; }
.fld { display: flex; flex-direction: column; gap: 3px; font-size: 12px; color: var(--text-color-secondary); }
.fld.grow { flex: 1; min-width: min(100%, 20rem); }
.fld input { background: var(--surface-ground); color: var(--text-color); border: 1px solid var(--surface-border); border-radius: 6px; padding: 8px; font: inherit; font-size: 14px; }
.fld input:focus-visible { outline: 2px solid var(--primary-color); }
.chk { display: flex; align-items: center; gap: 6px; font-size: 13px; padding-bottom: 8px; }
.mono { font-family: ui-monospace, monospace; word-break: break-all; }
.tablewrap { overflow-x: auto; margin-top: 8px; }
.ltable { width: 100%; border-collapse: collapse; min-width: 52rem; }
.ltable th { text-align: left; font-size: 12px; font-weight: 500; color: var(--text-color-secondary); padding: 6px 8px; border-bottom: 1px solid var(--surface-border); }
.ltable td { padding: 8px; border-bottom: 1px solid var(--surface-border); vertical-align: top; }
.ltable .num { text-align: right; font-variant-numeric: tabular-nums; white-space: nowrap; }
.entry { font-weight: 600; word-break: break-word; }
.opens { display: flex; gap: 4px; flex-wrap: wrap; }
.open { background: var(--surface-ground); color: var(--primary-color); border: 1px solid var(--surface-border); border-radius: 6px; padding: 3px 8px; font: inherit; font-size: 12px; cursor: pointer; }
.open:disabled { color: var(--text-color-secondary); opacity: 0.5; cursor: not-allowed; }
.open:focus-visible { outline: 2px solid var(--primary-color); }
.bad { color: #fca5a5; }
.calls td { background: var(--surface-ground); }
.calllist { margin: 4px 0 0; padding-left: 20px; font-size: 13px; }
.calllist li { padding: 1px 0; word-break: break-word; }
.state { padding: 8px; border-radius: 6px; font-size: 13px; background: var(--surface-ground); }
.state.bad { background: rgba(248, 113, 113, 0.12); color: #fca5a5; }
.link { background: none; border: 0; color: var(--primary-color); cursor: pointer; padding: 0; font: inherit; }
@media (max-width: 640px) {
  .ltable { min-width: 0; }
  .ltable thead { display: none; }
  .ltable tr { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 4px 8px; padding: 8px 0; border-bottom: 1px solid var(--surface-border); }
  .ltable td { border: 0; padding: 0; }
  .ltable td:first-child, .ltable td:last-child { grid-column: 1 / -1; }
  .ltable .num { text-align: left; }
  .ltable td[data-label]::before { content: attr(data-label) ": "; color: var(--text-color-secondary); font-size: 12px; }
}
</style>
