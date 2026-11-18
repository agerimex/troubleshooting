<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { withBase } from 'vitepress'

// Landing page (index.md). Design adapted from the web-agent proposal in
// spec-16.11/Troubleshooting_GitHub_Pages. The quality gate is the main story;
// only features that exist today are shown, plus one section marked planned.

const repo = 'https://github.com/agerimex/troubleshooting'
const shot = (file: string) => withBase('/landing/' + file)
const link = (path: string) => withBase(path)

interface Tab {
  id: string
  label: string
  title: string
  copy: string[]
  image: string
  width: number
  height: number
  caption: string
  alt: string
  handbook: string
  scroll?: boolean
}

const tabs: Tab[] = [
  {
    id: 'quality', label: 'Quality', title: 'Check a run, or open a CI result.',
    copy: [
      'Paste the rules from your repository, name the test run and a baseline. Every check shows its value, limit, advisory target and the baseline.',
      'A CI result.json opens with the same evidence links. When the live spans have expired, the snapshot saved in the result is shown instead.'
    ],
    image: 'quality.webp', width: 2160, height: 2594, caption: 'Quality',
    alt: 'Quality page: rules, run and baseline, a failed gate with three failing checks and their evidence links.',
    handbook: '/guide/quality', scroll: true
  },
  {
    id: 'sequence', label: 'Sequence', title: 'See which services talk to each other.',
    copy: [
      'The sequence view gives each recorded service a lane and draws arrows for calls between them. Open a call to inspect its spans and logs.',
      'A client call and the server span it reached are one arrow; both stay selectable.'
    ],
    image: 'sequence.webp', width: 2160, height: 1452, caption: 'Sequence diagram',
    alt: 'Sequence view with service lanes and arrows for recorded calls.',
    handbook: '/guide/timeline-sequence'
  },
  {
    id: 'timeline', label: 'Timeline & logs', title: 'Read logs in context.',
    copy: [
      'See overlapping spans on one time axis. Select a step to see its duration, attributes and exactly the logs it wrote.',
      'Export the investigation as HTML or JSON to share the evidence.'
    ],
    image: 'timeline.webp', width: 2160, height: 1350, caption: 'Timeline & logs',
    alt: 'Timeline with nested spans and the selected span’s logs alongside.',
    handbook: '/guide/timeline-sequence'
  },
  {
    id: 'library', label: 'Execution shapes', title: 'Find the paths your system actually takes.',
    copy: [
      'Group executions by their recorded caller–callee calls. Compare frequency, failures and latency for each shape.',
      'Open a representative execution to see its sequence.'
    ],
    image: 'sequence-library.webp', width: 2160, height: 1452, caption: 'Execution shapes',
    alt: 'Sequence library listing distinct execution shapes with counts and percentiles.',
    handbook: '/articles/execution-shapes', scroll: true
  },
  {
    id: 'logs', label: 'Log Analysis', title: 'Follow a log back to its execution.',
    copy: [
      'Expand spans and read their own logs inline. Inspect attributes, exception events and links without losing the trace context.',
      'Jump from a row into the timeline or sequence of its execution.'
    ],
    image: 'log-analysis.webp', width: 2160, height: 3552, caption: 'Log Analysis',
    alt: 'Log Analysis span table with expanded spans and inline log rows.',
    handbook: '/guide/log-analysis', scroll: true
  },
  {
    id: 'problems', label: 'Problems', title: 'Start with a symptom. Get to the evidence.',
    copy: [
      'Recurring errors, slow calls and release regressions for a service and time window.',
      'Each finding explains the rule, the observed counts and the coverage. Open an example execution to investigate.'
    ],
    image: 'problems.webp', width: 2160, height: 1350, caption: 'Problems',
    alt: 'Problems list with error groups, slow calls and regressions.',
    handbook: '/guide/problems', scroll: true
  },
  {
    id: 'releases', label: 'Release comparison', title: 'Find what changed.',
    copy: [
      'Compare calls, error fractions and p50, p95 and p99 latencies between a baseline and a candidate release.',
      'Versioned rules flag changes worth investigating. Retained example traces show what happened.'
    ],
    image: 'releases.webp', width: 2160, height: 2951, caption: 'Release comparison',
    alt: 'Release comparison with baseline and candidate, latency and error deltas, and example traces.',
    handbook: '/guide/releases', scroll: true
  },
  {
    id: 'rules', label: 'Detection rules', title: 'Decide what deserves attention.',
    copy: [
      'Set thresholds for affected calls, slow operations and p95 release regressions. Every save is a revision.',
      'Raw-data and statistics retention are shown in Settings.'
    ],
    image: 'detection-rules.webp', width: 2160, height: 1520, caption: 'Detection rules',
    alt: 'Settings page with the detection rules form.',
    handbook: '/guide/settings', scroll: true
  }
]

const activeTab = ref(tabs[0].id)
const tabButtons = ref<HTMLButtonElement[]>([])

function onTabKey(event: KeyboardEvent, index: number) {
  const next = ({
    ArrowRight: (index + 1) % tabs.length,
    ArrowLeft: (index + tabs.length - 1) % tabs.length,
    Home: 0,
    End: tabs.length - 1
  } as Record<string, number>)[event.key]
  if (next === undefined) return
  event.preventDefault()
  activeTab.value = tabs[next].id
  nextTick(() => tabButtons.value[next]?.focus())
}

// Release comparison demo (illustrative numbers).
const baselineMs = [64, 82, 105, 120, 151]
const releases = {
  regression: {
    version: 'v1.5.0', title: 'Regression signal', icon: '!',
    description: 'The candidate is slower and returns more errors.',
    latencies: [92, 158, 241, 340, 381], latencyDelta: '↑ 183%', errors: '2.4', errorDelta: '↑ 2.2 pp'
  },
  fixed: {
    version: 'v1.5.1', title: 'Back within the baseline', icon: '✓',
    description: 'This comparison shows no regression signal.',
    latencies: [63, 79, 102, 115, 146], latencyDelta: '↓ 4%', errors: '0.2', errorDelta: '0 pp'
  }
} as const
type ReleaseKey = keyof typeof releases
const releaseKey = ref<ReleaseKey>('regression')
const release = computed(() => releases[releaseKey.value])
const percentiles = ['p50', 'p75', 'p90', 'p95', 'p99']

// Screenshot dialog.
const dialog = ref<HTMLDialogElement>()
const dialogTitle = ref('')
const dialogSrc = ref('')
function openShot(event: MouseEvent, src: string, title: string) {
  const d = dialog.value
  if (!d?.showModal || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return
  event.preventDefault()
  dialogTitle.value = title
  dialogSrc.value = src
  d.showModal()
  d.scrollTop = 0
}

// Quick start copy button.
const quickstart = `git clone ${repo}.git
cd troubleshooting
make all
make up`
const copyLabel = ref('Copy')
const codeEl = ref<HTMLElement>()
let resetCopy: ReturnType<typeof setTimeout> | undefined
async function copyCommands() {
  try {
    await navigator.clipboard.writeText(quickstart)
    copyLabel.value = 'Copied'
  } catch {
    const range = document.createRange()
    if (codeEl.value) range.selectNodeContents(codeEl.value)
    const selection = getSelection()
    selection?.removeAllRanges()
    selection?.addRange(range)
    copyLabel.value = 'Text selected'
  }
  clearTimeout(resetCopy)
  resetCopy = setTimeout(() => (copyLabel.value = 'Copy'), 2200)
}

const issueUrl = repo + '/issues/new?title=' + encodeURIComponent('Quality checks: my use case') +
  '&body=' + encodeURIComponent('What operation or scenario matters?\n\nWhat must remain true?\n\nWhat change broke it?\n\nWhat evidence is available?\n')
</script>

<template>
  <div class="lp">
    <section class="container hero" aria-labelledby="hero-title">
      <div class="hero-copy">
        <p class="eyebrow"><span class="status-dot"></span> OPEN-SOURCE QUALITY GATE FOR GO &amp; OPENTELEMETRY</p>
        <h1 id="hero-title">The tests passed.<br><em>Did the change add twenty database calls?</em></h1>
        <p class="hero-description">Troubleshooting checks every pull request’s test run against latency and call rules kept in your repository, and links each failure to the execution behind it: its spans, its logs, its database calls.</p>
        <div class="hero-actions">
          <a class="button button-primary" href="#how-it-works">Gate a pull request <span aria-hidden="true">→</span></a>
          <a class="text-link" :href="link('/articles/catch-n-plus-one-in-a-pull-request')">Follow a failing pull request</a>
        </div>
        <p class="hero-facts">Self-hosted <span aria-hidden="true">·</span> MIT licensed <span aria-hidden="true">·</span> GitHub Action &amp; CLI <span aria-hidden="true">·</span> Early-stage project</p>
      </div>

      <figure class="hero-output">
        <figcaption class="output-caption">
          <span>A pull request’s test run</span>
          <span class="example-label">Real output · fixture run</span>
        </figcaption>
        <div class="terminal card-dark">
          <pre class="pre-scroll" tabindex="0" aria-label="Command output of troubleshooting-check"><code><span class="code-dim">$ troubleshooting-check --rules quality.yaml \
    --run fx…-qg-candidate --baseline fx…-qg-baseline</span>
<span class="code-fail">Quality gate: FAILED</span>

<span class="code-fail">FAIL</span>    startup-p95  qg-api / start_session (server)
        p95 910 ms, limit 800 ms, 200 executions
        baseline: 420 ms (200 executions)
        evidence: slowest execution: 1010 ms

<span class="code-fail">FAIL</span>    db-calls  qg-api / start_session (server)
        up to 21 database calls per execution, limit 3,
        200 executions, 200 over the limit
        baseline: 2 (200 executions)
        permission_lookup × 20
        insert_session × 1
        evidence: <span class="code-link">…/#/executions?trace=9fc2b374…&amp;view=timeline</span>

<span class="code-fail">FAIL</span>    no-directory  qg-api / start_session (server)
        call to qg-directory in 200 of 200 executions
        baseline: 0 (200 executions)
<span class="code-dim">exit 1</span></code></pre>
          <p class="code-note">Exit 0 passed, 1 failed, 2 not enough data, 3 error. Service names and links shortened.</p>
        </div>
      </figure>
    </section>

    <div class="stack-band">
      <div class="container stack-row"><span>Built with</span><b>OpenTelemetry</b><b>ClickHouse</b><b>Go</b><b>Vue + TypeScript</b></div>
    </div>

    <section class="next-band" id="how-it-works" aria-labelledby="qg-title">
      <div class="section container section-heading qg-heading">
        <h2 id="qg-title">Catch what a change added.<br><em>Before it ships.</em></h2>
        <p>A change, written by a person or a coding agent, can pass every functional test and still make a request slower or call what it must not. Three steps put a check on every pull request.</p>
      </div>

      <div class="container qg-steps">
        <div class="qg-step">
          <h3><span>1</span> Tag the test run</h3>
          <p v-pre>One resource attribute on the service under test. Downstream services join through their recorded parents. In CI, use a new id for every run and attempt (<code>ci-${{ github.run_id }}-${{ github.run_attempt }}</code>): a pull request number repeats on every push.</p>
          <div class="terminal card-dark"><pre class="pre-scroll" tabindex="0" aria-label="Environment variable"><code>OTEL_RESOURCE_ATTRIBUTES=\
  troubleshooting.run=pr-42-1</code></pre></div>
        </div>
        <div class="qg-step">
          <h3><span>2</span> Keep the rules in the repository</h3>
          <p>A strict <code>quality.yaml</code>: a misspelled limit is an error, never silently ignored. <code>run.executions</code> says how many executions the tests make, so lost executions can’t pass.</p>
          <div class="terminal card-dark"><pre class="pre-scroll" tabindex="0" aria-label="quality.yaml"><code>version: 1
entry: {service: recording-api,
        operation: start_session}
run: {executions: 200}
checks:
  - {id: startup-p95, type: latency,
     max_ms: 800, target_ms: 500}
  - {id: db-calls, type: db_calls,
     max_per_execution: 3}
  - {id: no-directory,
     type: forbidden_call,
     to: directory-api}</code></pre></div>
        </div>
        <div class="qg-step">
          <h3><span>3</span> Gate the pull request</h3>
          <p>The limits come from the base branch, so a pull request can’t loosen its own gate. The Action posts one comment on the pull request, updated on every run, uploads HTML and JSON reports, then fails the step.</p>
          <div class="terminal card-dark"><pre v-pre class="pre-scroll" tabindex="0" aria-label="GitHub Actions step"><code>- uses: agerimex/troubleshooting/quality-check@main
  with:
    url: http://localhost:8095
    rules: quality.yaml
    rules-from: base
    run: pr-42-1     # the id from step 1
    fail-on: unresolved</code></pre></div>
        </div>
      </div>

      <div class="section container split qg-rules">
        <div>
          <h3>What a rule can say</h3>
          <ul class="next-list">
            <li><b>Latency limits</b> p95 (or any percentile) of an operation within a budget, with an advisory target.</li>
            <li><b>Call budgets</b> at most N database calls in every execution, not on average.</li>
            <li><b>Forbidden calls</b> this operation must not call that service, anywhere below it.</li>
          </ul>
        </div>
        <div>
          <h3>Not enough data is not a pass</h3>
          <ul class="next-list">
            <li><b>Lost executions</b> fewer executions than the rules declare: spans were not exported, not tagged or outside the window.</li>
            <li><b>Incomplete executions</b> a span whose parent was never recorded, a span that started more children than were recorded (when the sender records child counts, as <code>log-sender</code> does), or an execution that runs past the spans read.</li>
            <li><b>Nothing to measure</b> no valid durations for a latency limit, no database call at all for a call budget, or spans already expired.</li>
          </ul>
          <p class="qg-rules-note">Each of these leaves the gate unresolved, with the reason, and <code>fail-on: unresolved</code> fails the step. A pass holds for the recorded spans: every result says which losses its data can show.</p>
        </div>
      </div>

      <div class="container split qg-evidence">
        <div>
          <h3>Every failure opens its execution.</h3>
          <p>The worst execution behind <code>db-calls</code> opens in Timeline &amp; Sequence: twenty <code>permission_lookup</code> calls where the baseline makes one. <b>Back to Quality</b> returns to the result.</p>
          <p>No Troubleshooting server yet? <code>quality-check/stack</code> starts one inside the CI job. This repository’s CI runs <a :href="repo + '/tree/main/examples/quality-gate'">an example service</a> through it on every push: as written it passes with 2 database calls per request, and a planted change that reads prices one by one and calls <code>legacy-pricing</code> must fail. The API of Troubleshooting itself is checked the same way.</p>
          <a class="text-link" :href="shot('quality-evidence.webp')" @click="openShot($event, shot('quality-evidence.webp'), 'Evidence of db-calls in the timeline')">Full screenshot <span class="external-arrow" aria-hidden="true"></span></a>
        </div>
        <figure class="screenshot-frame screenshot-scroll card-dark" tabindex="0" aria-label="Scrollable evidence screenshot">
          <figcaption>Evidence of db-calls <span>Fixture data</span></figcaption>
          <img :src="shot('quality-evidence.webp')" width="2160" height="2300" loading="lazy" alt="Timeline of the execution with the most database calls: start_session with twenty permission_lookup spans, opened from the quality check db-calls.">
        </figure>
      </div>

      <div class="section container split qg-evidence">
        <figure class="screenshot-frame screenshot-scroll card-dark" tabindex="0" aria-label="Scrollable report screenshot">
          <figcaption>report.html <span>Example application</span></figcaption>
          <img :src="link('/articles/quality-report-regression.webp')" width="2440" height="1784" loading="lazy" alt="HTML report of a failed gate: the check orders-db-calls with the saved snapshot of the execution with 9 database calls.">
        </figure>
        <div>
          <h3>The pull request keeps the record.</h3>
          <p>The comment and the reports name what was checked: the tested commit (for a pull request, the merge commit, next to its head), the branch the rules came from, and the rules’ sha256. A comment for an older head is never written over a newer one.</p>
          <p>The JSON and HTML reports keep a snapshot of each evidence execution, so a failure still shows what happened after the spans have expired, or when the Troubleshooting of the job is gone. The Quality page opens the same <code>result.json</code>.</p>
          <a class="text-link" :href="link('/guide/quality')">How the quality gate works <span aria-hidden="true">→</span></a>
        </div>
      </div>
    </section>

    <section class="section container" id="product" aria-labelledby="product-title">
      <div class="section-heading">
        <h2 id="product-title">Then investigate<br><em>what the gate found.</em></h2>
        <p>Evidence opens in the same views you use for everything else: one execution as a timeline or sequence with its logs, recurring problems, execution shapes. Screenshots come from the working application with fixture data.</p>
      </div>
      <div class="product-tabs" role="tablist" aria-label="Product views">
        <button v-for="(tab, i) in tabs" :key="tab.id" ref="tabButtons" type="button" role="tab"
                :id="'tab-' + tab.id" :aria-controls="'panel-' + tab.id"
                :aria-selected="activeTab === tab.id" :tabindex="activeTab === tab.id ? 0 : -1"
                @click="activeTab = tab.id" @keydown="onTabKey($event, i)">{{ tab.label }}</button>
      </div>
      <article v-for="tab in tabs" v-show="activeTab === tab.id" :key="tab.id" class="product-panel"
               :id="'panel-' + tab.id" role="tabpanel" :aria-labelledby="'tab-' + tab.id" tabindex="0">
        <div class="product-panel-copy">
          <h3>{{ tab.title }}</h3>
          <p v-for="(line, i) in tab.copy" :key="i">{{ line }}</p>
          <div class="panel-links">
            <a class="text-link" :href="link(tab.handbook)">In the handbook <span aria-hidden="true">→</span></a>
            <a class="text-link" :href="shot(tab.image)" @click="openShot($event, shot(tab.image), tab.caption)">Full screenshot <span class="external-arrow" aria-hidden="true"></span></a>
          </div>
        </div>
        <figure class="screenshot-frame card-dark" :class="{ 'screenshot-scroll': tab.scroll }"
                :tabindex="tab.scroll ? 0 : undefined" :aria-label="tab.scroll ? 'Scrollable ' + tab.caption + ' screenshot' : undefined">
          <figcaption>{{ tab.caption }} <span>Fixture data</span></figcaption>
          <img :src="shot(tab.image)" :width="tab.width" :height="tab.height" loading="lazy" :alt="tab.alt">
        </figure>
      </article>
    </section>

    <section class="release-band" id="release-demo" aria-labelledby="release-title">
      <div class="section container split">
        <div>
          <h2 id="release-title">Beyond pull requests:<br><em>compare releases.</em></h2>
          <p>The gate checks a test run. In staging or production, compare latency and error fractions for the same operation across versions. Open a retained example to inspect the spans behind a regression signal.</p>
          <a class="text-link" :href="link('/articles/compare-p95-between-releases')">How release comparison works <span aria-hidden="true">→</span></a>
        </div>
        <figure class="comparison" :class="{ fixed: releaseKey === 'fixed' }">
          <figcaption><span>Release comparison</span><span class="example-label">Illustrative demo</span></figcaption>
          <div class="comparison-window card-dark">
            <div class="window-bar">
              <span class="window-dots" aria-hidden="true"><i></i><i></i><i></i></span>
              <span>orders-api / GET /orders</span>
              <span class="environment">staging</span>
            </div>
            <div class="release-picker">
              <div><span class="tiny-label">BASELINE</span><b>v1.4.0</b></div>
              <span class="release-arrow" aria-hidden="true">→</span>
              <div><span class="tiny-label">CANDIDATE</span><b>{{ release.version }}</b></div>
            </div>
            <div class="comparison-result">
              <span class="signal-icon" aria-hidden="true">{{ release.icon }}</span>
              <p><strong>{{ release.title }}</strong><br><span>{{ release.description }}</span></p>
            </div>
            <div class="metric-grid">
              <div><p>p95 latency</p><div class="metric"><strong>{{ release.latencies[3] }}</strong><span>ms</span></div><small><span>{{ release.latencyDelta }}</span> from 120 ms</small></div>
              <div><p>Error fraction</p><div class="metric"><strong>{{ release.errors }}</strong><span>%</span></div><small><span>{{ release.errorDelta }}</span> from 0.2%</small></div>
            </div>
            <div class="chart-heading"><span>Latency by percentile</span><span class="chart-legend"><i></i>Baseline <i></i>Candidate</span></div>
            <div class="chart" role="img" :aria-label="`Illustrative p95 latency: 120 milliseconds at baseline and ${release.latencies[3]} milliseconds in the candidate.`">
              <div class="chart-scale" aria-hidden="true"><span>400 ms</span><span>200 ms</span><span>0</span></div>
              <div class="chart-bars" aria-hidden="true">
                <div v-for="(p, i) in percentiles" :key="p" class="bar-group">
                  <div class="bar-pair"><i :style="{ '--height': baselineMs[i] / 4 + '%' }"></i><b :style="{ '--height': release.latencies[i] / 4 + '%' }"></b></div>
                  <span>{{ p }}</span>
                </div>
              </div>
            </div>
            <div class="demo-controls" role="group" aria-label="Select an example release">
              <span>Try a release</span>
              <button type="button" :aria-pressed="releaseKey === 'regression'" @click="releaseKey = 'regression'">Regression</button>
              <button type="button" :aria-pressed="releaseKey === 'fixed'" @click="releaseKey = 'fixed'">After the fix</button>
            </div>
            <p class="sr-only" role="status">{{ release.title }}. p95 {{ release.latencies[3] }} milliseconds, error fraction {{ release.errors }} percent.</p>
          </div>
        </figure>
      </div>
    </section>

    <section class="planned-band" id="requirements" aria-labelledby="req-title">
      <div class="section container split req-split">
        <div>
          <p class="eyebrow"><span class="status-dot"></span> NEW · REQUIREMENTS &amp; HISTORY</p>
          <h2 id="req-title">Requirements,<br><em>and their history.</em></h2>
          <p>Name what must stay true, with an owner and a reason, next to the checks that enforce it in <code>quality.yaml</code>. A requirement is met only when its own checks pass; one without a check is listed and never met.</p>
          <p>Record each run on <code>main</code> and every requirement gets a history: when it started failing, which commit first passed its checks again, and the evidence of every failure, kept after its spans expire.</p>
          <div class="start-links">
            <a class="text-link" :href="link('/guide/quality#requirements-and-their-history')">Requirements in the handbook <span aria-hidden="true">→</span></a>
            <a class="text-link next-cta" :href="issueUrl">Bring a real regression <span class="external-arrow" aria-hidden="true"></span></a>
          </div>
        </div>
        <figure class="screenshot-frame screenshot-scroll card-dark" tabindex="0" aria-label="Scrollable requirements screenshot">
          <figcaption>Requirements <span>Fixture data</span></figcaption>
          <img :src="shot('quality-requirements.webp')" width="2072" height="1760" loading="lazy" alt="Quality page, Requirements: three requirements of the project recording with owner, reason, check statuses and a row of history cells, met, violated twice, not enough data, met twice.">
        </figure>
      </div>
    </section>

    <section class="start-band" id="quick-start" aria-labelledby="start-title">
      <div class="section container split">
        <div>
          <h2 id="start-title">Run Troubleshooting<br>locally.</h2>
          <p>Start the stack, then connect an instrumented backend. If you find a bug or want to contribute, bring a small reproducible example to GitHub.</p>
          <div class="start-links">
            <a class="text-link" :href="repo">Browse source <span class="external-arrow" aria-hidden="true"></span></a>
            <a class="text-link" :href="repo + '/issues'">Issues <span class="external-arrow" aria-hidden="true"></span></a>
          </div>
        </div>
        <div class="start-right">
          <div class="terminal card-dark">
            <div class="code-header"><span>Quick start</span><button type="button" aria-label="Copy quick-start commands" @click="copyCommands">{{ copyLabel }}</button></div>
            <pre><code ref="codeEl">{{ quickstart }}</code></pre>
            <p class="code-note">Then open <a href="http://localhost:8095">localhost:8095</a></p>
          </div>
          <p class="requirements">Requires Git, Make, and Docker with Compose. First startup can take a few minutes.</p>
          <a class="text-link" :href="link('/guide/getting-started')">Connect a Go or OpenTelemetry application <span aria-hidden="true">→</span></a>
          <p class="sr-only" role="status">{{ copyLabel === 'Copy' ? '' : copyLabel }}</p>
        </div>
      </div>
    </section>

    <section class="section container split faq" aria-labelledby="faq-title">
      <h2 id="faq-title">Before you start</h2>
      <div>
        <details><summary>What does the quality gate need?</summary><p>Spans of your test run tagged with the attribute <code>troubleshooting.run</code> (one environment variable for an OpenTelemetry SDK), a <code>quality.yaml</code> in your repository, and a Troubleshooting the CI job can reach: your own, or one started inside the job with <code>quality-check/stack</code>. Database calls are counted from client spans with <code>db.system</code>; SQL text and logs are never parsed. <a :href="link('/guide/quality')">The handbook page</a> has the details.</p></details>
        <details><summary>Is a pass a guarantee?</summary><p>No. A pass describes the executions of this run: a forbidden call that was not observed is reported as “not observed in this run”. What the tests didn’t execute, or what wasn’t instrumented, can’t be judged. A verdict assumes the spans were not sampled out and were exported before the check: lost executions, spans naming a missing parent and, with child counts from <code>log-sender</code>, lost child spans are caught as not enough data; a child span lost without any trace of it can’t be seen in plain OpenTelemetry data, and the result says so.</p></details>
        <details><summary>Where does my telemetry go?</summary><p>The stack runs on your infrastructure. You operate the receiver and its ClickHouse storage. Review receiver authentication and retention before using it outside a development environment.</p></details>
        <details><summary>What instrumentation do I need?</summary><p>OpenTelemetry spans from any SDK or Collector over OTLP/gRPC, or the small Go library <code>log-sender</code> for spans plus logs that carry their trace and span. <a :href="link('/guide/getting-started')">Getting started</a> shows both.</p></details>
        <details><summary>Does the sequence view show my entire architecture?</summary><p>It shows recorded service interactions for observed executions. Missing instrumentation, sampling and unexecuted paths limit what can be inferred. Execution shapes group what was observed; they are not a complete static dependency graph.</p></details>
      </div>
    </section>

    <dialog ref="dialog" class="shot-dialog card-dark" aria-labelledby="shot-dialog-title">
      <div class="dialog-header"><strong id="shot-dialog-title">{{ dialogTitle }}</strong><form method="dialog"><button type="submit">Close</button></form></div>
      <img v-if="dialogSrc" :src="dialogSrc" :alt="dialogTitle">
    </dialog>
  </div>
</template>

<style scoped>
/* Tokens: light by default, dark with VitePress's .dark class. The product
   mock-ups (.card-dark) stay dark in both themes. */
.lp {
  --background: var(--vp-c-bg);
  --band: #f5f8f1;
  --border: #dde4d6;
  --text: #1b2316;
  --muted: #4f5b49;
  --subtle: #68745f;
  --accent: #3f6b12;
  --amber: #9a5a06;
  --label-border: #c9d2c1;
  --start-bg: #dcefc0;
  --start-text: #192511;
  --start-muted: #38522d;
  --mono: var(--vp-font-family-mono);
  --sans: var(--vp-font-family-base);
  background: var(--background);
  color: var(--text);
  font: 16px/1.65 var(--sans);
  -webkit-font-smoothing: antialiased;
}
.dark .lp {
  --band: #131813;
  --border: #333c32;
  --text: #edf2e8;
  --muted: #aeb9a9;
  --subtle: #96a38f;
  --accent: #d6f89a;
  --amber: #edbd7d;
  --label-border: #414c3c;
  --start-bg: #d6f89a;
}
.card-dark {
  --border: #333c32;
  --text: #edf2e8;
  --muted: #aeb9a9;
  --subtle: #96a38f;
  --accent: #d6f89a;
  --amber: #edbd7d;
  --label-border: #414c3c;
  color: var(--text);
}

h1, h2, h3, p, figure { margin: 0; }
img { display: block; max-width: 100%; height: auto; }
em { color: var(--accent); font-style: normal; }
a:focus-visible, button:focus-visible, [tabindex]:focus-visible, summary:focus-visible { outline: 2px solid var(--accent); outline-offset: 4px; }
.container { width: min(1220px, calc(100% - 96px)); margin-inline: auto; }
.section { padding-block: clamp(62px, 8vw, 108px); }
.sr-only {
  position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px;
  overflow: hidden; clip-path: inset(50%); white-space: nowrap;
}

.button, .text-link { display: inline-flex; gap: 10px; align-items: center; font-size: 14px; }
.button {
  justify-content: center; padding: 11px 15px; border: 1px solid transparent;
  border-radius: 6px; font-weight: 630; transition: background .18s, border-color .18s;
}
.button-primary { background: var(--vp-button-brand-bg); color: var(--vp-button-brand-text); padding: 13px 20px; }
.button-primary:hover { background: var(--vp-button-brand-hover-bg); color: var(--vp-button-brand-hover-text); }
.text-link { width: fit-content; font-weight: 600; border-bottom: 1px solid transparent; }
.text-link:hover { color: var(--accent); border-color: currentColor; }
.external-arrow {
  flex: none; display: block; position: relative; width: 10px; height: 10px;
  border-top: 1px solid currentColor; border-right: 1px solid currentColor;
}
.external-arrow::after {
  content: ""; position: absolute; top: 4px; right: -1px; width: 12px;
  border-top: 1px solid currentColor; transform: rotate(-45deg);
}

/* Hero */
.hero {
  display: grid; grid-template-columns: minmax(0, 1.05fr) minmax(0, 1fr); align-items: center;
  column-gap: clamp(30px, 5.5vw, 82px); padding-block: clamp(48px, 6vw, 88px);
}
.eyebrow { display: flex; align-items: center; gap: 8px; color: var(--accent); font: 500 11px/1.5 var(--mono); letter-spacing: .11em; }
.status-dot, .outline-dot { flex: none; width: 6px; height: 6px; border-radius: 50%; background: var(--accent); }
.outline-dot { border: 1px solid var(--accent); background: transparent; }
h1 { margin: 22px 0 24px; font-size: clamp(38px, 4vw, 56px); font-weight: 520; line-height: 1.075; letter-spacing: -.05em; }
h2 { font-size: clamp(32px, 4.1vw, 52px); font-weight: 530; line-height: 1.12; letter-spacing: -.045em; }
h3 { font-size: 26px; font-weight: 560; line-height: 1.2; letter-spacing: -.03em; }
.hero-description { max-width: 470px; color: var(--muted); font-size: 18px; }
.hero-actions { display: flex; flex-wrap: wrap; gap: 16px 24px; align-items: center; margin-top: 30px; }
.hero-facts { margin-top: 20px; font: 11px/1.5 var(--mono); color: var(--subtle); }
.hero-facts span { padding-inline: 4px; }

.hero-output { min-width: 0; }
.output-caption, .comparison figcaption {
  display: flex; align-items: center; justify-content: space-between; gap: 8px;
  margin-bottom: 10px; color: var(--muted); font: 11px/1.5 var(--mono);
}
.example-label { border: 1px solid var(--label-border); border-radius: 4px; padding: 3px 6px; color: var(--subtle); font: 10px/1.3 var(--mono); white-space: nowrap; }
.hero-output .terminal { box-shadow: 0 23px 60px #0003; }
.hero-output pre { font-size: 12.5px; line-height: 1.75; }

/* Stack band */
.stack-band { border-block: 1px solid var(--border); }
.stack-row {
  display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 12px 24px;
  padding-block: 24px; color: var(--muted); font-size: 14px;
}
.stack-row b { font-weight: 570; color: var(--text); }
.stack-row span { color: var(--subtle); font: 11px var(--mono); }

/* Product tabs */
.section-heading { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, .65fr); align-items: end; gap: 20px 38px; margin-bottom: 32px; }
.section-heading p { max-width: 360px; color: var(--muted); }
.product-tabs { display: flex; flex-wrap: wrap; gap: 4px 23px; border-bottom: 1px solid var(--border); }
.product-tabs button {
  background: none; border: 0; border-bottom: 2px solid transparent; padding: 13px 2px; margin-bottom: -1px;
  color: var(--muted); font-size: 14px; text-align: left; line-height: 1.35; cursor: pointer;
}
.product-tabs button:hover { color: var(--text); }
.product-tabs button[aria-selected="true"] { border-bottom-color: var(--accent); color: var(--accent); }
.product-panel {
  display: grid; grid-template-columns: minmax(0, .65fr) minmax(0, 1.35fr);
  gap: clamp(25px, 5vw, 65px); align-items: center; margin-top: 32px;
}
.product-panel-copy p { margin-top: 16px; color: var(--muted); }
.panel-links { display: flex; flex-wrap: wrap; gap: 12px 24px; margin-top: 24px; }
.screenshot-frame { overflow: hidden; border: 1px solid #404c39; border-radius: 7px; background: #15202e; }
.screenshot-frame figcaption {
  display: flex; justify-content: space-between; gap: 12px; background: #1e271b; color: #c4d3ba;
  padding: 11px 14px; font: 11px var(--mono);
}
.screenshot-frame figcaption span { color: var(--subtle); }
.screenshot-frame img { width: 100%; }
.screenshot-scroll { max-height: 440px; overflow-y: auto; scrollbar-color: #627656 #141d13; }

/* Split sections */
.release-band, .next-band, .planned-band { border-top: 1px solid var(--border); background: var(--band); }
.split { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: clamp(32px, 7vw, 92px); align-items: center; }
.split h2 { margin-top: 14px; }
.split > div:not(.terminal) > p:not(.eyebrow) { margin-top: 18px; color: var(--muted); max-width: 465px; }
.split > div > .text-link { margin-top: 22px; }

/* Release demo */
.comparison { --candidate: #edbd7d; min-width: 0; font-size: 13px; }
.comparison.fixed { --candidate: #d6f89a; }
.comparison-window {
  overflow: hidden; border: 1px solid #414d3c; border-radius: 8px;
  background: linear-gradient(155deg, #1d241b, #151a14 75%); box-shadow: 0 23px 60px #0003;
}
.window-bar, .release-picker, .demo-controls { display: flex; align-items: center; gap: 15px; padding: 12px 20px; color: var(--muted); font: 11px/1.45 var(--mono); }
.window-bar { border-bottom: 1px solid var(--border); }
.window-dots { flex: none; display: flex; gap: 4px; }
.window-dots i { width: 5px; height: 5px; border-radius: 50%; background: #53604c; }
.environment { margin-left: auto; color: #b5c4a9; background: #283624; padding: 2px 6px; border-radius: 3px; }
.release-picker { gap: 21px; padding-block: 15px; }
.tiny-label { display: block; margin-bottom: 3px; font-size: 9px; letter-spacing: .08em; }
.release-picker b { color: var(--text); font-weight: 500; font-size: 14px; }
.release-arrow { margin-top: 12px; }
.comparison-result {
  display: flex; align-items: center; gap: 10px; margin-inline: 20px; padding: 10px 12px;
  border: 1px solid #573f29; border-radius: 5px; background: #33291f; color: #dac4aa; font-size: 12px;
}
.comparison-result strong { color: var(--candidate); font-weight: 650; }
.fixed .comparison-result { border-color: #4c6140; background: #283524; color: #c6d4bf; }
.signal-icon {
  flex: none; display: grid; place-items: center; width: 21px; height: 21px;
  border: 1px solid currentColor; border-radius: 50%; color: var(--candidate); font: 12px var(--mono);
}
.metric-grid { display: grid; grid-template-columns: 1fr 1fr; margin: 17px 20px 16px; color: var(--muted); }
.metric-grid > div + div { border-left: 1px solid var(--border); padding-left: 18px; }
.metric { display: flex; align-items: baseline; gap: 4px; font: 15px/1.45 var(--mono); }
.metric strong { font-size: 34px; font-weight: 500; letter-spacing: -.055em; color: var(--text); }
.metric-grid small { font: 11px/1.45 var(--mono); }
.metric-grid small span { color: var(--candidate); }
.chart-heading { display: flex; justify-content: space-between; gap: 8px; margin: 0 20px 11px; color: var(--muted); font-size: 11px; }
.chart-legend { display: flex; align-items: center; gap: 5px; }
.chart-legend i { width: 7px; height: 7px; background: #7e946e; border-radius: 1px; }
.chart-legend i + i { margin-left: 4px; background: var(--candidate); }
.chart { position: relative; height: 118px; margin: 0 20px 15px 62px; font: 10px/1.4 var(--mono); color: var(--subtle); }
.chart-scale { position: absolute; inset: 0 0 20px; display: flex; flex-direction: column; justify-content: space-between; }
.chart-scale span { border-top: 1px dashed #3c4738; text-indent: -42px; line-height: 0; }
.chart-bars { position: absolute; inset: 0 0 0 5px; display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); }
.bar-group { display: grid; grid-template-rows: minmax(0, 1fr) 20px; text-align: center; }
.bar-group > span { padding-top: 3px; }
.bar-pair { display: flex; align-items: end; justify-content: center; gap: 4px; }
.bar-pair i, .bar-pair b {
  display: block; width: min(25%, 22px); height: var(--height); border-radius: 2px 2px 0 0;
  background: #7e946e; transition: height .35s ease, background .2s ease;
}
.bar-pair b { background: var(--candidate); }
.demo-controls { background: #121811; border-top: 1px solid var(--border); justify-content: end; gap: 5px; }
.demo-controls > span { margin-right: auto; }
.demo-controls button, .code-header button, .dialog-header button {
  border: 1px solid #43513e; border-radius: 4px; padding: 6px 9px; background: none;
  color: var(--muted); font: 11px/1.4 var(--mono); cursor: pointer;
}
.demo-controls button:hover, .code-header button:hover, .dialog-header button:hover { border-color: var(--subtle); }
.demo-controls button[aria-pressed="true"] { color: var(--accent); background: #26361d; border-color: #526c42; }

/* Next section */
.next-list { margin: 22px 0 0; padding: 0; list-style: none; display: grid; gap: 10px; max-width: 465px; }
.next-list li { padding-left: 14px; border-left: 2px solid var(--border); color: var(--muted); font-size: 14px; }
.next-list b { display: block; color: var(--text); font-weight: 600; }
.next-cta { color: var(--accent); }
.code-dim { color: #8d9a86; }
.code-fail { color: #edbd7d; font-weight: 600; }
.code-link { color: #a9d27a; }
.qg-heading { padding-bottom: 0; margin-bottom: 36px; }
.qg-steps { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 24px; }
.qg-step h3, .qg-evidence h3, .qg-rules h3 { font-size: 17px; font-weight: 650; line-height: 1.35; color: var(--text); }
.qg-step h3 span { display: inline-grid; place-items: center; width: 24px; height: 24px; margin-right: 8px; border-radius: 50%; background: var(--accent); color: var(--background); font-size: 13px; }
.qg-step > p { margin: 8px 0 14px; color: var(--muted); font-size: 14px; min-height: 4.8em; }
.qg-step .terminal pre { font-size: 12.5px; }
.qg-evidence h3, .qg-rules h3 { font-size: 22px; }
.qg-rules { align-items: start; padding-bottom: 0; }
.qg-rules .next-list { max-width: none; }
.qg-rules-note { margin-top: 16px; color: var(--muted); font-size: 14px; }
.qg-evidence + .qg-evidence { padding-top: 0; }
.planned-band { background: var(--background); }
.planned-band .split { align-items: start; }
.req-split .screenshot-frame { max-height: 520px; }
.req-split > div { padding-top: 24px; }
.qg-evidence { padding-block: clamp(48px, 6vw, 80px); align-items: start; }
.qg-evidence > div { padding-top: 40px; }
.qg-evidence .screenshot-frame { max-height: 560px; }
.qg-evidence p a { color: var(--accent); text-decoration: underline; text-underline-offset: 3px; }

/* Terminal cards */
.terminal { min-width: 0; background: #131a12; border: 1px solid #44513d; border-radius: 8px; overflow: hidden; }
.code-header {
  display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 13px 17px;
  border-bottom: 1px solid var(--border); font: 12px/1.5 var(--mono); color: #bfccba;
}
pre { margin: 0; padding: 20px 17px; font: 13px/1.9 var(--mono); white-space: pre-wrap; overflow-wrap: anywhere; color: var(--text); }
code { font: inherit; }
.pre-scroll { white-space: pre; overflow-x: auto; overflow-wrap: normal; }
.code-note { padding: 12px 17px; background: #1b2518; border-top: 1px solid var(--border); color: #b2c0ab; font-size: 12px; }
.code-note a { color: var(--accent); text-decoration: underline; text-underline-offset: 3px; }

/* Quick start */
.start-band { background: var(--start-bg); color: var(--start-text); }
.start-band .split > div > p:not(.eyebrow) { color: var(--start-muted); }
.start-band .text-link:hover { color: #17230e; }
.start-band .start-right > .text-link { margin-top: 14px; }
.start-band a:focus-visible, .start-band button:focus-visible { outline-color: #315217; }
.start-links { display: flex; flex-wrap: wrap; gap: 18px; margin-top: 22px; }
.requirements { font-size: 13px; margin-top: 14px; }

/* FAQ */
.faq { align-items: start; }
.faq h2 { margin-top: 0; }
.faq details { border-bottom: 1px solid var(--border); padding: 15px 0; }
.faq details:first-child { padding-top: 0; }
.faq summary { cursor: pointer; font-weight: 530; }
.faq details p { margin-top: 12px; color: var(--muted); font-size: 14px; }
.faq details a { color: var(--accent); text-decoration: underline; text-underline-offset: 3px; }
.faq code { font-size: 13px; }

/* Screenshot dialog */
.shot-dialog { width: min(1150px, 95vw); max-height: 91vh; border: 1px solid #55664a; border-radius: 8px; padding: 0; background: #151d13; }
.shot-dialog::backdrop { background: #030603df; }
.dialog-header {
  position: sticky; top: 0; background: #151d13; display: flex; align-items: center;
  justify-content: space-between; gap: 15px; padding: 12px 16px; font-size: 14px;
}
.shot-dialog img { width: 100%; }

@media (max-width: 1050px) {
  .container { width: calc(100% - 60px); }
  .hero-description { font-size: 16px; }
  .window-bar, .release-picker, .demo-controls { padding-inline: 14px; }
  .comparison-result, .metric-grid, .chart-heading { margin-inline: 14px; }
  .chart { margin-right: 14px; }
  .qg-steps { grid-template-columns: minmax(0, 1fr); }
  .qg-step > p { min-height: 0; }
}
@media (max-width: 790px) {
  .container { width: calc(100% - 32px); }
  .hero, .section-heading, .product-panel, .split { grid-template-columns: minmax(0, 1fr); }
  .hero { row-gap: 36px; padding-block: 40px 55px; }
  h1 { font-size: clamp(36px, 8.5vw, 56px); }
  .hero-output, .comparison { width: 100%; max-width: 640px; }
  .qg-evidence > figure { order: 2; }
  .qg-evidence > div { padding-top: 0; }
  .stack-row { justify-content: start; }
  .section-heading p { max-width: 470px; }
  .product-panel { align-items: start; }
  .split h2 { margin-top: 10px; }
}
@media (max-width: 430px) {
  .eyebrow { font-size: 9px; }
  h1 { font-size: 34px; margin-block: 20px; }
  .hero-actions { gap: 14px; }
  .button-primary { padding-inline: 14px; font-size: 13px; }
  .hero-facts { font-size: 10px; }
  .window-bar { gap: 8px; font-size: 10px; }
  .metric-grid > div + div { padding-left: 10px; }
  .metric-grid small { font-size: 10px; }
  .demo-controls { gap: 4px; }
  .demo-controls button { font-size: 10px; padding-inline: 7px; }
  .product-tabs { gap: 0 16px; }
  .product-tabs button { font-size: 12px; }
  pre { font-size: 11px; }
}
@media (prefers-reduced-motion: reduce) {
  * { transition: none !important; }
}
</style>
