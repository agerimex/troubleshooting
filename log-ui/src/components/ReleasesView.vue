<template>
  <div class="releases" data-testid="releases-view">
    <div class="card">
      <h2 class="title">Releases</h2>
      <p class="text-sm opacity-80">What changed between releases, and did the fix help: explicit baseline and candidate populations, per-metric evidence, and the statistics behind them.</p>
      <div class="tabs mt-2" role="tablist" aria-label="Releases">
        <button type="button" role="tab" :aria-selected="tab === 'compare'" :class="{ on: tab === 'compare' }" @click="tab = 'compare'" data-testid="releases-compare">Compare releases</button>
        <button type="button" role="tab" :aria-selected="tab === 'stats'" :class="{ on: tab === 'stats' }" @click="tab = 'stats'" data-testid="releases-stats">Operation statistics</button>
      </div>
    </div>
    <!-- Both stay mounted, so switching keeps their populations and results. -->
    <CompareView v-show="tab === 'compare'" :included="included" @open-report="$emit('open-report', $event)" @remove-included="(id: string, i: number) => $emit('remove-included', id, i)"/>
    <StatisticsView v-show="tab === 'stats'" @open-report="$emit('open-report', $event)"/>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref, type PropType } from 'vue'
import CompareView from '@/components/CompareView.vue'
import StatisticsView from '@/components/StatisticsView.vue'
import { type IncludedReport } from '@/services/comparison'

// ReleasesView: release comparison (with fix verification and its export)
// and the operation statistics it is computed from.
export default defineComponent({
  name: 'ReleasesView',
  components: { CompareView, StatisticsView },
  props: {
    included: { type: Object as PropType<Record<string, IncludedReport[]>>, required: true }
  },
  emits: ['open-report', 'remove-included'],
  setup () {
    const tab = ref<'compare' | 'stats'>('compare')
    return { tab }
  }
})
</script>

<style scoped>
.releases { font-size: 14px; }
.title { font-size: 25px; font-weight: 500; margin: 0; }
.tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--surface-border); }
.tabs button { background: none; border: 0; border-bottom: 2px solid transparent; color: var(--text-color-secondary); padding: 8px 12px; cursor: pointer; font: inherit; }
.tabs button.on { color: var(--text-color); border-bottom-color: var(--primary-color); font-weight: 600; }
.tabs button:focus-visible { outline: 2px solid var(--primary-color); }
</style>
