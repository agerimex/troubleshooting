<template>
  <div class="card pw" data-testid="problems-view">
    <div>
      <div v-show="!current">
        <div class="flex justify-between items-start flex-wrap gap-2">
          <div>
            <div class="text-xs opacity-80">Observed symptoms</div>
            <h2 class="title">Problems</h2>
            <p class="text-sm opacity-80">Recurring errors, slow calls and release changes worth investigating. An example opens in Timeline &amp; Sequence.</p>
          </div>
          <span v-if="list" class="pill" data-testid="problems-total">{{ list.total }} {{ list.total === 1 ? 'group' : 'groups' }}</span>
        </div>
        <form class="scope" @submit.prevent="loadList()" data-testid="problems-scope">
          <label class="fld">Service
            <input v-model="service" list="pw-services" placeholder="Choose a service" required class="w-64" data-testid="problems-service"/>
            <datalist id="pw-services"><option v-for="s in services" :key="s" :value="s"/></datalist>
          </label>
          <label class="fld">Environment
            <input v-model="environment" placeholder="any" class="w-36" data-testid="problems-environment"/>
          </label>
          <label class="fld">From
            <Calendar v-model="from" showTime hourFormat="24" dateFormat="dd.mm.yy" inputId="pw-from" data-testid="problems-from"/>
          </label>
          <label class="fld">To (exclusive)
            <Calendar v-model="to" showTime hourFormat="24" dateFormat="dd.mm.yy" inputId="pw-to" data-testid="problems-to"/>
          </label>
          <PrimeButton type="submit" label="Show problems" size="small" :disabled="!service.trim() || loading" :loading="loading" data-testid="problems-load"/>
        </form>
        <div v-if="list" class="text-xs opacity-80 mt-1" data-testid="problems-scope-text">
          {{ list.scope.service }}<template v-if="list.scope.environment != null"> · {{ list.scope.environment || 'environment not recorded' }}</template> · {{ windowText(String(list.scope.time_from), String(list.scope.time_to)) }} · rules revision {{ list.rules.revision }}{{ list.rules.revision === 0 ? ' (defaults)' : '' }}
        </div>

        <p v-if="loadError" class="state bad" role="alert" data-testid="problems-error">Problem summaries are unavailable: {{ loadError }} <button type="button" class="link" @click="loadList()">Retry</button></p>
        <template v-if="list">
          <p v-if="list.status === 'unavailable'" class="state bad" role="alert" data-testid="problems-unavailable">{{ list.condition }} <button type="button" class="link" @click="loadList()">Retry</button></p>
          <p v-else-if="list.status === 'expired'" class="state warn" data-testid="problems-expired">{{ list.condition }}</p>
          <p v-else-if="list.status === 'partial'" class="state warn" data-testid="problems-partial">{{ list.condition }}</p>
          <p v-else-if="list.status === 'empty'" class="state ok" data-testid="problems-empty">No problems observed in the covered scope. <a href="#" @click.prevent="$emit('open-settings')" data-testid="problems-rules-link">Detection rules (Settings)</a></p>

          <ul class="rows" data-testid="problem-rows">
            <li v-for="(p, i) in rows" :key="p.id" class="prow" data-testid="problem-row" :data-problem-id="p.id">
              <div class="pmain">
                <span class="type" :class="p.type">{{ problemTypeLabel(p) }}</span>
                <span v-if="!p.qualified" class="type muted" :title="p.attribution">record-based</span>
                <div class="ptitle" data-testid="problem-row-title">{{ p.title }}</div>
                <div class="text-xs opacity-80">{{ problemContext(p) }}<template v-if="p.lastSeen"> · last seen {{ time(p.lastSeen) }}</template></div>
              </div>
              <div class="pmeasure" data-testid="problem-row-measure">
                <div class="big">{{ headline(p).value }}</div>
                <div class="text-sm">{{ headline(p).label }}</div>
                <div class="text-xs opacity-80">{{ headline(p).detail }}</div>
              </div>
              <PrimeButton type="button" label="Investigate" size="small" :outlined="i > 0" @click="openProblem(p)" data-testid="problem-investigate"/>
            </li>
          </ul>
          <div v-if="list.next" class="mt-2">
            <PrimeButton type="button" label="Load more" size="small" text :loading="loadingMore" @click="loadMore" data-testid="problems-more"/>
            <span class="text-xs opacity-80">{{ rows.length }} of {{ list.total }}</span>
          </div>
          <p v-if="list.waiting" class="text-sm opacity-80 mt-2" data-testid="problems-waiting">{{ list.waiting }}</p>
          <details class="text-xs mt-3">
            <summary>How these groups are counted</summary>
            <p>Each error group belongs to one source kind (ERROR spans, exception events, ERROR logs): the same failure can appear in several, so their counts are never added. Affected calls are distinct recorded spans (several logs of one span are one call). Slow calls are distinct calls longer than the slow-call duration. A release regression shows a metric change between two evaluated populations, not a number of failed calls.</p>
            <p v-for="(s, name) in list.sources" :key="name">{{ name }}: {{ s.status }}<template v-if="s.condition && s.condition !== 'ok'"> — {{ s.condition }}</template></p>
          </details>
        </template>
      </div>
      <ProblemDetail v-if="current" :key="current.problem.id + ':' + detailSeq" :problem="current.problem" :scope="current.scope"
        :initialTab="current.problem.type === 'release_regression' ? 'compare' : 'investigate'" @back="current = null" @open-execution="$emit('open-execution', $event)"/>
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent, onMounted, ref, watch } from 'vue'
import PrimeButton from 'primevue/button'
import Calendar from 'primevue/calendar'
import problemsApi from '@/api/problems.api'
import { Problems } from '@/logs-api/problems.impl'
import ProblemDetail from '@/components/problems/ProblemDetail.vue'
import { windowText } from '@/services/comparison'
import { headline, problemContext, problemTypeLabel } from '@/services/problems'
import { formatNs, toNs } from '@/services/spanTree'
import { type Problem, type ProblemList, type ProblemScope } from '@/types/problems'

// ProblemsView is the Problems page: prepared problem groups of one service
// and window and a problem's detail; examples open in Timeline & Sequence and
// the detection rules live in Settings. The list stays mounted while a
// problem is open, so Back restores it.
export default defineComponent({
  name: 'ProblemsView',
  components: { PrimeButton, Calendar, ProblemDetail },
  props: {
    // Bumped when the detection rules were saved (in Settings).
    rulesRevision: { type: Number, default: 0 }
  },
  emits: ['open-execution', 'open-settings'],
  setup (props) {
    problemsApi.init(new Problems())
    const hour = 3600_000
    const now = Date.now()
    const from = ref<Date>(new Date(Math.floor(now / hour) * hour - 23 * hour))
    const to = ref<Date>(new Date(Math.floor(now / hour) * hour + hour))
    const service = ref('')
    const environment = ref('')
    const services = ref<string[]>([])
    const list = ref<ProblemList | null>(null)
    const rows = ref<Problem[]>([])
    const loadError = ref('')
    const loading = ref(false)
    const loadingMore = ref(false)
    const listScope = ref<ProblemScope | null>(null)
    const current = ref<{ problem: Problem, scope: ProblemScope } | null>(null)
    const detailSeq = ref(0)
    let seq = 0

    // The window end is exclusive and used as selected (whole hours stay
    // whole statistics buckets).
    const scopeNow = (): ProblemScope => ({
      service: service.value.trim(),
      environment: environment.value.trim() === '' ? undefined : environment.value.trim(),
      timeFrom: toNs(from.value),
      timeTo: toNs(to.value)
    })

    async function loadServices () {
      const [, s] = await problemsApi.services(toNs(from.value), toNs(to.value))
      services.value = s ?? []
      if (!service.value && services.value.length === 1) {
        service.value = services.value[0]
      }
    }
    onMounted(loadServices)
    watch([from, to], loadServices)

    async function loadList () {
      const scope = scopeNow()
      if (!scope.service) {
        return
      }
      const mine = ++seq
      loading.value = true
      loadError.value = ''
      const [err, res] = await problemsApi.problems({ ...scope, limit: 50 })
      if (mine !== seq) {
        return
      }
      loading.value = false
      if (err || !res) {
        loadError.value = err ?? 'no answer'
        list.value = null
        rows.value = []
        return
      }
      listScope.value = scope
      list.value = res
      rows.value = res.problems
    }
    // loadMore continues the same scope and rules revision.
    async function loadMore () {
      if (!list.value?.next || !listScope.value) {
        return
      }
      const mine = seq
      loadingMore.value = true
      const [err, res] = await problemsApi.problems({ ...listScope.value, limit: 50, cursor: list.value.next, rulesRevision: list.value.rules.revision })
      loadingMore.value = false
      if (mine !== seq) {
        return
      }
      if (err || !res) {
        loadError.value = err ?? 'no answer'
        return
      }
      if (res.status === 'unavailable') {
        list.value = { ...list.value, status: 'unavailable', condition: res.condition }
        return
      }
      rows.value = [...rows.value, ...res.problems]
      list.value = { ...res, problems: rows.value }
    }

    function openProblem (p: Problem) {
      detailSeq.value++
      current.value = { problem: p, scope: listScope.value! }
    }
    // Saved rules apply to the next evaluation: reload a shown list.
    watch(() => props.rulesRevision, () => {
      if (list.value) {
        loadList()
      }
    })
    const time = (ns: string) => formatNs(ns).slice(0, 17)
    return {
      from, to, service, environment, services, list, rows, loadError, loading, loadingMore, loadList, loadMore,
      current, detailSeq, openProblem, headline, problemContext, problemTypeLabel, windowText, time
    }
  }
})
</script>

<style scoped>
.pw { font-size: 14px; }
.title { font-size: 25px; font-weight: 500; margin: 0; }
.pill { font-size: 12px; background: var(--surface-hover); padding: 3px 8px; border-radius: 6px; }
.scope { display: flex; flex-wrap: wrap; align-items: flex-end; gap: 10px; margin-top: 12px; }
.fld { display: flex; flex-direction: column; gap: 3px; font-size: 12px; color: var(--text-color-secondary); }
.fld input { background: var(--surface-ground); color: var(--text-color); border: 1px solid var(--surface-border); border-radius: 6px; padding: 8px; font: inherit; font-size: 14px; }
.fld input:focus-visible { outline: 2px solid var(--primary-color); }
.rows { list-style: none; padding: 0; margin: 12px 0 0; border-top: 1px solid var(--surface-border); }
.prow { display: grid; grid-template-columns: minmax(0, 1fr) 13rem auto; gap: 16px; align-items: center; padding: 16px 0; border-bottom: 1px solid var(--surface-border); }
@media (max-width: 600px) { .prow { grid-template-columns: 1fr; gap: 8px; } }
.ptitle { font-size: 16px; font-weight: 600; margin: 6px 0 2px; }
.big { font-size: 18px; font-weight: 600; }
.type { font-size: 12px; padding: 2px 8px; border-radius: 4px; margin-right: 6px; }
.type.error { background: rgba(248, 113, 113, 0.15); color: #fca5a5; }
.type.slow_calls { background: rgba(96, 165, 250, 0.15); color: #93c5fd; }
.type.release_regression { background: rgba(251, 191, 36, 0.15); color: #fcd34d; }
.type.muted { background: rgba(148, 163, 184, 0.15); color: #cbd5e1; }
.state { padding: 8px; border-radius: 6px; font-size: 13px; margin-top: 10px; }
.state.bad { background: rgba(248, 113, 113, 0.12); color: #fca5a5; }
.state.warn { background: rgba(251, 191, 36, 0.12); color: #fcd34d; }
.state.ok { background: rgba(74, 222, 128, 0.1); }
.link { background: none; border: 0; color: var(--primary-color); cursor: pointer; padding: 0; font: inherit; }
</style>
