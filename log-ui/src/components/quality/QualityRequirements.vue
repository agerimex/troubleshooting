<template>
  <section aria-labelledby="quality-req-catalogue" data-testid="quality-catalogue">
    <h3 id="quality-req-catalogue" class="sr-only">Requirements of {{ project }}</h3>
    <p v-if="loading" class="text-sm opacity-80" role="status">Loading the requirements…</p>
    <p v-else-if="error" class="state bad" role="alert" data-testid="quality-catalogue-error">The requirements could not be read: {{ error }} <button type="button" class="link" @click="load">Retry</button></p>
    <template v-else-if="catalogue">
      <p v-if="!catalogue.latest" class="text-sm opacity-80" data-testid="quality-catalogue-empty">
        No evaluation of <code>{{ project }}</code> is recorded{{ refName ? ' on ' + refName : '' }}.
      </p>
      <template v-else>
        <div class="latest" data-testid="quality-catalogue-latest">
          <span>As of the latest evaluation:</span>
          <span class="gate" :class="catalogue.latest.gate">{{ gateLabel(catalogue.latest.gate) }}</span>
          <span class="text-sm">{{ latestSource }}</span>
          <span class="text-xs opacity-80">{{ recordedText(catalogue.latest.recorded_at) }}</span>
          <button type="button" class="link" @click="$emit('open-evaluation', catalogue.latest.id)" data-testid="quality-catalogue-open-latest">Open evaluation</button>
        </div>
        <p v-if="catalogue.requirements.length" class="text-sm summary-line" data-testid="quality-catalogue-counts">{{ catalogue.requirements.length }} requirements: {{ requirementCounts(catalogue.requirements) }}</p>
        <p v-else class="text-sm opacity-80" data-testid="quality-catalogue-none">
          The rules of this evaluation have no requirements yet. Add a <code>requirements:</code> list to <code>quality.yaml</code>: each names an owner, a reason and the checks that enforce it.
        </p>
        <ul class="reqlist">
          <li v-for="r in catalogue.requirements" :key="r.id" class="req" :data-testid="'quality-catalogue-req-' + r.id">
            <div class="rhead">
              <span class="rstatus" :class="r.status" :data-testid="'quality-catalogue-status-' + r.id">{{ requirementLabel(r.status) }}</span>
              <b>{{ r.id }}</b>
              <span class="rtitle">{{ r.title }}</span>
            </div>
            <p class="meta">
              <span v-if="r.owner">Owner: {{ r.owner }}</span>
              <span v-else class="opacity-80">No owner</span>
              <span v-if="r.why"> · {{ r.why }}</span>
            </p>
            <p class="text-sm">{{ r.summary }}</p>
            <div v-if="r.checks.length" class="chks">
              <span v-for="id in r.checks" :key="id" class="chk" :class="checkStatus(id)" :title="checkSummary(id)">{{ id }}: {{ statusLabel(checkStatus(id)) }}</span>
            </div>
            <div class="hist">
              <span class="text-xs opacity-80" :data-testid="'quality-catalogue-streak-' + r.id">{{ streakText(r) }}</span>
              <ol class="cells" :aria-label="'History of ' + r.id + ', oldest first'">
                <li v-for="(h, i) in historyCells(r.history)" :key="h.evaluation_id">
                  <button type="button" class="cell" :class="[h.status, { changed: rulesChanged(r.history, r.history.length - 1 - i) }]"
                    :title="h.label + (rulesChanged(r.history, r.history.length - 1 - i) ? ' · rules changed' : '')"
                    :aria-label="h.label" @click="$emit('open-evaluation', h.evaluation_id)"></button>
                </li>
              </ol>
            </div>
          </li>
        </ul>
        <p v-if="catalogue.unlinked.length" class="text-xs opacity-80" data-testid="quality-catalogue-unlinked">Checks without a requirement: {{ catalogue.unlinked.join(', ') }}</p>
        <p class="text-xs opacity-80 mt-2">A requirement's status comes from its checks in the latest evaluation; one without a check is listed and never met. The history shows the latest {{ catalogue.requirements[0]?.history.length ?? 0 }} recorded evaluations (at most 30), oldest on the left; a marked cell's rules differ from the one before it.</p>
      </template>
    </template>
  </section>
</template>

<script lang="ts">
import { computed, defineComponent, ref, watch } from 'vue'
import qualityApi from '@/api/quality.api'
import {
  gateLabel, historyCells, recordedText, requirementCounts, requirementLabel, rulesChanged, sourceText, statusLabel, streakText
} from '@/services/quality'
import { type Catalogue } from '@/types/quality'

// QualityRequirements is a project's requirements catalogue: each requirement
// as the latest recorded evaluation states it, with its history.
export default defineComponent({
  name: 'QualityRequirements',
  props: {
    project: { type: String, required: true },
    refName: { type: String, default: '' },
    reloadKey: { type: Number, default: 0 }
  },
  emits: ['open-evaluation'],
  setup (props) {
    const catalogue = ref<Catalogue | null>(null)
    const loading = ref(false)
    const error = ref('')
    let seq = 0
    async function load () {
      const my = ++seq
      loading.value = true
      error.value = ''
      const [err, cat] = await qualityApi.catalogue(props.project, props.refName)
      if (my !== seq) {
        return
      }
      loading.value = false
      if (err || !cat) {
        error.value = err ?? 'no answer'
        catalogue.value = null
        return
      }
      catalogue.value = cat
    }
    watch(() => [props.project, props.refName, props.reloadKey], load, { immediate: true })
    const byId = computed(() => new Map((catalogue.value?.checks ?? []).map((c) => [c.id, c])))
    const checkStatus = (id: string) => byId.value.get(id)?.status ?? 'insufficient'
    const checkSummary = (id: string) => byId.value.get(id)?.summary ?? ''
    const latestSource = computed(() => (catalogue.value?.latest ? sourceText(catalogue.value.latest.source) : ''))
    return {
      catalogue, loading, error, load, checkStatus, checkSummary, latestSource,
      gateLabel, historyCells, recordedText, requirementCounts, requirementLabel, rulesChanged, statusLabel, streakText
    }
  }
})
</script>

<style scoped>
.latest { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 12px; padding: 10px 12px; border: 1px solid var(--surface-border); border-radius: 8px; }
.gate { font-weight: 600; }
.gate.passed { color: #86efac; }
.gate.failed { color: #fca5a5; }
.gate.unresolved { color: #fcd34d; }
.summary-line { margin: 10px 0 0; }
.reqlist { list-style: none; padding: 0; margin: 8px 0 0; border-top: 1px solid var(--surface-border); }
.req { padding: 12px 0; border-bottom: 1px solid var(--surface-border); }
.rhead { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.rtitle { font-weight: 500; }
.meta { margin: 4px 0; font-size: 13px; color: var(--text-color-secondary); }
.rstatus { font-size: 12px; padding: 2px 8px; border-radius: 4px; font-weight: 600; }
.rstatus.met { background: rgba(74, 222, 128, 0.15); color: #86efac; }
.rstatus.violated { background: rgba(248, 113, 113, 0.15); color: #fca5a5; }
.rstatus.insufficient { background: rgba(251, 191, 36, 0.15); color: #fcd34d; }
.rstatus.not_evaluated { background: var(--surface-hover); color: var(--text-color-secondary); }
.chks { display: flex; flex-wrap: wrap; gap: 6px; margin: 6px 0; }
.chk { font-size: 12px; padding: 1px 8px; border-radius: 10px; border: 1px solid var(--surface-border); }
.chk.pass { color: #86efac; }
.chk.fail { color: #fca5a5; border-color: rgba(248, 113, 113, 0.5); }
.chk.insufficient { color: #fcd34d; }
.hist { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 12px; margin-top: 6px; }
.cells { display: flex; flex-wrap: wrap; gap: 3px; list-style: none; padding: 0; margin: 0; }
.cell { display: block; width: 14px; height: 14px; border-radius: 3px; border: 0; padding: 0; cursor: pointer; background: var(--surface-hover); }
.cell.met { background: #4ade80; }
.cell.violated { background: #f87171; }
.cell.insufficient { background: #fbbf24; }
.cell.not_evaluated { background: #6b7280; }
.cell.absent { background: transparent; border: 1px dashed var(--surface-border); }
.cell.changed { box-shadow: 0 0 0 2px var(--surface-card), 0 0 0 3px var(--primary-color); }
.cell:focus-visible { outline: 2px solid var(--primary-color); outline-offset: 2px; }
.state { padding: 8px; border-radius: 6px; font-size: 13px; margin-top: 10px; }
.state.bad { background: rgba(248, 113, 113, 0.12); color: #fca5a5; }
.link { background: none; border: 0; color: var(--primary-color); cursor: pointer; padding: 0; font: inherit; }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
</style>
