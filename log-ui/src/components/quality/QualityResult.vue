<template>
  <section class="result" aria-labelledby="quality-gate" data-testid="quality-result">
    <div class="gate" :class="result.gate" data-testid="quality-gate-box">
      <h3 id="quality-gate" data-testid="quality-gate">Quality gate: {{ gateLabel(result.gate) }}</h3>
      <p class="text-sm">
        Run <code>{{ result.run }}</code> · {{ windowText(result.time_from, result.time_to) }}
        <template v-if="result.baseline"> · baseline <code>{{ result.baseline }}</code> · {{ windowText(result.baseline_time_from, result.baseline_time_to) }}</template>
      </p>
      <p class="text-xs opacity-80">{{ sourceLabel }} · evaluated {{ result.evaluated_at }} · {{ result.contract }}</p>
      <p v-if="sourceText(result.source)" class="text-xs opacity-80" data-testid="quality-source">
        {{ sourceText(result.source) }}<template v-if="ciRunLink(result.source)"> · <a :href="ciRunLink(result.source)" target="_blank" rel="noopener noreferrer">CI run</a></template>
      </p>
      <p v-if="result.id" class="text-xs opacity-80" data-testid="quality-recorded">
        Recorded {{ recordedText(result.recorded_at) }} as <code>{{ result.id }}</code><template v-if="result.project"> in project <code>{{ result.project }}</code></template><template v-if="result.reevaluated"> · a re-evaluation of inputs recorded before</template>
      </p>
      <div class="exports">
        <button type="button" class="link" @click="saveJson" data-testid="quality-export-json">Export JSON</button>
        <button type="button" class="link" @click="saveHtml" data-testid="quality-export-html">Export HTML</button>
        <template v-if="result.id">
          <button type="button" class="link" @click="copyLink" data-testid="quality-copy-link">Copy link</button>
          <input v-if="linkFallback" class="fallback" readonly :value="linkFallback" aria-label="Link to this evaluation" data-testid="quality-link-fallback" @focus="($event.target as HTMLInputElement).select()"/>
          <span class="sr-only" role="status">{{ copied }}</span>
          <span v-if="copied" class="text-xs opacity-80" aria-hidden="true">{{ copied }}</span>
        </template>
      </div>
    </div>

    <section v-if="requirements.length" class="reqs" aria-labelledby="quality-req-title" data-testid="quality-requirements-result">
      <h4 id="quality-req-title">Requirements: {{ requirementCounts(requirements) }}</h4>
      <ul>
        <li v-for="r in requirements" :key="r.id" :data-testid="'quality-req-' + r.id">
          <span class="rstatus" :class="r.status" :data-testid="'quality-req-status-' + r.id">{{ requirementLabel(r.status) }}</span>
          <b>{{ r.id }}</b> {{ r.title }}
          <span v-if="r.owner" class="text-xs opacity-80"> · owner {{ r.owner }}</span>
          <div class="text-xs opacity-80">{{ r.summary }}<template v-if="r.status === 'met' && r.checks.length">: {{ r.checks.join(', ') }}</template></div>
          <div v-if="r.guardrail_summary" class="text-xs opacity-80" :data-testid="'quality-req-guardrails-' + r.id">{{ r.guardrail_summary }}</div>
        </li>
      </ul>
    </section>

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
          <template v-if="evidenceTarget(c, e)">
            <button type="button" class="link" @click="open(c, e, 'timeline')" :data-testid="'quality-open-timeline-' + c.id">Open timeline</button>
            <button type="button" class="link" @click="open(c, e, 'sequence')" :data-testid="'quality-open-sequence-' + c.id">Open sequence</button>
            <span v-if="snapshotCount(e)" class="text-xs opacity-80" :data-testid="'quality-snapshot-' + c.id"
              title="Shown when the live spans of this trace are not available">saved snapshot: {{ snapshotCount(e) }} spans</span>
          </template>
          <span v-else class="text-xs opacity-80">trace {{ e.trace_id }} (not a valid execution link)</span>
        </div>
      </li>
    </ul>
    <details v-if="rules" class="rules" data-testid="quality-record-rules">
      <summary>Rules of this evaluation</summary>
      <pre>{{ rules }}</pre>
    </details>
    <ul v-if="result.assumptions?.length" class="assumptions text-xs opacity-80" data-testid="quality-assumptions">
      <li v-for="(a, i) in result.assumptions" :key="i">{{ a }}</li>
    </ul>
    <p class="text-xs opacity-80 mt-2">{{ result.method }}. Statuses describe this run only: a pass means "not observed in this run", not a guarantee. Advisory checks, targets and the baseline are shown but never decide the gate.</p>
  </section>
</template>

<script lang="ts">
import { computed, defineComponent, ref, type PropType } from 'vue'
import { download } from '@/services/report'
import { type ExecutionView } from '@/services/navigation'
import {
  baselineText, ciRunLink, entryText, evaluationHash, evidenceTarget, exportHtml, exportJson, gateLabel, limitText, nsToIso,
  recordedText, requirementCounts, requirementLabel, savedSpans, sourceText, statusLabel, valueText
} from '@/services/quality'
import { type QualityCheckResult, type QualityEvaluation, type QualityEvidence } from '@/types/quality'

// QualityResult shows one evaluation: checked now, read from a CI result
// file, or recorded (with its rules and a link that reopens it).
export default defineComponent({
  name: 'QualityResult',
  props: {
    result: { type: Object as PropType<QualityEvaluation>, required: true },
    sourceLabel: { type: String, default: '' },
    rules: { type: String, default: '' }
  },
  emits: ['open-execution'],
  setup (props, { emit }) {
    const requirements = computed(() => props.result.requirements ?? [])
    function snapshotCount (e: QualityEvidence) {
      return savedSpans(e.snapshot, e.trace_id).length
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
    const fileBase = () => 'quality-' + (props.result.id || props.result.run || 'run').replace(/[^\w.-]+/g, '_')
    function saveJson () {
      download(fileBase() + '.json', 'application/json', exportJson(props.result))
    }
    function saveHtml () {
      download(fileBase() + '.html', 'text/html', exportHtml(props.result))
    }
    const copied = ref('')
    const linkFallback = ref('')
    async function copyLink () {
      const link = window.location.href.replace(/#.*$/, '') + evaluationHash(props.result.id ?? '')
      try {
        await navigator.clipboard.writeText(link)
        copied.value = 'Link copied'
        linkFallback.value = ''
      } catch {
        copied.value = ''
        linkFallback.value = link
      }
    }
    return {
      requirements, snapshotCount, open, windowText, saveJson, saveHtml, copyLink, copied, linkFallback,
      evidenceTarget, gateLabel, statusLabel, valueText, limitText, baselineText, entryText, sourceText, ciRunLink,
      recordedText, requirementCounts, requirementLabel
    }
  }
})
</script>

<style scoped>
.result { margin-top: 16px; }
.gate { padding: 12px; border-radius: 8px; border: 1px solid var(--surface-border); }
.gate h3 { font-size: 18px; margin: 0 0 4px; }
.gate.passed { background: rgba(74, 222, 128, 0.1); }
.gate.failed { background: rgba(248, 113, 113, 0.12); }
.gate.failed h3 { color: #fca5a5; }
.gate.unresolved { background: rgba(251, 191, 36, 0.12); }
.gate.unresolved h3 { color: #fcd34d; }
.exports { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 14px; margin-top: 6px; }
.fallback { min-width: 0; flex: 1 1 260px; background: var(--surface-ground); color: var(--text-color); border: 1px solid var(--surface-border); border-radius: 4px; padding: 2px 6px; font-size: 12px; }
.reqs { margin-top: 12px; }
.reqs h4 { font-size: 15px; margin: 0 0 4px; }
.reqs ul { list-style: none; padding: 0; margin: 0; }
.reqs li { padding: 6px 0; border-bottom: 1px solid var(--surface-border); }
.rstatus { display: inline-block; min-width: 96px; margin-right: 6px; font-size: 12px; padding: 1px 8px; border-radius: 4px; font-weight: 600; text-align: center; }
.rstatus.met { background: rgba(74, 222, 128, 0.15); color: #86efac; }
.rstatus.violated { background: rgba(248, 113, 113, 0.15); color: #fca5a5; }
.rstatus.insufficient { background: rgba(251, 191, 36, 0.15); color: #fcd34d; }
.rstatus.not_evaluated { background: var(--surface-hover); color: var(--text-color-secondary); }
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
.assumptions { margin: 10px 0 0; padding-left: 18px; }
.rules { margin-top: 10px; font-size: 13px; }
.rules summary { cursor: pointer; color: var(--primary-color); }
.rules pre { margin: 6px 0 0; padding: 8px; background: var(--surface-ground); border: 1px solid var(--surface-border); border-radius: 6px; overflow-x: auto; font-size: 12px; }
.link { background: none; border: 0; color: var(--primary-color); cursor: pointer; padding: 0; font: inherit; }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
code { overflow-wrap: anywhere; }
</style>
