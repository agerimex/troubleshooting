<template>
  <div class="card w-full" data-testid="statistics">
    <PopulationForm v-model="population" testid="stats-pop"/>
    <div class="flex flex-wrap items-center gap-3 mt-2">
      <label class="flex items-center gap-1 text-sm"><Checkbox v-model="byVersion" :binary="true" data-testid="stats-by-version"/> one row per version</label>
      <PrimeButton type="button" label="Show statistics" icon="pi pi-chart-bar" size="small" :disabled="!population.service" @click="load" data-testid="stats-load"/>
    </div>
    <p v-if="loading" class="mt-2">Loading statistics…</p>
    <p v-if="error" class="mt-2 text-red-500" data-testid="stats-error">Statistics are unavailable: {{ error }}</p>

    <template v-if="stats">
      <div class="mt-3 text-sm" data-testid="stats-header">
        <div><b>{{ stats.label }}</b> · {{ windowText(stats.population.time_from ?? '', stats.population.time_to ?? '') }} · {{ durationText(stats.durationSeconds) }}</div>
        <div>Values from {{ planSummary(stats.plan) }} · analysis cutoff {{ formatNs(stats.cutoff) }} · {{ stats.definitions.metrics }}, {{ stats.definitions.percentile }}</div>
      </div>
      <div v-if="stats.status !== 'ok'" class="mt-2 text-amber-500" data-testid="stats-status">
        {{ statsStatusText(stats) }}
        <PrimeButton v-if="stats.suggestedFrom && stats.suggestedTo" type="button" size="small" text data-testid="stats-use-whole-buckets"
          :label="'Use the whole hours ' + windowText(stats.suggestedFrom, stats.suggestedTo)" @click="useSuggested"/>
      </div>
      <template v-else>
        <div class="mt-2 flex items-center gap-2 text-sm" data-testid="stats-coverage">
          Collection coverage <Tag :value="stats.coverage.status" :severity="stats.coverage.status === 'complete' ? 'success' : 'warning'"/>
          <span>{{ stats.coverage.reasons.join('; ') }}</span>
        </div>
        <ul class="mt-1 text-sm text-amber-500">
          <li v-for="n in stats.notices" :key="n.code + n.message" data-testid="stats-notice">{{ n.message }}</li>
        </ul>
        <p class="mt-2" v-if="stats.condition !== 'ok'" data-testid="stats-condition">{{ stats.condition }}</p>
        <table class="w-full mt-2 text-sm st-table" v-if="stats.rows.length" data-testid="stats-rows">
          <thead>
            <tr><th>operation</th><th>span kind</th><th v-if="byVersionShown">version</th><th>recorded calls</th><th>rate</th><th>ERROR</th><th>OK</th><th>UNSET</th>
              <th>error fraction</th><th>p50</th><th>p95</th><th>p99</th><th>valid durations</th><th></th></tr>
          </thead>
          <tbody>
            <tr v-for="r in stats.rows" :key="r.operation + r.spanKind + (r.version ?? '-')" data-testid="stats-row">
              <td>{{ r.operation || 'not recorded' }}</td><td>{{ r.spanKind }}</td>
              <td v-if="byVersionShown">{{ r.version === '' ? 'not recorded' : r.version }}</td>
              <td data-testid="stats-calls">{{ r.calls }}</td>
              <td>{{ r.ratePerMinute.toFixed(2) }} /min</td>
              <td>{{ r.errors }}</td><td>{{ r.ok }}</td><td>{{ r.unset }}</td>
              <td data-testid="stats-error-fraction">{{ errorFractionText(r) }}</td>
              <td data-testid="stats-p50">{{ percentileText(r.percentiles.p50) }}</td>
              <td data-testid="stats-p95">{{ percentileText(r.percentiles.p95) }}</td>
              <td data-testid="stats-p99">{{ percentileText(r.percentiles.p99) }}</td>
              <td>{{ r.validDurations }}<span v-if="r.invalidDurations"> ({{ r.invalidDurations }} invalid excluded)</span></td>
              <td><PrimeButton type="button" size="small" text label="Slow examples" :disabled="r.percentiles.p95.valueMs === null"
                @click="showSlow(r)" data-testid="stats-slow-examples"/></td>
            </tr>
          </tbody>
        </table>
        <p class="text-xs mt-1">Recorded calls are canonical spans of each operation and span kind; they are never summed across span kinds. OK and UNSET are span statuses, not customer outcomes. Percentiles need at least 20 (p50), 200 (p95) and 1000 (p99) valid durations — a product heuristic, not a statistical guarantee.</p>

        <h3 class="mt-3 font-bold">Calls per hour (UTC)</h3>
        <table class="text-sm st-table" data-testid="stats-series">
          <thead><tr><th>bucket</th><th>from</th><th>coverage</th><th>recorded calls</th></tr></thead>
          <tbody>
            <tr v-for="b in stats.series" :key="b.from" data-testid="series-bucket">
              <td>{{ windowText(b.from, b.to) }}</td>
              <td>{{ b.source }}{{ b.state ? ' (' + b.state + ')' : '' }}</td>
              <td>{{ b.coverage }}</td>
              <td data-testid="series-calls">{{ seriesText(b) }}</td>
            </tr>
          </tbody>
        </table>
      </template>

      <ExamplesPanel v-if="examples" :request="examples.request" :title="examples.title" @close="examples = null" @open="openExample"/>

      <h3 class="mt-4 font-bold">Error groups</h3>
      <p v-if="groupsError" class="text-red-500" data-testid="groups-error">Error groups are unavailable: {{ groupsError }}</p>
      <template v-if="groups">
        <ul class="text-sm">
          <li v-for="(s, name) in groups.sources" :key="name" :class="{ 'text-red-500': s.status !== 'ok' }" data-testid="groups-source">
            {{ name }}: {{ s.status === 'ok' ? 'available, coverage ' + s.coverage.status : 'unavailable — its groups are missing, not zero (' + (s.error ?? s.status) + ')' }}
          </li>
        </ul>
        <p class="text-xs">{{ groups.definitions.sourceKind }}. Grouping {{ groups.definitions.errors }}: {{ groups.definitions.grouping }}.</p>
        <p v-if="groups.groups.length === 0" class="mt-1" data-testid="groups-empty">{{ groups.condition }}</p>
        <table v-else class="w-full mt-1 text-sm st-table" data-testid="groups">
          <thead><tr><th>source</th><th>group</th><th>grouped by</th><th>operation</th><th>occurrences</th><th>affected traces</th><th>unlinked</th><th>first / last</th><th>versions</th><th></th></tr></thead>
          <tbody>
            <tr v-for="g in groups.groups" :key="g.key" data-testid="group-row">
              <td>{{ sourceKindLabel(g.sourceKind) }}</td>
              <td class="message" data-testid="group-title">{{ groupTitle(g) }}</td>
              <td>{{ g.method === 'exception' ? 'exception type + frame' : 'normalized message' }}</td>
              <td>{{ g.operation || 'not recorded' }}</td>
              <td data-testid="group-occurrences">{{ g.occurrences }}</td>
              <td>{{ g.affectedTraces }}</td>
              <td>{{ g.unlinked }}</td>
              <td class="whitespace-nowrap">{{ formatNs(g.firstSeen) }}<br>{{ formatNs(g.lastSeen) }}</td>
              <td>{{ (g.versions ?? []).map((v) => (v.version || 'not recorded') + ': ' + v.occurrences).join(', ') }}</td>
              <td><PrimeButton type="button" size="small" text label="Examples" @click="showGroup(g)" data-testid="group-examples"/></td>
            </tr>
          </tbody>
        </table>
      </template>
    </template>
  </div>
</template>

<script lang="ts">
import { computed, defineComponent, ref } from 'vue'
import PrimeButton from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Tag from 'primevue/tag'
import PopulationForm from '@/components/PopulationForm.vue'
import ExamplesPanel from '@/components/ExamplesPanel.vue'
import statsApi from '@/api/stats.api'
import { Stats } from '@/logs-api/stats.impl'
import { formatNs } from '@/services/spanTree'
import {
  defaultWindow, durationText, errorFractionText, groupTitle, percentileText, planSummary, sourceKindLabel, statsStatusText, windowText
} from '@/services/comparison'
import { type BucketPoint, type CallStats, type ErrorGroup, type ErrorGroups, type Example, type ExamplesRequest, type Population, type PopulationStats } from '@/types/stats'

export default defineComponent({
  name: 'StatisticsView',
  components: { PrimeButton, Checkbox, Tag, PopulationForm, ExamplesPanel },
  emits: ['open-report'],
  setup (_, { emit }) {
    statsApi.init(new Stats())
    const population = ref<Population>({ service: '', ...defaultWindow() })
    const byVersion = ref(false)
    const byVersionShown = ref(false)
    const stats = ref<PopulationStats | null>(null)
    const groups = ref<ErrorGroups | null>(null)
    const error = ref('')
    const groupsError = ref('')
    const loading = ref(false)
    const examples = ref<{ request: ExamplesRequest, title: string } | null>(null)
    let seq = 0
    let shown: Population = population.value

    async function load () {
      const mine = ++seq
      loading.value = true
      error.value = ''
      groupsError.value = ''
      examples.value = null
      shown = { ...population.value }
      const [[err, s], [gErr, g]] = await Promise.all([
        statsApi.callStats(shown, { groupByVersion: byVersion.value, series: true }),
        statsApi.errorGroups(shown)
      ])
      if (mine !== seq) {
        return
      }
      loading.value = false
      byVersionShown.value = byVersion.value
      stats.value = s
      groups.value = g
      error.value = err ?? ''
      groupsError.value = gErr ?? ''
    }

    function useSuggested () {
      if (stats.value?.suggestedFrom && stats.value.suggestedTo) {
        population.value = { ...population.value, timeFrom: stats.value.suggestedFrom, timeTo: stats.value.suggestedTo }
        load()
      }
    }

    function seriesText (b: BucketPoint): string {
      if (b.source === 'unavailable') {
        return 'unavailable (outside retained coverage)'
      }
      if (!b.available) {
        const why = b.coverage === 'unknown' ? 'collection coverage unknown' : (b.coverage === 'open' ? 'bucket not over yet' : 'coverage gap')
        return (b.calls !== null ? `${b.calls} recorded, ` : '') + why + ' — not a confirmed zero'
      }
      return String(b.calls)
    }

    function showSlow (r: CallStats) {
      const p95 = r.percentiles.p95.valueMs ?? 0
      const pop: Population = { ...shown, operation: r.operation, spanKind: r.spanKind }
      if (r.version !== undefined) {
        pop.version = r.version
      }
      examples.value = {
        request: { population: pop, kind: 'slow', thresholdMs: p95, cutoff: stats.value?.cutoff },
        title: `Observed slow calls ≥ ${p95.toFixed(1)} ms (the displayed p95) of ${r.operation}`
      }
    }

    function showGroup (g: ErrorGroup) {
      examples.value = {
        request: { population: { ...shown }, kind: 'error', group: g, cutoff: groups.value?.cutoff },
        title: `Occurrences of ${groupTitle(g)} (${sourceKindLabel(g.sourceKind)})`
      }
    }

    function openExample (ex: Example) {
      emit('open-report', {
        traceId: ex.traceId,
        window: { from: shown.timeFrom, to: shown.timeTo },
        recordId: ex.recordId,
        spanId: ex.spanId
      })
    }

    const hasStats = computed(() => stats.value !== null)
    return {
      population, byVersion, byVersionShown, stats, groups, error, groupsError, loading, examples, hasStats,
      load, useSuggested, seriesText, showSlow, showGroup, openExample,
      formatNs, windowText, durationText, planSummary, statsStatusText, percentileText, errorFractionText, groupTitle, sourceKindLabel
    }
  }
})
</script>

<style scoped>
.st-table th, .st-table td { text-align: left; vertical-align: top; padding: 0.25rem 0.5rem; border-bottom: 1px solid rgba(255, 255, 255, 0.1); }
.message { white-space: pre-wrap; word-break: break-word; }
</style>
