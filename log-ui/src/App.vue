<template>
  <TracePage/>
</template>

<script lang="ts">
import { defineComponent } from 'vue'
import TracePage from './components/TracePage.vue'

export default defineComponent({
  name: 'TroubleShooting',
  components: {
    TracePage
  },
  beforeMount () {
  },
  async created () {
    document.title = 'Log App'
  }
})
</script>

<style scoped>
</style>
