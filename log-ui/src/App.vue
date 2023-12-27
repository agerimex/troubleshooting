<template>
  <header>
  </header>

  <main class="w-full">
    <DataTable v-if="false" :value="logsList" paginator :rows="10" :rowsPerPageOptions="[5, 10, 20, 50]" tableStyle="min-width: 50rem"
                currentPageReportTemplate="{first} to {last} of {totalRecords}">
      <Column field="id" header="id"></Column>
      <Column field="level" header="level"></Column>
      <Column field="msg" header="msg"></Column>
      <Column field="time" header="time"></Column>
    </DataTable>

    <DataTable v-if="false" :value="spansList" paginator :rows="10" :rowsPerPageOptions="[5, 10, 20, 50]" tableStyle="min-width: 50rem">
      <Column field="id" header="id"></Column>
      <Column field="name" header="name"></Column>
      <Column field="service" header="service"></Column>
      <Column field="time" header="time"></Column>
      <Column field="traceId" header="traceId"></Column>
      <Column field="spanId" header="spanId"></Column>
      <Column field="parentSpanId" header="parentSpanId"></Column>
      <Column field="Tags" header="Tags"></Column>
      <Column field="ServiceTags" header="ServiceTags"></Column>
    </DataTable>

    <div class="card w-full">
      <TreeTable :value="nodes" :lazy="true" :paginator="true" :rows="100" :rowsPerPageOptions="[10, 100, 500, 1000]" 
          paginatorTemplate="RowsPerPageDropdown CurrentPageReport NextPageLink" v-model:first="firstRow"
          currentPageReportTemplate="{first} - {last} (Total of parents by filter {totalRecords})"
          @filter="onFilter($event)" filterDisplay="row"
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
        
        <Column field="humanTime" header="humanTime" :expander="true">
          <template #filter>
            <Calendar id="calendar-24h" v-model="filterTimeFrom" v-on:update:modelValue="changeTime()" showTime hourFormat="24" :showSeconds="true" dateFormat="dd.mm.yy"/>
          </template>
        </Column>
        <Column field="name" header="Name" :expander="false"></Column>
        <Column field="status" header="status" :showFilterMenu="false" :filterMenuStyle="{ width: '14rem' }" style="min-width: 12rem">
          <template #body="{ node }">
            <Tag :value="node.data.status" :severity="getSeverity(node.data.status)" />
          </template>
          <template #filter>
            <div class="flex">
              <Dropdown v-model="filterStatus" @change="filterStatusChange()" :options="statuses" placeholder="Select One" class="p-column-filter" style="min-width: 12rem" :showClear="true">
                <template #option="slotProps">
                  <Tag :value="slotProps.option" :severity="getSeverity(slotProps.option)" />
                </template>
              </Dropdown>
            </div>
          </template>
        </Column>
        
        <Column v-for="col of selectedColumns" :field="col.field" :header="col.header" :key="col.field">
          <template #filter>
            <div class="flex" v-if="col.filter">
              <InputText v-model="filters[col.field]" type="text" class="p-column-filter" :placeholder="col.filterPlaceHolder" v-on:update:modelValue="filterChanged()"/>
            </div>
          </template>
        </Column>
        <Column field="duration" header="duration" headerStyle="width: 5rem; text-align: right" bodyStyle="text-align: right; overflow: visible"></Column>
      </TreeTable>
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
import logsApi from './api/logs.api'
import spansApi from './api/spans.api'
import { Logs } from './logs-api/logs.impl'
import { Spans } from './logs-api/spans.impl'
import { format } from "date-fns"
import { type SpanFilter } from './types/filter'

interface Filters {
  [key: string]: any
}
// import ColumnGroup from 'primevue/columngroup'  // optional
// import Row from 'primevue/row'              // optional
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
    Dropdown
  },
  beforeMount () {
    // logsApi.init(new Logs())
  },
  async created () {
    // router.push('/auth')
    document.title = 'Logs APP'
  },
  setup () {
    const logsList = ref()

    const spansList = ref()
    const spansTree = ref()

    spansApi.init(new Spans()).then(() => { fetchSpans() })
    logsApi.init(new Logs()).then(() => { fetchLogs() })

    const nodes = ref()
    //const rows = ref(10)
    const loading = ref(false)
    const totalRecords = ref(0)
    const lastRowTime = ref("0")
    const rowsPerPage = ref(100)
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
    // logsList.value = logsApi.allLogs()
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
      console.log("filters.value['service']", filters.value['service'])
      const filter: SpanFilter = {timeFrom: timeFrom, rowsPerPage: rowsPerPage.value, status: filterStatus.value, serviceName: filters.value['service'] }
      return filter
    }

    async function fetchSpans() {
      const filter = spanFilter()
      const [error, res] = await spansApi.viewSpans(filter)
      if (error === null && res !== null) {
        spansList.value = res
        nodes.value = await loadNodes(0, spansTree.value.rows)
        await getchCountSpans(filter)
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

        console.log(nodes.value)
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
      //imitate delay of a backend call
      // const [error, res] = await spansApi.viewSpans({rowsPerPage: rowsPerPage.value, timeFrom: lastRowTime.value})
      // if (error === null && res !== null) {
      //   spansList.value = res
      //   loading.value = false
      //   nodes.value = await loadNodes(first, rowsPerPage.value)
      // }
    }

    const onFilter = (event: any) => {
      //lazyParams.value.filters = filters.value ;
      //loadLazyData(event);
      console.log(event)
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
          duration: row.duration
        },
        leaf: row.childSpanCount == 0
      }
    }

    const loadNodes = async (first: any, rows: any) => {
      // await spansApi.init(new Spans())
      // logsList.value = logsApi.allLogs()
      // const [error, res] = await spansApi.viewSpans()
      // if (error === null && res !== null) {
      console.log(spansList.value)
      // spansList.value = res
      let nodes = []
      for(let i = 0; i < rows; i++) {
        if (i < spansList.value.length) {
          nodes.push(createItem(spansList.value[i]))
        }
      }
      if (spansList.value.length > 0) {
        lastRowTime.value = spansList.value[spansList.value.length - 1].timeStamp
      }

      console.log('lastRowTime!!!!!', lastRowTime)
      
      return nodes
    }

    const onToggle = (val: any) => {
      selectedColumns.value = columns.value.filter(col => val.includes(col))
    }

    const getSeverity = (status: any) => {
      console.log('severity = ', status)
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
      // const filter = spanFilter()
      // loading.value = true

      // const [error, res] = await spansApi.viewSpans(filter)
      //   if (error === null && res !== null) {
      //     console.log(res)
      //     spansList.value = res

      //   loading.value = false
      //   nodes.value = await loadNodes(0, rowsPerPage.value)
      // }
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
      onFilter,
      filterChanged,
      getSeverity,
      statuses,
      filterStatus,
      filterStatusChange,
      filterTimeFrom,
      firstRow,
      changeTime,
      onRefresh
    }
  }
})
</script>


<style scoped>
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
