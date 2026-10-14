<template>
  <div class="card ex-page" data-testid="executions-view">
    <SequenceLibrary v-show="!target || libraryOpen" :hasOpenExecution="!!target" @open-execution="openFromLibrary" @close="libraryOpen = false"/>

    <div v-if="target" v-show="!libraryOpen">
      <div class="flex justify-end">
        <PrimeButton type="button" size="small" label="Sequence library" icon="pi pi-list" @click="libraryOpen = true" data-testid="open-library"/>
      </div>
      <ExecutionViewer :key="targetKey" :traceId="target.traceId" :initialWindow="target.window" :spanId="target.spanId ?? ''" :recordId="target.recordId ?? ''"
        :initialViewChoice="target.view" :origin="target.origin ?? null" :context="target.context ?? null" :included="included"
        @back="back" @open-execution="$emit('open-execution', $event)" @include="$emit('include', $event)"/>
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref, watch, type PropType } from 'vue'
import PrimeButton from 'primevue/button'
import ExecutionViewer from '@/components/ExecutionViewer.vue'
import SequenceLibrary from '@/components/SequenceLibrary.vue'
import { type ExecutionTarget } from '@/services/navigation'

// ExecutionsView is Timeline & Sequence: the sequence library (distinct
// execution shapes with their percentiles) and the focused viewer of one
// execution. The library stays mounted, so returning to it keeps its window,
// filters and page.
export default defineComponent({
  name: 'ExecutionsView',
  components: { PrimeButton, ExecutionViewer, SequenceLibrary },
  props: {
    target: { type: Object as PropType<ExecutionTarget | null>, default: null },
    targetKey: { type: Number, default: 0 },
    included: { type: Boolean, default: false }
  },
  emits: ['open-execution', 'back', 'include'],
  setup (props, { emit }) {
    const libraryOpen = ref(false)
    // A newly opened execution (from any page) is shown, not the library.
    watch(() => props.targetKey, () => { libraryOpen.value = false })

    function openFromLibrary (t: ExecutionTarget) {
      libraryOpen.value = false
      emit('open-execution', t)
    }
    // Back from an execution opened in the library returns to the library on
    // this page; other origins are handled by the app.
    function back () {
      if (props.target?.origin?.page === 'executions') {
        libraryOpen.value = true
        return
      }
      emit('back')
    }
    return { libraryOpen, openFromLibrary, back }
  }
})
</script>

<style scoped>
.ex-page { font-size: 14px; }
</style>
