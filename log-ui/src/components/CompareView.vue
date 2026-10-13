<template>
  <div class="card w-full" data-testid="compare">
    <div class="grid-2">
      <div>
        <h3 class="font-bold">Baseline</h3>
        <PopulationForm v-model="baseline" testid="baseline"/>
      </div>
      <div>
        <h3 class="font-bold">Candidate</h3>
        <PopulationForm v-model="candidate" testid="candidate"/>
      </div>
    </div>
    <div class="flex flex-wrap items-center gap-2 mt-2">
      <PrimeButton type="button" label="Compare candidate against baseline" icon="pi pi-sort-alt" size="small"
        :disabled="!baseline.service || !candidate.service" :loading="comparing" @click="compare()" data-testid="compare-run"/>
      <PrimeButton type="button" label="Copy baseline filters to candidate" size="small" text @click="copyFilters" data-testid="compare-copy"/>
      <PrimeButton type="button" label="Swap" size="small" text @click="swap" data-testid="compare-swap"/>
      <span class="text-xs opacity-80">Version text has no order: you choose which population is the baseline.</span>
    </div>
    <div class="flex flex-wrap items-center gap-2 mt-2 text-sm">
      <span>Saved comparisons:</span>
      <Dropdown v-model="savedId" :options="saved" optionLabel="label" optionValue="id" placeholder="open a saved comparison" class="w-96" style="max-width: 48rem"
        @update:modelValue="openSaved" data-testid="saved-analyses"/>
      <PrimeButton type="button" icon="pi pi-refresh" size="small" text title="Reload the list" @click="loadSaved" data-testid="saved-reload"/>
    </div>
    <p v-if="error" class="mt-2 text-red-500" data-testid="compare-error">The comparison could not be generated: {{ error }}</p>

    <template v-if="analysis">
      <div class="mt-4 p-2 border rounded" data-testid="analysis">
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-lg font-bold" data-testid="analysis-title">{{ analysis.title }}</span>
        </div>
        <div class="text-sm" data-testid="analysis-revision">
          Revision <code>{{ analysis.id }}</code><span v-if="analysis.regenerates"> · regenerates <code>{{ analysis.regenerates }}</code></span>
          · generated {{ formatNs(analysis.generatedAt) }} · rules {{ analysis.rules.version }} · {{ analysis.contracts.metrics }}, {{ analysis.contracts.errors }}
        </div>
        <div class="flex flex-wrap items-center gap-2 mt-1">
          <PrimeButton type="button" label="Regenerate (new revision)" icon="pi pi-refresh" size="small" text @click="regenerate" data-testid="analysis-regenerate"/>
          <PrimeButton type="button" label="Export HTML" icon="pi pi-download" size="small" text @click="exportAs('html')" data-testid="analysis-export-html"/>
          <PrimeButton type="button" label="Export JSON" icon="pi pi-download" size="small" text @click="exportAs('json')" data-testid="analysis-export-json"/>
          <span class="text-xs">Exports contain this revision exactly as shown and {{ includedHere.length }} selected report(s).</span>
        </div>
        <p class="mt-1 text-sm italic" data-testid="analysis-interpretation">{{ analysis.interpretation }}</p>

        <table class="w-full mt-2 text-sm cmp-table" data-testid="analysis-populations">
          <thead><tr><th></th><th>population</th><th>window</th><th>status</th><th>coverage</th><th>values from</th><th>cutoff</th></tr></thead>
          <tbody>
            <tr v-for="side in sides" :key="side.name" :data-testid="'population-' + side.name">
              <td><b>{{ side.name }}</b></td>
              <td>{{ side.stats.label }}</td>
              <td>{{ windowText(side.stats.population.time_from ?? '', side.stats.population.time_to ?? '') }} ({{ durationText(side.stats.durationSeconds) }})</td>
              <td :class="{ 'text-amber-500': side.stats.status !== 'ok' }">{{ statsStatusText(side.stats) }}</td>
              <td>{{ side.stats.coverage.status }}</td>
              <td>{{ planSummary(side.stats.plan) }}</td>
              <td>{{ formatNs(side.stats.cutoff) }}</td>
            </tr>
          </tbody>
        </table>
        <ul class="mt-1 text-sm">
          <li v-for="m in analysis.mismatches" :key="m.dimension" class="text-red-500" data-testid="analysis-mismatch">
            The populations differ in {{ m.dimension }} (baseline {{ m.baseline }}, candidate {{ m.candidate }}): conclusions for {{ m.affects.join(', ') }} are unavailable until reconciled.
          </li>
          <li v-for="n in allNotices" :key="n" class="text-amber-500" data-testid="analysis-notice">{{ n }}</li>
        </ul>

        <div v-for="(row, i) in analysis.rows" :key="row.operation + row.spanKind" class="mt-3" data-testid="analysis-row">
          <div class="font-bold">{{ row.operation || 'operation not recorded' }} <span class="font-normal text-sm">({{ row.spanKind }})</span></div>
          <p v-for="note in row.notes" :key="note" class="text-sm text-amber-500" data-testid="row-note">{{ note }}</p>
          <table class="w-full text-sm cmp-table">
            <thead><tr><th>metric</th><th>baseline</th><th>candidate</th><th>absolute change</th><th>relative change</th><th>result</th><th>why</th></tr></thead>
            <tbody>
              <tr v-for="m in row.metrics" :key="m.metric" :data-testid="'metric-' + m.metric">
                <td>{{ metricLabel(m.metric) }}</td>
                <td data-testid="metric-baseline">{{ metricValue(row, 'baseline', m.metric, m.baseline) }}</td>
                <td data-testid="metric-candidate">{{ metricValue(row, 'candidate', m.metric, m.candidate) }}</td>
                <td data-testid="metric-absolute">{{ formatAbsolute(m) }}</td>
                <td data-testid="metric-relative">{{ formatRelative(m) }}</td>
                <td><Tag :value="classificationLabel(m.classification)" :severity="classificationSeverity(m.classification)" data-testid="metric-class"/></td>
                <td class="text-xs">{{ m.reasons.join('; ') }}</td>
              </tr>
            </tbody>
          </table>
          <div class="text-xs mt-1" data-testid="row-samples">
            Baseline: {{ sampleText(row.baseline) }} · Candidate: {{ sampleText(row.candidate) }}
          </div>
          <PrimeButton v-if="slowThreshold(row) !== null" type="button" size="small" text data-testid="row-slow-examples"
            :label="'Slow candidate examples ≥ ' + slowThreshold(row)!.toFixed(1) + ' ms'" @click="showSlow(i)"/>
        </div>

        <h3 class="mt-4 font-bold">Error groups</h3>
        <p class="text-xs">Each source kind is counted separately; an ERROR-status span, its exception and an ERROR log can describe the same failure and are never added. Labels describe the selected scopes only: not observed is not "resolved", newly observed is not "first ever".</p>
        <p v-if="analysis.errorGroups.length === 0" data-testid="analysis-no-groups">No error groups were recorded in either scope.</p>
        <table v-else class="w-full text-sm cmp-table" data-testid="analysis-groups">
          <thead><tr><th>source</th><th>group</th><th>operation</th><th>baseline</th><th>candidate</th><th>change</th><th>label</th><th></th></tr></thead>
          <tbody>
            <tr v-for="g in analysis.errorGroups" :key="g.key" data-testid="analysis-group">
              <td>{{ sourceKindLabel(g.sourceKind) }}</td>
              <td class="message" data-testid="analysis-group-title">{{ groupTitle(g) }}</td>
              <td>{{ g.operation || 'not recorded' }}</td>
              <td data-testid="group-baseline">{{ occurrencesText(g.baseline.occurrences) }}<div v-if="g.baseline.firstSeen" class="text-xs">{{ formatNs(g.baseline.firstSeen) }} – {{ formatNs(g.baseline.lastSeen ?? '') }}</div></td>
              <td data-testid="group-candidate">{{ occurrencesText(g.candidate.occurrences) }}<div v-if="g.candidate.firstSeen" class="text-xs">{{ formatNs(g.candidate.firstSeen) }} – {{ formatNs(g.candidate.lastSeen ?? '') }}</div></td>
              <td>{{ g.absoluteChange === null ? 'unavailable' : (g.absoluteChange > 0 ? '+' : '') + g.absoluteChange }}</td>
              <td data-testid="group-label">{{ groupLabelText(g.label) }}<div v-if="g.reason" class="text-xs">{{ g.reason }}</div></td>
              <td class="whitespace-nowrap">
                <PrimeButton type="button" size="small" text label="Candidate examples" :disabled="!g.candidate.occurrences" @click="showGroup(g, 'candidate')" data-testid="group-candidate-examples"/>
                <PrimeButton type="button" size="small" text label="Baseline examples" :disabled="!g.baseline.occurrences" @click="showGroup(g, 'baseline')" data-testid="group-baseline-examples"/>
              </td>
            </tr>
          </tbody>
        </table>

        <ExamplesPanel v-if="examples" :request="examples.request" :title="examples.title" @close="examples = null" @open="openExample"/>

        <h3 class="mt-4 font-bold">Selected reports for the export ({{ includedHere.length }})</h3>
        <p v-if="includedHere.length === 0" class="text-sm">Open an example and choose “Include in comparison export” to add its combined report.</p>
        <ul class="text-sm">
          <li v-for="(r, i) in includedHere" :key="r.report.traceId + r.includedAt" data-testid="included-report">
            trace <code>{{ r.report.traceId }}</code> · {{ r.context.example }} · report generated {{ formatNs(r.report.generatedAt) }}
            <PrimeButton type="button" icon="pi pi-times" size="small" text title="Remove from the export" @click="$emit('remove-included', analysis!.id, i)" data-testid="included-remove"/>
          </li>
        </ul>
      </div>
    </template>
  </div>
</template>

<script lang="ts">
import { computed, defineComponent, ref, type PropType } from 'vue'
import PrimeButton from 'primevue/button'
import Dropdown from 'primevue/dropdown'
import Tag from 'primevue/tag'
import PopulationForm from '@/components/PopulationForm.vue'
import ExamplesPanel from '@/components/ExamplesPanel.vue'
import statsApi from '@/api/stats.api'
import { Stats } from '@/logs-api/stats.impl'
import { formatNs } from '@/services/spanTree'
import { download } from '@/services/report'
import {
  classificationLabel, classificationSeverity, defaultWindow, durationText, exportComparisonHtml, exportComparisonJson, formatAbsolute, formatMetric,
  formatRelative, groupLabelText, groupTitle, metricLabel, occurrencesText, percentileText, planSummary, slowThreshold, sourceKindLabel,
  statsStatusText, windowText, type IncludedReport
} from '@/services/comparison'
import {
  type Analysis, type AnalysisSummary, type CallStats, type ComparisonRow, type Example, type ExamplesRequest, type GroupComparison, type Population
} from '@/types/stats'

// populationOf turns the stored population definition of an analysis back
// into a Population (null = any, '' = not recorded).
function populationOf (p: Record<string, string | null>): Population {
  const opt = (v: string | null | undefined) => (v === null || v === undefined ? undefined : v)
  return {
    service: p.service ?? '', operation: opt(p.operation), spanKind: opt(p.span_kind), environment: opt(p.environment),
    version: opt(p.version), installation: opt(p.installation), segment: opt(p.segment),
    timeFrom: p.time_from ?? '', timeTo: p.time_to ?? ''
  }
}

export default defineComponent({
  name: 'CompareView',
  components: { PrimeButton, Dropdown, Tag, PopulationForm, ExamplesPanel },
  props: {
    // Reports the user included, per analysis id.
    included: { type: Object as PropType<Record<string, IncludedReport[]>>, required: true }
  },
  emits: ['open-report', 'remove-included'],
  setup (props, { emit }) {
    statsApi.init(new Stats())
    const initialWindow = defaultWindow()
    const baseline = ref<Population>({ service: '', ...initialWindow })
    const candidate = ref<Population>({ service: '', ...initialWindow })
    const analysis = ref<Analysis | null>(null)
    const error = ref('')
    const comparing = ref(false)
    const saved = ref<{ id: string, label: string }[]>([])
    const savedId = ref<string | null>(null)
    const examples = ref<{ request: ExamplesRequest, title: string, side: 'baseline' | 'candidate', example: string } | null>(null)
    let seq = 0

    async function loadSaved () {
      const [, list] = await statsApi.analyses()
      saved.value = (list ?? []).map((a: AnalysisSummary) => ({ id: a.id, label: `${formatNs(a.createdAt)} · ${a.title}` }))
    }

    async function compare (regenerates?: string) {
      const mine = ++seq
      comparing.value = true
      error.value = ''
      const [err, a] = await statsApi.compare(baseline.value, candidate.value, regenerates)
      if (mine !== seq) {
        return
      }
      comparing.value = false
      if (err !== null || a === null) {
        error.value = err ?? 'invalid response'
        return
      }
      analysis.value = a
      examples.value = null
      loadSaved()
    }

    // regenerate recomputes the shown analysis' populations as a new revision;
    // the earlier one stays available.
    function regenerate () {
      if (!analysis.value) {
        return
      }
      baseline.value = populationOf(analysis.value.baseline.population)
      candidate.value = populationOf(analysis.value.candidate.population)
      compare(analysis.value.id)
    }

    async function openSaved (id: string | null) {
      if (!id) {
        return
      }
      const mine = ++seq
      const [err, a] = await statsApi.analysis(id)
      if (mine !== seq) {
        return
      }
      if (err !== null || a === null) {
        error.value = err ?? 'invalid response'
        return
      }
      error.value = ''
      analysis.value = a
      examples.value = null
      baseline.value = populationOf(a.baseline.population)
      candidate.value = populationOf(a.candidate.population)
    }

    function copyFilters () {
      candidate.value = { ...baseline.value, version: candidate.value.version, timeFrom: candidate.value.timeFrom, timeTo: candidate.value.timeTo }
    }
    function swap () {
      const b = baseline.value
      baseline.value = candidate.value
      candidate.value = b
    }

    const sides = computed(() => analysis.value
      ? [{ name: 'baseline', stats: analysis.value.baseline }, { name: 'candidate', stats: analysis.value.candidate }]
      : [])

    const allNotices = computed(() => {
      if (!analysis.value) {
        return []
      }
      const a = analysis.value
      return [
        ...a.notices.filter((n) => n.code !== 'mismatch').map((n) => n.message),
        ...a.baseline.notices.map((n) => 'Baseline: ' + n.message),
        ...a.candidate.notices.map((n) => 'Candidate: ' + n.message)
      ]
    })

    const includedHere = computed(() => analysis.value ? (props.included[analysis.value.id] ?? []) : [])

    function metricValue (row: ComparisonRow, side: 'baseline' | 'candidate', metric: string, value: number | null): string {
      const stats = row[side]
      if (stats && metric in stats.percentiles) {
        return percentileText(stats.percentiles[metric])
      }
      return formatMetric(metric, value)
    }

    function sampleText (s: CallStats | null): string {
      if (!s) {
        return 'no calls observed'
      }
      return `${s.calls} calls, ${s.validDurations} valid durations` + (s.invalidDurations ? `, ${s.invalidDurations} invalid excluded` : '')
    }

    function populationFor (side: 'baseline' | 'candidate'): Population {
      return populationOf(analysis.value![side].population)
    }

    function showSlow (i: number) {
      const a = analysis.value!
      const row = a.rows[i]
      const threshold = slowThreshold(row)!
      const pop = { ...populationFor('candidate'), operation: row.operation, spanKind: row.spanKind }
      examples.value = {
        request: { population: pop, kind: 'slow', thresholdMs: threshold, cutoff: a.candidate.cutoff },
        title: `Observed slow candidate calls of ${row.operation} at or above the displayed p95 (${threshold.toFixed(1)} ms)`,
        side: 'candidate',
        example: `observed slow call (≥ ${threshold.toFixed(1)} ms, the candidate p95) of ${row.operation}`
      }
    }

    function showGroup (g: GroupComparison, side: 'baseline' | 'candidate') {
      const a = analysis.value!
      examples.value = {
        request: { population: populationFor(side), kind: 'error', group: g, cutoff: a[side].cutoff },
        title: `${side === 'candidate' ? 'Candidate' : 'Baseline'} occurrences of ${groupTitle(g)} (${sourceKindLabel(g.sourceKind)})`,
        side,
        example: `${side} occurrence of the error group ${groupTitle(g)} (${sourceKindLabel(g.sourceKind)})`
      }
    }

    // openExample opens the Milestone 1 report of the example's trace, in the
    // originating population's window, keeping the comparison's context.
    function openExample (ex: Example) {
      const a = analysis.value!
      const side = examples.value!.side
      const pop = a[side].population
      emit('open-report', {
        traceId: ex.traceId,
        window: { from: pop.time_from, to: pop.time_to },
        recordId: ex.recordId,
        spanId: ex.spanId,
        context: {
          analysisId: a.id,
          analysisTitle: a.title,
          analysisGeneratedAt: a.generatedAt,
          example: examples.value!.example,
          population: `${side}: ${a[side].label}`
        }
      })
    }

    function exportAs (format: 'html' | 'json') {
      if (!analysis.value) {
        return
      }
      const now = new Date()
      const name = `comparison-${analysis.value.id}`
      if (format === 'json') {
        download(name + '.json', 'application/json', JSON.stringify(exportComparisonJson(analysis.value, includedHere.value, now), null, 2))
      } else {
        download(name + '.html', 'text/html', exportComparisonHtml(analysis.value, includedHere.value, now))
      }
    }

    loadSaved()

    return {
      baseline, candidate, analysis, error, comparing, saved, savedId, examples, sides, allNotices, includedHere,
      compare, regenerate, openSaved, loadSaved, copyFilters, swap, metricValue, sampleText, showSlow, showGroup, openExample, exportAs,
      formatNs, windowText, durationText, planSummary, statsStatusText, classificationLabel, classificationSeverity, formatAbsolute,
      formatRelative, metricLabel, groupTitle, groupLabelText, sourceKindLabel, occurrencesText, slowThreshold
    }
  }
})
</script>

<style scoped>
.grid-2 { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 1rem; }
@media (max-width: 1200px) { .grid-2 { grid-template-columns: minmax(0, 1fr); } }
.cmp-table th, .cmp-table td { text-align: left; vertical-align: top; padding: 0.25rem 0.5rem; border-bottom: 1px solid rgba(255, 255, 255, 0.1); }
.message { white-space: pre-wrap; word-break: break-word; }
</style>
