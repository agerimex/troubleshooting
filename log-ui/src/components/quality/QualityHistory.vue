<template>
  <section aria-labelledby="quality-history-title" data-testid="quality-history">
    <h3 id="quality-history-title" class="sr-only">Recorded evaluations of {{ project }}</h3>
    <p v-if="error" class="state bad" role="alert" data-testid="quality-history-error">The history could not be read: {{ error }} <button type="button" class="link" @click="load(false)">Retry</button></p>
    <p v-else-if="!loading && rows.length === 0" class="text-sm opacity-80" data-testid="quality-history-empty">
      No evaluation of <code>{{ project }}</code> is recorded{{ refName ? ' on ' + refName : '' }}.
    </p>
    <div v-if="rows.length" class="tablewrap">
      <table data-testid="quality-history-table">
        <thead><tr><th scope="col">Recorded</th><th scope="col">Gate</th><th scope="col">Commit</th><th scope="col">Run</th><th scope="col">Requirements</th><th scope="col"><span class="sr-only">Open</span></th></tr></thead>
        <tbody>
          <tr v-for="e in rows" :key="e.id" :data-testid="'quality-history-row-' + e.id">
            <td class="nowrap">{{ recordedText(e.recorded_at) }}</td>
            <td><span class="gate" :class="e.gate">{{ gateShort(e.gate) }}</span></td>
            <td><code v-if="e.source.commit">{{ e.source.commit.slice(0, 12) }}</code><span v-if="e.source.ref" class="text-xs opacity-80 ref">{{ e.source.ref }}</span></td>
            <td><code>{{ e.run }}</code></td>
            <td class="text-sm">{{ e.requirements.length ? requirementCounts(e.requirements) : '–' }}</td>
            <td><button type="button" class="link" @click="$emit('open-evaluation', e.id)" :data-testid="'quality-history-open-' + e.id">Open</button></td>
          </tr>
        </tbody>
      </table>
    </div>
    <p v-if="loading" class="text-sm opacity-80" role="status">Loading the history…</p>
    <button v-else-if="next" type="button" class="link more" @click="load(true)" data-testid="quality-history-more">Load older evaluations</button>
  </section>
</template>

<script lang="ts">
import { defineComponent, ref, watch } from 'vue'
import qualityApi from '@/api/quality.api'
import { recordedText, requirementCounts } from '@/services/quality'
import { type EvaluationSummary } from '@/types/quality'

// QualityHistory lists a project's recorded evaluations, newest first, with
// paging by the recorded time.
export default defineComponent({
  name: 'QualityHistory',
  props: {
    project: { type: String, required: true },
    refName: { type: String, default: '' },
    reloadKey: { type: Number, default: 0 }
  },
  emits: ['open-evaluation'],
  setup (props) {
    const rows = ref<EvaluationSummary[]>([])
    const next = ref('')
    const loading = ref(false)
    const error = ref('')
    let seq = 0
    async function load (more: boolean) {
      const my = ++seq
      loading.value = true
      error.value = ''
      const [err, page] = await qualityApi.evaluations({ project: props.project, ref: props.refName, before: more ? next.value : undefined })
      if (my !== seq) {
        return
      }
      loading.value = false
      if (err || !page) {
        error.value = err ?? 'no answer'
        return
      }
      rows.value = more ? [...rows.value, ...page.evaluations] : page.evaluations
      next.value = page.next
    }
    watch(() => [props.project, props.refName, props.reloadKey], () => {
      rows.value = []
      next.value = ''
      load(false)
    }, { immediate: true })
    const gateShort = (g: string) => ({ passed: 'Passed', failed: 'Failed', unresolved: 'Unresolved' } as Record<string, string>)[g] ?? g
    return { rows, next, loading, error, load, gateShort, recordedText, requirementCounts }
  }
})
</script>

<style scoped>
.tablewrap { overflow-x: auto; }
table { width: 100%; border-collapse: collapse; font-size: 13px; }
th, td { text-align: left; padding: 6px 8px; border-bottom: 1px solid var(--surface-border); vertical-align: top; }
th { font-weight: 600; color: var(--text-color-secondary); font-size: 12px; }
.nowrap { white-space: nowrap; }
.ref { margin-left: 6px; }
.gate { font-weight: 600; }
.gate.passed { color: #86efac; }
.gate.failed { color: #fca5a5; }
.gate.unresolved { color: #fcd34d; }
.more { margin-top: 8px; }
.state { padding: 8px; border-radius: 6px; font-size: 13px; margin-top: 10px; }
.state.bad { background: rgba(248, 113, 113, 0.12); color: #fca5a5; }
.link { background: none; border: 0; color: var(--primary-color); cursor: pointer; padding: 0; font: inherit; }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
</style>
