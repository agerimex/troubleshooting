<template>
  <main class="w-full">
    <div class="card w-full">
      <TreeTable :value="nodes" :lazy="true" :paginator="true" :rows=25 :rowsPerPageOptions="[10, 25, 100, 500, 1000]" 
          paginatorTemplate="RowsPerPageDropdown CurrentPageReport NextPageLink" v-model:first="firstRow"
          currentPageReportTemplate="{first} - {last} (Total of parents by filter {totalRecords})"
          filterDisplay="row"
          :globalFilterFields="['name','country.name', 'company', 'representative.name']"
          :loading="loading" @nodeExpand="onExpand" @page="onPage" :totalRecords="totalRecords" v-model:filters="filters" ref="spansTree">
        <template #header>
          <div style="text-align:left">
            <MultiSelect :modelValue="selectedColumns" @update:modelValue="onToggle" :options="columns" optionLabel="header" class="w-full" display="chip"/>
          </div>
        </template>
        <template #paginatorstart>
          <PrimeButton type="button" icon="pi pi-refresh" text @click="onRefresh" />
        </template>
        <template #paginatorend>
          <PrimeButton v-if="false" type="button" icon="pi pi-download" text />
        </template>
        
        <Column field="humanTime" header="time" style="width: 18rem" :expander="true">
          <template #filter>
            <Calendar id="calendar-24h" v-model="filterTimeFrom" v-on:update:modelValue="changeTime()" showTime hourFormat="24" :showSeconds="true" dateFormat="dd.mm.yy"/>
          </template>
        </Column>
        <Column field="name" header="name" style="width: 18rem" :expander="false">
          <template #filter>
            <InputText v-model="filterByMethodName" placeholder="Fiter by method name" type="text" class="p-column-filter" v-on:update:modelValue="filterChanged()"/>
          </template>
        </Column>
        <Column field="status" header="status" style="width: 10rem">
          <template #body="{ node }">
            <Tag :value="node.data.status" :severity="getSeverity(node.data.status)"/>
          </template>
          <template #filter>
            <div class="flex">
              <Dropdown v-model="filterStatus" @change="filterStatusChange()" :options="statuses" placeholder="Select one" class="p-column-filter" style="width: 9rem" :showClear="true">
                <template #option="slotProps">
                  <Tag :value="slotProps.option" :severity="getSeverity(slotProps.option)" />
                </template>
              </Dropdown>
            </div>
          </template>
        </Column>
        <Column v-for="col of selectedColumns" :field="col.field" :header="col.header" :key="col.field">
          <template #body="{ node }">
            <div @dblclick="JSON.stringify(showDetails(node.data[col.field]))" class="multiline-cell" :title="JSON.stringify(node.data[col.field])">{{ node.data ? node.data[col.field] : "" }}</div>
          </template>
          <template #filter>
            <div class="flex" v-if="col.filter">
              <InputText v-model="filters[col.field]" type="text" class="p-column-filter" :placeholder="col.filterPlaceHolder" v-on:update:modelValue="filterChanged()"/>
            </div>
          </template>
        </Column>
        <Column field="duration" header="duration, ms" headerStyle="width: 6rem; text-align: right" bodyStyle="text-align: right; overflow: visible"></Column>
      </TreeTable>
      <PrimeDialog v-model:visible="dialogVisible">
        <vue-json-pretty class="text-center" :showSelectController="false" :data="selectedCellText" />
        <template #footer>
          <PrimeButton @click="closeDialog" label="Close" />
        </template>
      </PrimeDialog>
    </div>
  </main>
</template>

<script lang="ts">
import { ref, defineComponent } from 'vue'
import DataTable from 'primevue/datatable'
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
import logsApi from '@/api/logs.api'
import spansApi from '@/api/spans.api'
import { Logs } from '@/logs-api/logs.impl'
import { Spans } from '@/logs-api/spans.impl'
import { format } from "date-fns"
import { type SpanFilter } from '@/types/filter'
import VueJsonPretty from 'vue-json-pretty'
import 'vue-json-pretty/lib/styles.css'
import { format as sqlFormatter } from 'sql-formatter'

interface Filters {
  [key: string]: any
}
export default defineComponent({
  name: 'LogsUI',
  components: {
    DataTable,
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
    VueJsonPretty
  },
  beforeMount () {
  },
  async created () {
    document.title = 'Logs APP'
  },
  setup () {
    const logsList = ref()

    const spansList = ref()
    const spansTree = ref()

    const selectedCellText = ref()
    const dialogVisible = ref(false)

    const filterByMethodName = ref('')

    spansApi.init(new Spans()).then(() => { fetchSpans() })
    
    // coming soon
    logsApi.init(new Logs()).then(() => { fetchLogs() })

    const nodes = ref()
    const loading = ref(false)
    const totalRecords = ref(0)
    const lastRowTime = ref("0")
    const rowsPerPage = ref(25)
    const statuses = ref(['unset', 'error', 'ok'])
    const filterStatus = ref()
    const filterTimeFrom = ref(new Date(Date.now() - 5*60*1000))
    const firstRow = ref(0)

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
    loading.value = true
    const selectedColumns = ref(defaultColumns.value)

    const filters = ref<Filters>({})
    const filterMode = ref({ label: 'Lenient', value: 'lenient' })
    const filterOptions = ref([
        { label: 'Lenient', value: 'lenient' },
        { label: 'Strict', value: 'strict' }
    ])

    async function fetchLogs() {
      const [error, res] = await logsApi.allLogs()
      if (error === null && res !== null) {
        logsList.value = res
      }
    }

    async function getchCountSpans(filter: SpanFilter = {}) {
      const [error, res] = await spansApi.countOfSpans(filter)
      if (error === null && res !== null) {
        totalRecords.value = res
      }
    }

    function spanFilter() {
      let timeFrom = filterTimeFrom.value.getTime().toString() + '000000'
      if (lastRowTime.value !== "0") {
        timeFrom = lastRowTime.value
      }
      const filter: SpanFilter = {timeFrom: timeFrom, rowsPerPage: rowsPerPage.value, status: filterStatus.value, serviceName: filters.value['service'], methodName: filterByMethodName.value }
      return filter
    }

    function spanFilterNoTime() {
      let timeFrom = filterTimeFrom.value.getTime().toString() + '000000'
      const filter: SpanFilter = {timeFrom: timeFrom, rowsPerPage: rowsPerPage.value, status: filterStatus.value, serviceName: filters.value['service'], methodName: filterByMethodName.value }
      return filter
    }

    async function fetchSpans() {
      const filter = spanFilter()
      const [error, res] = await spansApi.viewSpans(filter)
      if (error === null && res !== null) {
        spansList.value = res
        nodes.value = await loadNodes(0, spansTree.value.rows)
        await getchCountSpans(spanFilterNoTime())
        loading.value = false
      } else {
        loading.value = false
        spansList.value = []
        nodes.value = []
        totalRecords.value = 0
      }
    }

    const onExpand = async (node: any) => {
      if (!node.children) {
        loading.value = true

        const [error, res] = await spansApi.viewSpans({parentId: node.key, rowsPerPage: 100000, timeFrom: "0"})
        if (error === null && res !== null) {
          let lazyNode = {...node}

          lazyNode.children = []
          for(let i = 0; i < res.length; i++) {
            lazyNode.children.push(createItem(res[i]))
          }

          let newNodes = nodes.value.map((n: { key: any }) => {
            if (n.key === node.key) {
              n = lazyNode
            }
            return n
          })
          nodes.value = newNodes
        }
        spansList.value = res
        loading.value = false
      }
    }

    const onPage = async (event: any) => {
      loading.value = true
      var first = event.first
      if(rowsPerPage.value !== event.rows) {
        rowsPerPage.value = event.rows
        lastRowTime.value = "0"
        firstRow.value = 0
        first = 0
      }

      fetchSpans()
    }

    function filterChanged () {
      onRefresh()
    }

    function createItem (row: any) {
      var timeEvent = new Date(+row.timeStamp.slice(0, -6))
      return {
        key: row.spanId,
        data: {
          humanTime: format(timeEvent, 'dd.MM.yy HH:mm:ss') + '.' + timeEvent.getMilliseconds(),
          name: row.name,
          traceId: row.traceId,
          spanId: row.spanId,
          parentSpanId: row.parentSpanId,
          service: row.service,
          tags: row.tags,
          serviceTags: row.serviceTags,
          msg: row.msg,
          statusCode: row.statusCode,
          statusMessage: row.statusMessage,
          status: row.status,
          duration: Math.round(row.duration / 1000)
        },
        leaf: row.childSpanCount == 0
      }
    }

    const loadNodes = async (first: any, rows: any) => {
      let nodes = []
      for(let i = 0; i < rows; i++) {
        if (i < spansList.value.length) {
          nodes.push(createItem(spansList.value[i]))
        }
      }
      if (spansList.value.length > 0) {
        lastRowTime.value = spansList.value[spansList.value.length - 1].timeStamp
      }
      
      return nodes
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

    async function filterStatusChange () {
      lastRowTime.value = "0"
      firstRow.value = 0

      fetchSpans()
    }

    function changeTime () {
      lastRowTime.value = "0"
      firstRow.value = 0

      fetchSpans()
    }

    function onRefresh () {
      lastRowTime.value = "0"
      firstRow.value = 0

      fetchSpans()
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

    function showDetails(content: string) {
      selectedCellText.value = content
      const formattedContent = formatContent(content)
      if (formattedContent !== null) {
          selectedCellText.value = formattedContent as string | object
      }
      dialogVisible.value = true
    }

    function closeDialog() {
      dialogVisible.value = false
    }

    return {
      logsList,
      spansList,
      nodes,
      onExpand,
      onPage,
      totalRecords,
      loading,
      selectedColumns,
      defaultColumns,
      columns,
      onToggle,
      filters,
      filterMode,
      filterOptions,
      spansTree,
      filterChanged,
      getSeverity,
      statuses,
      filterStatus,
      filterStatusChange,
      filterTimeFrom,
      firstRow,
      changeTime,
      onRefresh,
      selectedCellText,
      dialogVisible,
      showDetails,
      closeDialog,
      filterByMethodName
    }
  }
})
</script>


<style scoped>
.multiline-cell {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  overflow: hidden;
  max-height: calc(1.5em * 3);
  -webkit-line-clamp: 3;
  line-height: 1.5em;
}

header {
  line-height: 1.5;
}

.logo {
  display: block;
  margin: 0 auto 2rem;
}

@media (min-width: 1024px) {
  header {
    display: flex;
    place-items: center;
    padding-right: calc(var(--section-gap) / 2);
  }

  .logo {
    margin: 0 2rem 0 0;
  }

  header .wrapper {
    display: flex;
    place-items: flex-start;
    flex-wrap: wrap;
  }
}
</style>
