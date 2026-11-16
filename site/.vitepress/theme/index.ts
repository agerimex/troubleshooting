import DefaultTheme from 'vitepress/theme'
import type { Theme } from 'vitepress'
import HomeLanding from './HomeLanding.vue'
import './custom.css'

// The default theme plus the landing page component used by index.md.
export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('HomeLanding', HomeLanding)
  }
} satisfies Theme
