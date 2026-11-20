<script setup lang="ts">
import { computed, ref } from 'vue'
import { withBase } from 'vitepress'
const runs = [
  {name:'Baseline', calls:2, ms:420, dir:0, summary:'Session startup meets all three checks.', note:'One permission lookup and one insert per startup.'},
  {name:'Pull request', calls:21, ms:910, dir:200, summary:'Starting a session made 21 database calls. The limit was 3.', note:'Twenty permission lookups and one insert per startup.'},
  {name:'After the fix', calls:2, ms:560, dir:0, summary:'All three checks pass under the same rules.', note:'One batched lookup and one insert. Startup is within budget, though still slower than the baseline.'}
]
const current = ref(1)
const run = computed(() => runs[current.value])
const repo='https://github.com/agerimex/troubleshooting'
const doc=(s:string)=>withBase(s)
const ruleFile=repo+'/blob/main/fixtures/regression/qghistory.go'
const prCheck='https://github.com/agerimex/troubleshooting/pull/30#issuecomment-6992712426'
const walkthrough='/guide/investigate-a-slow-request.html'
</script>

<template>
<div class="landing">
  <section class="hero wrap" aria-labelledby="title">
    <div>
      <p class="kicker"><span class="dot"></span>Traces, logs and runtime checks</p>
      <h1 id="title">Find out why a request <em>failed or slowed down.</em></h1>
      <p class="lead">Follow a request across services, database calls, and logs. Compare releases to see what changed. Once you understand a regression, add a runtime check to catch it in future pull requests.</p>
      <div class="actions"><a class="primary" :href="doc(walkthrough)">Explore a slow request <span aria-hidden="true">→</span></a><a class="simple" :href="prCheck">See a real PR check ↗</a></div>
      <p class="facts">Self-hosted · OpenTelemetry · Go or any OTLP SDK</p>
    </div>
    <figure class="hero-shot"><a :href="doc(walkthrough)"><img :src="doc('/screenshots/investigate-hero.png')" alt="Timeline of one slow start_session: check_permissions with permission_lookup database spans repeated one after another." width="822" height="560"></a><figcaption>Product screenshot · fixture data. One slow <code>start_session</code> of the new version: twenty permission lookups, one insert, and a call to the directory service.</figcaption></figure>
  </section>

  <section id="investigate" class="band" aria-labelledby="investigate-title"><div class="wrap">
    <p class="kicker">A worked investigation</p>
    <div class="intro"><h2 id="investigate-title">A session start got slower. Here is how you find out why.</h2><p>Fixture data from this repository’s regression fixture: two versions of a session-start operation, 200 starts each. No installation needed to follow along.</p></div>
    <ol class="steps">
      <li><a :href="doc('/screenshots/investigate-compare.png')" target="_blank" rel="noopener" aria-label="Open the full screenshot"><img :src="doc('/screenshots/investigate-compare.png')" alt="Release comparison of start_session: p95 420 ms in the baseline version, 910 ms in the candidate, marked as a regression signal." loading="lazy"></a><h3>1. Compare the releases.</h3><p><code>start_session</code> p95 rose from 420 ms to 910 ms between the two versions. Releases marks it as a regression signal.</p></li>
      <li><a :href="doc('/screenshots/investigate-examples.png')" target="_blank" rel="noopener" aria-label="Open the full screenshot"><img :src="doc('/screenshots/investigate-examples.png')" alt="The slowest candidate calls of start_session, each with a link to open its report." loading="lazy"></a><h3>2. Open a slow request.</h3><p>The slow examples are recorded executions at or above the p95. Each one opens as a timeline with its logs.</p></li>
      <li><a :href="doc('/screenshots/investigate-sequence-repeats.png')" target="_blank" rel="noopener" aria-label="Open the full screenshot"><img :src="doc('/screenshots/investigate-sequence-repeats.png')" alt="Sequence view: permission_lookup repeated 1 of 20, 2 of 20 and so on, from the API to an uninstrumented PostgreSQL." loading="lazy"></a><h3>3. See the extra work.</h3><p>The new version reads permissions twenty times instead of once, and calls the directory service. The baseline does neither.</p></li>
    </ol>
    <a class="simple" :href="doc(walkthrough)">Read the whole walkthrough →</a>
  </div></section>

  <section id="gate" class="wrap gate" aria-labelledby="gate-title">
    <p class="kicker">Then keep it from coming back</p>
    <div class="intro"><h2 id="gate-title">Turn what you found into a check on every pull request.</h2><p>Write the limit down next to the code. A limit of three database calls per start flags the same behaviour in CI: the slow version makes 21. The check runs on your test run’s traces and links each failure to the execution behind it.</p></div>
    <div class="gate-grid">
      <div class="ledger">
        <div class="ledger-row"><span class="kind">Query budget</span><div><h3>Each start makes at most three database calls.</h3><p>Catches a query per item before it ships, even when the tests are fast enough.</p></div><code>max ≤ 3 calls</code></div>
        <div class="ledger-row"><span class="kind">Architecture</span><div><h3>Starting a session must never call Directory.</h3><p>Looks for that dependency anywhere in the recorded path.</p></div><code>0 calls</code></div>
        <div class="ledger-row"><span class="kind">Performance</span><div><h3>95% of starts finish within 800 ms.</h3><p>The 95th percentile across the run’s 200 starts.</p></div><code>p95 ≤ 800 ms</code></div>
        <p class="note">The real example is a different one: <a :href="prCheck">this repository’s own API checked on pull request #30</a>, with its own limits and measurements.</p>
      </div>
      <div class="console" aria-label="Interactive reference test runs">
        <div class="console-head"><span><i></i> Run report · fixture data</span><code>start_session</code></div>
        <div class="console-body">
          <div class="switch" role="group" aria-label="Select fixture run">
            <button v-for="(r,i) in runs" :key="r.name" :aria-pressed="i===current" @click="current=i">{{r.name}}</button>
          </div>
          <div aria-live="polite" aria-atomic="true">
            <div class="metric"><span class="metric-number">{{run.calls}}</span><span>database calls<br>per startup <small>limit ≤ 3</small></span></div>
            <div class="ticks" aria-hidden="true"><i v-for="n in 21" :key="n" :class="{on:n<=run.calls,over:n>3 && n<=run.calls}"></i></div>
            <div class="verdict" :class="{fail:current===1}"><b>{{current===1?'× Failed':'✓ Passed'}}</b><span>{{run.summary}}</span></div>
            <p class="observed">{{run.note}}</p>
            <dl><div><dt>Startup p95 <small>· limit 800 ms</small></dt><dd>{{run.ms}} ms <span class="row-verdict" :class="{over:run.ms>800}">{{run.ms>800?'× over the limit':'✓'}}</span></dd></div><div><dt>Startups calling Directory <small>· limit 0</small></dt><dd>{{run.dir}} / 200 <span class="row-verdict" :class="{over:run.dir>0}">{{run.dir>0?'× over the limit':'✓'}}</span></dd></div></dl>
          </div>
        </div>
        <div class="console-foot"><span>Regression fixture · 200 starts per run</span><a :href="repo+'/blob/main/fixtures/regression/qg.go'">View data ↗</a></div>
      </div>
    </div>
  </section>

  <section class="band"><div class="wrap setup" id="start">
    <div><p class="kicker">Use it in your pipeline</p><h2>Start with one operation you care about.</h2><p class="lead small">Bring an instrumented service and an existing test. Run the stack in a CI job or connect it to your own instance.</p><pre v-pre class="local-cmd action-cmd" aria-label="GitHub Actions step for the quality gate"><code>- uses: agerimex/troubleshooting/quality-check@quality-check/v0.1.0
  with:
    url: http://localhost:8095
    run: ci-${{ github.run_id }}-${{ github.run_attempt }}
    rules: quality.yaml
    rules-from: base   # a pull request can't loosen its limits</code></pre>
    <a class="primary" :href="doc('/guide/quality.html')">Set up the quality gate →</a></div>
    <ol><li><h3>Choose a limit.</h3><p>Add the operation and its checks to <code>quality.yaml</code>. Tag the test’s OpenTelemetry spans with a unique run ID.</p></li><li><h3>Check the pull request.</h3><p>The GitHub Action or CLI evaluates the run. Review the result in the PR and open the HTML or JSON report.</p></li><li><h3>Check the release candidate.</h3><p>Run the same gate in your release test job. Record evaluations to follow requirements over time. To investigate changes in deployed versions, use <a :href="doc('/guide/releases.html')">release comparison</a>.</p></li></ol>
  </div></section>
  <section class="wrap more"><div><h2>Before you try it</h2><p>Start with the <a :href="doc('/articles/catch-n-plus-one-in-a-pull-request.html')">walkthrough of a failing CI run</a> or explore the <a :href="repo+'/tree/main/examples/quality-gate'">example application</a>.</p>
  <h3 id="local">Run it locally</h3><pre class="local-cmd" aria-label="Commands to start Troubleshooting locally"><code>git clone {{repo}}.git
cd troubleshooting
make all
make up</code></pre><p>Requires Git, Make and Docker with Compose; the first start can take a few minutes. Then open <code>localhost:8095</code> and <a :href="doc('/guide/getting-started.html')">connect a Go or OpenTelemetry application</a>.</p></div>
  <div><details id="telemetry"><summary><h3>Where does my telemetry go?</h3></summary><p>The stack runs on your infrastructure: you operate the receiver and its ClickHouse storage. Review receiver authentication and retention before using it outside a development environment.</p></details>
  <details id="instrumentation"><summary><h3>What instrumentation do I need?</h3></summary><p>OpenTelemetry spans from any SDK or Collector over OTLP/gRPC, or the small Go library <code>log-sender</code> for spans plus logs that carry their trace and span. <a :href="doc('/guide/getting-started.html')">Getting started</a> shows both.</p></details><details id="what-a-pass-means"><summary><h3>What does a pass establish?</h3></summary><p>That the executions of this run met your rules. A pass is not proof: what the tests didn't run can't be judged, and a run with too little data is "not enough data", never a pass. <a :href="doc('/guide/quality.html#how-to-read-a-result')">How to read a result</a>.</p></details>
  <details id="after-ci"><summary><h3>What survives after the CI job?</h3></summary><p>Save the HTML or JSON artifact to retain the result and bounded evidence snapshots. Record evaluations on a persistent instance to build a requirement history.</p></details>
  <details id="outside-ci"><summary><h3>Can I investigate outside CI?</h3></summary><p>Explore <a :href="doc('/guide/log-analysis.html')">spans and logs</a>, <a :href="doc('/guide/timeline-sequence.html')">timelines and service calls</a>, or <a :href="doc('/guide/problems.html')">recurring problems</a>. The sequence shows the services observed in a particular execution.</p></details></div></section>
</div></template>
<style scoped>
.landing{--ink:#18291d;--muted:#4a5b4e;--paper:#fafbf6;--band:#edf2e8;--line:#c8d4c5;--green:#356528;background:var(--paper);color:var(--ink);font-family:var(--vp-font-family-base);line-height:1.5;overflow-wrap:break-word}
.dark .landing{--ink:#eef5e6;--muted:#c2cfbf;--paper:#121b16;--band:#19271e;--line:#425144;--green:#abd18e}
.wrap{width:min(1200px,calc(100% - 80px));margin-inline:auto}
.hero{display:grid;grid-template-columns:1.04fr 1fr;gap:clamp(28px,5vw,80px);padding-block:clamp(54px,6.5vw,96px);align-items:center}
.kicker,.facts,.kind,.console,.ledger-row>code,figcaption{font-family:var(--vp-font-family-mono),monospace}
.kicker{font-size:12px;font-weight:650;color:var(--green);letter-spacing:.08em;text-transform:uppercase;display:flex;align-items:center;gap:9px;margin:0 0 17px}.dot,.console-head i{display:inline-block;width:8px;height:8px;background:var(--green);border-radius:50%}
h1,h2,h3,p,figure,dl,dd{margin:0}h1,h2{letter-spacing:-.055em;line-height:1.02;text-wrap:balance}h1{font-size:clamp(44px,4.9vw,76px);font-weight:640}h1 em{font-style:normal;color:var(--green)}
h2{font-size:clamp(32px,3.6vw,55px);font-weight:630}h3{font-weight:640;font-size:20px;letter-spacing:-.025em;line-height:1.28}
.lead{color:var(--muted);font-size:clamp(17px,1.4vw,21px);line-height:1.55;max-width:53ch;margin:22px 0}
.actions{display:flex;align-items:center;gap:24px;flex-wrap:wrap;margin-top:28px}
.primary,.simple{font-weight:650;font-size:15px}.primary{display:inline-flex;align-items:center;gap:14px;background:var(--ink);color:var(--paper);border-radius:6px;padding:13px 18px}.simple{color:var(--green);text-underline-offset:4px}.primary:hover{opacity:.87}.simple:hover{text-decoration:underline}.facts{color:var(--muted);font-size:12px;margin-top:19px}
.console{background:#17251c;color:#f3f7ee;box-shadow:0 12px 34px #17251c18;border:1px solid #2b4231;border-radius:12px;overflow:hidden;min-width:0}
.console-head,.console-foot{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;background:#1d3023;padding:13px 21px;color:#c8d7c9;font-size:12px}.console-head i{background:#b1dc8e;margin-right:6px}.console-head code{font-size:12px}.console-body{padding:20px 24px}
.switch{display:flex;gap:4px;flex-wrap:wrap}.switch button,.selectors button{cursor:pointer}.switch button{font-size:12px;color:#d4ded2;border:1px solid #526357;padding:7px 10px;border-radius:4px}.switch button[aria-pressed=true]{background:#d9ebcd;color:#213424;border-color:#d9ebcd}
.metric{display:flex;align-items:center;gap:16px;margin:18px 0 8px}.metric-number{font-family:var(--vp-font-family-base);font-weight:580;font-size:clamp(65px,6vw,91px);line-height:1;letter-spacing:-.08em}.metric>span+span{font-size:15px;line-height:1.3}.metric small,dt small{display:block;color:#afc4b2;font-size:11px;margin-top:3px}.ticks{display:grid;grid-template-columns:repeat(21,1fr);gap:3px;margin:16px 0 22px}.ticks i{height:16px;background:#354639}.ticks .on{background:#c3e49b}.ticks .over{background:#edaa76}
.verdict{display:flex;align-items:center;gap:10px;font-size:12px}.verdict b{background:#d2e7be;color:#253824;white-space:nowrap;border-radius:3px;padding:2px 6px}.verdict.fail b{background:#f5c59f;color:#41250f}.observed{color:#c1d0c1;font-size:12px;margin:8px 0 17px;min-height:3em}
dl>div{display:flex;align-items:center;justify-content:space-between;border-top:1px solid #394c3e;padding:8px 0;gap:10px;font-size:12px}dd{white-space:nowrap;color:#fff}.console-foot a{text-decoration:underline;text-underline-offset:3px}
.band{background:var(--band);border-block:1px solid var(--line)}.band>.wrap,.evidence,.more{padding-block:clamp(48px,5.8vw,85px)}
.intro,.setup,.more{display:grid;grid-template-columns:1fr 1fr;gap:8%}.intro>p,.evidence-grid p,.setup li p,.more p,.ledger-row p{color:var(--muted);font-size:16px;line-height:1.6}
.ledger{margin:32px 0 19px}.ledger-row{display:grid;grid-template-columns:135px 1fr auto;align-items:center;gap:24px;padding:18px 0;border-top:1px solid var(--line)}.ledger-row:last-child{border-bottom:1px solid var(--line)}.kind,.ledger-row>code{font-size:13px;color:var(--green)}.ledger-row p{margin-top:3px}
.evidence h2{max-width:750px}.evidence-grid{display:grid;grid-template-columns:.72fr 1.55fr;gap:5%;align-items:start;margin-top:32px}.evidence-grid figure{min-width:0;border:1px solid var(--line);border-radius:8px;overflow:hidden;background:var(--band)}.evidence-grid img{width:100%;aspect-ratio:1.7;object-fit:cover;object-position:top;display:block}.evidence-grid figcaption{font-size:12px;color:var(--muted);padding:9px 14px}.selectors{display:grid;margin-bottom:25px}.selectors button{display:flex;justify-content:space-between;padding:10px;border-bottom:1px solid var(--line);text-align:left}.selectors button[aria-pressed=true]{background:var(--band);color:var(--green);font-weight:650}.evidence-grid p{margin:8px 0 13px}.small{font-size:17px}
.setup ol{list-style:none;counter-reset:step;padding:0;margin:0}.setup li{counter-increment:step;padding:0 0 18px 42px;position:relative}.setup li:before{content:counter(step,decimal-leading-zero);position:absolute;left:0;color:var(--green);font:14px var(--vp-font-family-mono)}.setup li p{margin-top:6px}.setup li:last-child{padding-bottom:0}.setup p a,.more p a{text-decoration:underline;text-underline-offset:3px;color:var(--green)}
.more>div{min-width:0}.more h3{margin-top:28px}.local-cmd{background:#17251c;color:#f3f7ee;border-radius:8px;padding:14px 18px;margin:12px 0 0;overflow-x:auto;font:13px/1.6 var(--vp-font-family-mono),monospace}.local-cmd code{background:none;padding:0;color:inherit}.more p code{font-size:14px}
.more h2{font-size:34px}.more p{margin:12px 0}.more details{padding:12px 0;border-top:1px solid var(--line)}summary{cursor:pointer;font-weight:600}summary h3{display:inline;font-size:inherit;font-weight:600;margin:0;letter-spacing:normal}.more summary h3{margin-top:0}.row-verdict{margin-left:8px;color:#c3e49b}.row-verdict.over{color:#edaa76}.action-cmd{margin:16px 0 20px}.setup>div,.setup>ol{min-width:0}.landing :focus-visible{outline:3px solid #78a6df;outline-offset:3px}#evidence,#start{scroll-margin-top:85px}
.hero-shot{margin:0;min-width:0;border:1px solid var(--line);border-radius:10px;overflow:hidden;background:#17251c}.hero-shot img{display:block;width:100%;height:auto;max-height:520px;object-fit:cover;object-position:top}.hero-shot figcaption{font:12px/1.5 var(--vp-font-family-mono),monospace;color:#c1d0c1;padding:10px 14px}.hero-shot code{color:#f3f7ee}
.steps{list-style:none;padding:0;margin:32px 0 22px;display:grid;grid-template-columns:repeat(3,1fr);gap:28px}.steps li{min-width:0}.steps img{display:block;width:100%;aspect-ratio:1.45;object-fit:cover;object-position:top left;border:1px solid var(--line);border-radius:8px;background:#17251c}.steps h3{margin-top:14px}.steps p{color:var(--muted);font-size:15px;line-height:1.6;margin:6px 0 0}
.gate{padding-block:clamp(48px,5.8vw,85px)}.gate-grid{display:grid;grid-template-columns:1.1fr 1fr;gap:5%;align-items:start;margin-top:32px}.gate-grid>*{min-width:0}.gate .ledger{margin:0}.note{color:var(--muted);font-size:14px;margin-top:16px}.note a{color:var(--green);text-decoration:underline;text-underline-offset:3px}
@media(max-width:850px){.wrap{width:calc(100% - 40px)}.hero{grid-template-columns:1fr 1fr;gap:20px}.steps{grid-template-columns:1fr 1fr}.console-body{padding:16px}.console-head,.console-foot{padding:11px 16px}.evidence-grid{grid-template-columns:.85fr 1.25fr}.ledger-row{grid-template-columns:100px 1fr auto}}
@media(max-width:690px){.steps,.gate-grid{grid-template-columns:1fr}.wrap{width:calc(100% - 36px)}.hero,.intro,.setup,.more,.evidence-grid{grid-template-columns:1fr;gap:24px}.hero{padding-block:40px}.actions{gap:16px}.ledger-row{grid-template-columns:1fr auto;gap:5px 12px}.kind{grid-column:1/-1}.ledger-row>code{font-size:11px}.console-body{padding:16px}.more{gap:12px}.selectors{margin-bottom:16px}.evidence-grid{margin-top:22px}.facts{font-size:11px}}
</style>
