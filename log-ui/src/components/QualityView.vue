<template>
  <div class="card qv" data-testid="quality-view">
    <div class="text-xs opacity-80">Test runs</div>
    <h2 class="title">Quality</h2>
    <p class="text-sm opacity-80">Requirements kept in your repository, checked on every test run: latency limits, database calls per execution and forbidden calls. Every failure opens the execution behind it. A missing measurement is never a pass.</p>

    <div class="tabs" role="tablist" aria-label="Quality views">
      <button v-for="(t, i) in TABS" :key="t.value" ref="tabButtons" type="button" role="tab" :id="'quality-tab-' + t.value"
        :aria-selected="tab === t.value" :aria-controls="'quality-panel-' + t.value" :tabindex="tab === t.value ? 0 : -1"
        :data-testid="'quality-tab-' + t.value" @click="selectTab(t.value)" @keydown="onTabKey($event, i)">{{ t.label }}</button>
    </div>

    <div v-show="tab !== 'check'" :id="'quality-panel-' + tab" role="tabpanel" :aria-labelledby="'quality-tab-' + tab" class="panel">
      <template v-if="saved">
        <button type="button" class="link back" @click="closeSaved" data-testid="quality-saved-back">← Back to {{ tab === 'history' ? 'the history' : 'the requirements' }}</button>
        <QualityResult :result="saved.evaluation" :rules="saved.rules" sourceLabel="Recorded evaluation" @open-execution="$emit('open-execution', $event)"/>
      </template>
      <template v-else>
        <p v-if="savedError" class="state bad" role="alert" data-testid="quality-saved-error">{{ savedError }}</p>
        <p v-if="projectsError" class="state bad" role="alert" data-testid="quality-projects-error">The recorded projects could not be read: {{ projectsError }} <button type="button" class="link" @click="loadProjects">Retry</button></p>
        <div v-else-if="projectsLoaded && projects.length === 0" class="empty" data-testid="quality-projects-empty">
          <p><b>Nothing is recorded yet.</b> A recorded evaluation keeps its result, its rules and the spans of its evidence, so each requirement gets a history: when it started failing and which commit first passed its checks again.</p>
          <p class="text-sm">Record from CI with the Action's <code>record</code> input (on by default for pushes), or with <code>troubleshooting-check --record</code>. Requirements are a <code>requirements:</code> list in <code>quality.yaml</code>; until then, <b>Check a run</b> works without recording.</p>
        </div>
        <template v-else-if="project">
          <div class="filters">
            <label class="fld">Project
              <select v-model="project" data-testid="quality-project">
                <option v-for="p in projects" :key="p.project" :value="p.project">{{ p.project }} ({{ p.evaluations }})</option>
              </select>
            </label>
            <label class="fld">Branch or tag
              <input v-model.lazy="refName" placeholder="any" data-testid="quality-ref" @keydown.enter="($event.target as HTMLInputElement).blur()"/>
            </label>
            <button type="button" class="link" @click="reloadKey++" data-testid="quality-reload">Refresh</button>
          </div>
          <QualityRequirements v-if="tab === 'requirements'" :project="project" :refName="refName.trim()" :reloadKey="reloadKey" @open-evaluation="openEvaluation"/>
          <QualityHistory v-else :project="project" :refName="refName.trim()" :reloadKey="reloadKey" @open-evaluation="openEvaluation"/>
        </template>
        <p v-else class="text-sm opacity-80" role="status">Loading…</p>
      </template>
    </div>

    <div v-show="tab === 'check'" id="quality-panel-check" role="tabpanel" aria-labelledby="quality-tab-check" class="panel">
      <form class="form" @submit.prevent="evaluate" data-testid="quality-form">
        <label class="fld rules">Rules (quality.yaml)
          <textarea v-model="rulesYaml" rows="12" spellcheck="false" data-testid="quality-rules" aria-describedby="quality-rules-help"></textarea>
          <span id="quality-rules-help" class="help">YAML as in <code>quality.yaml</code>; unknown keys are errors. <button type="button" class="link" @click="rulesYaml = EXAMPLE_RULES" data-testid="quality-example">Insert example</button></span>
        </label>
        <div class="side">
          <label class="fld">Run (troubleshooting.run)
            <input v-model="run" required placeholder="pr-42-1" data-testid="quality-run"/>
          </label>
          <label class="fld">Window
            <select v-model="preset" data-testid="quality-window">
              <option v-for="w in WINDOW_PRESETS" :key="w.value" :value="w.value">{{ w.label }}</option>
            </select>
          </label>
          <label class="fld">Baseline run (optional)
            <input v-model="baseline" placeholder="main-118" data-testid="quality-baseline"/>
          </label>
          <label class="fld">Baseline window
            <select v-model="baselinePreset" :disabled="!baseline.trim()" data-testid="quality-baseline-window">
              <option value="">Same as the run</option>
              <option v-for="w in WINDOW_PRESETS" :key="w.value" :value="w.value">{{ w.label }}</option>
            </select>
          </label>
          <div class="actions">
            <PrimeButton type="submit" label="Check run" size="small" :loading="loading" :disabled="loading || !run.trim() || !rulesYaml.trim()" data-testid="quality-evaluate"/>
            <label class="file link" data-testid="quality-open-file">Open a result file
              <input type="file" accept=".json,application/json" class="sr-only" @change="openFile" data-testid="quality-file"/>
            </label>
          </div>
          <p class="text-xs opacity-80">A check made here is not recorded.</p>
        </div>
      </form>

      <p v-if="error" class="state bad" role="alert" data-testid="quality-error">{{ error }}</p>
      <QualityResult v-if="result" :result="result" :sourceLabel="source" @open-execution="$emit('open-execution', $event)"/>
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent, nextTick, ref, watch, type PropType } from 'vue'
import PrimeButton from 'primevue/button'
import qualityApi from '@/api/quality.api'
import { Quality } from '@/logs-api/quality.impl'
import QualityResult from '@/components/quality/QualityResult.vue'
import QualityRequirements from '@/components/quality/QualityRequirements.vue'
import QualityHistory from '@/components/quality/QualityHistory.vue'
import { EXAMPLE_RULES, WINDOW_PRESETS, evaluationHash, parseRecord, parseResult, presetWindow } from '@/services/quality'
import { type QualityEvaluation, type QualityProject, type QualityRecord } from '@/types/quality'

const DRAFT_KEY = 'troubleshooting.quality.draft'
const TAB_KEY = 'troubleshooting.quality.tab'

type Tab = 'requirements' | 'history' | 'check'
const TABS: { value: Tab, label: string }[] = [
  { value: 'requirements', label: 'Requirements' },
  { value: 'history', label: 'History' },
  { value: 'check', label: 'Check a run' }
]

function readStorage (key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}
function writeStorage (key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  } catch {
    // storage unavailable: nothing to remember
  }
}

// QualityView (docs/quality-gate.md): the requirements catalogue and history
// of recorded evaluations (stage 5), and checking a run as typed or opening
// a CI result file (not recorded).
export default defineComponent({
  name: 'QualityView',
  components: { PrimeButton, QualityResult, QualityRequirements, QualityHistory },
  props: {
    // A recorded evaluation to open (#/quality?evaluation=<id>).
    evaluationRoute: { type: Object as PropType<{ id: string, n: number } | null>, default: null }
  },
  emits: ['open-execution', 'navigate'],
  setup (props, { emit }) {
    qualityApi.init(new Quality())
    // The rules and run ids stay in this browser as a convenience only.
    let draft: any = {}
    try {
      draft = JSON.parse(readStorage(DRAFT_KEY) ?? '{}') ?? {}
    } catch {
      draft = {}
    }
    const rulesYaml = ref<string>(typeof draft.rulesYaml === 'string' ? draft.rulesYaml : EXAMPLE_RULES)
    const run = ref<string>(typeof draft.run === 'string' ? draft.run : '')
    const baseline = ref<string>(typeof draft.baseline === 'string' ? draft.baseline : '')
    const preset = ref<string>(WINDOW_PRESETS.some((w) => w.value === draft.preset) ? draft.preset : '1h')
    const baselinePreset = ref<string>(WINDOW_PRESETS.some((w) => w.value === draft.baselinePreset) ? draft.baselinePreset : '')
    watch([rulesYaml, run, baseline, preset, baselinePreset], () => {
      writeStorage(DRAFT_KEY, JSON.stringify({ rulesYaml: rulesYaml.value, run: run.value, baseline: baseline.value, preset: preset.value, baselinePreset: baselinePreset.value }))
    })

    // The tab: the one chosen last; else Requirements once something is
    // recorded, and Check a run before that.
    const storedTab = readStorage(TAB_KEY)
    const tab = ref<Tab>(TABS.some((t) => t.value === storedTab) ? storedTab as Tab : 'check')
    const tabButtons = ref<HTMLButtonElement[]>([])
    function selectTab (t: Tab) {
      tab.value = t
      writeStorage(TAB_KEY, t)
      if (saved.value) {
        closeSaved()
      }
    }
    function onTabKey (event: KeyboardEvent, i: number) {
      const next = ({ ArrowRight: (i + 1) % TABS.length, ArrowLeft: (i + TABS.length - 1) % TABS.length, Home: 0, End: TABS.length - 1 } as Record<string, number>)[event.key]
      if (next === undefined) {
        return
      }
      event.preventDefault()
      selectTab(TABS[next].value)
      nextTick(() => tabButtons.value[next]?.focus())
    }

    // Recorded projects.
    const projects = ref<QualityProject[]>([])
    const projectsLoaded = ref(false)
    const projectsError = ref('')
    const project = ref('')
    const refName = ref('')
    const reloadKey = ref(0)
    async function loadProjects () {
      projectsError.value = ''
      const [err, list] = await qualityApi.projects()
      projectsLoaded.value = true
      if (err || !list) {
        projectsError.value = err ?? 'no answer'
        return
      }
      projects.value = list
      if (!project.value || !list.some((p) => p.project === project.value)) {
        project.value = list[0]?.project ?? ''
      }
      if (list.length > 0 && storedTab === null && !props.evaluationRoute) {
        tab.value = 'requirements'
      }
    }
    loadProjects()

    // A recorded evaluation, opened from the catalogue, the history or a link.
    const saved = ref<QualityRecord | null>(null)
    const savedError = ref('')
    let savedSeq = 0
    async function openEvaluation (id: string) {
      const my = ++savedSeq
      savedError.value = ''
      const [err, data] = await qualityApi.record(id)
      if (my !== savedSeq) {
        return
      }
      const [perr, rec] = err ? [err, null] : parseRecord(data)
      if (perr || !rec) {
        savedError.value = 'The evaluation ' + id + ' could not be opened: ' + perr
        saved.value = null
        return
      }
      if (tab.value === 'check') {
        tab.value = 'history'
      }
      saved.value = rec
      const p = rec.evaluation.project
      if (p && !projects.value.some((x) => x.project === p)) {
        await loadProjects()
      }
      if (p) {
        project.value = p
      }
      emit('navigate', evaluationHash(id))
    }
    function closeSaved () {
      savedSeq++
      saved.value = null
      emit('navigate', '#/quality')
    }
    watch(() => props.evaluationRoute, (r) => {
      if (r) {
        openEvaluation(r.id)
      }
    }, { immediate: true })

    // Checking a run as typed, or a CI result file.
    const loading = ref(false)
    const error = ref('')
    const result = ref<QualityEvaluation | null>(null)
    const source = ref('')
    // A newer request wins: an older answer arriving later is ignored.
    let seq = 0
    async function evaluate () {
      const my = ++seq
      loading.value = true
      error.value = ''
      const now = Date.now()
      const w = presetWindow(preset.value, now)
      const b = baseline.value.trim()
      const bw = b && baselinePreset.value ? presetWindow(baselinePreset.value, now) : null
      const [err, ev] = await qualityApi.evaluate({
        rulesYaml: rulesYaml.value, run: run.value.trim(), timeFrom: w.from, timeTo: w.to,
        baseline: b || undefined, baselineFrom: bw?.from, baselineTo: bw?.to
      })
      if (my !== seq) {
        return
      }
      loading.value = false
      if (err || !ev) {
        error.value = 'The run could not be checked: ' + (err ?? 'no answer')
        return
      }
      result.value = ev
      source.value = 'Checked now'
    }
    async function openFile (event: Event) {
      const input = event.target as HTMLInputElement
      const file = input.files?.[0]
      input.value = ''
      if (!file) {
        return
      }
      seq++
      loading.value = false
      const [err, ev] = parseResult(await file.text())
      if (err || !ev) {
        error.value = file.name + ': ' + err
        return
      }
      error.value = ''
      result.value = ev
      source.value = 'From the file ' + file.name
    }

    return {
      TABS, EXAMPLE_RULES, WINDOW_PRESETS, tab, tabButtons, selectTab, onTabKey,
      projects, projectsLoaded, projectsError, project, refName, reloadKey, loadProjects,
      saved, savedError, openEvaluation, closeSaved,
      rulesYaml, run, baseline, preset, baselinePreset, loading, error, result, source, evaluate, openFile
    }
  }
})
</script>

<style scoped>
.qv { font-size: 14px; }
.title { font-size: 25px; font-weight: 500; margin: 0; }
.tabs { display: flex; flex-wrap: wrap; gap: 2px 18px; margin-top: 12px; border-bottom: 1px solid var(--surface-border); }
.tabs button { background: none; border: 0; border-bottom: 2px solid transparent; padding: 8px 2px; margin-bottom: -1px; color: var(--text-color-secondary); font: inherit; cursor: pointer; }
.tabs button[aria-selected="true"] { color: var(--primary-color); border-bottom-color: var(--primary-color); }
.tabs button:focus-visible { outline: 2px solid var(--primary-color); outline-offset: 2px; }
.panel { margin-top: 12px; }
.back { margin-bottom: 4px; }
.empty { padding: 12px; border: 1px dashed var(--surface-border); border-radius: 8px; }
.empty p + p { margin-top: 6px; }
.filters { display: flex; flex-wrap: wrap; align-items: end; gap: 10px 16px; margin-bottom: 12px; }
.form { display: grid; grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr); gap: 16px; }
@media (max-width: 800px) { .form { grid-template-columns: minmax(0, 1fr); } }
.side { display: flex; flex-direction: column; gap: 10px; }
.fld { display: flex; flex-direction: column; gap: 3px; font-size: 12px; color: var(--text-color-secondary); min-width: 0; }
.fld input, .fld select, .fld textarea { background: var(--surface-ground); color: var(--text-color); border: 1px solid var(--surface-border); border-radius: 6px; padding: 8px; font: inherit; font-size: 14px; }
.fld textarea { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: 13px; resize: vertical; width: 100%; }
.fld input:focus-visible, .fld select:focus-visible, .fld textarea:focus-visible { outline: 2px solid var(--primary-color); }
.help { font-size: 12px; }
.actions { display: flex; flex-wrap: wrap; gap: 14px; align-items: center; }
.file { cursor: pointer; }
.file:focus-within { outline: 2px solid var(--primary-color); outline-offset: 2px; }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
.state { padding: 8px; border-radius: 6px; font-size: 13px; margin-top: 10px; }
.state.bad { background: rgba(248, 113, 113, 0.12); color: #fca5a5; }
.link { background: none; border: 0; color: var(--primary-color); cursor: pointer; padding: 0; font: inherit; }
code { overflow-wrap: anywhere; }
</style>
