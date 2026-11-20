import { defineConfig } from 'vitepress'

// The product name is a placeholder until a distinct name is chosen.
// Change it here (and in index.md) and every page follows.
const name = 'Troubleshooting'
const repo = 'https://github.com/agerimex/troubleshooting'
const hostname = 'https://agerimex.github.io/troubleshooting/'

export default defineConfig({
  lang: 'en-US',
  title: name,
  description:
    'Investigate application behavior with OpenTelemetry traces and correlated logs: follow a request across services, database calls and logs, find recurring problems and compare releases. A runtime quality gate checks known requirements against pull-request and release test runs.',
  // Served from https://agerimex.github.io/troubleshooting/
  base: '/troubleshooting/',
  cleanUrls: true,
  lastUpdated: true,
  sitemap: { hostname },
  head: [
    ['meta', { name: 'theme-color', content: '#3f6b12' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:site_name', content: name }],
    ['meta', { property: 'og:image', content: hostname + 'screenshots/compare-result.png' }]
  ],
  themeConfig: {
    nav: [
      { text: 'Investigate', link: '/guide/investigate-a-slow-request' },
      { text: 'Quality gate', link: '/guide/quality' },
      { text: 'Handbook', link: '/guide/getting-started' },
      { text: 'Articles', link: '/articles/' },
      { text: 'GitHub', link: repo }
    ],
    sidebar: {
      '/guide/': [
        {
          text: 'Handbook',
          items: [
            { text: 'Getting started', link: '/guide/getting-started' },
            { text: 'Investigate a slow request', link: '/guide/investigate-a-slow-request' },
            { text: 'Quality gate', link: '/guide/quality' },
            { text: 'Log Analysis', link: '/guide/log-analysis' },
            { text: 'Timeline & Sequence', link: '/guide/timeline-sequence' },
            { text: 'Problems', link: '/guide/problems' },
            { text: 'Releases', link: '/guide/releases' },
            { text: 'Settings', link: '/guide/settings' },
            { text: 'Reference', link: '/guide/reference' }
          ]
        }
      ],
      '/articles/': [
        {
          text: 'Articles',
          items: [
            { text: 'Catch an N+1 query in a pull request', link: '/articles/catch-n-plus-one-in-a-pull-request' },
            { text: 'Compared with Tracetest and Jaeger', link: '/articles/compared-with-tracetest-and-jaeger' },
            { text: 'Compare p95 latency between releases', link: '/articles/compare-p95-between-releases' },
            { text: 'Logs and traces in one view', link: '/articles/logs-and-traces-in-one-view' },
            { text: 'Execution shapes from traces', link: '/articles/execution-shapes' },
            { text: 'Recurring errors, not single errors', link: '/articles/recurring-error-groups' }
          ]
        }
      ]
    },
    socialLinks: [{ icon: 'github', link: repo }],
    editLink: { pattern: repo + '/edit/main/site/:path', text: 'Edit this page on GitHub' },
    search: { provider: 'local' },
    footer: { message: 'Open source on GitHub.' }
  }
})
