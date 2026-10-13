<template>
  <div class="population-form flex flex-col gap-2" :data-testid="testid">
    <div class="flex flex-wrap items-center gap-2">
      <InputText :modelValue="modelValue.service" @update:modelValue="(v) => set('service', v ?? '')" placeholder="Service (required)" class="w-64"
        :data-testid="testid + '-service'"/>
      <label class="text-sm">From</label>
      <Calendar :modelValue="from" @update:modelValue="setFrom" showTime hourFormat="24" dateFormat="dd.mm.yy" :data-testid="testid + '-from'"/>
      <label class="text-sm">to (exclusive)</label>
      <Calendar :modelValue="to" @update:modelValue="setTo" showTime hourFormat="24" dateFormat="dd.mm.yy" :data-testid="testid + '-to'"/>
    </div>
    <div class="flex flex-wrap items-center gap-2">
      <div v-for="d in dimensions" :key="d.key" class="flex flex-wrap items-center gap-1">
        <Dropdown :modelValue="modeOf(d.key)" @update:modelValue="(m) => setMode(d.key, m)" :options="modes" optionLabel="label" optionValue="value"
          class="w-36" :data-testid="testid + '-' + d.key + '-mode'">
          <template #value="{ value }">{{ d.label }}: {{ modes.find((m) => m.value === value)?.label }}</template>
        </Dropdown>
        <InputText v-if="modeOf(d.key) === 'is'" :modelValue="modelValue[d.key] ?? ''" @update:modelValue="(v) => set(d.key, v ?? '')"
          :placeholder="d.label" class="w-36" :data-testid="testid + '-' + d.key"/>
      </div>
    </div>
    <div class="text-xs opacity-80" :data-testid="testid + '-window'">
      Window {{ formatNs(modelValue.timeFrom) }} – {{ formatNs(modelValue.timeTo) }} [from, to) · UTC hour buckets; a bound inside an hour is computed from raw records while they are retained.
    </div>
  </div>
</template>

<script lang="ts">
import { computed, defineComponent, reactive, watch, type PropType } from 'vue'
import InputText from 'primevue/inputtext'
import Calendar from 'primevue/calendar'
import Dropdown from 'primevue/dropdown'
import { formatNs, nsToDate, toNs } from '@/services/spanTree'
import { type Population } from '@/types/stats'

type DimKey = 'operation' | 'spanKind' | 'environment' | 'version' | 'installation' | 'segment'

// PopulationForm edits a population. An optional dimension is "any" (not
// sent), "is" a value, or "not recorded" (the explicit unknown group). The
// window end is exclusive and used exactly as selected, so whole hours stay
// whole buckets.
export default defineComponent({
  name: 'PopulationForm',
  components: { InputText, Calendar, Dropdown },
  props: {
    modelValue: { type: Object as PropType<Population>, required: true },
    testid: { type: String, default: 'population' }
  },
  emits: ['update:modelValue'],
  setup (props, { emit }) {
    const dimensions: { key: DimKey, label: string }[] = [
      { key: 'operation', label: 'Operation' },
      { key: 'spanKind', label: 'Span kind' },
      { key: 'environment', label: 'Environment' },
      { key: 'version', label: 'Version' },
      { key: 'installation', label: 'Installation' },
      { key: 'segment', label: 'Segment' }
    ]
    const modes = [
      { label: 'any', value: 'any' },
      { label: 'is', value: 'is' },
      { label: 'not recorded', value: 'missing' }
    ]
    const from = computed(() => nsToDate(props.modelValue.timeFrom))
    const to = computed(() => nsToDate(props.modelValue.timeTo))

    function update (patch: Partial<Population>) {
      emit('update:modelValue', { ...props.modelValue, ...patch })
    }
    // The mode of each dimension is kept here: "is" with an empty text means
    // any value until a value is typed, never "not recorded".
    const mode = reactive<Record<DimKey, string>>({} as Record<DimKey, string>)
    for (const d of dimensions) {
      const v = props.modelValue[d.key]
      mode[d.key] = v === undefined ? 'any' : (v === '' ? 'missing' : 'is')
    }
    // A value set from outside (e.g. copied from the other population) shows its mode.
    watch(() => props.modelValue, (p) => {
      for (const d of dimensions) {
        const v = p[d.key]
        if (v !== undefined) {
          mode[d.key] = v === '' ? 'missing' : 'is'
        } else if (mode[d.key] === 'missing') {
          mode[d.key] = 'any'
        }
      }
    })
    function set (key: keyof Population, value: string) {
      const dim = dimensions.some((d) => d.key === key)
      update({ [key]: dim ? (value.trim() === '' ? undefined : value.trim()) : value } as Partial<Population>)
    }
    function modeOf (key: DimKey): string {
      return mode[key]
    }
    function setMode (key: DimKey, m: string) {
      mode[key] = m
      const current = props.modelValue[key]
      const value = m === 'any' ? undefined : (m === 'missing' ? '' : (current ? current : undefined))
      update({ [key]: value } as Partial<Population>)
    }
    function setFrom (d: Date | Date[] | (Date | null)[] | null | undefined) {
      if (d instanceof Date) {
        update({ timeFrom: toNs(d) })
      }
    }
    function setTo (d: Date | Date[] | (Date | null)[] | null | undefined) {
      if (d instanceof Date) {
        update({ timeTo: toNs(d) })
      }
    }
    return { dimensions, modes, from, to, set, modeOf, setMode, setFrom, setTo, formatNs }
  }
})
</script>

<style scoped>
/* The rows stack; without this they sat side by side and overflowed narrow screens. */
.population-form { display: flex; flex-direction: column; min-width: 0; }
</style>
