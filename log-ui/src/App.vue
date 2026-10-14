<template>
  <main class="w-full">
    <header class="topbar">
      <div class="brand"><span class="logo" aria-hidden="true">t</span> troubleshooting</div>
      <nav class="mainnav" aria-label="Main">
        <button v-for="p in PAGES" :key="p.value" type="button" :class="{ on: page === p.value }" :aria-current="page === p.value ? 'page' : undefined"
          @click="go(p.value)" :data-testid="'main-nav-' + p.value">{{ p.label }}</button>
      </nav>
    </header>
    <p v-if="routeError" class="route-error" role="alert" data-testid="route-error">{{ routeError }}</p>

    <!-- Pages stay mounted once visited, so returning to one restores its
         filters, page, expansion and selection. -->
    <TracePage v-show="page === 'logs'" @open-execution="openExecution"/>
    <ExecutionsView v-show="page === 'executions'" :target="target" :targetKey="targetKey" :included="isIncluded"
      @open-execution="openExecution" @back="returnToOrigin" @include="include"/>
    <ProblemsView v-if="visited.problems" v-show="page === 'problems'" :rulesRevision="rulesRevision" @open-execution="openExecution" @open-settings="go('settings')"/>
    <ReleasesView v-if="visited.releases" v-show="page === 'releases'" :included="included" @open-report="openFromRelease" @remove-included="removeIncluded"/>
    <SettingsView v-if="visited.settings" v-show="page === 'settings'" @rules-saved="rulesRevision++"/>
  </main>
</template>

<script lang="ts">
import { computed, defineComponent, onBeforeUnmount, reactive, ref } from 'vue'
import TracePage from './components/TracePage.vue'
import ExecutionsView from './components/ExecutionsView.vue'
import ProblemsView from './components/problems/ProblemsView.vue'
import ReleasesView from './components/ReleasesView.vue'
import SettingsView from './components/SettingsView.vue'
import { type IncludedReport } from './services/comparison'
import { PAGES, investigationHash, parseRoute, type ExecutionTarget, type Page } from './services/navigation'
import { type Report } from './types/report'
import { type ReportContext } from './types/stats'

// The five destinations of the product blueprint, in order: Log Analysis
// (the default), Timeline & Sequence, Problems, Releases, Settings.
export default defineComponent({
  name: 'TroubleShooting',
  components: {
    TracePage,
    ExecutionsView,
    ProblemsView,
    ReleasesView,
    SettingsView
  },
  setup () {
    const page = ref<Page>('logs')
    const visited = reactive({ problems: false, releases: false, settings: false })
    const target = ref<ExecutionTarget | null>(null)
    // A new key per opened execution: a response for an earlier one can never
    // reach the one shown now.
    const targetKey = ref(0)
    const rulesRevision = ref(0)
    const routeError = ref('')

    function show (p: Page) {
      page.value = p
      if (p === 'problems' || p === 'releases' || p === 'settings') {
        visited[p] = true
      }
    }
    // The hash names the page or the open execution, so links and the
    // browser's Back work; it carries identifiers only.
    let lastHash = ''
    function setHash (h: string) {
      if (window.location.hash !== h) {
        lastHash = h
        window.location.hash = h
      }
    }
    function go (p: Page) {
      routeError.value = ''
      show(p)
      setHash(p === 'executions' && target.value ? investigationHash(target.value) : '#/' + p)
    }
    function openExecution (t: ExecutionTarget) {
      routeError.value = ''
      targetKey.value++
      target.value = t
      show('executions')
      setHash(investigationHash(t))
    }
    function returnToOrigin () {
      go(target.value?.origin?.page ?? 'logs')
    }
    function sameTarget (a: ExecutionTarget | null, b: ExecutionTarget) {
      return !!a && a.traceId === b.traceId && a.window.from === b.window.from && a.window.to === b.window.to &&
        (a.spanId ?? '') === (b.spanId ?? '') && (a.recordId ?? '') === (b.recordId ?? '')
    }
    function applyHash () {
      const h = window.location.hash
      if (h === lastHash) {
        return
      }
      lastHash = h
      const route = parseRoute(h)
      routeError.value = route.error
      if (route.target && !sameTarget(target.value, route.target)) {
        targetKey.value++
        target.value = route.target
      }
      show(route.page)
    }
    applyHash()
    window.addEventListener('hashchange', applyHash)
    onBeforeUnmount(() => window.removeEventListener('hashchange', applyHash))

    // Release examples open in the same viewer; the comparison stays their
    // context (for "Include in comparison export").
    function openFromRelease (t: { traceId: string, window: { from: string, to: string }, recordId?: string, spanId?: string, context?: ReportContext }) {
      openExecution({ ...t, origin: { page: 'releases', label: t.context ? 'the comparison ' + JSON.stringify(t.context.analysisTitle) : 'Releases' } })
    }

    // Reports the user selected as supporting examples, per comparison
    // revision, exactly as they were generated.
    const included = ref<Record<string, IncludedReport[]>>({})
    const isIncluded = computed(() => {
      const ctx = target.value?.context
      return !!ctx && (included.value[ctx.analysisId] ?? []).some((r) => r.report.traceId === target.value!.traceId)
    })
    function include (report: Report) {
      const ctx = target.value?.context
      if (!ctx) {
        return
      }
      const list = included.value[ctx.analysisId] ?? []
      included.value = {
        ...included.value,
        [ctx.analysisId]: [...list.filter((r) => r.report.traceId !== report.traceId), { report, context: ctx, includedAt: (BigInt(Date.now()) * 1000000n).toString() }]
      }
    }
    function removeIncluded (analysisId: string, index: number) {
      const list = [...(included.value[analysisId] ?? [])]
      list.splice(index, 1)
      included.value = { ...included.value, [analysisId]: list }
    }
    return {
      PAGES, page, visited, go, target, targetKey, openExecution, returnToOrigin, openFromRelease, rulesRevision, routeError,
      included, isIncluded, include, removeIncluded
    }
  },
  async created () {
    document.title = 'Troubleshooting'
  }
})
</script>

<style scoped>
.topbar { display: flex; align-items: center; gap: 16px; flex-wrap: wrap; margin-bottom: 12px; }
.brand { display: flex; align-items: center; gap: 8px; font-weight: 600; }
.logo { display: inline-grid; place-items: center; width: 26px; height: 26px; border-radius: 6px; background: var(--primary-color); color: var(--primary-color-text); font-weight: 700; }
.mainnav { display: flex; gap: 6px; flex-wrap: wrap; }
.mainnav button { background: var(--surface-card); color: var(--text-color); border: 1px solid var(--surface-border); border-radius: 8px; padding: 8px 14px; cursor: pointer; font: inherit; font-weight: 500; }
.mainnav button.on { background: var(--primary-color); color: var(--primary-color-text); border-color: var(--primary-color); }
.mainnav button:focus-visible { outline: 2px solid var(--primary-color); outline-offset: 2px; }
.route-error { padding: 8px; border-radius: 6px; font-size: 13px; background: rgba(248, 113, 113, 0.12); color: #fca5a5; margin-bottom: 8px; }
@media (max-width: 480px) {
  .mainnav { width: 100%; }
  .mainnav button { flex: 1 1 40%; padding: 8px; }
}
</style>
