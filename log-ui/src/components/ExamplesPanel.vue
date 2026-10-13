<template>
  <div class="mt-2 p-2 border rounded" data-testid="examples">
    <div class="flex items-center gap-2">
      <b>{{ title }}</b>
      <PrimeButton type="button" icon="pi pi-times" size="small" text title="Close" @click="$emit('close')" data-testid="examples-close"/>
    </div>
    <p v-if="loading">Loading examples…</p>
    <p v-else-if="error" class="text-red-500" data-testid="examples-error">Examples are unavailable: {{ error }}</p>
    <template v-else-if="result">
      <p v-if="result.status !== 'ok'" class="text-amber-500" data-testid="examples-status">{{ result.condition }}</p>
      <template v-else>
        <p class="text-sm" data-testid="examples-summary">
          {{ result.shown }} of {{ result.total }} shown<span v-if="result.truncated"> · truncated: more examples match (limit {{ result.limit }})</span>.
          {{ result.description }}
        </p>
        <table class="w-full text-sm mt-1 ex-table" v-if="result.examples.length">
          <thead><tr><th>time</th><th>trace</th><th>span / record</th><th v-if="request.kind === 'slow'">duration</th><th>version</th><th>message as recorded</th><th></th></tr></thead>
          <tbody>
            <tr v-for="(ex, i) in result.examples" :key="i" data-testid="example-row">
              <td class="whitespace-nowrap">{{ formatNs(ex.time) }}</td>
              <td><code v-if="ex.traceId" class="text-xs">{{ ex.traceId }}</code><i v-else class="text-amber-500">no TraceId recorded</i></td>
              <td><code class="text-xs">{{ ex.recordId || ex.spanId || '–' }}</code></td>
              <td v-if="request.kind === 'slow'">{{ ex.durationMs != null ? ex.durationMs.toFixed(1) + ' ms' : 'unavailable' }}</td>
              <td>{{ ex.version || 'not recorded' }}</td>
              <td class="message">{{ ex.message }}</td>
              <td>
                <PrimeButton type="button" icon="pi pi-sitemap" size="small" text label="Open report" :disabled="!ex.traceId"
                  :title="ex.traceId ? 'Open the combined report of this trace' : 'No TraceId was recorded: there is no trace to open'"
                  @click="open(ex)" data-testid="open-example-report"/>
              </td>
            </tr>
          </tbody>
        </table>
      </template>
    </template>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref, watch, type PropType } from 'vue'
import PrimeButton from 'primevue/button'
import statsApi from '@/api/stats.api'
import { Stats } from '@/logs-api/stats.impl'
import { formatNs } from '@/services/spanTree'
import { type Example, type ExamplesRequest, type ExamplesResult } from '@/types/stats'

// ExamplesPanel lists a bounded set of examples of a population. They are
// examples, never presented as the full population.
export default defineComponent({
  name: 'ExamplesPanel',
  components: { PrimeButton },
  props: {
    request: { type: Object as PropType<ExamplesRequest>, required: true },
    title: { type: String, required: true }
  },
  emits: ['close', 'open'],
  setup (props, { emit }) {
    statsApi.init(new Stats())
    const result = ref<ExamplesResult | null>(null)
    const error = ref('')
    const loading = ref(false)
    let seq = 0
    async function load () {
      const mine = ++seq
      loading.value = true
      error.value = ''
      const [err, res] = await statsApi.examples({ limit: 10, ...props.request })
      if (mine !== seq) {
        return
      }
      loading.value = false
      result.value = res
      error.value = err ?? ''
    }
    function open (ex: Example) {
      emit('open', ex)
    }
    watch(() => props.request, load, { immediate: true })
    return { result, error, loading, open, formatNs }
  }
})
</script>

<style scoped>
.ex-table th, .ex-table td { text-align: left; vertical-align: top; padding: 0.2rem 0.4rem; }
.message { white-space: pre-wrap; word-break: break-word; }
</style>
