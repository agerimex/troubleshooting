<template>
  <div class="w-full">
    <div class="card w-full" data-testid="log-analysis">
      <h2 class="la-title">Log Analysis</h2>
      <p class="text-sm opacity-80">Traces and logs in one expandable table. Expanding a span shows its child spans and the logs that recorded exactly that span.</p>
      <div v-if="linkText" class="la-link mt-2" data-testid="link-fallback">
        {{ linkCopied ? 'Link copied. ' : 'The clipboard is not available: select and copy the link. ' }}
        <input class="la-link-input" readonly :value="linkText" aria-label="Investigation link" @focus="selectAll" data-testid="link-text"/>
        <button type="button" class="la-plain" @click="linkText = ''">Dismiss</button>
      </div>
      <div class="mt-2 la-table" @contextmenu="onContextMenu">
      <TreeTable :value="nodes" :lazy="true" :paginator="true" :rows="rowsPerPage" :rowsPerPageOptions="[10, 25, 100, 500, 1000]"
          paginatorTemplate="RowsPerPageDropdown NextPageLink" v-model:first="firstRow"
          filterDisplay="row"
          :loading="loading" v-model:expandedKeys="expandedKeys" @nodeExpand="onExpand" @page="onPage" :totalRecords="paginatorTotal">
        <template #header>
          <div class="flex flex-wrap items-center gap-2">
            <label for="window-from">From</label>
            <Calendar inputId="window-from" v-model="windowFrom" @update:modelValue="onRefresh()" showTime hourFormat="24" :showSeconds="true" dateFormat="dd.mm.yy"/>
            <label for="window-to">To</label>
            <Calendar inputId="window-to" v-model="windowTo" @update:modelValue="onRefresh()" showTime hourFormat="24" :showSeconds="true" dateFormat="dd.mm.yy" showButtonBar placeholder="now"/>
            <SelectButton v-model="scope" :options="scopeOptions" optionLabel="label" optionValue="value" :allowEmpty="false" @change="onScopeChange" data-testid="scope"/>
            <InputText v-model="traceIdInput" placeholder="Trace id" class="w-72" @keyup.enter="openTraceById" data-testid="trace-id-input"/>
            <PrimeButton type="button" label="Open timeline" size="small" @click="openTraceById" data-testid="open-trace-by-id"/>
          </div>
          <div class="flex flex-wrap items-center gap-2 mt-2 text-sm">
            <span data-testid="window">Searched window: {{ formatNs(searchWindow.from) }} – {{ formatNs(searchWindow.to) }}</span>
          </div>
          <div class="flex flex-wrap items-center gap-2 mt-1 text-sm" data-testid="total">
            <span>{{ totalText }}</span>
            <PrimeButton v-if="total.state === 'failed'" type="button" label="Retry total" size="small" text @click="retryTotal" data-testid="retry-total"/>
          </div>
          <div class="text-xs opacity-80 mt-1" data-testid="filter-scope">
            Name, status and service filters select spans by their own recorded values; they never search log text. Logs appear under an expanded span by exact trace and span id, whatever their severity.
          </div>
          <div v-if="notice" class="flex items-center gap-2 mt-2 text-red-600" data-testid="notice">
            <span>{{ notice }}</span>
            <PrimeButton v-if="noticeRetry" type="button" label="Retry" size="small" text @click="noticeRetry()" data-testid="retry"/>
          </div>
          <div class="mt-2" style="text-align:left">
            <MultiSelect :modelValue="selectedColumns" @update:modelValue="onToggle" :options="columns" optionLabel="header" class="w-full" display="chip" data-testid="columns"/>
          </div>
        </template>
        <template #empty>
          <span v-if="!loading && !loadFailed" data-testid="empty">No spans match these filters in the searched window.</span>
        </template>
        <template #paginatorstart>
          <PrimeButton type="button" icon="pi pi-refresh" text @click="onRefresh" data-testid="refresh"/>
        </template>

        <Column field="humanTime" header="time" style="width: 12rem" :expander="true">
          <template #body="{ node }">
            <span class="row-key" :data-row-key="node.key"></span>
            <span v-if="node.data.kind === 'notice'" class="italic text-amber-500" data-testid="node-notice">{{ node.data.text }}</span>
            <span v-else-if="node.data.kind === 'error'" class="text-red-600" data-testid="node-error">
              {{ node.data.text }}
              <PrimeButton type="button" label="Retry" size="small" text @click="retryChildren(node.data.parentKey)" data-testid="retry-children"/>
            </span>
            <span v-else-if="node.data.kind === 'group'" class="text-sm font-semibold" data-testid="log-group">{{ node.data.text }}</span>
            <span v-else class="whitespace-nowrap" :class="{ 'la-selected': node.key === selectedKey }" :data-selected="node.key === selectedKey">{{ node.data.humanTime }}</span>
          </template>
        </Column>
        <Column field="name" header="name" style="width: 12rem" :expander="false">
          <template #body="{ node }">
            <template v-if="!node.data.kind">
              <span data-testid="span-name">{{ node.data.name }}</span>
              <Tag v-if="node.data.parentMissing" class="ml-2" severity="warning" :value="node.data.parentMissing" data-testid="parent-missing"/>
              <Tag v-else-if="node.data.parentSpanId !== ROOT_PARENT && isTopRow(node)" class="ml-2" severity="secondary" value="child span"/>
              <div class="text-xs opacity-70">{{ node.data.spanKind ? node.data.spanKind.toUpperCase() + ' · ' : '' }}span</div>
            </template>
            <template v-else-if="node.data.kind === 'log'">
              <span data-testid="log-row-name">Log</span>
              <div class="text-xs opacity-70" data-testid="log-row-association">{{ node.data.association }}</div>
            </template>
          </template>
          <template #filter>
            <InputText v-model="filterByMethodName" placeholder="Filter by span name" type="text" class="p-column-filter" v-on:update:modelValue="filterChanged()" data-testid="filter-name"/>
          </template>
        </Column>
        <Column field="status" header="status" style="width: 6.5rem">
          <template #body="{ node }">
            <Tag v-if="!node.data.kind" :value="node.data.status" :severity="getSeverity(node.data.status)"/>
            <Tag v-else-if="node.data.kind === 'log'" :value="node.data.log.severity || 'not recorded'" :severity="logSeverity(node.data.log.severity)" data-testid="log-row-severity"/>
          </template>
          <template #filter>
            <div class="flex">
              <Dropdown v-model="filterStatus" @change="filterChanged()" :options="statuses" placeholder="Any span status" class="p-column-filter" style="width: 9rem" :showClear="true" data-testid="filter-status">
                <template #option="slotProps">
                  <Tag :value="slotProps.option" :severity="getSeverity(slotProps.option)" />
                </template>
              </Dropdown>
            </div>
          </template>
        </Column>
        <Column v-for="col of selectedColumns" :field="col.field" :header="col.header" :key="col.field" :style="col.field === 'msg' ? 'min-width: 16rem' : 'min-width: 8rem'">
          <template #body="{ node }">
            <div v-if="node.data.kind === 'log' && col.field === 'msg'" class="log-msg" data-testid="log-row-message">{{ node.data.log.message }}</div>
            <div v-else-if="!node.data.kind || node.data.kind === 'log'" @dblclick="showDetails(node.data[col.field])" class="multiline-cell" :title="JSON.stringify(node.data[col.field])">{{ node.data[col.field] }}</div>
          </template>
          <template #filter>
            <div class="flex" v-if="col.filter">
              <InputText v-model="filters[col.field]" type="text" class="p-column-filter" :placeholder="col.filterPlaceHolder" v-on:update:modelValue="filterChanged()" :data-testid="'filter-' + col.field"/>
            </div>
          </template>
        </Column>
        <Column field="duration" header="duration, ms" headerStyle="width: 6rem; text-align: right" bodyStyle="text-align: right; overflow: visible"></Column>
        <Column header="" style="width: 7rem">
          <template #body="{ node }">
            <template v-if="!node.data.kind">
              <PrimeButton type="button" icon="pi pi-sitemap" size="small" text title="Open timeline" aria-label="Open timeline" @click="openNode(node, 'timeline')" data-testid="open-trace"/>
              <PrimeButton type="button" icon="pi pi-info-circle" size="small" text :label="evidenceLabel(node.data)" title="Events and links" @click="showEvidence(node.data)" data-testid="evidence"/>
            </template>
            <PrimeButton v-else-if="node.data.kind === 'log'" type="button" icon="pi pi-info-circle" size="small" text title="Record details" aria-label="Record details" @click="inspected = node.data.log" data-testid="log-details"/>
            <PrimeButton v-if="!node.data.kind || node.data.kind === 'log'" type="button" icon="pi pi-ellipsis-h" size="small" text title="Row actions" aria-label="Row actions"
              aria-haspopup="menu" @click="openMenu(node, $event)" data-testid="row-actions"/>
          </template>
        </Column>
      </TreeTable>
      </div>

      <ActionMenu :open="menu.open" :items="menu.items" :x="menu.x" :y="menu.y" :opener="menu.opener" @close="menu.open = false"/>

      <PrimeDialog v-model:visible="dialogVisible">
        <vue-json-pretty class="text-center" :showSelectController="false" :data="selectedCellText" />
        <template #footer>
          <PrimeButton @click="dialogVisible = false" label="Close" />
        </template>
      </PrimeDialog>

      <PrimeDialog :visible="inspected !== null" @update:visible="(v: boolean) => { if (!v) inspected = null }" header="Log record" :style="{ width: '56rem' }" data-testid="log-record-dialog">
        <div v-if="inspected">
          <p><Tag :value="inspected.severity || 'not recorded'" :severity="logSeverity(inspected.severity)"/> {{ formatNs(inspected.timeUnixNano) }} · {{ inspected.service }}</p>
          <pre class="log-msg">{{ inspected.message }}</pre>
          <p class="text-sm">record <code>{{ inspected.recordId }}</code> · trace <code>{{ inspected.traceId || 'not recorded' }}</code> · span <code>{{ inspected.spanId || 'not recorded' }}</code></p>
          <table class="text-sm mt-2">
            <tr v-for="(value, key) in inspected.attributes" :key="key">
              <td class="pr-4 align-top">{{ key }}</td><td>{{ value }}</td>
            </tr>
          </table>
        </div>
        <template #footer>
          <PrimeButton @click="inspected = null" label="Close" />
        </template>
      </PrimeDialog>

      <SpanEvidenceDialog :span="evidence" @close="evidence = null" @open-trace="openLinkedTrace"/>
    </div>
  </div>
</template>

<script lang="ts">
import { ref, reactive, computed, defineComponent } from 'vue'
import TreeTable from 'primevue/treetable'
import MultiSelect from 'primevue/multiselect'
import SelectButton from 'primevue/selectbutton'
import PrimeButton from 'primevue/button'
import InputText from 'primevue/inputtext'
import Calendar from 'primevue/calendar'
import Tag from 'primevue/tag'
import Dropdown from 'primevue/dropdown'
import Column from 'primevue/column'
import PrimeDialog from 'primevue/dialog'
import SpanEvidenceDialog from '@/components/SpanEvidenceDialog.vue'
import ActionMenu, { type Action } from '@/components/ActionMenu.vue'
import spansApi from '@/api/spans.api'
import logsApi from '@/api/logs.api'
import { Spans } from '@/logs-api/spans.impl'
import { Logs } from '@/logs-api/logs.impl'
import { type Span, type SpanFilter } from '@/types/filter'
import { type LogRecord } from '@/types/report'
import {
  ROOT_PARENT, toNs, toNsExclusiveEnd, formatNs, createItem, messageNode, replaceNode, findNode,
  type TreeNode, type SpanNodeData, type LogNodeData
} from '@/services/spanTree'
import { fromResponse } from '@/services/report'
import { logSeverity } from '@/services/severity'
import {
  copyText, investigationLink, logsOfSpanId, traceLogsOf, unattachedLogs,
  type ExecutionTarget, type ExecutionView, type TraceLogs
} from '@/services/navigation'
import VueJsonPretty from 'vue-json-pretty'
import 'vue-json-pretty/lib/styles.css'
import { format as sqlFormatter } from 'sql-formatter'

interface Filters {
  [key: string]: any
}

export interface SearchWindow {
  from: string
  to: string
}

type Total = { state: 'loading' } | { state: 'ok', count: number } | { state: 'failed', error: string }

// Children loaded per expanded node; log-analysis caps a page at 1000 rows and
// says whether more exist.
const maxChildren = 1000

export default defineComponent({
  name: 'LogsUI',
  components: {
    TreeTable,
    Column,
    SelectButton,
    InputText,
    Calendar,
    MultiSelect,
    PrimeButton,
    Tag,
    Dropdown,
    PrimeDialog,
    SpanEvidenceDialog,
    ActionMenu,
    VueJsonPretty
  },
  emits: ['open-execution'],
  setup (_props, { emit }) {
    const selectedCellText = ref()
    const dialogVisible = ref(false)
    const evidence = ref<SpanNodeData | null>(null)
    const inspected = ref<LogRecord | null>(null)

    const filterByMethodName = ref('')

    logsApi.init(new Logs())
    spansApi.init(new Spans()).then(() => { onRefresh() })

    const nodes = ref<TreeNode[]>([])
    // Owned here, not by TreeTable, so it can be cleared when the rows are
    // replaced; otherwise reloaded rows keep the "expanded" icon without children.
    const expandedKeys = ref<Record<string, boolean>>({})
    const loading = ref(false)
    // loadFailed distinguishes a failed load from an empty result.
    const loadFailed = ref(false)
    // Shown above the table: why the list failed to load, with a retry action.
    const notice = ref('')
    const noticeRetry = ref<(() => void) | null>(null)
    function showNotice(text: string, retry: (() => void) | null) {
      notice.value = text
      noticeRetry.value = retry
    }
    function clearNotice() {
      showNotice('', null)
    }

    // The search window: From and To (empty To = now). Each traversal (refresh
    // or filter change) resolves it once into searchWindow, which then stays
    // fixed for paging, expansion and counts, so they all describe the same data.
    const windowFrom = ref(new Date(Date.now() - 15*60*1000))
    const windowTo = ref<Date | null>(null)
    const searchWindow = ref<SearchWindow>({ from: toNs(windowFrom.value), to: toNsExclusiveEnd(new Date()) })
    function resolveWindow() {
      searchWindow.value = { from: toNs(windowFrom.value), to: toNsExclusiveEnd(windowTo.value ?? new Date()) }
    }

    const scopeOptions = [
      { label: 'Root spans', value: 'root' },
      { label: 'Any depth', value: 'all' }
    ]
    const scope = ref<'root' | 'all'>('root')
    // Searching by status, name or service looks at every depth (a failed child
    // under a successful root must be found) unless the user chose otherwise.
    let scopeChosen = false
    function onScopeChange() {
      scopeChosen = true
      onRefresh()
    }

    const rowsPerPage = ref(25)
    const statuses = ref(['unset', 'error', 'ok'])
    const filterStatus = ref()
    const firstRow = ref(0)

    // Paging: the cursor is the last row of the previous page; nextCursor the
    // last row of the page shown. Paging only moves forward.
    const cursor = ref<Span | null>(null)
    let nextCursor: Span | null = null
    const pageRows = ref(0)
    const hasMore = ref(false)
    const total = ref<Total>({ state: 'loading' })

    const traceIdInput = ref('')

    // The paginator only needs to know whether a next page exists; that comes
    // from hasMore, not from the total, so a failed count can't block paging.
    // With hasMore it reports one row beyond the current page, so the
    // paginator (which divides by rowsPerPage) enables Next.
    const paginatorTotal = computed(() => hasMore.value
      ? firstRow.value + rowsPerPage.value + 1
      : firstRow.value + pageRows.value)

    const totalText = computed(() => {
      const rows = pageRows.value > 0 ? `Rows ${firstRow.value + 1}–${firstRow.value + pageRows.value}` : 'No rows'
      switch (total.value.state) {
        case 'ok': return `${rows} of ${total.value.count}`
        case 'failed': return `${rows} · Total unavailable: ${total.value.error}`
        default: return `${rows} · counting…`
      }
    })

    const columns = ref([
      {field: 'traceId', header: 'traceId', filter: false, filterPlaceHolder: 'Filter by trace'},
      {field: 'spanId', header: 'spanId', filter: false,  filterPlaceHolder: 'Filter by span'},
      {field: 'parentSpanId', header: 'parentSpanId', filter: false, filterPlaceHolder: 'Filter by parentSpan'},
      {field: 'tags', header: 'tags', filter: false, filterPlaceHolder: 'Filter by tags'},
      {field: 'serviceTags', header: 'serviceTags', filter: false, filterPlaceHolder: 'Filter by service tags'},
      {field: 'msg', header: 'msg', filter: false, filterPlaceHolder: 'Filter by msg'},
      {field: 'service', header: 'service', filter: true, filterPlaceHolder: 'Filter by service'},
      {field: 'statusMessage', header: 'statusMessage', filter: false, filterPlaceHolder: 'Filter by statusMessage'}
    ])
    const defaultColumns = ref([
      {field: 'msg', header: 'msg', filter: false, filterPlaceHolder: 'Filter by msg'},
      {field: 'service', header: 'service', filter: true, filterPlaceHolder: 'Filter by service'}
    ])
    const selectedColumns = ref(defaultColumns.value)

    const filters = ref<Filters>({})

    function searchFilter(withCursor: boolean): SpanFilter {
      const filter: SpanFilter = {
        timeFrom: searchWindow.value.from,
        timeTo: searchWindow.value.to,
        scope: scope.value,
        rowsPerPage: rowsPerPage.value,
        status: filterStatus.value || undefined,
        serviceName: filters.value['service'] || undefined,
        methodName: filterByMethodName.value || undefined
      }
      if (withCursor && cursor.value !== null) {
        filter.afterTime = cursor.value.timeStamp
        filter.afterTraceId = cursor.value.traceId
        filter.afterSpanId = cursor.value.spanId
      }
      return filter
    }

    // Bumped by every list, trace or count load. A response is applied only if
    // no newer load started meanwhile; otherwise a slow response for an old
    // filter would overwrite the rows of the current one.
    let requestSeq = 0

    // A span of the search list can be expanded even without child spans: its
    // logs aren't known until its trace's logs are read. Loaded children get an
    // arrow only if they have child spans or logs of their own (spanItemOf), so
    // the arrows show where a trace goes deeper.
    const spanItem = (s: Span): TreeNode => ({ ...createItem(s), leaf: false })

    async function fetchSpans() {
      const seq = ++requestSeq
      loading.value = true
      clearNotice()
      // Snapshot the filter now: the user may change it while we wait.
      const filter = searchFilter(true)
      const firstPage = cursor.value === null
      if (firstPage) {
        total.value = { state: 'loading' }
      }
      const [error, page] = await spansApi.viewSpans(filter)
      if (seq !== requestSeq) {
        return
      }
      expandedKeys.value = {}
      loading.value = false
      if (error !== null || page === null) {
        loadFailed.value = true
        nodes.value = []
        pageRows.value = 0
        hasMore.value = false
        // Without this an API failure looks the same as "no matching spans".
        showNotice('Could not load spans: ' + error, fetchSpans)
        if (firstPage) {
          total.value = { state: 'failed', error: 'the list did not load' }
        }
        return
      }
      loadFailed.value = false
      nodes.value = page.spans.map(spanItem)
      pageRows.value = page.spans.length
      hasMore.value = page.hasMore
      nextCursor = page.spans.length > 0 ? page.spans[page.spans.length - 1] : null
      // The total doesn't depend on the page, so it is counted once per traversal.
      if (firstPage) {
        fetchTotal(seq)
      }
    }

    // fetchTotal counts the whole list. If it fails the rows stay usable and
    // the total says it is unavailable, never 0 or an earlier value.
    async function fetchTotal(seq: number) {
      total.value = { state: 'loading' }
      const [error, count] = await spansApi.countOfSpans(searchFilter(false))
      if (seq !== requestSeq) {
        return
      }
      total.value = error === null && count !== null
        ? { state: 'ok', count }
        : { state: 'failed', error: error ?? 'invalid response' }
    }

    function retryTotal() {
      fetchTotal(requestSeq)
    }

    // The logs of a trace are read once per traversal, the first time one of
    // its spans is expanded: one bounded, trace-anchored report request (its
    // spans tell trace-level from unmatched logs). Never a scan by service or
    // message.
    let traceLogs = new Map<string, Promise<TraceLogs | string>>()
    function logsOfTrace (traceId: string): Promise<TraceLogs | string> {
      let p = traceLogs.get(traceId)
      if (!p) {
        p = logsApi.traceReport({ traceId, timeFrom: searchWindow.value.from, timeTo: searchWindow.value.to }).then(([error, response]) => {
          const report = response ? fromResponse(response) : null
          if (error !== null || report === null) {
            traceLogs.delete(traceId)
            return error ?? 'invalid response'
          }
          return traceLogsOf(report)
        })
        traceLogs.set(traceId, p)
      }
      return p
    }

    function logNode (log: LogRecord, association: string, parentKey: string): TreeNode {
      const data: LogNodeData = {
        kind: 'log', log, humanTime: formatNs(log.timeUnixNano), association, msg: log.message, service: log.service,
        traceId: log.traceId, spanId: log.spanId, duration: '—'
      }
      return { key: parentKey + ':log:' + log.recordId, data, leaf: true }
    }

    // Children and logs of a span in recorded time order; equal times keep
    // spans before logs, then by key, so the order is deterministic.
    const timeOf = (n: TreeNode) => BigInt(n.data.kind === 'log' ? n.data.log.timeUnixNano : n.data.kind === undefined ? n.data.timeStamp : '0')
    function byTime (a: TreeNode, b: TreeNode): number {
      const ta = timeOf(a)
      const tb = timeOf(b)
      if (ta !== tb) {
        return ta < tb ? -1 : 1
      }
      const la = a.data.kind === 'log' ? 1 : 0
      const lb = b.data.kind === 'log' ? 1 : 0
      return la !== lb ? la - lb : (a.key < b.key ? -1 : 1)
    }

    async function loadChildren(node: TreeNode) {
      const data = node.data as SpanNodeData
      loading.value = true
      const seq = requestSeq
      const [[error, page], logs] = await Promise.all([
        spansApi.viewSpans({
          timeFrom: searchWindow.value.from, timeTo: searchWindow.value.to,
          traceId: data.traceId, parentId: data.spanId, scope: 'all', rowsPerPage: maxChildren
        }),
        logsOfTrace(data.traceId)
      ])
      if (seq !== requestSeq) {
        // The rows were reloaded meanwhile; this node is no longer shown.
        return
      }
      let children: TreeNode[] = []
      const messages: TreeNode[] = []
      // A child is a leaf when it has no child spans and, as far as the read
      // logs tell, no logs; with unavailable or partial logs it stays
      // expandable, since its logs are unknown.
      const knownLogs = typeof logs !== 'string' && logs.status === 'ok' && !logs.truncated ? logs : null
      const spanItemOf = (s: Span): TreeNode => ({
        ...createItem(s),
        leaf: s.childSpanCount === 0 && knownLogs !== null && logsOfSpanId(knownLogs, s.spanId).length === 0
      })
      if (error !== null || page === null) {
        messages.push(messageNode(node.key, 'error', 'Could not load child spans: ' + error))
      } else {
        children = page.spans.map(spanItemOf)
        if (page.hasMore) {
          messages.push(messageNode(node.key, 'notice', `Only the first ${maxChildren} child spans are shown; more exist in the searched window.`))
        }
      }
      if (typeof logs === 'string' || logs.status !== 'ok') {
        const why = typeof logs === 'string' ? logs : logs.error
        messages.push({ key: node.key + ':logs-error', data: { kind: 'error', text: 'The logs of this trace are unavailable (' + why + '): they are missing, not absent.', parentKey: node.key }, leaf: true })
      } else {
        children.push(...logsOfSpanId(logs, data.spanId).map((l) => logNode(l, 'recorded this span', node.key)))
        if (logs.truncated) {
          messages.push({ key: node.key + ':logs-notice', data: { kind: 'notice', text: 'Only part of this trace\'s logs were read (report limit): some logs of this span may be missing.', parentKey: node.key }, leaf: true })
        }
        // Trace-level and unmatched logs stay in their own group under the
        // root span, never under a guessed span.
        const unattached = data.parentSpanId === ROOT_PARENT ? unattachedLogs(logs) : []
        if (unattached.length) {
          const groupKey = node.key + ':unattached'
          messages.push({
            key: groupKey,
            data: { kind: 'group', text: `Trace-level / unmatched-span logs (${unattached.length})`, parentKey: node.key },
            leaf: false,
            children: unattached.map((u) => logNode(u.log, u.reason, groupKey))
          })
        }
      }
      children.sort(byTime)
      // Nothing under it: the row loses its arrow instead of showing an empty
      // notice row.
      const leaf = children.length === 0 && messages.length === 0
      nodes.value = replaceNode(nodes.value, node.key, { ...node, leaf, children: [...children, ...messages] })
      loading.value = false
    }

    const onExpand = async (node: TreeNode) => {
      if (!node.children) {
        await loadChildren(node)
      }
    }

    function retryChildren(parentKey: string) {
      const parent = findNode(nodes.value, parentKey)
      if (parent) {
        traceLogs.delete((parent.data as SpanNodeData).traceId)
        loadChildren({ ...parent, children: undefined })
      }
    }

    // isTopRow tells whether a node is a row of the search list rather than a
    // loaded child.
    function isTopRow(node: TreeNode) {
      return nodes.value.some((n) => n.key === node.key)
    }

    const onPage = async (event: any) => {
      if (rowsPerPage.value !== event.rows) {
        rowsPerPage.value = event.rows
        onRefresh()
        return
      }
      cursor.value = nextCursor
      fetchSpans()
    }

    function filterChanged () {
      const searching = !!(filterStatus.value || filterByMethodName.value || filters.value['service'])
      if (searching && !scopeChosen) {
        scope.value = 'all'
      }
      onRefresh()
    }

    // onRefresh starts a new traversal: the window is resolved again and
    // paging starts over.
    function onRefresh () {
      resolveWindow()
      cursor.value = null
      nextCursor = null
      firstRow.value = 0
      traceLogs = new Map()
      fetchSpans()
    }

    const traceIdPattern = /^[0-9a-fA-F]{32}$/

    function openTraceById() {
      const id = traceIdInput.value.trim()
      if (!traceIdPattern.test(id)) {
        showNotice('A trace id has 32 hex digits.', null)
        return
      }
      resolveWindow()
      openTrace(id.toLowerCase())
    }

    // --- opening executions: the same commands from the row button, the
    // right-click menu and the keyboard

    const selectedKey = ref<string | null>(null)

    function targetOf (node: TreeNode, view?: ExecutionView): ExecutionTarget | null {
      const d = node.data as SpanNodeData | LogNodeData
      if (d.kind === 'log') {
        if (!d.log.traceId) {
          return null
        }
        return { traceId: d.log.traceId, window: { ...searchWindow.value }, spanId: d.log.spanId || undefined, recordId: d.log.recordId, view, origin: { page: 'logs', label: 'Log Analysis' } }
      }
      return { traceId: d.traceId, window: { ...searchWindow.value }, spanId: d.spanId, view, origin: { page: 'logs', label: 'Log Analysis' } }
    }

    function openNode (node: TreeNode, view?: ExecutionView) {
      const t = targetOf(node, view)
      if (t) {
        selectedKey.value = node.key
        emit('open-execution', t)
      }
    }

    // openTrace opens a trace with the search window resolved for the current
    // list, so the report covers the same time.
    function openTrace(traceId: string) {
      emit('open-execution', { traceId, window: { ...searchWindow.value }, origin: { page: 'logs', label: 'Log Analysis' } } as ExecutionTarget)
    }

    function openLinkedTrace(traceId: string) {
      evidence.value = null
      openTrace(traceId)
    }

    const linkText = ref('')
    const linkCopied = ref(false)
    async function copyLink (node: TreeNode) {
      const t = targetOf(node)
      if (!t) {
        return
      }
      selectedKey.value = node.key
      linkText.value = investigationLink(window.location.href, { ...t, origin: undefined })
      linkCopied.value = await copyText(linkText.value)
    }
    function selectAll (e: Event) {
      (e.target as HTMLInputElement).select()
    }

    const menu = reactive<{ open: boolean, items: Action[], x: number, y: number, opener: HTMLElement | null }>({ open: false, items: [], x: 0, y: 0, opener: null })

    function actionsOf (node: TreeNode): Action[] {
      const d = node.data as SpanNodeData | LogNodeData
      const noTrace = d.kind === 'log' && !d.log.traceId ? 'No TraceId recorded: no execution can be opened, and none is guessed.' : undefined
      const items: Action[] = [
        { id: 'open-timeline', label: 'Open timeline', disabled: noTrace, run: () => openNode(node, 'timeline') },
        { id: 'open-sequence', label: 'Open sequence', disabled: noTrace, run: () => openNode(node, 'sequence') },
        { id: 'copy-link', label: 'Copy investigation link', disabled: noTrace, run: () => copyLink(node) }
      ]
      if (d.kind === 'log') {
        items.push({ id: 'details', label: 'Record details', run: () => { selectedKey.value = node.key; inspected.value = d.log } })
      } else {
        items.push({ id: 'details', label: 'Events and links', run: () => { selectedKey.value = node.key; evidence.value = d } })
      }
      return items
    }

    function openMenu (node: TreeNode, event: MouseEvent) {
      const opener = event.currentTarget as HTMLElement
      const r = opener.getBoundingClientRect()
      menu.items = actionsOf(node)
      menu.x = r.left
      menu.y = r.bottom + 4
      menu.opener = opener
      menu.open = true
    }

    // Right-click (or the context-menu key) on a span or log row opens the
    // same actions as its row button.
    function onContextMenu (event: MouseEvent) {
      const row = (event.target as HTMLElement).closest('tr')
      const key = row?.querySelector('[data-row-key]')?.getAttribute('data-row-key')
      const node = key ? findNode(nodes.value, key) : null
      if (!node || (node.data.kind !== undefined && node.data.kind !== 'log')) {
        return
      }
      event.preventDefault()
      const button = row!.querySelector('[data-testid=row-actions]') as HTMLElement | null
      const r = (event.target as HTMLElement).getBoundingClientRect()
      menu.items = actionsOf(node)
      menu.x = event.clientX || r.left
      menu.y = event.clientY || r.bottom
      menu.opener = button
      menu.open = true
    }

    const onToggle = (val: any) => {
      selectedColumns.value = columns.value.filter(col => val.includes(col))
    }

    const getSeverity = (status: any) => {
      switch (status) {
        case 'error':
            return 'danger'

        case 'ok':
            return 'success'

        case 'unset':
            return 'info'
      }
    }

    function evidenceLabel(data: SpanNodeData) {
      const parts = []
      if (data.events.length > 0) {
        parts.push(data.events.length + (data.events.length === 1 ? ' event' : ' events'))
      }
      if (data.links.length > 0) {
        parts.push(data.links.length + (data.links.length === 1 ? ' link' : ' links'))
      }
      return parts.join(' · ')
    }

    function showEvidence(data: SpanNodeData) {
      evidence.value = data
    }

    function formatContent(content: string): string | object | null {
      try {
        const correctedContent = content.replace(/'/g, '"')
        const json = JSON.parse(correctedContent)
        return json
      } catch (error) {
        try {
            const formattedSql = sqlFormatter(content)
            return formattedSql
        } catch (error) {
            return null
        }
      }
    }

    function showDetails(content: any) {
      selectedCellText.value = content
      if (typeof content === 'string') {
        const formattedContent = formatContent(content)
        if (formattedContent !== null) {
          selectedCellText.value = formattedContent as string | object
        }
      }
      dialogVisible.value = true
    }

    return {
      ROOT_PARENT,
      nodes,
      expandedKeys,
      notice,
      noticeRetry,
      loading,
      loadFailed,
      retryTotal,
      onExpand,
      retryChildren,
      isTopRow,
      onPage,
      paginatorTotal,
      total,
      totalText,
      rowsPerPage,
      selectedColumns,
      columns,
      onToggle,
      filters,
      filterChanged,
      getSeverity,
      logSeverity,
      statuses,
      filterStatus,
      windowFrom,
      windowTo,
      searchWindow,
      scope,
      scopeOptions,
      onScopeChange,
      firstRow,
      onRefresh,
      traceIdInput,
      openTraceById,
      openTrace,
      openNode,
      openLinkedTrace,
      selectedKey,
      menu,
      openMenu,
      onContextMenu,
      linkText,
      linkCopied,
      selectAll,
      formatNs,
      selectedCellText,
      dialogVisible,
      showDetails,
      evidence,
      inspected,
      evidenceLabel,
      showEvidence,
      filterByMethodName
    }
  }
})
</script>


<style scoped>
.la-title {
  font-size: 25px;
  font-weight: 500;
  margin: 0;
}

.la-link {
  padding: 8px;
  border-radius: 6px;
  background: var(--surface-ground);
  font-size: 13px;
}

.la-link-input {
  width: 100%;
  margin: 4px 0;
  background: var(--surface-card);
  color: var(--text-color);
  border: 1px solid var(--surface-border);
  border-radius: 6px;
  padding: 6px;
  font-size: 12px;
}

.la-plain {
  background: none;
  border: 0;
  color: var(--primary-color);
  cursor: pointer;
  padding: 0;
}

.la-selected {
  border-left: 3px solid var(--primary-color);
  padding-left: 4px;
}

.la-table {
  overflow-x: auto;
}

@media (min-width: 641px) {
  .la-table :deep(table) {
    min-width: 66rem;
  }
}

.row-key {
  display: none;
}

.log-msg {
  white-space: pre-wrap;
  word-break: break-word;
  font-family: ui-monospace, monospace;
  font-size: 0.85rem;
}

.multiline-cell {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  overflow: hidden;
  max-height: calc(1.5em * 3);
  -webkit-line-clamp: 3;
  line-height: 1.5em;
}

.stacktrace {
  white-space: pre-wrap;
  font-size: 0.8rem;
  max-height: 24rem;
  overflow: auto;
  padding: 0.5rem;
  border: 1px solid rgba(255, 255, 255, 0.2);
}

/* Narrow screens: each row becomes labeled fields (NAV-EXP-017). */
@media (max-width: 640px) {
  /* Column labels go; the filter row stays usable. */
  :deep(.p-treetable-thead > tr:first-child) {
    display: none;
  }
  :deep(.p-treetable-thead > tr) {
    display: flex;
    flex-wrap: wrap;
  }
  :deep(.p-treetable-thead > tr > th) {
    width: auto !important;
    border: 0 !important;
  }
  :deep(.p-treetable-thead > tr > th:empty) {
    display: none;
  }
  :deep(.p-treetable-tbody > tr) {
    display: flex;
    flex-wrap: wrap;
    border-bottom: 1px solid var(--surface-border);
  }
  :deep(.p-treetable-tbody > tr > td) {
    border: 0 !important;
    width: auto !important;
    padding: 4px 8px !important;
  }
  :deep(.p-treetable-tbody > tr > td:first-child) {
    width: 100% !important;
  }
}
</style>
