<template>
  <div class="viewer" data-testid="report">
    <div v-if="origin" class="origin" data-testid="origin">
      <button type="button" class="link" @click="$emit('back')" data-testid="return-origin">← Back to {{ originPageLabel }}</button>
      <span class="text-xs opacity-80">Opened from {{ origin.label }}. The origin is context only: this execution is a report of its own.</span>
    </div>

    <div class="head mt-2">
      <div>
        <div class="text-xs opacity-80">Combined execution report</div>
        <h2 class="title">Timeline &amp; Sequence</h2>
        <div class="text-sm opacity-80" data-testid="execution-heading">
          <template v-if="entry">{{ entry.name }} · {{ entry.service }}<template v-if="entry.parentSpanId !== ROOT_PARENT"> · entry span not recorded (first loaded span shown)</template></template>
          <template v-else>trace evidence</template>
        </div>
        <div class="text-xs mt-1">trace <code data-testid="trace-id">{{ traceId }}</code></div>
      </div>
      <div class="actions">
        <PrimeButton type="button" size="small" outlined label="Investigation link" icon="pi pi-link" @click="copyLink" data-testid="copy-link"/>
        <PrimeButton type="button" size="small" outlined label="Export JSON" :disabled="!report" @click="exportAs('json')" data-testid="export-json"/>
        <PrimeButton type="button" size="small" outlined label="Export HTML" :disabled="!report" @click="exportAs('html')" data-testid="export-html"/>
      </div>
    </div>
    <div v-if="linkText" class="state mt-2" data-testid="link-fallback">
      {{ linkCopied ? 'Link copied. ' : 'The clipboard is not available: select and copy the link. ' }}
      <input class="link-input" readonly :value="linkText" aria-label="Investigation link" @focus="($event.target as HTMLInputElement).select()" data-testid="link-text"/>
      <div class="text-xs opacity-80 mt-1">It holds the trace, the window and the selection only; opening it generates the report again, so it can include evidence that arrived later.</div>
    </div>

    <div v-if="context" class="mt-2 p-2 border rounded text-sm" data-testid="report-context">
      Opened from the comparison <b>{{ context.analysisTitle }}</b> (revision <code>{{ context.analysisId }}</code>, generated {{ formatNs(context.analysisGeneratedAt) }})
      as an {{ context.example }} · {{ context.population }}.
      This report is generated now: it can include evidence that arrived after that comparison, which keeps its own values. Widening the window here does not change the comparison.
      <div class="mt-1">
        <PrimeButton v-if="!included" type="button" label="Include in comparison export" icon="pi pi-plus" size="small" :disabled="!report"
          @click="include" data-testid="include-report"/>
        <Tag v-else severity="success" value="Included in the comparison export" data-testid="report-included"/>
      </div>
    </div>

    <div class="flex flex-wrap items-center gap-2 mt-2 text-sm">
      <span data-testid="report-scope">Scope: one trace within {{ formatNs(reportWindow.from) }} – {{ formatNs(reportWindow.to) }} [from, to).</span>
      <span v-if="report" data-testid="generated-at">Revision generated at {{ formatNs(report.generatedAt) }}.</span>
      <PrimeButton type="button" label="Regenerate" icon="pi pi-refresh" size="small" text @click="generate" data-testid="regenerate"/>
      <PrimeButton type="button" label="Widen by 1 h on each side" size="small" text @click="widen" data-testid="widen-window"/>
    </div>

    <p v-if="loading && !report" class="mt-3" data-testid="report-loading">Generating the report…</p>
    <div v-if="loadError" class="state bad mt-3" role="alert" data-testid="notice">
      Could not generate the report: {{ loadError }}
      <button type="button" class="link" @click="generate" data-testid="retry">Retry</button>
    </div>

    <template v-if="report">
      <div class="mt-3" data-testid="completeness">
        <Tag :severity="state.complete ? 'success' : 'danger'" :value="state.complete ? 'Complete' : 'Incomplete'"/>
        <span v-if="state.complete" class="ml-2 text-sm">Every source returned all matching records of this trace within the window. That does not prove every event was instrumented or delivered.</span>
        <ul v-else class="mt-1 text-sm text-red-500">
          <li v-for="reason in state.reasons" :key="reason" data-testid="limitation">{{ reason }}</li>
        </ul>
      </div>
      <ul v-if="report.warnings.length" class="mt-2 text-sm text-amber-500">
        <li v-for="w in report.warnings" :key="w.section + w.code" data-testid="report-warning">{{ w.message }}</li>
      </ul>
      <p v-if="originMissing" class="state warn mt-2" data-testid="origin-missing">{{ originMissing }}</p>

      <div class="sections text-sm mt-2">
        <div data-testid="spans-summary">
          <span :class="{ bad: report.spans.status !== 'ok' }">Spans: {{ sectionSummary(report.spans, 'spans') }}</span>
          <span v-if="report.spans.status !== 'ok' && report.spans.error" class="bad" data-testid="spans-error"> {{ report.spans.error }}</span>
          <PrimeButton v-if="report.spans.status !== 'ok'" type="button" label="Retry" size="small" text :loading="retrying.spans" @click="retrySection('spans')" data-testid="retry-spans"/>
        </div>
        <div data-testid="logs-summary">
          <span :class="{ bad: report.logs.status !== 'ok' }">Logs: {{ sectionSummary(report.logs, 'logs') }}</span>
          <span v-if="report.logs.status !== 'ok' && report.logs.error" class="bad" data-testid="logs-error"> {{ report.logs.error }}</span>
          <PrimeButton v-if="report.logs.status !== 'ok'" type="button" label="Retry" size="small" text :loading="retrying.logs" @click="retrySection('logs')" data-testid="retry-logs"/>
        </div>
      </div>
      <p v-if="report.logs.status === 'ok' && report.logs.records.length === 0" class="text-sm mt-1" data-testid="no-logs">{{ NO_LOGS_TEXT }}</p>

      <div class="flex flex-wrap items-center justify-between gap-3 mt-3">
        <div class="text-xs opacity-80 flex flex-wrap gap-3" data-testid="report-summary">
          <span>{{ report.spans.records.length }} spans</span>
          <span>{{ report.logs.records.length }} logs</span>
          <span v-if="tl.totalMs > 1">{{ fmt(tl.totalMs) }} ms from first start to last end</span>
        </div>
        <div v-if="report.spans.records.length" class="seg" role="tablist" aria-label="Execution view">
          <button type="button" role="tab" :aria-selected="view === 'timeline'" :class="{ on: view === 'timeline' }" @click="setView('timeline')" data-testid="view-timeline">Timeline</button>
          <button type="button" role="tab" :aria-selected="view === 'sequence'" :class="{ on: view === 'sequence' }" @click="setView('sequence')" data-testid="view-sequence">Sequence</button>
        </div>
      </div>

      <div class="grid-inv mt-2">
        <div>
          <div v-if="report.spans.status !== 'ok'" class="state bad" data-testid="spans-unavailable">
            The span source is unavailable ({{ report.spans.error || report.spans.status }}): no timeline or sequence is drawn; the logs remain available.
          </div>
          <div v-else-if="report.spans.records.length === 0" class="state warn" data-testid="no-spans">
            {{ NO_SPANS_TEXT }} The logs remain available; no span tree is reconstructed from them.
          </div>
          <template v-else>
            <p v-if="report.spans.truncated" class="state warn" data-testid="spans-truncated">{{ report.spans.shown }} of {{ report.spans.total ?? 'more' }} spans were loaded (limit {{ report.spans.limit }}): the rest are not in this report.</p>
            <SequenceView v-if="view === 'sequence' && seqModel" :model="seqModel" :selected="selected" @select="select" @select-log="selectLog" @show-timing="setView('timeline')"/>
            <TraceTimeline v-else :spans="report.spans.records" :selected="selected" :counts="counts" @select="select"/>
          </template>
        </div>
        <div>
          <div v-if="endpoints" class="seg pair mb-2" role="group" aria-label="Endpoint of the correlated operation" data-testid="pair-switch">
            <button type="button" :aria-pressed="selected === endpoints.client" :class="{ on: selected === endpoints.client }" @click="select(endpoints.client)" data-testid="pair-client">CLIENT · {{ nameOf(endpoints.client) }}</button>
            <button type="button" :aria-pressed="selected === endpoints.server" :class="{ on: selected === endpoints.server }" @click="select(endpoints.server)" data-testid="pair-server">SERVER · {{ nameOf(endpoints.server) }}</button>
          </div>
          <LogInspector :key="report.generatedAt + report.traceId" :report="report" :span="selectedSpan" :initialLog="recordId" :focusLog="focusLog"
            @retry-logs="retrySection('logs')" @scope="logScope = $event"/>
          <PrimeButton v-if="selectedSpan" type="button" icon="pi pi-info-circle" size="small" text :label="evidenceLabel(selectedSpan)"
            title="Events (exceptions) and links of the selected span" @click="evidence = selectedSpan" data-testid="evidence"/>
        </div>
      </div>

      <div v-if="links.length" class="mt-3 text-sm" data-testid="linked-traces">
        <b>Linked traces</b> <span class="text-xs opacity-80">(associations recorded by spans; not loaded, not a dependency)</span>
        <div v-for="(link, i) in links" :key="i" data-testid="linked-trace">
          from {{ link.fromSpanName }}: trace <code>{{ link.traceId }}</code> span <code>{{ link.spanId }}</code>
          <Tag class="ml-1" :severity="link.inspected ? 'success' : 'secondary'" :value="link.inspected ? 'in this trace' : 'not inspected'"/>
          <PrimeButton v-if="!link.inspected" type="button" label="Open its report" size="small" text @click="$emit('open-execution', { traceId: link.traceId, window: { ...reportWindow } })" data-testid="open-linked-report"/>
        </div>
      </div>

      <SpanEvidenceDialog :span="evidence" :reportTraceId="traceId" @close="evidence = null"
        @open-trace="(id: string) => { evidence = null; $emit('open-execution', { traceId: id, window: { ...reportWindow } }) }"/>

      <details class="text-xs mt-3">
        <summary>Evidence and identifiers</summary>
        <p data-testid="scope-boundary">{{ SCOPE_BOUNDARY_TEXT }}</p>
        <p>Span status is what the instrumentation recorded for the operation; it is not a log severity and does not by itself prove the business outcome.</p>
        <p>Report window {{ windowText(report.timeFrom, report.timeTo) }} · revision generated {{ formatNs(report.generatedAt) }}</p>
        <p>Timeline and Sequence are two views of this one revision: switching views or log scope doesn't change it; exports contain every span and log of it.</p>
      </details>
    </template>
  </div>
</template>

<script lang="ts">
import { computed, defineComponent, reactive, ref, type PropType } from 'vue'
import PrimeButton from 'primevue/button'
import Tag from 'primevue/tag'
import TraceTimeline from '@/components/problems/TraceTimeline.vue'
import SequenceView from '@/components/problems/SequenceView.vue'
import LogInspector from '@/components/problems/LogInspector.vue'
import SpanEvidenceDialog from '@/components/SpanEvidenceDialog.vue'
import { type Span } from '@/types/filter'
import logsApi from '@/api/logs.api'
import { Logs } from '@/logs-api/logs.impl'
import { type Report } from '@/types/report'
import { type ReportContext } from '@/types/stats'
import { addMs, formatNs, ROOT_PARENT } from '@/services/spanTree'
import { windowText } from '@/services/comparison'
import {
  NO_LOGS_TEXT, NO_SPANS_TEXT, SCOPE_BOUNDARY_TEXT, completeness, download, exportHtml, exportJson, fromResponse, mergeSections, reportLinks, sectionSummary
} from '@/services/report'
import { initialSelection, spanKey, spanLogCounts, timeline } from '@/services/problems'
import { buildSequence, exportSequenceHtml, pairEndpoints, sequenceExport } from '@/services/sequence'
import {
  PAGES, VIEW_PREFERENCE_KEY, copyText, initialView, investigationLink, type ExecutionView, type Origin
} from '@/services/navigation'

const hourMs = 60 * 60 * 1000

// ExecutionViewer is the focused viewer of Timeline & Sequence: one combined
// report (spans and logs of one trace in an explicit window) shown as
// Timeline or Sequence with the related-log inspector. Every page opens
// executions here; the origin is optional context with a return action.
export default defineComponent({
  name: 'ExecutionViewer',
  components: { PrimeButton, Tag, TraceTimeline, SequenceView, LogInspector, SpanEvidenceDialog },
  props: {
    traceId: { type: String, required: true },
    initialWindow: { type: Object as PropType<{ from: string, to: string }>, required: true },
    spanId: { type: String, default: '' },
    // The log the execution was opened for: selected, never a filter.
    recordId: { type: String, default: '' },
    initialViewChoice: { type: String as PropType<ExecutionView | undefined>, default: undefined },
    origin: { type: Object as PropType<Origin | null>, default: null },
    context: { type: Object as PropType<ReportContext | null>, default: null },
    included: { type: Boolean, default: false }
  },
  emits: ['back', 'open-execution', 'include'],
  setup (props, { emit }) {
    logsApi.init(new Logs())

    const savedView = (): string | null => { try { return window.localStorage.getItem(VIEW_PREFERENCE_KEY) } catch { return null } }
    const view = ref<ExecutionView>(initialView(props.initialViewChoice, savedView()))
    function setView (v: ExecutionView) {
      view.value = v
      try { window.localStorage.setItem(VIEW_PREFERENCE_KEY, v) } catch { /* a convenience only */ }
    }

    const reportWindow = ref({ ...props.initialWindow })
    const report = ref<Report | null>(null)
    const loading = ref(false)
    const loadError = ref('')
    const retrying = reactive({ spans: false, logs: false })
    const selected = ref<string | null>(null)
    const focusLog = ref<{ recordId: string } | null>(null)
    const logScope = ref('span')
    const originMissing = ref('')
    const linkText = ref('')
    const linkCopied = ref(false)
    const evidence = ref<Span | null>(null)
    function evidenceLabel (s: Span) {
      const events = s.events?.length ?? 0
      const links = s.links?.length ?? 0
      return `${events} ${events === 1 ? 'event' : 'events'} · ${links} ${links === 1 ? 'link' : 'links'}`
    }

    // generation is bumped by every (re)generation; a response for an older
    // one, or a section retry started before it, is dropped.
    let generation = 0
    let selectionApplied = false

    async function generate () {
      const mine = ++generation
      loading.value = true
      loadError.value = ''
      const [error, response] = await logsApi.traceReport({ traceId: props.traceId, timeFrom: reportWindow.value.from, timeTo: reportWindow.value.to })
      if (mine !== generation) {
        return
      }
      loading.value = false
      const generated = response ? fromResponse(response) : null
      if (error !== null || generated === null || generated.traceId !== props.traceId) {
        loadError.value = error ?? 'invalid response'
        return
      }
      if (!selectionApplied) {
        selectionApplied = true
        applyOriginSelection(generated)
      } else {
        selected.value = initialSelection(generated, '', selected.value)
      }
      report.value = generated
    }

    // applyOriginSelection selects the evidence the execution was opened
    // for; if it is not in this report that is said, and nothing else is
    // selected in its place.
    function applyOriginSelection (r: Report) {
      originMissing.value = ''
      let spanId = props.spanId
      if (props.recordId) {
        const log = r.logs.records.find((l) => l.recordId === props.recordId)
        if (!log) {
          originMissing.value = r.logs.status === 'ok'
            ? 'The log this execution was opened for is not in this report (outside the window or expired).'
            : 'The log this execution was opened for can\'t be shown: the log source is unavailable.'
        } else if (!spanId && log.spanId) {
          spanId = log.spanId
        }
        if (log) {
          focusLog.value = { recordId: log.recordId }
        }
      }
      selected.value = initialSelection(r, spanId, null)
      if (spanId && selected.value === null && !originMissing.value) {
        originMissing.value = r.spans.status === 'ok'
          ? `The span ${spanId} this execution was opened for is not in this report (outside the window or expired).`
          : 'The selected span can\'t be shown: the span source is unavailable.'
      }
    }

    async function retrySection (section: 'spans' | 'logs') {
      const current = report.value
      if (current === null) {
        return
      }
      const mine = generation
      retrying[section] = true
      const [error, response] = await logsApi.traceReport({ traceId: current.traceId, timeFrom: current.timeFrom, timeTo: current.timeTo, sections: [section] })
      retrying[section] = false
      if (mine !== generation || report.value !== current || response === null || error !== null || response.traceId !== current.traceId) {
        return
      }
      report.value = mergeSections(current, response)
    }

    function widen () {
      reportWindow.value = { from: addMs(reportWindow.value.from, -hourMs), to: addMs(reportWindow.value.to, hourMs) }
      generate()
    }

    function include () {
      if (report.value) {
        emit('include', report.value)
      }
    }

    const state = computed(() => report.value ? completeness(report.value) : { complete: false, reasons: [] })
    const links = computed(() => report.value ? reportLinks(report.value) : [])
    const counts = computed(() => report.value ? spanLogCounts(report.value) : new Map())
    const tl = computed(() => timeline(report.value?.spans.records ?? []))
    const selectedSpan = computed(() => report.value?.spans.records.find((s) => spanKey(s) === selected.value) ?? null)
    // The entry span: the recorded root, else the earliest loaded span
    // (labeled, never presented as the root).
    const entry = computed(() => {
      const spans = report.value?.spans.records ?? []
      return spans.find((s) => s.parentSpanId === ROOT_PARENT) ??
        [...spans].sort((a, b) => (BigInt(a.timeStamp) < BigInt(b.timeStamp) ? -1 : 1))[0] ?? null
    })
    const seqModel = computed(() => report.value && report.value.spans.records.length ? buildSequence(report.value) : null)
    const endpoints = computed(() => seqModel.value ? pairEndpoints(seqModel.value, selected.value) : null)
    const nameOf = (key: string) => report.value?.spans.records.find((s) => spanKey(s) === key)?.name ?? key
    function select (key: string) {
      selected.value = key
    }
    function selectLog (e: { spanKey: string, recordId: string }) {
      selected.value = e.spanKey
      focusLog.value = { recordId: e.recordId }
    }

    async function copyLink () {
      const span = selectedSpan.value?.spanId
      linkText.value = investigationLink(window.location.href, {
        traceId: props.traceId, window: { ...reportWindow.value }, spanId: span, view: view.value
      })
      linkCopied.value = await copyText(linkText.value)
    }

    // Exports the displayed revision as generated; it never queries again.
    function exportAs (format: 'html' | 'json') {
      if (!report.value) {
        return
      }
      const now = new Date()
      const name = `trace-${report.value.traceId}-report`
      const viewState = { view: view.value, selected: selected.value, showInternal: true, logScope: logScope.value }
      if (format === 'json') {
        const base = exportJson(report.value, now)
        const data = seqModel.value ? { ...base, presentation: sequenceExport(seqModel.value, report.value, viewState) } : base
        download(name + '.json', 'application/json', JSON.stringify(data, null, 2))
      } else if (seqModel.value) {
        download(name + '.html', 'text/html', exportSequenceHtml(report.value, seqModel.value, viewState, now))
      } else {
        download(name + '.html', 'text/html', exportHtml(report.value, now))
      }
    }

    const originPageLabel = computed(() => PAGES.find((p) => p.value === props.origin?.page)?.label ?? 'the previous page')
    const fmt = (v: number) => v.toFixed(v >= 10 ? 0 : 1)

    generate()

    return {
      view, setView, reportWindow, report, loading, loadError, retrying, selected, select, selectLog, focusLog, logScope, originMissing,
      generate, retrySection, widen, include, state, links, counts, tl, selectedSpan, entry, seqModel, endpoints, nameOf,
      copyLink, linkText, linkCopied, evidence, evidenceLabel, exportAs, originPageLabel, fmt, formatNs, windowText, sectionSummary,
      ROOT_PARENT, NO_LOGS_TEXT, NO_SPANS_TEXT, SCOPE_BOUNDARY_TEXT
    }
  }
})
</script>

<style scoped>
.viewer { font-size: 14px; }
.head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; flex-wrap: wrap; }
.title { font-size: 25px; font-weight: 500; margin: 2px 0; }
.actions { display: flex; gap: 6px; flex-wrap: wrap; }
.origin { display: flex; gap: 12px; align-items: baseline; flex-wrap: wrap; }
.sections { display: flex; flex-direction: column; gap: 2px; }
.bad { color: #fca5a5; }
.seg { display: inline-flex; border: 1px solid var(--surface-border); border-radius: 8px; padding: 3px; gap: 2px; }
.seg button { background: none; border: 0; color: var(--text-color-secondary); padding: 6px 14px; border-radius: 6px; cursor: pointer; font: inherit; }
.seg button.on { background: var(--surface-card); color: var(--text-color); font-weight: 600; }
.seg button:focus-visible { outline: 2px solid var(--primary-color); }
.seg.pair { display: flex; }
.seg.pair button { flex: 1; font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.mb-2 { margin-bottom: 8px; }
.grid-inv { display: grid; grid-template-columns: minmax(0, 1.65fr) minmax(0, 1fr); gap: 16px; align-items: start; }
@media (max-width: 850px) { .grid-inv { grid-template-columns: minmax(0, 1fr); } }
.state { padding: 8px; border-radius: 6px; font-size: 13px; background: var(--surface-ground); }
.state.bad { background: rgba(248, 113, 113, 0.12); color: #fca5a5; }
.state.warn { background: rgba(251, 191, 36, 0.12); color: #fcd34d; }
.link { background: none; border: 0; color: var(--primary-color); cursor: pointer; padding: 0; font: inherit; }
.link-input { width: 100%; margin-top: 4px; background: var(--surface-card); color: var(--text-color); border: 1px solid var(--surface-border); border-radius: 6px; padding: 6px; font: inherit; font-size: 12px; }
</style>
