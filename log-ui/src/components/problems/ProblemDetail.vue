<template>
  <div data-testid="problem-detail">
    <a href="#" class="back" @click.prevent="$emit('back')" data-testid="problem-back">← Problems</a>
    <div class="head">
      <div>
        <div class="flex items-center gap-2 mt-2">
          <span class="type" :class="problem.type" data-testid="problem-type">{{ problemTypeLabel(problem) }}</span>
          <span class="text-xs opacity-80" data-testid="problem-id">{{ problem.id }}</span>
        </div>
        <h2 class="title" data-testid="problem-title">{{ problem.title }}</h2>
        <div class="text-sm opacity-80">{{ problem.service }} / {{ problem.operation || 'operation not recorded' }}<template v-if="problem.environment"> · {{ problem.environment }}</template></div>
      </div>
    </div>
    <div class="counts mt-2" data-testid="problem-counts">
      <template v-if="problem.error">
        <span v-if="problem.error.affectedCalls != null && problem.error.affectedCalls > 0"><b>{{ n(problem.error.affectedCalls) }}</b> affected calls</span>
        <span><b>{{ n(problem.error.occurrences) }}</b> matching {{ sourceKindShort(problem.error.sourceKind) }}</span>
        <span><b>{{ n(problem.error.affectedTraces) }}</b> traces</span>
        <span v-if="problem.error.unlinked > 0"><b>{{ n(problem.error.unlinked) }}</b> without TraceId</span>
      </template>
      <template v-else-if="problem.slow">
        <span><b>{{ n(problem.slow.calls) }}</b> calls over {{ n(problem.slow.thresholdMs) }} ms</span>
      </template>
      <template v-else-if="problem.regression">
        <span><b>p95 {{ ms(problem.regression.metric.baseline) }} → {{ ms(problem.regression.metric.candidate) }}</b></span>
        <span>{{ n(problem.regression.baselineCalls) }} / {{ n(problem.regression.candidateCalls) }} calls evaluated</span>
      </template>
    </div>
    <p v-if="problem.attribution" class="state warn" data-testid="problem-attribution">{{ problem.attribution }}</p>
    <div class="text-xs opacity-80 mt-1">{{ releaseContext }} · {{ windowText(scope.timeFrom, scope.timeTo) }}<template v-if="problem.firstSeen"> · first {{ time(problem.firstSeen) }}</template><template v-if="problem.lastSeen"> · last {{ time(problem.lastSeen) }}</template></div>

    <details class="why mt-2" data-testid="why-flagged">
      <summary>Why was this flagged?</summary>
      <dl>
        <dt>Rule</dt><dd>{{ problem.explanation.rule }} (revision {{ problem.explanation.ruleRevision }}{{ problem.explanation.ruleRevision === 0 ? ', defaults' : '' }})</dd>
        <dt>Observed</dt><dd data-testid="why-observed">{{ problem.explanation.observed }}</dd>
        <dt>Threshold</dt><dd>{{ problem.explanation.threshold }}</dd>
        <dt>Grouping</dt><dd>{{ problem.explanation.basis }}</dd>
        <dt>Scope</dt><dd>{{ problem.explanation.scope }}</dd>
        <dt v-if="problem.explanation.coverage">Coverage</dt><dd v-if="problem.explanation.coverage">{{ problem.explanation.coverage }}</dd>
      </dl>
      <p class="text-xs opacity-80">A problem names an observed symptom, not a proven cause. This explanation was produced with the rules shown; it is evaluated when the list is loaded, not stored.</p>
    </details>

    <div class="tabs mt-3" role="tablist">
      <button type="button" role="tab" :aria-selected="tab === 'investigate'" :class="{ on: tab === 'investigate' }" @click="tab = 'investigate'" data-testid="tab-investigate">Investigate</button>
      <button type="button" role="tab" :aria-selected="tab === 'compare'" :class="{ on: tab === 'compare' }" @click="tab = 'compare'" data-testid="tab-compare">Compare releases</button>
    </div>

    <div v-show="tab === 'investigate'" class="mt-3" data-testid="investigate">
      <div class="flex flex-wrap items-end justify-between gap-3">
        <div class="ex">
          <div class="text-xs opacity-80">{{ exampleHeading }}</div>
          <p v-if="examplesLoading" data-testid="examples-loading">Loading retained examples…</p>
          <p v-else-if="examplesError" class="state bad" data-testid="problem-examples-error">
            Retained examples are unavailable: {{ examplesError }} <button type="button" class="link" @click="loadExamples">Retry</button>
          </p>
          <p v-else-if="examples && examples.status !== 'ok'" class="state warn" data-testid="problem-examples-status">{{ examples.condition }}</p>
          <p v-else-if="examples && examples.examples.length === 0" data-testid="problem-examples-none">No retained example matches in this window.</p>
          <template v-else-if="examples">
            <div v-if="examples.examples.length === 1" class="single" data-testid="example-single">{{ time(examples.examples[0].time) }} · {{ exampleLabel(examples.examples[0]) }}</div>
            <select v-else v-model="exampleIndex" class="ex-select" aria-label="Retained example" data-testid="example-select">
              <option v-for="(e, i) in examples.examples" :key="exampleKey(e)" :value="i">{{ time(e.time) }} · {{ exampleLabel(e) }}</option>
            </select>
            <div class="text-xs opacity-80 mt-1">{{ examples.shown }} of {{ examples.total }} retained examples<template v-if="examples.truncated"> (bounded)</template> · examples, not the affected population</div>
          </template>
        </div>
        <div v-if="example && example.traceId" class="open-actions" data-testid="example-actions">
          <PrimeButton type="button" size="small" label="Open timeline" @click="openExample('timeline')" data-testid="example-open-timeline"/>
          <PrimeButton type="button" size="small" outlined label="Open sequence" @click="openExample('sequence')" data-testid="example-open-sequence"/>
        </div>
      </div>
      <p v-if="example && example.traceId" class="text-xs opacity-80 mt-2">The example opens in Timeline &amp; Sequence as its own report, with this occurrence's span and log selected; it shows one occurrence, not every call of the group.</p>

      <div v-if="unlinked" class="state warn mt-3" data-testid="unlinked-example">
        <b>Retained log example</b> · {{ time(unlinked.time) }} · {{ unlinked.service }}
        <div class="msg-full mt-1" data-testid="unlinked-message">{{ unlinked.message }}</div>
        <div class="text-xs mt-1">No trace can be opened: this record has no TraceId, so it can't be correlated with calls or other logs.</div>
      </div>
    </div>

    <div v-show="tab === 'compare'" class="mt-3">
      <ProblemCompare :problem="problem" :scope="scope"/>
    </div>
  </div>
</template>

<script lang="ts">
import { computed, defineComponent, onMounted, ref, type PropType } from 'vue'
import PrimeButton from 'primevue/button'
import statsApi from '@/api/stats.api'
import { Stats } from '@/logs-api/stats.impl'
import ProblemCompare from '@/components/problems/ProblemCompare.vue'
import { windowText } from '@/services/comparison'
import { exampleKey, exampleLabel, ms, problemTypeLabel, sourceKindShort, version } from '@/services/problems'
import { formatNs } from '@/services/spanTree'
import { type ExecutionTarget, type ExecutionView } from '@/services/navigation'
import { scopePopulation, type Problem, type ProblemScope } from '@/types/problems'
import { type Example, type ExamplesRequest, type ExamplesResult } from '@/types/stats'

// ProblemDetail: the problem's header and explanation, then Investigate (a
// bounded list of retained examples; the chosen occurrence opens in Timeline
// & Sequence) and Compare releases. Both tabs stay mounted, so switching
// keeps their selections.
export default defineComponent({
  name: 'ProblemDetail',
  components: { PrimeButton, ProblemCompare },
  props: {
    problem: { type: Object as PropType<Problem>, required: true },
    scope: { type: Object as PropType<ProblemScope>, required: true },
    initialTab: { type: String as PropType<'investigate' | 'compare'>, default: 'investigate' }
  },
  emits: ['back', 'open-execution'],
  setup (props, { emit }) {
    statsApi.init(new Stats())
    const tab = ref<'investigate' | 'compare'>(props.initialTab)
    const examples = ref<ExamplesResult | null>(null)
    const examplesError = ref('')
    const examplesLoading = ref(false)
    const exampleIndex = ref(0)

    // The examples' report window: the problem's window, or the candidate
    // population's for a regression.
    const reportWindow = computed(() => {
      const c = props.problem.regression?.candidate
      return c ? { from: String(c.time_from), to: String(c.time_to) } : { from: props.scope.timeFrom, to: props.scope.timeTo }
    })

    function examplesRequest (): ExamplesRequest {
      const pop = scopePopulation(props.scope)
      const p = props.problem
      if (p.error) {
        return {
          population: pop,
          kind: 'error',
          limit: 20,
          group: { sourceKind: p.error.sourceKind, service: p.error.service, operation: p.error.operation, environment: p.error.environment, fingerprint: p.error.fingerprint }
        }
      }
      if (p.slow) {
        return { population: { ...pop, operation: p.operation, spanKind: p.spanKind, environment: p.environment || pop.environment }, kind: 'slow', thresholdMs: p.slow.thresholdMs, limit: 20 }
      }
      const c = p.regression!.candidate
      return {
        population: {
          service: p.service,
          operation: p.operation,
          spanKind: p.spanKind,
          environment: c.environment ?? undefined,
          version: c.version ?? undefined,
          installation: c.installation ?? undefined,
          segment: c.segment ?? undefined,
          timeFrom: String(c.time_from),
          timeTo: String(c.time_to)
        },
        kind: 'slow',
        thresholdMs: p.regression!.metric.candidate ?? 0,
        limit: 20
      }
    }

    async function loadExamples () {
      examplesLoading.value = true
      examplesError.value = ''
      const [err, res] = await statsApi.examples(examplesRequest())
      examplesLoading.value = false
      if (err || !res) {
        examplesError.value = err ?? 'no answer'
        return
      }
      examples.value = res
      exampleIndex.value = 0
    }
    onMounted(loadExamples)

    const example = computed<Example | null>(() => examples.value?.examples[exampleIndex.value] ?? null)
    const unlinked = computed(() => example.value && !example.value.traceId ? example.value : null)

    // openExample opens the chosen occurrence with its span and log
    // selected; the problem is only the origin.
    function openExample (view: ExecutionView) {
      const ex = example.value
      if (!ex || !ex.traceId) {
        return
      }
      const target: ExecutionTarget = {
        traceId: ex.traceId,
        window: { ...reportWindow.value },
        spanId: ex.spanId || undefined,
        recordId: ex.recordId || undefined,
        view,
        origin: { page: 'problems', label: 'the problem ' + JSON.stringify(props.problem.title) }
      }
      emit('open-execution', target)
    }

    const exampleHeading = computed(() => {
      if (props.problem.error?.sourceKind === 'log' && !example.value?.spanId) {
        return 'Retained log example'
      }
      return 'Retained example call'
    })
    const releaseContext = computed(() => {
      const p = props.problem
      const vs = p.error?.versions ?? p.slow?.versions ?? []
      if (p.regression) {
        return `${version(p.regression.baseline.version)} → ${version(p.regression.candidate.version)}`
      }
      return vs.length ? vs.map((v) => `${version(v.version)}: ${v.occurrences.toLocaleString('en-US')}`).join(', ') : 'version not recorded'
    })

    const n = (v: number) => v.toLocaleString('en-US')
    const time = (ns: string) => formatNs(ns).slice(0, 21)
    return {
      tab, examples, examplesError, examplesLoading, exampleIndex, loadExamples, example, unlinked, openExample, exampleHeading, releaseContext,
      n, time, ms, problemTypeLabel, sourceKindShort, exampleLabel, exampleKey, windowText
    }
  }
})
</script>

<style scoped>
.back { color: var(--text-color); text-decoration: underline; font-size: 14px; }
.head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; flex-wrap: wrap; }
.title { font-size: 25px; font-weight: 500; margin: 4px 0; }
.type { font-size: 12px; padding: 2px 8px; border-radius: 4px; }
.type.error { background: rgba(248, 113, 113, 0.15); color: #fca5a5; }
.type.slow_calls { background: rgba(96, 165, 250, 0.15); color: #93c5fd; }
.type.release_regression { background: rgba(251, 191, 36, 0.15); color: #fcd34d; }
.counts { display: flex; flex-wrap: wrap; gap: 18px; font-size: 14px; }
.counts b { font-size: 18px; font-weight: 600; }
.why summary { cursor: pointer; font-size: 13px; }
.why dl { display: grid; grid-template-columns: 7rem 1fr; gap: 4px 12px; font-size: 13px; margin: 8px 0; }
.why dt { color: var(--text-color-secondary); }
.tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--surface-border); }
.tabs button { background: none; border: 0; border-bottom: 2px solid transparent; color: var(--text-color-secondary); padding: 8px 12px; cursor: pointer; font: inherit; }
.tabs button.on { color: var(--text-color); border-bottom-color: var(--primary-color); font-weight: 600; }
.tabs button:focus-visible { outline: 2px solid var(--primary-color); }
.ex { min-width: min(100%, 28rem); flex: 1; max-width: 32rem; }
.ex-select { width: 100%; background: var(--surface-ground); color: var(--text-color); border: 1px solid var(--surface-border); border-radius: 6px; padding: 8px; font: inherit; }
.single { padding: 8px 0; }
.open-actions { display: flex; gap: 6px; flex-wrap: wrap; }
.state { padding: 8px; border-radius: 6px; font-size: 13px; }
.state.bad { background: rgba(248, 113, 113, 0.12); color: #fca5a5; }
.state.warn { background: rgba(251, 191, 36, 0.12); color: #fcd34d; }
.msg-full { white-space: pre-wrap; word-break: break-word; color: var(--text-color); }
.link { background: none; border: 0; color: var(--primary-color); cursor: pointer; padding: 0; font: inherit; }
</style>
