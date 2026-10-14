<template>
  <div class="card settings" data-testid="settings-view">
    <h2 class="title">Settings</h2>
    <div class="tabs mt-2" role="tablist" aria-label="Settings">
      <button type="button" role="tab" :aria-selected="tab === 'detection'" :class="{ on: tab === 'detection' }" @click="go('detection')" data-testid="settings-detection">Detection</button>
      <button type="button" role="tab" :aria-selected="tab === 'retention'" :class="{ on: tab === 'retention' }" @click="go('retention')" data-testid="settings-retention">Data retention</button>
    </div>

    <div v-show="tab === 'detection'" class="mt-3">
      <DetectionRules @dirty="rulesDirty = $event" @saved="$emit('rules-saved')"/>
    </div>

    <div v-if="tab === 'retention'" class="mt-3" data-testid="retention">
      <p v-if="loading">Reading the effective retention…</p>
      <p v-else-if="error" class="state bad" role="alert" data-testid="retention-error">The retention is unavailable: {{ error }} <button type="button" class="link" @click="load">Retry</button></p>
      <template v-else-if="policy">
        <dl class="grid-dl">
          <dt>Raw spans</dt><dd data-testid="retention-traces">{{ duration(policy.tracesSeconds) }}</dd>
          <dt>Raw logs</dt><dd data-testid="retention-logs">{{ duration(policy.logsSeconds) }}</dd>
          <dt>Statistics</dt><dd data-testid="retention-stats">{{ duration(policy.statsSeconds) }}</dd>
          <dt>Statistics bucket</dt><dd>{{ duration(policy.bucketSeconds) }}</dd>
          <dt>Allowed lateness</dt><dd>{{ duration(policy.latenessSeconds) }} (late records still update a bucket for this long; then it is final)</dd>
        </dl>
        <p v-if="!policy.compatible" class="state warn" data-testid="retention-incompatible">The raw retention is too short for the allowed lateness: finished statistics could be rebuilt from incomplete raw data.</p>
        <p class="text-xs opacity-80 mt-2">Raw records and statistics expire independently: comparisons and problem counts stay available after the raw spans and logs behind them expired, but their examples can't be opened then. {{ policy.physicalDeletion }}</p>
        <p class="text-xs opacity-80 mt-1">Retention is set on the receiver (TRACE_RETENTION, LOG_RETENTION, STATS_RETENTION) and shown here as read from the database; it can't be changed from this page.</p>
      </template>
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref } from 'vue'
import DetectionRules from '@/components/problems/DetectionRules.vue'
import statsApi from '@/api/stats.api'
import { Stats } from '@/logs-api/stats.impl'
import { type RetentionPolicy } from '@/types/stats'

// SettingsView: what is detected (versioned detection rules, saved with
// optimistic concurrency) and how long data is retained (read-only).
export default defineComponent({
  name: 'SettingsView',
  components: { DetectionRules },
  emits: ['rules-saved'],
  setup () {
    statsApi.init(new Stats())
    const tab = ref<'detection' | 'retention'>('detection')
    const rulesDirty = ref(false)
    const policy = ref<RetentionPolicy | null>(null)
    const error = ref('')
    const loading = ref(false)

    async function load () {
      loading.value = true
      error.value = ''
      const [err, p] = await statsApi.retention()
      loading.value = false
      if (err || !p) {
        error.value = err ?? 'no answer'
        return
      }
      policy.value = p
    }
    function go (t: 'detection' | 'retention') {
      if (tab.value === 'detection' && t !== 'detection' && rulesDirty.value &&
        !window.confirm('The detection rules have unsaved changes. Leave them unsaved? (They stay in the form until you discard them.)')) {
        return
      }
      tab.value = t
      if (t === 'retention' && !policy.value) {
        load()
      }
    }
    function duration (seconds: number): string {
      if (seconds % 86400 === 0) {
        return `${seconds / 86400} ${seconds === 86400 ? 'day' : 'days'}`
      }
      if (seconds % 3600 === 0) {
        return `${seconds / 3600} h`
      }
      return `${Math.round(seconds / 60)} min`
    }
    return { tab, go, rulesDirty, policy, error, loading, load, duration }
  }
})
</script>

<style scoped>
.settings { font-size: 14px; }
.title { font-size: 25px; font-weight: 500; margin: 0; }
.tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--surface-border); }
.tabs button { background: none; border: 0; border-bottom: 2px solid transparent; color: var(--text-color-secondary); padding: 8px 12px; cursor: pointer; font: inherit; }
.tabs button.on { color: var(--text-color); border-bottom-color: var(--primary-color); font-weight: 600; }
.tabs button:focus-visible { outline: 2px solid var(--primary-color); }
.grid-dl { display: grid; grid-template-columns: 10rem 1fr; gap: 6px 12px; }
.grid-dl dt { color: var(--text-color-secondary); }
.state { padding: 8px; border-radius: 6px; font-size: 13px; }
.state.bad { background: rgba(248, 113, 113, 0.12); color: #fca5a5; }
.state.warn { background: rgba(251, 191, 36, 0.12); color: #fcd34d; }
.link { background: none; border: 0; color: var(--primary-color); cursor: pointer; padding: 0; font: inherit; }
</style>
