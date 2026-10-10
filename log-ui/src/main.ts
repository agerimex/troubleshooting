import "primeflex/primeflex.css"
import "primevue/resources/themes/lara-dark-amber/theme.css"
import "primevue/resources/primevue.min.css"
import "primeicons/primeicons.css"
import "./style.css"

import { createApp } from 'vue'
import PrimeVue from 'primevue/config'
import App from './App.vue'


export const app = createApp(App)
app
  .use(PrimeVue)
  .mount('#app')
