import './assets/main.css'
import "primeicons/primeicons.css"

import { createApp } from 'vue'
import PrimeVue from 'primevue/config'
import Tailwind from "primevue/passthrough/tailwind"
import App from './App.vue'


export const app = createApp(App)
app
  .use(PrimeVue, { unstyled: true, pt: Tailwind })
  .mount('#app')
