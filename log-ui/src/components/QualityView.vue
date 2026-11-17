<template>
  <div class="card qv" data-testid="quality-view">
    <div class="text-xs opacity-80">Test runs</div>
    <h2 class="title">Quality</h2>
    <p class="text-sm opacity-80">Check a test run against your rules: latency limits, database calls per execution and forbidden calls. Every failure opens the execution behind it. A missing measurement is never a pass.</p>

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
      </div>
    </form>

    <p v-if="error" class="state bad" role="alert" data-testid="quality-error">{{ error }}</p>

    <section v-if="result" class="result" aria-labelledby="quality-gate" data-testid="quality-result">
      <div class="gate" :class="result.gate" data-testid="quality-gate-box">
        <h3 id="quality-gate" data-testid="quality-gate">Quality gate: {{ gateLabel(result.gate) }}</h3>
        <p class="text-sm">
          Run <code>{{ result.run }}</code> · {{ windowText(result.time_from, result.time_to) }}
          <template v-if="result.baseline"> · baseline <code>{{ result.baseline }}</code> · {{ windowText(result.baseline_time_from, result.baseline_time_to) }}</template>
        </p>
        <p class="text-xs opacity-80">{{ source }} · evaluated {{ result.evaluated_at }} · {{ result.contract }}</p>
        <p v-if="sourceText(result.source)" class="text-xs opacity-80" data-testid="quality-source">
          {{ sourceText(result.source) }}<template v-if="ciRunLink(result.source)"> · <a :href="ciRunLink(result.source)" target="_blank" rel="noopener noreferrer">CI run</a></template>
        </p>
        <div class="exports">
          <button type="button" class="link" @click="saveJson" data-testid="quality-export-json">Export JSON</button>
          <button type="button" class="link" @click="saveHtml" data-testid="quality-export-html">Export HTML</button>
        </div>
      </div>

      <ul class="checks" data-testid="quality-checks">
        <li v-for="c in result.checks" :key="c.id" class="check" :data-testid="'quality-check-' + c.id">
          <div class="head">
            <span class="status" :class="c.status" :data-testid="'quality-status-' + c.id">{{ statusLabel(c.status) }}</span>
            <b>{{ c.id }}</b><span v-if="c.advisory" class="pill">advisory</span>
            <span class="text-xs opacity-80">{{ entryText(c) }}</span>
          </div>
          <dl class="nums">
            <div><dt>Value{{ c.unit && c.unit !== 'ms' ? ' (' + c.unit + ')' : '' }}</dt><dd :data-testid="'quality-value-' + c.id">{{ valueText(c.value, c.unit) }}</dd></div>
            <div><dt>Limit</dt><dd>{{ limitText(c) }}</dd></div>
            <div v-if="c.baseline"><dt>Baseline</dt><dd :data-testid="'quality-baseline-' + c.id">{{ baselineText(c) }}</dd></div>
            <div><dt>Executions</dt><dd>{{ c.executions }}</dd></div>
          </dl>
          <p class="summary">{{ c.summary }}</p>
          <ul v-if="c.details.length" class="details">
            <li v-for="(d, i) in c.details" :key="i">{{ d }}</li>
          </ul>
          <div v-for="(e, i) in c.evidence" :key="i" class="evidence">
            <span>Evidence: {{ e.note }}</span>
            <template v-if="targetOf(c, e)">
              <button type="button" class="link" @click="open(c, e, 'timeline')" :data-testid="'quality-open-timeline-' + c.id">Open timeline</button>
              <button type="button" class="link" @click="open(c, e, 'sequence')" :data-testid="'quality-open-sequence-' + c.id">Open sequence</button>
              <span v-if="snapshotCount(e)" class="text-xs opacity-80" :data-testid="'quality-snapshot-' + c.id"
                title="Shown when the live spans of this trace are not available">saved snapshot: {{ snapshotCount(e) }} spans</span>
            </template>
            <span v-else class="text-xs opacity-80">trace {{ e.trace_id }} (not a valid execution link)</span>
          </div>
        </li>
      </ul>
      <p class="text-xs opacity-80 mt-2">{{ result.method }}. Statuses describe this run only: a pass means "not observed in this run", not a guarantee. Advisory checks, targets and the baseline are shown but never decide the gate.</p>
    </section>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref, watch } from 'vue'
import PrimeButton from 'primevue/button'
import qualityApi from '@/api/quality.api'
import { Quality } from '@/logs-api/quality.impl'
import { download } from '@/services/report'
import { type ExecutionView } from '@/services/navigation'
import {
  EXAMPLE_RULES, WINDOW_PRESETS, baselineText, entryText, evidenceTarget, exportHtml, exportJson, gateLabel, limitText,
  ciRunLink, nsToIso, parseResult, presetWindow, savedSpans, sourceText, statusLabel, valueText
} from '@/services/quality'
import { type QualityCheckResult, type QualityEvaluation, type QualityEvidence } from '@/types/quality'

const DRAFT_KEY = 'troubleshooting.quality.draft'

// QualityView checks a test run against rules (docs/quality-gate.md), or
// shows a result file from CI (troubleshooting-check --json). Evaluations
// are not stored: the result is what the API answered just now, or the file.
export default defineComponent({
  name: 'QualityView',
  components: { PrimeButton },
  emits: ['open-execution'],
  setup (_, { emit }) {
    qualityApi.init(new Quality())
    // The rules and run ids stay in this browser as a convenience only.
    let draft: any = {}
    try {
      draft = JSON.parse(localStorage.getItem(DRAFT_KEY) ?? '{}') ?? {}
    } catch {
      draft = {}
    }
    const rulesYaml = ref<string>(typeof draft.rulesYaml === 'string' ? draft.rulesYaml : EXAMPLE_RULES)
    const run = ref<string>(typeof draft.run === 'string' ? draft.run : '')
    const baseline = ref<string>(typeof draft.baseline === 'string' ? draft.baseline : '')
    const preset = ref<string>(WINDOW_PRESETS.some((w) => w.value === draft.preset) ? draft.preset : '1h')
    const baselinePreset = ref<string>(WINDOW_PRESETS.some((w) => w.value === draft.baselinePreset) ? draft.baselinePreset : '')
    watch([rulesYaml, run, baseline, preset, baselinePreset], () => {
      try {
        localStorage.setItem(DRAFT_KEY, JSON.stringify({ rulesYaml: rulesYaml.value, run: run.value, baseline: baseline.value, preset: preset.value, baselinePreset: baselinePreset.value }))
      } catch {
        // storage unavailable: nothing to remember
      }
    })

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

    // snapshotCount: the spans of the evidence's saved snapshot (a CI result).
    function snapshotCount (e: QualityEvidence) {
      return savedSpans(e.snapshot, e.trace_id).length
    }
    function targetOf (c: QualityCheckResult, e: QualityEvidence) {
      return evidenceTarget(c, e)
    }
    function open (c: QualityCheckResult, e: QualityEvidence, view: ExecutionView) {
      const t = evidenceTarget(c, e, view)
      if (t) {
        emit('open-execution', t)
      }
    }
    function windowText (from?: string, to?: string) {
      return nsToIso(from) + ' – ' + nsToIso(to) + ' (exclusive)'
    }
    const fileBase = () => 'quality-' + (result.value?.run ?? 'run').replace(/[^\w.-]+/g, '_')
    function saveJson () {
      if (result.value) {
        download(fileBase() + '.json', 'application/json', exportJson(result.value))
      }
    }
    function saveHtml () {
      if (result.value) {
        download(fileBase() + '.html', 'text/html', exportHtml(result.value))
      }
    }

    return {
      EXAMPLE_RULES, WINDOW_PRESETS, rulesYaml, run, baseline, preset, baselinePreset, loading, error, result, source,
      evaluate, openFile, targetOf, open, snapshotCount, windowText, saveJson, saveHtml,
      gateLabel, statusLabel, valueText, limitText, baselineText, entryText, sourceText, ciRunLink
    }
  }
})
</script>

<style scoped>
.qv { font-size: 14px; }
.title { font-size: 25px; font-weight: 500; margin: 0; }
.form { display: grid; grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr); gap: 16px; margin-top: 12px; }
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
.result { margin-top: 16px; }
.gate { padding: 12px; border-radius: 8px; border: 1px solid var(--surface-border); }
.gate h3 { font-size: 18px; margin: 0 0 4px; }
.gate.passed { background: rgba(74, 222, 128, 0.1); }
.gate.failed { background: rgba(248, 113, 113, 0.12); }
.gate.failed h3 { color: #fca5a5; }
.gate.unresolved { background: rgba(251, 191, 36, 0.12); }
.gate.unresolved h3 { color: #fcd34d; }
.exports { display: flex; gap: 14px; margin-top: 6px; }
.checks { list-style: none; padding: 0; margin: 12px 0 0; border-top: 1px solid var(--surface-border); }
.check { padding: 12px 0; border-bottom: 1px solid var(--surface-border); }
.head { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.status { font-size: 12px; padding: 2px 8px; border-radius: 4px; font-weight: 600; }
.status.pass { background: rgba(74, 222, 128, 0.15); color: #86efac; }
.status.fail { background: rgba(248, 113, 113, 0.15); color: #fca5a5; }
.status.insufficient { background: rgba(251, 191, 36, 0.15); color: #fcd34d; }
.pill { font-size: 12px; background: var(--surface-hover); padding: 1px 8px; border-radius: 6px; }
.nums { display: flex; flex-wrap: wrap; gap: 8px 24px; margin: 8px 0 4px; }
.nums dt { font-size: 12px; color: var(--text-color-secondary); }
.nums dd { margin: 0; font-weight: 600; }
.summary { margin: 4px 0; }
.details { margin: 4px 0; padding-left: 18px; font-size: 13px; }
.evidence { display: flex; flex-wrap: wrap; gap: 6px 14px; align-items: center; font-size: 13px; margin-top: 4px; }
.state { padding: 8px; border-radius: 6px; font-size: 13px; margin-top: 10px; }
.state.bad { background: rgba(248, 113, 113, 0.12); color: #fca5a5; }
.link { background: none; border: 0; color: var(--primary-color); cursor: pointer; padding: 0; font: inherit; }
code { overflow-wrap: anywhere; }
</style>
