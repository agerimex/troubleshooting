<template>
  <div data-testid="detection-rules">
    <div class="flex justify-between items-start flex-wrap gap-2">
      <div>
        <h2 class="title">What counts as a problem?</h2>
        <div class="text-xs opacity-80" data-testid="rules-revision">
          <template v-if="saved && saved.revision > 0">Revision {{ saved.revision }} · saved {{ time(saved.savedAt) }} · effective from {{ time(saved.effectiveAt) }}</template>
          <template v-else-if="saved">Built-in defaults (not saved yet) · applies to every service of this installation</template>
        </div>
      </div>
      <span v-if="form" class="pill">{{ enabledCount }} rule{{ enabledCount === 1 ? '' : 's' }} enabled</span>
    </div>
    <p v-if="loadError" class="state bad" data-testid="rules-load-error">The rules could not be loaded: {{ loadError }} <button type="button" class="link" @click="load">Retry</button></p>
    <p v-if="!form && !loadError">Loading…</p>

    <form v-if="form" class="mt-3" @submit.prevent="save" novalidate>
      <fieldset class="rule">
        <label class="check"><input type="checkbox" v-model="form.errors.enabled" data-testid="rule-errors-enabled"/> <b>Error signals</b></label>
        <p class="text-xs opacity-80">An ERROR span, an exception event or an ERROR log can open a problem; each source kind is grouped and counted separately.</p>
        <div class="fields">
          <label class="field">Open after
            <input type="number" min="1" step="1" v-model.number="form.errors.minAffectedCalls" :disabled="!form.errors.enabled" :aria-invalid="!!errorOf('errors.minAffectedCalls')" data-testid="rule-errors-calls"/>
            <span class="hint">affected calls in the selected window</span>
            <span v-if="errorOf('errors.minAffectedCalls')" class="ferr" role="alert">{{ errorOf('errors.minAffectedCalls') }}</span>
          </label>
          <label class="field">Group matching errors
            <select v-model="form.errors.grouping" :disabled="!form.errors.enabled" data-testid="rule-errors-grouping">
              <option value="operation_signature">By operation and error signature</option>
            </select>
            <span class="hint">Kept across releases (errors-v1)</span>
          </label>
        </div>
        <p class="text-xs opacity-80">Errors whose records name no span can't count as calls: they are listed as "Call attribution unavailable" and never satisfy the threshold.</p>
      </fieldset>

      <fieldset class="rule">
        <label class="check"><input type="checkbox" v-model="form.slow.enabled" data-testid="rule-slow-enabled"/> <b>Slow calls</b></label>
        <p class="text-xs opacity-80">Group repeated calls of an operation that exceed an absolute duration.</p>
        <div class="fields">
          <label class="field">Duration above
            <input type="number" min="0" step="any" v-model.number="form.slow.durationMs" :disabled="!form.slow.enabled" :aria-invalid="!!errorOf('slow.durationMs')" data-testid="rule-slow-duration"/>
            <span class="hint">milliseconds per call</span>
            <span v-if="errorOf('slow.durationMs')" class="ferr" role="alert" data-testid="rule-slow-duration-error">{{ errorOf('slow.durationMs') }}</span>
          </label>
          <label class="field">Open after
            <input type="number" min="1" step="1" v-model.number="form.slow.minCalls" :disabled="!form.slow.enabled" :aria-invalid="!!errorOf('slow.minCalls')" data-testid="rule-slow-calls"/>
            <span class="hint">slow calls in the selected window</span>
            <span v-if="errorOf('slow.minCalls')" class="ferr" role="alert">{{ errorOf('slow.minCalls') }}</span>
          </label>
        </div>
      </fieldset>

      <fieldset class="rule">
        <label class="check"><input type="checkbox" v-model="form.regressions.enabled" data-testid="rule-regressions-enabled"/> <b>Release regressions</b></label>
        <p class="text-xs opacity-80">Compare operation latency between releases. Both thresholds must be met; only generated comparisons with an eligible p95 count.</p>
        <div class="fields">
          <label class="field">p95 increase
            <input type="number" min="0" step="any" v-model.number="form.regressions.p95RelativePct" :disabled="!form.regressions.enabled" :aria-invalid="!!errorOf('regressions.p95RelativePct')" data-testid="rule-reg-pct"/>
            <span class="hint">percent or more</span>
            <span v-if="errorOf('regressions.p95RelativePct')" class="ferr" role="alert">{{ errorOf('regressions.p95RelativePct') }}</span>
          </label>
          <span class="and">and</span>
          <label class="field">p95 increase
            <input type="number" min="0" step="any" v-model.number="form.regressions.p95AbsoluteMs" :disabled="!form.regressions.enabled" :aria-invalid="!!errorOf('regressions.p95AbsoluteMs')" data-testid="rule-reg-ms"/>
            <span class="hint">milliseconds or more</span>
            <span v-if="errorOf('regressions.p95AbsoluteMs')" class="ferr" role="alert">{{ errorOf('regressions.p95AbsoluteMs') }}</span>
          </label>
        </div>
        <p class="text-xs opacity-80">The comparison's own error-fraction rule (+1 percentage point with at least 1,000 calls) and sample minimums stay as they are.</p>
      </fieldset>

      <details class="text-xs mt-2">
        <summary>Evaluation details</summary>
        <p>Problems are evaluated when the list is loaded, over the selected window, with the latest saved revision. Error groups and call counts come from the hourly prepared error statistics; slow calls from retained raw spans (unavailable after raw retention); regressions from stored comparisons. Saving doesn't rescan history or rewrite saved comparisons.</p>
      </details>

      <div class="actions mt-3">
        <PrimeButton type="submit" label="Save rules" :loading="state === 'saving'" :disabled="state === 'saving' || !dirty" data-testid="rules-save"/>
        <span class="text-sm" role="status" aria-live="polite" data-testid="rules-state">
          <template v-if="state === 'saving'">Saving…</template>
          <template v-else-if="state === 'saved' && !dirty">Saved as revision {{ saved?.revision }}. Applies from the next evaluation.</template>
          <template v-else-if="state === 'invalid'"><span class="bad">Not saved: correct the highlighted fields.</span></template>
          <template v-else-if="state === 'failed'"><span class="bad">Not saved: {{ message }}. Your changes are kept.</span></template>
          <template v-else-if="state === 'conflict'"><span class="bad">Not saved: revision {{ conflictRevision }} was saved elsewhere since you started. Your edits are kept.</span>
            <button type="button" class="link" @click="reloadLatest" data-testid="rules-reload">Load revision {{ conflictRevision }} (discards your edits)</button></template>
          <template v-else-if="dirty">Unsaved changes</template>
          <template v-else>Applies from the next evaluation</template>
        </span>
        <PrimeButton v-if="dirty && state !== 'saving'" type="button" label="Discard changes" text size="small" @click="discard" data-testid="rules-discard"/>
      </div>
    </form>
  </div>
</template>

<script lang="ts">
import { computed, defineComponent, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import PrimeButton from 'primevue/button'
import problemsApi from '@/api/problems.api'
import { Problems } from '@/logs-api/problems.impl'
import { formatNs } from '@/services/spanTree'
import { validateRules } from '@/services/problems'
import { type DetectionRules, type FieldError } from '@/types/problems'

type SaveState = 'idle' | 'saving' | 'saved' | 'invalid' | 'failed' | 'conflict'

// DetectionRules edits the three rule templates. A save creates a revision on
// the server only after validation; failures and conflicts keep the edits.
export default defineComponent({
  name: 'DetectionRules',
  components: { PrimeButton },
  emits: ['dirty', 'saved'],
  setup (_, { emit }) {
    problemsApi.init(new Problems())
    const saved = ref<DetectionRules | null>(null)
    const form = ref<DetectionRules | null>(null)
    const loadError = ref('')
    const state = ref<SaveState>('idle')
    const message = ref('')
    const fieldErrors = ref<FieldError[]>([])
    const conflict = ref<DetectionRules | null>(null)
    const clone = (r: DetectionRules) => JSON.parse(JSON.stringify(r)) as DetectionRules

    async function load () {
      loadError.value = ''
      const [err, rules] = await problemsApi.rules()
      if (err || !rules) {
        loadError.value = err ?? 'no answer'
        return
      }
      saved.value = rules
      form.value = clone(rules)
      state.value = 'idle'
      fieldErrors.value = []
    }
    onMounted(load)

    const editable = (r: DetectionRules) => JSON.stringify({ e: r.errors, s: r.slow, g: r.regressions })
    const dirty = computed(() => !!form.value && !!saved.value && editable(form.value) !== editable(saved.value))
    watch(dirty, (d) => emit('dirty', d), { immediate: true })
    const beforeUnload = (e: BeforeUnloadEvent) => {
      if (dirty.value) {
        e.preventDefault()
        e.returnValue = ''
      }
    }
    onMounted(() => window.addEventListener('beforeunload', beforeUnload))
    onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))

    const enabledCount = computed(() => form.value ? [form.value.errors.enabled, form.value.slow.enabled, form.value.regressions.enabled].filter(Boolean).length : 0)
    const errorOf = (field: string) => fieldErrors.value.find((f) => f.field === field)?.message ?? ''

    async function save () {
      if (!form.value || !saved.value) {
        return
      }
      const local = validateRules(form.value)
      if (local.length) {
        fieldErrors.value = local
        state.value = 'invalid'
        return
      }
      fieldErrors.value = []
      state.value = 'saving'
      // Always the revision these edits started from: after a conflict the
      // user must load the newer revision before saving over it.
      const result = await problemsApi.saveRules(clone(form.value), saved.value.revision)
      switch (result.kind) {
        case 'saved':
          saved.value = result.rules
          form.value = clone(result.rules)
          conflict.value = null
          state.value = 'saved'
          emit('saved', result.rules)
          break
        case 'invalid':
          fieldErrors.value = result.fieldErrors
          state.value = 'invalid'
          break
        case 'conflict':
          conflict.value = result.current
          state.value = 'conflict'
          message.value = result.message
          break
        case 'failed':
          state.value = 'failed'
          message.value = result.message
          break
      }
    }
    // reloadLatest takes the other session's revision (after the user chose
    // to discard their edits).
    function reloadLatest () {
      if (conflict.value) {
        saved.value = conflict.value
        form.value = clone(conflict.value)
        conflict.value = null
        state.value = 'idle'
      } else {
        load()
      }
    }
    function discard () {
      if (saved.value) {
        form.value = clone(saved.value)
        fieldErrors.value = []
        state.value = 'idle'
      }
    }
    const conflictRevision = computed(() => conflict.value?.revision ?? '?')
    const time = (ns?: string) => ns ? formatNs(ns).slice(0, 17) : ''
    return { saved, form, loadError, load, state, message, dirty, enabledCount, errorOf, save, reloadLatest, discard, conflictRevision, time }
  }
})
</script>

<style scoped>
.title { font-size: 25px; font-weight: 500; margin: 0; }
.pill { font-size: 12px; background: var(--surface-hover); padding: 3px 8px; border-radius: 6px; }
.rule { border: 0; border-top: 1px solid var(--surface-border); padding: 14px 0; margin: 0; }
.check { display: flex; align-items: center; gap: 8px; font-size: 16px; cursor: pointer; }
.check input { width: 18px; height: 18px; accent-color: var(--primary-color); }
.fields { display: flex; flex-wrap: wrap; gap: 14px; align-items: flex-start; margin-top: 8px; padding-left: 26px; }
.field { display: flex; flex-direction: column; gap: 4px; font-size: 13px; min-width: 14rem; }
.field input, .field select { background: var(--surface-ground); color: var(--text-color); border: 1px solid var(--surface-border); border-radius: 6px; padding: 8px; font: inherit; }
.field input:disabled, .field select:disabled { opacity: 0.55; }
.field input[aria-invalid="true"] { border-color: #f87171; }
.field input:focus-visible, .field select:focus-visible, .check input:focus-visible { outline: 2px solid var(--primary-color); }
.hint { font-size: 12px; color: var(--text-color-secondary); }
.ferr { font-size: 12px; color: #fca5a5; }
.and { align-self: center; color: var(--text-color-secondary); font-size: 13px; }
.actions { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; border-top: 1px solid var(--surface-border); padding-top: 14px; }
.state { padding: 8px; border-radius: 6px; font-size: 13px; }
.state.bad, .bad { color: #fca5a5; }
.link { background: none; border: 0; color: var(--primary-color); cursor: pointer; padding: 0 0 0 8px; font: inherit; }
</style>
