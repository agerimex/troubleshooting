<template>
  <PrimeDialog :visible="visible" @update:visible="(v: boolean) => { if (!v) $emit('close') }" modal header="Services and span names"
    :style="{ width: 'min(960px, 96vw)' }" :draggable="false" data-testid="name-picker">
    <p class="text-sm opacity-80 np-intro">
      Pick services on the left to see their span names on the right. A service whose names you pick is narrowed to them; a picked service without picked names is shown whole.
    </p>
    <div v-if="state === 'loading'" class="np-state" data-testid="name-picker-loading">Loading the names of {{ windowText }}…</div>
    <div v-else-if="state === 'failed'" class="np-state bad" role="alert" data-testid="name-picker-error">
      The names could not be loaded: {{ error }}
      <button type="button" class="np-link" @click="load" data-testid="name-picker-retry">Retry</button>
    </div>
    <template v-else>
      <div v-for="(n, i) in notes" :key="i" class="np-note" data-testid="name-picker-note">{{ n }}</div>
      <div class="np-panes">
        <section class="np-pane" aria-label="Services">
          <div class="np-head">
            <strong>Services</strong>
            <span class="text-xs opacity-80">{{ picks.services.length }} of {{ services.length }} picked</span>
          </div>
          <InputText v-model="serviceSearch" placeholder="Search services" class="w-full" aria-label="Search services" data-testid="name-picker-service-search"/>
          <ul class="np-list" data-testid="name-picker-services">
            <li v-for="s in shownServices" :key="s.service">
              <label class="np-item">
                <input type="checkbox" :checked="picks.services.includes(s.service)" @change="picks = toggleService(picks, s.service)" data-testid="name-picker-service"/>
                <span class="np-name">{{ s.service }}</span>
                <span class="np-count">{{ s.names }} name{{ s.names === 1 ? '' : 's' }} · ≈{{ count(s.spans) }}</span>
              </label>
            </li>
            <li v-if="shownServices.length === 0" class="np-empty">No service matches.</li>
          </ul>
        </section>
        <section class="np-pane" aria-label="Span names">
          <div class="np-head">
            <strong>Span names</strong>
            <span class="text-xs opacity-80">{{ picks.services.length ? 'of the picked services' : 'of all services' }}</span>
          </div>
          <div class="np-tools">
            <InputText v-model="nameSearch" placeholder="Search span names" class="np-grow" aria-label="Search span names" data-testid="name-picker-name-search"/>
            <label class="np-check"><input type="checkbox" v-model="entryOnly" data-testid="name-picker-entry-only"/> Only names that start an execution</label>
          </div>
          <div class="np-list" data-testid="name-picker-names">
            <div v-for="g in groups" :key="g.service" class="np-group">
              <div class="np-group-head">{{ g.service }}</div>
              <label v-for="x in g.names" :key="g.service + '\u0000' + x.name" class="np-item">
                <input type="checkbox" :checked="isNamePicked(picks, g.service, x.name)" @change="picks = toggleName(picks, g.service, x.name)" data-testid="name-picker-name"/>
                <span class="np-name">{{ x.name }}</span>
                <span v-if="x.entry" class="np-root" title="Started an execution (a root span) in this window">root</span>
                <span class="np-count">≈{{ count(x.spans) }}</span>
              </label>
            </div>
            <div v-if="groups.length === 0" class="np-empty">No span name matches.</div>
          </div>
        </section>
      </div>
    </template>
    <template #footer>
      <div class="np-footer">
        <span class="text-sm opacity-80" data-testid="name-picker-summary">{{ summary || 'Nothing picked: every service and span name' }}</span>
        <span class="np-buttons">
          <PrimeButton type="button" label="Clear" text size="small" @click="picks = NO_PICKS" :disabled="!hasPicks(picks)" data-testid="name-picker-clear"/>
          <PrimeButton type="button" label="Cancel" text size="small" @click="$emit('close')" data-testid="name-picker-cancel"/>
          <PrimeButton type="button" label="Apply" size="small" @click="apply" :disabled="state !== 'ready'" data-testid="name-picker-apply"/>
        </span>
      </div>
    </template>
  </PrimeDialog>
</template>

<script lang="ts">
import { computed, defineComponent, ref, watch, type PropType } from 'vue'
import PrimeDialog from 'primevue/dialog'
import PrimeButton from 'primevue/button'
import InputText from 'primevue/inputtext'
import spansApi from '@/api/spans.api'
import { type SpanNameEntry } from '@/types/filter'
import { hasPicks, isNamePicked, nameGroups, NO_PICKS, pickSummary, servicesOf, toggleName, toggleService, visibleServices, type Picks } from '@/services/namePicker'

// NamePicker loads the services and span names of the searched window and lets
// the user pick them; Apply emits the picks. The names come from hourly
// statistics where they exist and raw spans elsewhere, so counts are approximate.
export default defineComponent({
  name: 'NamePicker',
  components: { PrimeDialog, PrimeButton, InputText },
  props: {
    visible: { type: Boolean, required: true },
    timeFrom: { type: String, required: true },
    timeTo: { type: String, required: true },
    windowText: { type: String, default: 'the searched window' },
    initial: { type: Object as PropType<Picks>, default: () => NO_PICKS }
  },
  emits: ['close', 'apply'],
  setup (props, { emit }) {
    const state = ref<'loading' | 'failed' | 'ready'>('loading')
    const error = ref('')
    const names = ref<SpanNameEntry[]>([])
    const notes = ref<string[]>([])
    const picks = ref<Picks>(props.initial)
    const serviceSearch = ref('')
    const nameSearch = ref('')
    const entryOnly = ref(false)
    let seq = 0

    async function load () {
      const mine = ++seq
      state.value = 'loading'
      const [err, list] = await spansApi.spanNames(props.timeFrom, props.timeTo)
      if (mine !== seq) return
      if (err || !list || (list.status !== 'ok' && list.status !== 'partial')) {
        error.value = err ?? list?.error ?? list?.status ?? 'unknown error'
        state.value = 'failed'
        return
      }
      names.value = list.names
      const n: string[] = []
      if (list.truncated) n.push(`Only the first ${list.names.length} names are listed; narrow the window to see the rest.`)
      if (list.status === 'partial') n.push('Some hours of the window are no longer retained: names that occurred only then are missing.')
      notes.value = n
      state.value = 'ready'
    }

    watch(() => props.visible, (v) => {
      if (v) {
        picks.value = props.initial
        void load()
      }
    }, { immediate: true })

    const services = computed(() => servicesOf(names.value))
    const shownServices = computed(() => visibleServices(services.value, serviceSearch.value))
    const groups = computed(() => nameGroups(names.value, picks.value.services, nameSearch.value, entryOnly.value))
    const summary = computed(() => pickSummary(picks.value))
    const count = (n: number) => n.toLocaleString('en-US')
    function apply () {
      emit('apply', picks.value)
    }
    return {
      state, error, notes, picks, serviceSearch, nameSearch, entryOnly, services, shownServices, groups, summary, count, load, apply,
      toggleService, toggleName, isNamePicked, hasPicks, NO_PICKS
    }
  }
})
</script>

<style scoped>
.np-intro { margin: 0 0 12px; }
.np-state { padding: 16px 0; }
.np-state.bad { color: #fca5a5; }
.np-note { font-size: 13px; color: #fcd34d; margin-bottom: 8px; }
.np-link { background: none; border: 0; color: var(--primary-color); cursor: pointer; font: inherit; padding: 0 4px; }
.np-panes { display: grid; grid-template-columns: minmax(0, 2fr) minmax(0, 3fr); gap: 16px; }
@media (max-width: 720px) { .np-panes { grid-template-columns: 1fr; } }
.np-pane { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
.np-head { display: flex; justify-content: space-between; align-items: baseline; gap: 8px; }
.np-tools { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; }
.np-grow { flex: 1 1 12rem; min-width: 0; }
.np-check { display: inline-flex; align-items: center; gap: 6px; font-size: 13px; cursor: pointer; }
.np-list { list-style: none; margin: 0; padding: 4px; border: 1px solid var(--surface-border); border-radius: 6px; height: 22rem; overflow: auto; }
.np-group + .np-group { margin-top: 6px; }
.np-group-head { font-size: 12px; font-weight: 600; color: var(--text-color-secondary); padding: 6px 6px 2px; position: sticky; top: -4px; background: var(--surface-card); }
.np-item { display: flex; align-items: center; gap: 8px; padding: 5px 6px; border-radius: 4px; cursor: pointer; font-size: 13px; min-width: 0; }
.np-item:hover { background: var(--surface-hover); }
.np-item input, .np-check input { accent-color: var(--primary-color); flex: none; }
.np-name { flex: 1; min-width: 0; overflow-wrap: anywhere; }
.np-count { color: var(--text-color-secondary); font-size: 12px; white-space: nowrap; font-variant-numeric: tabular-nums; }
.np-root { font-size: 11px; padding: 0 6px; border-radius: 999px; border: 1px solid var(--primary-color); color: var(--primary-color); flex: none; }
.np-empty { padding: 8px; font-size: 13px; color: var(--text-color-secondary); }
.np-footer { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 8px; width: 100%; }
.np-buttons { display: inline-flex; gap: 6px; }
</style>
