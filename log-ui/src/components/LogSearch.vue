<template>
  <div class="card w-full">
    <div class="flex flex-wrap items-center gap-2">
      <label for="log-window-from">From</label>
      <Calendar inputId="log-window-from" v-model="windowFrom" showTime hourFormat="24" :showSeconds="true" dateFormat="dd.mm.yy"/>
      <label for="log-window-to">To</label>
      <Calendar inputId="log-window-to" v-model="windowTo" showTime hourFormat="24" :showSeconds="true" dateFormat="dd.mm.yy" showButtonBar placeholder="now"/>
      <InputText v-model="service" placeholder="Service contains" class="w-48" @keyup.enter="search" data-testid="log-filter-service"/>
      <Dropdown v-model="severity" :options="severities" placeholder="Any severity" :showClear="true" class="w-40" data-testid="log-filter-severity"/>
      <InputText v-model="message" placeholder="Message contains" class="w-72" @keyup.enter="search" data-testid="log-filter-message"/>
      <PrimeButton type="button" label="Search logs" icon="pi pi-search" size="small" @click="search" data-testid="log-search"/>
    </div>
    <div class="mt-2 text-sm" data-testid="log-window">Searched window: {{ formatNs(searchWindow.from) }} – {{ formatNs(searchWindow.to) }}</div>
    <div v-if="error" class="flex items-center gap-2 mt-2 text-red-600" data-testid="log-search-error">
      <span>Could not search logs: {{ error }}</span>
      <PrimeButton type="button" label="Retry" size="small" text @click="load(lastFilter, false)" data-testid="log-search-retry"/>
    </div>

    <table class="w-full mt-3 text-sm log-table" data-testid="log-results">
      <thead>
        <tr><th>time</th><th>log severity</th><th>service</th><th>trace</th><th>message</th><th></th></tr>
      </thead>
      <tbody>
        <tr v-for="log in logs" :key="log.recordId" data-testid="log-row">
          <td class="whitespace-nowrap">{{ formatNs(log.timeUnixNano) }}</td>
          <td><Tag :value="log.severity || 'not recorded'" :severity="logSeverity(log.severity)"/></td>
          <td>{{ log.service }}</td>
          <td>
            <code v-if="log.traceId" class="text-xs">{{ log.traceId }}</code>
            <span v-else class="italic text-amber-500" data-testid="no-trace-id">no TraceId recorded</span>
          </td>
          <td class="message">{{ log.message }}</td>
          <td class="whitespace-nowrap">
            <PrimeButton type="button" icon="pi pi-eye" size="small" text title="Inspect this log record" @click="inspected = log" data-testid="inspect-log"/>
            <PrimeButton type="button" icon="pi pi-sitemap" size="small" text label="Open report" :disabled="!log.traceId"
              :title="log.traceId ? 'Open the investigation report of this log\'s trace' : 'No TraceId was recorded, so there is no trace to open'"
              @click="openReport(log)" data-testid="open-log-report"/>
          </td>
        </tr>
      </tbody>
    </table>
    <p v-if="!loading && !error && searched && logs.length === 0" class="mt-2" data-testid="log-empty">No log records match these filters in the searched window.</p>
    <p v-if="loading" class="mt-2">Searching…</p>
    <div v-if="hasMore && !loading" class="mt-2 flex items-center gap-2">
      <span class="text-sm">{{ logs.length }} records shown; more match.</span>
      <PrimeButton type="button" label="Load more" size="small" text @click="loadMore" data-testid="log-load-more"/>
    </div>

    <PrimeDialog :visible="inspected !== null" @update:visible="(v: boolean) => { if (!v) inspected = null }" header="Log record" :style="{ width: '56rem' }" data-testid="log-dialog">
      <div v-if="inspected">
        <p><Tag :value="inspected.severity || 'not recorded'" :severity="logSeverity(inspected.severity)"/> {{ formatNs(inspected.timeUnixNano) }} · {{ inspected.service }}</p>
        <pre class="log-message" data-testid="log-dialog-message">{{ inspected.message }}</pre>
        <p class="text-sm">record <code>{{ inspected.recordId }}</code></p>
        <p class="text-sm">trace <code v-if="inspected.traceId">{{ inspected.traceId }}</code><i v-else>not recorded</i>
          · span <code v-if="inspected.spanId">{{ inspected.spanId }}</code><i v-else>not recorded</i></p>
        <p v-if="!inspected.traceId" class="mt-2 text-amber-500" data-testid="no-correlation">
          Trace correlation is unavailable because no TraceId was recorded for this log.
          A SpanId or a timestamp alone does not identify a trace, so no report is opened from it.
        </p>
        <table class="text-sm mt-2">
          <tr v-for="(value, key) in inspected.attributes" :key="key">
            <td class="pr-4 align-top">{{ key }}</td><td>{{ value }}</td>
          </tr>
        </table>
      </div>
      <template #footer>
        <PrimeButton v-if="inspected && inspected.traceId" label="Open report" @click="openReport(inspected)" data-testid="log-dialog-open-report"/>
        <PrimeButton @click="inspected = null" label="Close" />
      </template>
    </PrimeDialog>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref } from 'vue'
import PrimeButton from 'primevue/button'
import InputText from 'primevue/inputtext'
import Calendar from 'primevue/calendar'
import Dropdown from 'primevue/dropdown'
import Tag from 'primevue/tag'
import PrimeDialog from 'primevue/dialog'
import logsApi from '@/api/logs.api'
import { Logs } from '@/logs-api/logs.impl'
import { type LogFilter, type LogRecord } from '@/types/report'
import { toNs, toNsExclusiveEnd, formatNs } from '@/services/spanTree'
import { logSeverity } from '@/services/severity'

const pageSize = 100

export default defineComponent({
  name: 'LogSearch',
  components: { PrimeButton, InputText, Calendar, Dropdown, Tag, PrimeDialog },
  emits: ['open-report'],
  setup (_props, { emit }) {
    logsApi.init(new Logs())

    const windowFrom = ref(new Date(Date.now() - 15 * 60 * 1000))
    const windowTo = ref<Date | null>(null)
    const searchWindow = ref({ from: toNs(windowFrom.value), to: toNsExclusiveEnd(new Date()) })
    const service = ref('')
    const severity = ref<string | null>(null)
    const message = ref('')
    const severities = ['TRACE', 'DEBUG', 'INFO', 'WARN', 'ERROR', 'FATAL']

    const logs = ref<LogRecord[]>([])
    const hasMore = ref(false)
    const loading = ref(false)
    const searched = ref(false)
    const error = ref('')
    const inspected = ref<LogRecord | null>(null)
    let lastFilter: LogFilter = { timeFrom: '0', timeTo: '0' }
    // A response is applied only if no newer search started meanwhile.
    let seq = 0

    async function load (filter: LogFilter, append: boolean) {
      const mine = ++seq
      lastFilter = filter
      loading.value = true
      error.value = ''
      const [err, page] = await logsApi.searchLogs(filter)
      if (mine !== seq) {
        return
      }
      loading.value = false
      searched.value = true
      if (err !== null || page === null) {
        error.value = err ?? 'invalid response'
        if (!append) {
          logs.value = []
          hasMore.value = false
        }
        return
      }
      logs.value = append ? [...logs.value, ...page.logs] : page.logs
      hasMore.value = page.hasMore
    }

    // search resolves the window once; Load more keeps it.
    function search () {
      searchWindow.value = { from: toNs(windowFrom.value), to: toNsExclusiveEnd(windowTo.value ?? new Date()) }
      load({
        timeFrom: searchWindow.value.from,
        timeTo: searchWindow.value.to,
        serviceName: service.value.trim() || undefined,
        severity: severity.value || undefined,
        message: message.value.trim() || undefined,
        rowsPerPage: pageSize
      }, false)
    }

    function loadMore () {
      const last = logs.value[logs.value.length - 1]
      if (last) {
        load({ ...lastFilter, afterTime: last.timeUnixNano, afterRecordId: last.recordId }, true)
      }
    }

    // The search filters only find the entry record: the report gets the
    // record's TraceId and the searched window, nothing else.
    function openReport (log: LogRecord) {
      if (!log.traceId) {
        return
      }
      inspected.value = null
      emit('open-report', { traceId: log.traceId, window: { ...searchWindow.value }, recordId: log.recordId })
    }

    return {
      windowFrom, windowTo, searchWindow, service, severity, message, severities, logs, hasMore, loading, searched,
      error, inspected, lastFilter, load, search, loadMore, openReport, formatNs, logSeverity
    }
  }
})
</script>

<style scoped>
.log-table th, .log-table td {
  text-align: left;
  vertical-align: top;
  padding: 0.25rem 0.5rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}
.message, .log-message {
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
