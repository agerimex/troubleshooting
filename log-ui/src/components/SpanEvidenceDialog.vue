<template>
  <PrimeDialog :visible="span !== null" @update:visible="(v: boolean) => { if (!v) $emit('close') }" header="Span evidence" :style="{ width: '64rem' }" data-testid="evidence-dialog">
    <div v-if="span">
      <p>
        <b>{{ span.name }}</b> · {{ span.service }} ·
        span status <Tag :value="span.status" :severity="statusSeverity(span.status)"/>
        <span v-if="span.statusMessage" class="ml-2" data-testid="status-message">{{ span.statusMessage }}</span>
      </p>
      <p class="text-sm">trace <code>{{ span.traceId }}</code> · span <code>{{ span.spanId }}</code> · parent <code>{{ span.parentSpanId }}</code></p>

      <h3 class="mt-4 font-bold">Events <span class="font-normal text-sm">(recorded on the span; not log records)</span></h3>
      <p v-if="events.length === 0" class="text-sm" data-testid="no-events">No events were recorded for this span. That alone does not show that no error happened.</p>
      <div v-for="(event, i) in events" :key="i" class="border-t py-2" data-testid="event">
        <div><span class="text-sm">{{ formatNs(event.timeUnixNano) }}</span> <b data-testid="event-name">{{ event.name }}</b></div>
        <div v-if="event.attributes['exception.type'] || event.attributes['exception.message']" class="text-red-500" data-testid="exception">
          {{ event.attributes['exception.type'] }}<span v-if="event.attributes['exception.type'] && event.attributes['exception.message']">: </span>{{ event.attributes['exception.message'] }}
        </div>
        <pre v-if="event.attributes['exception.stacktrace']" class="stacktrace" data-testid="stacktrace">{{ event.attributes['exception.stacktrace'] }}</pre>
        <table class="text-sm">
          <tr v-for="[key, value] in otherAttributes(event.attributes)" :key="key">
            <td class="pr-4 align-top">{{ key }}</td><td>{{ value }}</td>
          </tr>
        </table>
      </div>

      <h3 class="mt-4 font-bold">Links</h3>
      <p v-if="links.length === 0" class="text-sm" data-testid="no-links">No links were recorded for this span.</p>
      <div v-for="(link, i) in links" :key="i" class="border-t py-2" data-testid="link">
        <div>
          trace <code data-testid="link-trace">{{ link.traceId }}</code> · span <code data-testid="link-span">{{ link.spanId }}</code>
          <Tag v-if="link.traceId !== reportTraceId" class="ml-2" severity="secondary" value="not inspected" data-testid="link-not-inspected"/>
          <PrimeButton type="button" label="Open linked trace" size="small" text @click="$emit('open-trace', link.traceId)" data-testid="open-linked-trace"/>
        </div>
        <table class="text-sm">
          <tr v-for="(value, key) in link.attributes" :key="key">
            <td class="pr-4 align-top">{{ key }}</td><td>{{ value }}</td>
          </tr>
        </table>
      </div>
    </div>
    <template #footer>
      <PrimeButton @click="$emit('close')" label="Close" />
    </template>
  </PrimeDialog>
</template>

<script lang="ts">
import { computed, defineComponent, type PropType } from 'vue'
import PrimeDialog from 'primevue/dialog'
import PrimeButton from 'primevue/button'
import Tag from 'primevue/tag'
import { type Span } from '@/types/filter'
import { formatNs } from '@/services/spanTree'
import { spanStatusSeverity as statusSeverity } from '@/services/severity'

const shownApart = ['exception.type', 'exception.message', 'exception.stacktrace']

export default defineComponent({
  name: 'SpanEvidenceDialog',
  components: { PrimeDialog, PrimeButton, Tag },
  props: {
    span: { type: Object as PropType<Omit<Span, 'duration'> | null>, default: null },
    // The trace being inspected; links to other traces are marked "not inspected".
    reportTraceId: { type: String, default: '' }
  },
  emits: ['close', 'open-trace'],
  setup (props) {
    const events = computed(() => props.span?.events ?? [])
    const links = computed(() => props.span?.links ?? [])
    function otherAttributes (attributes: Record<string, string>) {
      return Object.entries(attributes).filter(([key]) => !shownApart.includes(key))
    }
    return { events, links, otherAttributes, formatNs, statusSeverity }
  }
})
</script>

<style scoped>
.stacktrace {
  white-space: pre-wrap;
  font-size: 0.8rem;
  max-height: 24rem;
  overflow: auto;
  padding: 0.5rem;
  border: 1px solid rgba(255, 255, 255, 0.2);
}
</style>
