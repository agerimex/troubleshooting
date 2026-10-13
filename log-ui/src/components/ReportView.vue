<template>
  <div class="card w-full" data-testid="report">
    <div class="flex flex-wrap items-center gap-2">
      <PrimeButton type="button" label="Back to search" icon="pi pi-arrow-left" size="small" text @click="$emit('back')" data-testid="back-to-search"/>
      <span class="text-lg">Investigation report · trace <code data-testid="trace-id">{{ traceId }}</code></span>
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
      <span v-if="report" data-testid="generated-at">Generated at {{ formatNs(report.generatedAt) }}.</span>
      <PrimeButton type="button" label="Regenerate" icon="pi pi-refresh" size="small" text @click="generate" data-testid="regenerate"/>
      <PrimeButton type="button" label="Widen by 1 h on each side" size="small" text @click="widen" data-testid="widen-window"/>
      <PrimeButton type="button" label="Export HTML" icon="pi pi-download" size="small" text :disabled="!report" @click="exportAs('html')" data-testid="export-html"/>
      <PrimeButton type="button" label="Export JSON" icon="pi pi-download" size="small" text :disabled="!report" @click="exportAs('json')" data-testid="export-json"/>
    </div>

    <p v-if="loading && !report" class="mt-3">Generating the report…</p>
    <div v-if="loadError" class="flex items-center gap-2 mt-3 text-red-600" data-testid="notice">
      <span>Could not generate the report: {{ loadError }}</span>
      <PrimeButton type="button" label="Retry" size="small" text @click="generate" data-testid="retry"/>
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
      <p class="mt-2 text-sm italic" data-testid="scope-boundary">{{ SCOPE_BOUNDARY_TEXT }}</p>

      <!-- Spans -->
      <h2 class="mt-4 text-lg font-bold">Spans <span class="text-sm font-normal">(source: traces)</span></h2>
      <div class="flex items-center gap-2 text-sm" data-testid="spans-summary">
        <span :class="{ 'text-red-500': report.spans.status !== 'ok' }">{{ sectionSummary(report.spans, 'spans') }}</span>
        <span v-if="report.spans.status !== 'ok' && report.spans.error" class="text-red-500" data-testid="spans-error">{{ report.spans.error }}</span>
        <PrimeButton v-if="report.spans.status !== 'ok'" type="button" label="Retry" size="small" text :loading="retrying.spans" @click="retrySection('spans')" data-testid="retry-spans"/>
      </div>
      <p v-if="report.spans.status === 'ok' && report.spans.records.length === 0" class="mt-2" data-testid="no-spans">{{ NO_SPANS_TEXT }}</p>
      <p v-if="report.spans.status === 'ok' && report.spans.records.length > 0" class="text-xs mt-1">
        Span status is what the instrumentation recorded for the operation; it is not a log severity and does not by itself prove the business outcome.
      </p>
      <TreeTable v-if="report.spans.records.length > 0" :value="tree" v-model:expandedKeys="expandedKeys" class="mt-2" data-testid="span-tree">
        <Column field="humanTime" header="time" style="width: 16rem" :expander="true">
          <template #body="{ node }">
            <span class="whitespace-nowrap">{{ node.data.humanTime }}</span>
          </template>
        </Column>
        <Column header="operation">
          <template #body="{ node }">
            <span data-testid="span-name" :class="{ 'font-bold': focused && node.key === focusKey }">{{ node.data.name }}</span>
            <Tag v-if="node.data.parentMissing" class="ml-2" severity="warning" :value="node.data.parentMissing" data-testid="parent-missing"/>
          </template>
        </Column>
        <Column header="service">
          <template #body="{ node }">{{ node.data.service }}</template>
        </Column>
        <Column header="version">
          <template #body="{ node }">
            <span data-testid="span-version" :class="{ 'italic opacity-70': versionOf(node) === null }">{{ versionOf(node) ?? 'unavailable' }}</span>
          </template>
        </Column>
        <Column header="span status" style="width: 8rem">
          <template #body="{ node }">
            <Tag :value="node.data.status" :severity="spanStatusSeverity(node.data.status)" data-testid="span-status"/>
          </template>
        </Column>
        <Column header="duration, ms" bodyStyle="text-align: right">
          <template #body="{ node }">{{ node.data.duration }}</template>
        </Column>
        <Column header="" style="width: 16rem">
          <template #body="{ node }">
            <PrimeButton type="button" icon="pi pi-list" size="small" text :label="logCountLabel(node)" title="Show the logs of this span" @click="focus(node.key)" data-testid="focus-span"/>
            <PrimeButton type="button" icon="pi pi-info-circle" size="small" text :label="evidenceLabel(node.data)" title="Events and links" @click="evidence = spanOf(node.key)" data-testid="evidence"/>
          </template>
        </Column>
      </TreeTable>

      <div v-if="links.length" class="mt-3 text-sm" data-testid="linked-traces">
        <b>Linked traces</b>
        <div v-for="(link, i) in links" :key="i" data-testid="linked-trace">
          from {{ link.fromSpanName }}: trace <code>{{ link.traceId }}</code> span <code>{{ link.spanId }}</code>
          <Tag class="ml-1" :severity="link.inspected ? 'success' : 'secondary'" :value="link.inspected ? 'in this trace' : 'not inspected'"/>
          <PrimeButton v-if="!link.inspected" type="button" label="Open its report" size="small" text @click="$emit('open-report', { traceId: link.traceId, window: { ...reportWindow } })" data-testid="open-linked-report"/>
        </div>
      </div>

      <!-- Logs -->
      <h2 class="mt-4 text-lg font-bold">Log records <span class="text-sm font-normal">(source: logs)</span></h2>
      <div class="flex items-center gap-2 text-sm" data-testid="logs-summary">
        <span :class="{ 'text-red-500': report.logs.status !== 'ok' }">{{ sectionSummary(report.logs, 'logs') }}</span>
        <span v-if="report.logs.status !== 'ok' && report.logs.error" class="text-red-500" data-testid="logs-error">{{ report.logs.error }}</span>
        <PrimeButton v-if="report.logs.status !== 'ok'" type="button" label="Retry" size="small" text :loading="retrying.logs" @click="retrySection('logs')" data-testid="retry-logs"/>
      </div>
      <div v-if="focused" class="mt-2 p-2 border rounded" data-testid="focused-span">
        Logs of span <b>{{ focused.name }}</b> · service {{ focused.service }} · operation {{ focused.name }} ·
        span status <Tag :value="focused.status" :severity="spanStatusSeverity(focused.status)"/> · {{ durationMsText(focused.duration) }} ms
        <PrimeButton type="button" label="Show all logs of this trace" size="small" text @click="focusKey = null" data-testid="show-all-logs"/>
      </div>
      <p v-if="report.logs.status === 'ok' && report.logs.records.length === 0" class="mt-2" data-testid="no-logs">{{ NO_LOGS_TEXT }}</p>
      <p v-else-if="focused && visibleLogs.length === 0" class="mt-2" data-testid="no-span-logs">No log record of this report recorded this span's identity.</p>
      <table v-if="visibleLogs.length" class="w-full mt-2 text-sm log-table" data-testid="report-logs">
        <thead>
          <tr><th>time</th><th>log severity</th><th>service</th><th>association</th><th>message</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="log in visibleLogs" :key="log.recordId" :class="{ selected: log.recordId === highlightRecordId }"
              :data-selected="log.recordId === highlightRecordId" data-testid="report-log-row">
            <td class="whitespace-nowrap">{{ formatNs(log.timeUnixNano) }}</td>
            <td><Tag :value="log.severity || 'not recorded'" :severity="logSeverity(log.severity)" data-testid="log-severity"/></td>
            <td data-testid="log-service">{{ log.service }}</td>
            <td data-testid="log-association" :class="{ 'italic text-amber-500': association(log, report).kind !== 'span' }">{{ association(log, report).label }}</td>
            <td class="message" data-testid="log-message">{{ log.message }}</td>
            <td><PrimeButton type="button" icon="pi pi-eye" size="small" text title="Inspect" @click="inspected = log" data-testid="inspect-report-log"/></td>
          </tr>
        </tbody>
      </table>
    </template>

    <SpanEvidenceDialog :span="evidence" :reportTraceId="traceId" @close="evidence = null"
      @open-trace="(id: string) => { evidence = null; $emit('open-report', { traceId: id, window: { ...reportWindow } }) }"/>

    <PrimeDialog :visible="inspected !== null" @update:visible="(v: boolean) => { if (!v) inspected = null }" header="Log record" :style="{ width: '56rem' }" data-testid="report-log-dialog">
      <div v-if="inspected && report">
        <p><Tag :value="inspected.severity || 'not recorded'" :severity="logSeverity(inspected.severity)"/> {{ formatNs(inspected.timeUnixNano) }} · {{ inspected.service }}</p>
        <pre class="message">{{ inspected.message }}</pre>
        <p class="text-sm">record <code>{{ inspected.recordId }}</code> · trace <code>{{ inspected.traceId }}</code> · {{ association(inspected, report).label }}</p>
        <table class="text-sm mt-2">
          <tr v-for="(value, key) in inspected.attributes" :key="key">
            <td class="pr-4 align-top">{{ key }}</td><td>{{ value }}</td>
          </tr>
        </table>
      </div>
      <template #footer>
        <PrimeButton @click="inspected = null" label="Close" />
      </template>
    </PrimeDialog>
  </div>
</template>

<script lang="ts">
import { computed, defineComponent, reactive, ref, type PropType } from 'vue'
import TreeTable from 'primevue/treetable'
import Column from 'primevue/column'
import PrimeButton from 'primevue/button'
import PrimeDialog from 'primevue/dialog'
import Tag from 'primevue/tag'
import SpanEvidenceDialog from '@/components/SpanEvidenceDialog.vue'
import logsApi from '@/api/logs.api'
import { Logs } from '@/logs-api/logs.impl'
import { type Span } from '@/types/filter'
import { type LogRecord, type Report } from '@/types/report'
import { type ReportContext } from '@/types/stats'
import { addMs, buildTraceTree, durationMsText, expandAll, formatNs, spanKey, type SpanNodeData, type TreeNode } from '@/services/spanTree'
import {
  NO_LOGS_TEXT, NO_SPANS_TEXT, SCOPE_BOUNDARY_TEXT, association, completeness, download, exportHtml, exportJson,
  fromResponse, logsOfSpan, mergeSections, reportLinks, sectionSummary, serviceVersion, sortedLogs
} from '@/services/report'
import { logSeverity, spanStatusSeverity } from '@/services/severity'

const hourMs = 60 * 60 * 1000

export interface ReportTarget {
  traceId: string
  window: { from: string, to: string }
  recordId?: string
  spanId?: string
  context?: ReportContext
}

export default defineComponent({
  name: 'ReportView',
  components: { TreeTable, Column, PrimeButton, PrimeDialog, Tag, SpanEvidenceDialog },
  props: {
    traceId: { type: String, required: true },
    initialWindow: { type: Object as PropType<{ from: string, to: string }>, required: true },
    // The log the report was opened from; it is highlighted, never a filter.
    highlightRecordId: { type: String, default: '' },
    // The span the report was opened for (an example); it is focused once.
    focusSpanId: { type: String, default: '' },
    // The comparison the report was opened from, if any.
    context: { type: Object as PropType<ReportContext | null>, default: null },
    included: { type: Boolean, default: false }
  },
  emits: ['back', 'open-report', 'include'],
  setup (props, { emit }) {
    logsApi.init(new Logs())

    const reportWindow = ref({ ...props.initialWindow })
    const report = ref<Report | null>(null)
    const loading = ref(false)
    const loadError = ref('')
    const retrying = reactive({ spans: false, logs: false })
    const focusKey = ref<string | null>(null)
    const expandedKeys = ref<Record<string, boolean>>({})
    const evidence = ref<Span | null>(null)
    const inspected = ref<LogRecord | null>(null)

    // generation is bumped by every (re)generation; a response for an older
    // one, or a section retry started before it, is dropped.
    let generation = 0

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
      report.value = generated
      if (focusKey.value && !spanOf(focusKey.value)) {
        focusKey.value = null
      }
      expandedKeys.value = expandAll(tree.value)
      if (!focusApplied && props.focusSpanId) {
        focusApplied = true
        const key = spanKey(props.traceId, props.focusSpanId)
        if (spanOf(key)) {
          focusKey.value = key
        }
      }
    }
    let focusApplied = false

    // include hands the report, exactly as generated, to the comparison export.
    function include () {
      if (report.value) {
        emit('include', report.value)
      }
    }

    async function retrySection (section: 'spans' | 'logs') {
      if (report.value === null) {
        return
      }
      const mine = generation
      retrying[section] = true
      const [error, response] = await logsApi.traceReport({
        traceId: report.value.traceId, timeFrom: report.value.timeFrom, timeTo: report.value.timeTo, sections: [section]
      })
      retrying[section] = false
      if (mine !== generation || report.value === null || response === null || error !== null || response.traceId !== report.value.traceId) {
        return
      }
      report.value = mergeSections(report.value, response)
      expandedKeys.value = expandAll(tree.value)
    }

    function widen () {
      reportWindow.value = { from: addMs(reportWindow.value.from, -hourMs), to: addMs(reportWindow.value.to, hourMs) }
      generate()
    }

    const tree = computed<TreeNode[]>(() => report.value ? buildTraceTree(report.value.spans.records, report.value.spans.truncated) : [])
    const state = computed(() => report.value ? completeness(report.value) : { complete: false, reasons: [] })
    const links = computed(() => report.value ? reportLinks(report.value) : [])

    function spanOf (key: string): Span | null {
      return report.value?.spans.records.find((s) => spanKey(s.traceId, s.spanId) === key) ?? null
    }
    const focused = computed(() => focusKey.value ? spanOf(focusKey.value) : null)
    function focus (key: string) {
      focusKey.value = key
    }

    const visibleLogs = computed(() => {
      if (!report.value) {
        return []
      }
      return focused.value ? logsOfSpan(report.value, focused.value) : sortedLogs(report.value.logs.records)
    })

    const logCounts = computed(() => {
      const counts = new Map<string, number>()
      for (const log of report.value?.logs.records ?? []) {
        if (log.spanId) {
          const key = spanKey(log.traceId, log.spanId)
          counts.set(key, (counts.get(key) ?? 0) + 1)
        }
      }
      return counts
    })
    function logCountLabel (node: TreeNode) {
      const n = logCounts.value.get(node.key) ?? 0
      return n === 1 ? '1 log' : `${n} logs`
    }

    function versionOf (node: TreeNode) {
      const span = spanOf(node.key)
      return span ? serviceVersion(span) : null
    }

    function evidenceLabel (data: SpanNodeData) {
      const parts = []
      if (data.events.length > 0) {
        parts.push(data.events.length + (data.events.length === 1 ? ' event' : ' events'))
      }
      if (data.links.length > 0) {
        parts.push(data.links.length + (data.links.length === 1 ? ' link' : ' links'))
      }
      return parts.join(' · ')
    }

    // Exports the displayed report as generated; it never queries again.
    function exportAs (format: 'html' | 'json') {
      if (!report.value) {
        return
      }
      const now = new Date()
      const name = `trace-${report.value.traceId}-report`
      if (format === 'json') {
        download(name + '.json', 'application/json', JSON.stringify(exportJson(report.value, now), null, 2))
      } else {
        download(name + '.html', 'text/html', exportHtml(report.value, now))
      }
    }

    generate()

    return {
      reportWindow, report, loading, loadError, retrying, focusKey, focused, expandedKeys, evidence, inspected, tree, state,
      links, visibleLogs, generate, retrySection, include, widen, focus, spanOf, logCountLabel, versionOf, evidenceLabel, exportAs,
      association, sectionSummary, formatNs, durationMsText, logSeverity, spanStatusSeverity, NO_LOGS_TEXT, NO_SPANS_TEXT, SCOPE_BOUNDARY_TEXT
    }
  }
})
</script>

<style scoped>
.log-table th, .log-table td {
  text-align: left;
  vertical-align: top;
  padding: 0.25rem 0.5rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}
.message {
  white-space: pre-wrap;
  word-break: break-word;
}
tr.selected td {
  background: rgba(251, 191, 36, 0.15);
}
</style>
