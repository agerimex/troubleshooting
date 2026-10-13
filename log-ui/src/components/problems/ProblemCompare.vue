<template>
  <div data-testid="problem-compare">
    <p v-if="versionsError" class="state bad" data-testid="compare-versions-error">
      Releases of this operation are unavailable: {{ versionsError }}
      <button type="button" class="link" @click="loadVersions">Retry</button>
    </p>
    <div class="pickers">
      <label class="picker">
        <span class="lbl">Baseline</span>
        <select v-model="baseline" data-testid="compare-baseline" :disabled="running">
          <option value="">Choose a release</option>
          <option v-for="v in versions" :key="'b' + v.version" :value="key(v.version ?? '')">{{ versionText(v.version) }}</option>
        </select>
        <span class="hint" v-if="baselineInfo">{{ baselineInfo }}</span>
      </label>
      <label class="picker">
        <span class="lbl">Candidate</span>
        <select v-model="candidate" data-testid="compare-candidate" :disabled="running">
          <option value="">Choose a release</option>
          <option v-for="v in versions" :key="'c' + v.version" :value="key(v.version ?? '')">{{ versionText(v.version) }}</option>
        </select>
        <span class="hint" v-if="candidateInfo">{{ candidateInfo }}</span>
      </label>
    </div>
    <div class="flex flex-wrap items-center gap-2 mt-2">
      <PrimeButton type="button" label="Compare" size="small" :disabled="!canCompare || running" :loading="running" @click="run()" data-testid="compare-run-releases"/>
      <span class="text-xs opacity-80">Version text has no order: you choose the baseline. Each release is the operation's calls of that version in the selected window.</span>
    </div>
    <p v-if="error" class="state bad" role="alert" data-testid="compare-error">The comparison failed: {{ error }}</p>

    <template v-if="analysis">
      <div class="signals mt-3" data-testid="compare-signals">
        <template v-if="signals.length">
          <span v-for="(s, i) in signals" :key="i" class="chip" :class="s.cls" data-testid="compare-signal">{{ s.text }}</span>
        </template>
        <span v-else class="opacity-80 text-sm" data-testid="compare-no-signal">No change signal under the comparison rules.</span>
      </div>
      <p class="text-xs opacity-80 mt-1">
        {{ analysis.title }} · generated {{ formatNs(analysis.generatedAt) }} · {{ analysis.rules.version }}
        <span v-if="fromStored"> · the stored comparison this problem was detected from</span>
      </p>
      <p v-for="(m, i) in analysis.mismatches" :key="'m' + i" class="state warn" data-testid="compare-mismatch">
        Incomparable: the populations differ in {{ m.dimension }} ({{ m.baseline }} vs {{ m.candidate }}); affected conclusions are withheld.
      </p>
      <div v-for="row in rows" :key="row.operation + row.spanKind" class="mt-2">
        <div class="text-sm"><b>{{ row.operation || 'operation not recorded' }}</b> <span class="opacity-80">({{ row.spanKind || 'kind not recorded' }})</span></div>
        <table class="metrics" data-testid="compare-table">
          <thead><tr><th>Metric</th><th class="num">{{ sideTitle('baseline') }}</th><th class="num">{{ sideTitle('candidate') }}</th><th class="num">Change</th><th>Result</th></tr></thead>
          <tbody>
            <tr v-for="m in row.metrics" :key="m.metric" :data-metric="m.metric" data-testid="compare-metric">
              <td>{{ metricLabel(m.metric) }}</td>
              <td class="num">{{ formatMetric(m.metric, m.baseline) }}</td>
              <td class="num"><b>{{ formatMetric(m.metric, m.candidate) }}</b></td>
              <td class="num">{{ changeText(m) }}</td>
              <td><span class="chip" :class="clsOf(m.classification)" :title="m.reasons.join('; ')">{{ classificationLabel(m.classification) }}</span></td>
            </tr>
            <tr v-if="groupRow" class="group" data-testid="compare-group-row">
              <td>{{ groupRow.label }}</td>
              <td class="num">{{ groupRow.baseline }}</td>
              <td class="num"><b>{{ groupRow.candidate }}</b></td>
              <td class="num">{{ groupRow.change }}</td>
              <td class="text-xs">{{ groupRow.result }}</td>
            </tr>
          </tbody>
        </table>
        <p class="text-xs opacity-80">
          Valid durations: {{ row.baseline?.validDurations ?? 0 }} / {{ row.candidate?.validDurations ?? 0 }} ·
          percentiles need {{ analysis.rules.percentileMinimum.p50 }} (p50), {{ analysis.rules.percentileMinimum.p95 }} (p95), {{ analysis.rules.percentileMinimum.p99 }} (p99)
        </p>
      </div>
      <p v-if="rows.length === 0" class="mt-2" data-testid="compare-no-rows">Neither population has recorded calls of this operation.</p>

      <div class="fix mt-3">
        <label class="text-sm">Fixed release
          <select v-model="fixed" data-testid="compare-fixed" :disabled="running">
            <option value="">Choose</option>
            <option v-for="v in versions" :key="'f' + v.version" :value="key(v.version ?? '')">{{ versionText(v.version) }}</option>
          </select>
        </label>
        <PrimeButton type="button" size="small" :label="'Check ' + (fixed ? versionText(unkey(fixed)) : 'fixed release')" :disabled="!fixed || running || !candidate"
          @click="checkFix" data-testid="compare-check-fix"/>
        <PrimeButton v-if="original && original !== baseline && candidate" type="button" size="small" outlined
          :label="'Compare with the original baseline ' + versionText(unkey(original))" :disabled="running" @click="againstOriginal" data-testid="compare-original"/>
        <PrimeButton type="button" size="small" text icon="pi pi-download" label="Export HTML" @click="exportHtml" data-testid="compare-export-html"/>
        <PrimeButton type="button" size="small" text icon="pi pi-download" label="Export JSON" @click="exportJson" data-testid="compare-export-json"/>
      </div>
      <details class="text-xs mt-2">
        <summary>Comparison scope and quality</summary>
        <p>{{ analysis.rules.description }}</p>
        <p>Baseline: {{ analysis.baseline.label }} · {{ windowText(String(analysis.baseline.population.time_from), String(analysis.baseline.population.time_to)) }} · {{ statsStatusText(analysis.baseline) }} · coverage {{ analysis.baseline.coverage.status }}</p>
        <p>Candidate: {{ analysis.candidate.label }} · {{ windowText(String(analysis.candidate.population.time_from), String(analysis.candidate.population.time_to)) }} · {{ statsStatusText(analysis.candidate) }} · coverage {{ analysis.candidate.coverage.status }}</p>
        <p v-for="(n, i) in analysis.notices" :key="i">{{ n.message }}</p>
        <p>{{ analysis.interpretation }}</p>
      </details>
    </template>
  </div>
</template>

<script lang="ts">
import { computed, defineComponent, onMounted, ref, type PropType } from 'vue'
import PrimeButton from 'primevue/button'
import statsApi from '@/api/stats.api'
import { Stats } from '@/logs-api/stats.impl'
import { classificationLabel, exportComparisonHtml, exportComparisonJson, formatAbsolute, formatMetric, formatRelative, metricLabel, statsStatusText, windowText } from '@/services/comparison'
import { download } from '@/services/report'
import { formatNs } from '@/services/spanTree'
import { version as versionText } from '@/services/problems'
import { type Problem, type ProblemScope } from '@/types/problems'
import { type Analysis, type CallStats, type MetricComparison, type Population } from '@/types/stats'

// A select can't hold "" for "version not recorded" next to "" for "nothing
// chosen", so versions are keyed.
const key = (v: string) => 'v:' + v
const unkey = (k: string) => k.slice(2)

// ProblemCompare compares releases of the problem's operation: call
// statistics, p50/p95/p99 and the problem's own error group, with the
// comparison rules and their eligibility. The populations are explicit; no
// baseline is inferred from version text.
export default defineComponent({
  name: 'ProblemCompare',
  components: { PrimeButton },
  props: {
    problem: { type: Object as PropType<Problem>, required: true },
    scope: { type: Object as PropType<ProblemScope>, required: true }
  },
  setup (props) {
    statsApi.init(new Stats())
    const versions = ref<CallStats[]>([])
    const versionsError = ref('')
    const baseline = ref('')
    const candidate = ref('')
    const fixed = ref('')
    const original = ref('')
    const analysis = ref<Analysis | null>(null)
    const fromStored = ref(false)
    const running = ref(false)
    const error = ref('')
    let seq = 0

    function population (v: string): Population {
      return {
        service: props.problem.service,
        operation: props.problem.operation,
        spanKind: props.problem.spanKind || kindOf(),
        environment: props.problem.environment || props.scope.environment,
        version: v,
        timeFrom: props.scope.timeFrom,
        timeTo: props.scope.timeTo
      }
    }
    // kindOf: the operation's span kind when its calls have exactly one.
    function kindOf (): string | undefined {
      const kinds = new Set(versions.value.map((r) => r.spanKind))
      return kinds.size === 1 ? [...kinds][0] : undefined
    }

    async function loadVersions () {
      versionsError.value = ''
      const p: Population = { service: props.problem.service, operation: props.problem.operation, environment: props.problem.environment || props.scope.environment, timeFrom: props.scope.timeFrom, timeTo: props.scope.timeTo }
      const [err, stats] = await statsApi.callStats(p, { groupByVersion: true })
      if (err || !stats) {
        versionsError.value = err ?? 'no answer'
        return
      }
      if (stats.status !== 'ok') {
        versionsError.value = stats.condition
      }
      const seen = new Map<string, CallStats>()
      for (const r of stats.rows) {
        const v = r.version ?? ''
        const prev = seen.get(v)
        seen.set(v, prev ? { ...prev, calls: prev.calls + r.calls } : r)
      }
      versions.value = [...seen.values()]
    }

    async function run (b = baseline.value, c = candidate.value) {
      if (!b || !c) {
        return
      }
      const mine = ++seq
      running.value = true
      error.value = ''
      const [err, a] = await statsApi.compare(population(unkey(b)), population(unkey(c)))
      if (mine !== seq) {
        return
      }
      running.value = false
      if (err || !a) {
        error.value = err ?? 'no answer'
        return
      }
      analysis.value = a
      fromStored.value = false
    }

    function checkFix () {
      if (!original.value) {
        original.value = baseline.value
      }
      baseline.value = candidate.value
      candidate.value = fixed.value
      run()
    }
    function againstOriginal () {
      baseline.value = original.value
      run()
    }

    onMounted(async () => {
      const reg = props.problem.regression
      if (reg) {
        baseline.value = key(reg.baseline.version ?? '')
        candidate.value = key(reg.candidate.version ?? '')
        original.value = baseline.value
        const mine = ++seq
        const [err, a] = await statsApi.analysis(reg.analysisId)
        if (mine === seq) {
          if (a) {
            analysis.value = a
            fromStored.value = true
          } else {
            error.value = 'the stored comparison could not be loaded: ' + (err ?? 'not found')
          }
        }
      } else {
        const counts = props.problem.error?.versions ?? props.problem.slow?.versions ?? []
        const top = [...counts].sort((a, b) => b.occurrences - a.occurrences)[0]
        if (top) {
          candidate.value = key(top.version)
        }
      }
      await loadVersions()
    })

    const canCompare = computed(() => !!baseline.value && !!candidate.value && baseline.value !== candidate.value)
    const rows = computed(() => analysis.value?.rows.filter((r) => r.operation === props.problem.operation) ?? [])
    const infoOf = (k: string) => {
      const v = versions.value.find((r) => key(r.version ?? '') === k)
      return v ? `${windowText(props.scope.timeFrom, props.scope.timeTo)} · ${v.calls.toLocaleString('en-US')} calls` : ''
    }
    const baselineInfo = computed(() => infoOf(baseline.value))
    const candidateInfo = computed(() => infoOf(candidate.value))
    const sideTitle = (side: 'baseline' | 'candidate') => versionText(analysis.value?.[side].population.version as string | undefined)
    const changeText = (m: MetricComparison) => {
      const abs = formatAbsolute(m)
      const rel = formatRelative(m)
      return m.classification === 'descriptive' && m.metric === 'calls' ? abs : [abs, rel].filter((x) => x && x !== 'unavailable').join(' · ') || 'unavailable'
    }
    const clsOf = (c: string) => c === 'regression_signal' ? 'bad' : c === 'improvement_signal' ? 'good' : c === 'within_threshold' ? 'ok' : c === 'descriptive' ? 'neutral' : 'warn'
    const signals = computed(() => rows.value.flatMap((r) => r.metrics
      .filter((m) => m.classification === 'regression_signal' || m.classification === 'improvement_signal')
      .map((m) => ({ cls: clsOf(m.classification), text: `${classificationLabel(m.classification)}: ${metricLabel(m.metric)} ${changeText(m)}` }))))
    // The problem's own error group in both populations, labeled by source.
    const groupRow = computed(() => {
      const g = props.problem.error
      if (!g || !analysis.value) {
        return null
      }
      const found = analysis.value.errorGroups.find((x) => x.sourceKind === g.sourceKind && x.fingerprint === g.fingerprint && x.operation === g.operation)
      const label = `${g.exceptionType || g.signature.slice(0, 40)} (${g.sourceKind === 'log' ? 'ERROR logs' : g.sourceKind === 'exception' ? 'exception events' : 'ERROR spans'})`
      if (!found) {
        return { label, baseline: '0', candidate: '0', change: '0', result: 'not recorded in either population' }
      }
      const fmt = (v: number | null) => v == null ? 'unavailable' : v.toLocaleString('en-US')
      return {
        label,
        baseline: fmt(found.baseline.occurrences),
        candidate: fmt(found.candidate.occurrences),
        change: found.absoluteChange == null ? 'unavailable' : (found.absoluteChange >= 0 ? '+' : '') + found.absoluteChange,
        result: found.label.replace(/_/g, ' ')
      }
    })
    function exportHtml () {
      if (analysis.value) {
        download(`comparison-${analysis.value.id}.html`, 'text/html', exportComparisonHtml(analysis.value, [], new Date()))
      }
    }
    function exportJson () {
      if (analysis.value) {
        download(`comparison-${analysis.value.id}.json`, 'application/json', JSON.stringify(exportComparisonJson(analysis.value, [], new Date()), null, 2))
      }
    }
    return {
      versions, versionsError, loadVersions, baseline, candidate, fixed, original, analysis, fromStored, running, error, run, checkFix, againstOriginal,
      canCompare, rows, baselineInfo, candidateInfo, sideTitle, changeText, clsOf, signals, groupRow, exportHtml, exportJson,
      key, unkey, versionText, metricLabel, formatMetric, classificationLabel, formatNs, windowText, statsStatusText
    }
  }
})
</script>

<style scoped>
.pickers { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
@media (max-width: 600px) { .pickers { grid-template-columns: 1fr; } }
.picker { display: flex; flex-direction: column; gap: 4px; }
.lbl { font-weight: 600; font-size: 14px; }
.hint { font-size: 12px; color: var(--text-color-secondary); }
select { background: var(--surface-ground); color: var(--text-color); border: 1px solid var(--surface-border); border-radius: 6px; padding: 8px; font: inherit; }
select:focus-visible { outline: 2px solid var(--primary-color); }
.state { margin-top: 8px; padding: 8px; border-radius: 6px; font-size: 13px; }
.state.bad { background: rgba(248, 113, 113, 0.12); color: #fca5a5; }
.state.warn { background: rgba(251, 191, 36, 0.12); color: #fcd34d; }
.metrics { width: 100%; border-collapse: collapse; font-size: 13px; margin-top: 4px; }
.metrics th { text-align: left; font-weight: 500; color: var(--text-color-secondary); padding: 6px; border-bottom: 1px solid var(--surface-border); }
.metrics td { padding: 8px 6px; border-bottom: 1px solid var(--surface-border); }
.metrics .num { text-align: right; }
.metrics tr.group { background: var(--surface-hover); }
.chip { display: inline-block; font-size: 12px; padding: 2px 8px; border-radius: 4px; margin-right: 6px; }
.chip.bad { background: rgba(248, 113, 113, 0.18); color: #fca5a5; }
.chip.good { background: rgba(74, 222, 128, 0.15); color: #86efac; }
.chip.ok { background: rgba(96, 165, 250, 0.15); color: #93c5fd; }
.chip.warn { background: rgba(251, 191, 36, 0.15); color: #fcd34d; }
.chip.neutral { background: rgba(148, 163, 184, 0.18); color: #cbd5e1; }
.fix { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.fix select { margin-left: 6px; padding: 5px; }
.link { background: none; border: 0; color: var(--primary-color); cursor: pointer; padding: 0; font: inherit; }
</style>
