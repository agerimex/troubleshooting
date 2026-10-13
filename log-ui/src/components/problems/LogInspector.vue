<template>
  <section class="inspector" data-testid="log-inspector" aria-label="Selected span and logs">
    <template v-if="span">
      <div class="text-xs opacity-80">Selected span</div>
      <h3 class="title" data-testid="inspector-span">{{ span.name }}</h3>
      <div class="text-xs opacity-80" data-testid="inspector-span-meta">
        {{ span.service }}<template v-if="span.spanKind"> · {{ span.spanKind.toUpperCase() }}</template> · {{ durationText }} · {{ spanLogs.length }} log{{ spanLogs.length === 1 ? '' : 's' }} in this span · {{ (span.status || 'unset').toUpperCase() }} · <span data-testid="inspector-span-version">{{ spanVersion ? 'version ' + spanVersion : 'version unavailable' }}</span>
      </div>
    </template>
    <template v-else>
      <div class="text-xs opacity-80">No span selected</div>
      <h3 class="title">Logs of the trace</h3>
    </template>

    <div class="tabs mt-2" role="tablist" aria-label="Log scope">
      <button type="button" role="tab" :aria-selected="scope === 'span'" :class="{ on: scope === 'span' }" :disabled="!span"
        @click="scope = 'span'" data-testid="scope-span">Selected span</button>
      <button type="button" role="tab" :aria-selected="scope === 'trace'" :class="{ on: scope === 'trace' }"
        @click="scope = 'trace'" data-testid="scope-trace">All trace logs ({{ report.logs.records.length }})</button>
    </div>

    <div v-if="report.logs.status !== 'ok'" class="state bad" role="alert" data-testid="inspector-logs-unavailable">
      The log source is unavailable ({{ report.logs.error || report.logs.status }}): logs are missing, not absent.
      <button type="button" class="link" @click="$emit('retry-logs')" data-testid="inspector-retry-logs">Retry logs</button>
    </div>
    <template v-else>
      <div v-if="report.logs.truncated" class="state warn" data-testid="inspector-logs-truncated">
        {{ report.logs.shown }} of {{ report.logs.total ?? 'more' }} logs were loaded (limit {{ report.logs.limit }}); the rest are not shown.
      </div>

      <template v-if="scope === 'span' && span">
        <div class="tabs small mt-2" role="tablist" aria-label="Log view">
          <button type="button" role="tab" :aria-selected="view === 'lanes'" :class="{ on: view === 'lanes' }" @click="view = 'lanes'" data-testid="view-lanes">Task lanes</button>
          <button type="button" role="tab" :aria-selected="view === 'list'" :class="{ on: view === 'list' }" @click="view = 'list'" data-testid="view-list">Log list</button>
        </div>
        <p v-if="spanLogs.length === 0" class="mt-2" data-testid="no-span-logs-recorded">No logs recorded for this span.</p>
        <div v-else-if="view === 'lanes'" class="lanes mt-2" data-testid="task-lanes">
          <div class="axis text-xs opacity-80"><span>{{ fmt(laneFrom) }}</span><span>{{ fmt((laneFrom + laneTo) / 2) }}</span><span>{{ fmt(laneTo) }}</span></div>
          <div v-for="lane in lanes" :key="lane.task" class="lane" data-testid="task-lane">
            <div class="lane-name" data-testid="task-lane-name">{{ lane.task }}</div>
            <div class="lane-track">
              <button v-for="p in lane.points" :key="p.log.recordId" type="button" class="point" :class="[sevClass(p.log.severity), { picked: p.log.recordId === selectedLog }]"
                :style="{ left: lanePct(p.offsetMs) + '%' }" :title="p.log.severity + ' +' + fmt(p.offsetMs) + ' ms: ' + p.log.message"
                :aria-label="p.log.severity + ' log at +' + fmt(p.offsetMs) + ' ms'" :aria-pressed="p.log.recordId === selectedLog"
                @click="pick(p.log.recordId)" data-testid="lane-point">{{ shape(p.log.severity) }}</button>
            </div>
          </div>
          <div class="text-xs opacity-80 mt-1">● INFO · ◆ WARN · ✕ ERROR · event time, ms from trace start · points, not task durations</div>
        </div>
        <table v-else class="loglist mt-2" data-testid="log-list">
          <tbody>
            <tr v-for="l in spanLogs" :key="l.log.recordId" :class="{ picked: l.log.recordId === selectedLog }" @click="pick(l.log.recordId)" data-testid="log-list-row">
              <td class="whitespace-nowrap">+{{ fmt(offset(l.log)) }} ms</td>
              <td><span class="sev" :class="sevClass(l.log.severity)">{{ l.log.severity || '–' }}</span></td>
              <td class="opacity-80">{{ l.log.attributes?.['task.id'] || '' }}</td>
              <td class="msg" data-testid="span-log-message"><button type="button" class="link plain" @click.stop="pick(l.log.recordId)">{{ l.log.message }}</button></td>
            </tr>
          </tbody>
        </table>
      </template>

      <template v-else>
        <p v-if="traceLogs.length === 0" class="mt-2" data-testid="no-trace-logs">No logs recorded for this trace within the report window.</p>
        <table v-else class="loglist mt-2" data-testid="trace-log-list">
          <tbody>
            <tr v-for="l in traceLogs" :key="l.log.recordId" :class="{ picked: l.log.recordId === selectedLog }" @click="pick(l.log.recordId)" data-testid="trace-log-row">
              <td class="whitespace-nowrap">+{{ fmt(offset(l.log)) }} ms</td>
              <td><span class="sev" data-testid="trace-log-severity" :class="sevClass(l.log.severity)">{{ l.log.severity || '–' }}</span></td>
              <td class="assoc" :class="{ limit: l.association.kind !== 'span' }" data-testid="trace-log-association">{{ l.association.label }}</td>
              <td class="msg" data-testid="trace-log-message"><button type="button" class="link plain" @click.stop="pick(l.log.recordId)">{{ l.log.message }}</button></td>
            </tr>
          </tbody>
        </table>
      </template>

      <div v-if="pickedLog" class="detail mt-2" data-testid="log-detail">
        <div class="text-xs">
          <span class="sev" :class="sevClass(pickedLog.severity)">{{ pickedLog.severity || 'severity not recorded' }}</span>
          +{{ fmt(offset(pickedLog)) }} ms<template v-if="pickedLog.attributes?.['task.id']"> · {{ pickedLog.attributes['task.id'] }}</template> · {{ pickedLog.service }}
        </div>
        <div class="msg-full" data-testid="log-detail-message">{{ pickedLog.message }}</div>
        <details class="text-xs mt-1">
          <summary>Attributes and identifiers</summary>
          <div>record {{ pickedLog.recordId }} · span {{ pickedLog.spanId || 'not recorded' }} · {{ formatNs(pickedLog.timeUnixNano) }}</div>
          <div v-for="(v, k) in pickedLog.attributes" :key="k"><code>{{ k }}</code> = {{ v }}</div>
        </details>
      </div>
    </template>

    <details v-if="span" class="text-xs mt-3" data-testid="span-details">
      <summary>Span details</summary>
      <div>trace {{ span.traceId }} · span {{ span.spanId }} · parent {{ span.parentSpanId }}<template v-if="span.spanKind"> · kind {{ span.spanKind }}</template></div>
      <div>start {{ formatNs(span.timeStamp) }}<template v-if="span.statusMessage"> · status message: {{ span.statusMessage }}</template></div>
      <div v-for="(t, i) in [...(span.serviceTags ?? []), ...(span.tags ?? [])]" :key="i"><code>{{ t.key }}</code> = {{ t.value }}</div>
      <div v-for="(link, i) in span.links ?? []" :key="'l' + i">link → trace {{ link.traceId }} span {{ link.spanId }} (a reference, not loaded)</div>
    </details>
  </section>
</template>

<script lang="ts">
import { computed, defineComponent, ref, watch, type PropType } from 'vue'
import { isErrorSeverity, offsetMs, scopedLogs, spanInterval, taskLanes, traceStart, type LogScope } from '@/services/problems'
import { durationMsText, formatNs } from '@/services/spanTree'
import { serviceVersion } from '@/services/report'
import { type Span } from '@/types/filter'
import { type LogRecord, type Report } from '@/types/report'

// LogInspector shows the selected span's logs (task lanes or a list, by
// recorded time) or every log of the trace with its association. Switching
// scopes or views uses the loaded report only.
export default defineComponent({
  name: 'LogInspector',
  props: {
    report: { type: Object as PropType<Report>, required: true },
    span: { type: Object as PropType<Span | null>, default: null },
    initialLog: { type: String, default: '' },
    // focusLog opens one log of the selected span (a log marker was picked);
    // a new object each time, so the same log can be focused again.
    focusLog: { type: Object as PropType<{ recordId: string } | null>, default: null }
  },
  emits: ['retry-logs', 'scope'],
  setup (props, { emit }) {
    const scope = ref<LogScope>(props.span ? 'span' : 'trace')
    watch(scope, (v) => emit('scope', v), { immediate: true })
    const view = ref<'lanes' | 'list'>('lanes')
    const selectedLog = ref(props.initialLog)
    // Without a span the trace scope is the only one; a newly selected span
    // shows its own logs again.
    watch(() => props.span, (s) => { scope.value = s ? 'span' : 'trace' })
    watch(() => props.focusLog, (f) => {
      if (f) {
        selectedLog.value = f.recordId
        scope.value = props.span ? 'span' : 'trace'
        view.value = 'list'
      }
    })
    const origin = computed(() => traceStart(props.report.spans.records) ?? (props.report.logs.records.length ? BigInt(props.report.logs.records[0].timeUnixNano) : 0n))
    const spanLogs = computed(() => scopedLogs(props.report, props.span, 'span'))
    const traceLogs = computed(() => scopedLogs(props.report, props.span, 'trace'))
    const lanes = computed(() => taskLanes(spanLogs.value.map((l) => l.log), origin.value))
    const laneRange = computed(() => {
      const offsets = lanes.value.flatMap((l) => l.points.map((p) => p.offsetMs))
      if (props.span) {
        const iv = spanInterval(props.span, origin.value)
        offsets.push(iv.startMs)
        if (iv.endMs !== null) {
          offsets.push(iv.endMs)
        }
      }
      return [Math.min(...offsets), Math.max(...offsets)]
    })
    const laneFrom = computed(() => laneRange.value[0])
    const laneTo = computed(() => laneRange.value[1])
    const lanePct = (v: number) => laneTo.value === laneFrom.value ? 50 : 3 + ((v - laneFrom.value) / (laneTo.value - laneFrom.value)) * 94
    const pickedLog = computed(() => props.report.logs.records.find((l) => l.recordId === selectedLog.value) ?? null)
    const offset = (l: LogRecord) => offsetMs(l.timeUnixNano, origin.value)
    const fmt = (v: number) => (Math.abs(v) >= 10 ? v.toFixed(0) : v.toFixed(1))
    const shape = (sev: string) => isErrorSeverity(sev) ? '✕' : sev === 'WARN' ? '◆' : '●'
    const sevClass = (sev: string) => isErrorSeverity(sev) ? 'bad' : sev === 'WARN' ? 'warn' : 'info'
    const spanVersion = computed(() => props.span ? serviceVersion(props.span) : null)
    const durationText = computed(() => props.span ? (props.span.duration < 0 ? 'duration invalid' : durationMsText(props.span.duration) + ' ms') : '')
    function pick (id: string) {
      selectedLog.value = id
    }
    return { scope, view, selectedLog, spanVersion, spanLogs, traceLogs, lanes, laneFrom, laneTo, lanePct, pickedLog, offset, fmt, shape, sevClass, durationText, pick, formatNs }
  }
})
</script>

<style scoped>
.inspector { background: var(--surface-card); border: 1px solid var(--surface-border); border-radius: 8px; padding: 14px; }
.title { font-size: 16px; font-weight: 600; margin: 2px 0; }
.tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--surface-border); }
.tabs button { background: none; border: 0; border-bottom: 2px solid transparent; color: var(--text-color-secondary); padding: 6px 8px; cursor: pointer; font: inherit; font-size: 13px; }
.tabs button.on { color: var(--text-color); border-bottom-color: var(--primary-color); font-weight: 600; }
.tabs button:disabled { opacity: 0.5; cursor: default; }
.tabs button:focus-visible, .point:focus-visible, .link:focus-visible { outline: 2px solid var(--primary-color); }
.state { margin-top: 8px; padding: 8px; border-radius: 6px; font-size: 13px; }
.state.bad { background: rgba(248, 113, 113, 0.12); color: #fca5a5; }
.state.warn { background: rgba(251, 191, 36, 0.12); color: #fcd34d; }
.axis { display: flex; justify-content: space-between; }
.lane { margin-top: 8px; }
.lane-name { font-size: 12px; color: var(--text-color-secondary); }
.lane-track { position: relative; height: 26px; border-bottom: 1px solid var(--surface-border); }
.point { position: absolute; top: 4px; transform: translateX(-50%); background: none; border: 0; cursor: pointer; font-size: 13px; line-height: 1; padding: 3px; border-radius: 4px; }
.point.info { color: #94a3b8; }
.point.warn { color: #fbbf24; }
.point.bad { color: #f87171; font-weight: 700; }
.point.picked { background: var(--surface-hover); outline: 1px solid var(--primary-color); }
.loglist { width: 100%; font-size: 13px; border-collapse: collapse; }
.loglist td { padding: 4px 6px; border-bottom: 1px solid var(--surface-border); vertical-align: top; }
.loglist tr { cursor: pointer; }
.loglist tr.picked { background: var(--surface-hover); }
.msg { word-break: break-word; }
.assoc { font-size: 12px; color: var(--text-color-secondary); }
.assoc.limit { color: #fcd34d; }
.sev { font-size: 11px; font-weight: 600; padding: 1px 5px; border-radius: 4px; margin-right: 4px; }
.sev.bad { background: rgba(248, 113, 113, 0.18); color: #fca5a5; }
.sev.warn { background: rgba(251, 191, 36, 0.15); color: #fcd34d; }
.sev.info { background: rgba(148, 163, 184, 0.18); color: #cbd5e1; }
.detail { background: var(--surface-ground); border-radius: 6px; padding: 10px; }
.msg-full { margin-top: 4px; white-space: pre-wrap; word-break: break-word; }
.link { background: none; border: 0; color: var(--primary-color); cursor: pointer; padding: 0; font: inherit; }
.link.plain { color: inherit; text-align: left; }
</style>
