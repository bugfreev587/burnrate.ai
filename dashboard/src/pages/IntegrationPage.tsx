import { useState, useEffect, useRef } from 'react'
import Navbar from '../components/Navbar'
import './IntegrationPage.css'

/* ── section definitions ──────────────────────────────────────────────────── */

interface Section {
  id: string
  title: string
  icon: string
}

const SECTIONS: Section[] = [
  { id: 'overview',       title: 'Overview',                                    icon: 'info' },
  { id: 'scenario-1',     title: 'Anthropic \u2014 Subscription',              icon: 'anthropic' },
  { id: 'scenario-2',     title: 'Anthropic \u2014 API Usage',                 icon: 'anthropic' },
  { id: 'scenario-3',     title: 'Anthropic \u2014 BYOK',                      icon: 'anthropic' },
  { id: 'scenario-4',     title: 'OpenAI \u2014 Subscription (Codex)',         icon: 'openai' },
  { id: 'scenario-5',     title: 'OpenAI \u2014 BYOK',                         icon: 'openai' },
  { id: 'endpoints',      title: 'API Endpoints',                               icon: 'api' },
  { id: 'budget',         title: 'Budget Headers',                              icon: 'budget' },
  { id: 'notifications',  title: 'Notification Setup',                          icon: 'notification' },
  { id: 'troubleshoot',   title: 'Troubleshooting',                             icon: 'help' },
  { id: 'faq',            title: 'FAQ',                                          icon: 'faq' },
]

/* ── icon map ─────────────────────────────────────────────────────────────── */

function SectionIcon({ type }: { type: string }) {
  switch (type) {
    case 'anthropic':
      return <span className="ig-icon ig-icon--anthropic">A</span>
    case 'openai':
      return <span className="ig-icon ig-icon--openai">O</span>
    case 'api':
      return <span className="ig-icon ig-icon--api">&lt;/&gt;</span>
    case 'budget':
      return <span className="ig-icon ig-icon--budget">$</span>
    case 'notification':
      return <span className="ig-icon ig-icon--notification">&#x1f514;</span>
    case 'help':
      return <span className="ig-icon ig-icon--help">?</span>
    case 'faq':
      return <span className="ig-icon ig-icon--faq">Q</span>
    default:
      return <span className="ig-icon ig-icon--info">i</span>
  }
}

/* ── reusable components ──────────────────────────────────────────────────── */

function CodeBlock({ children, lang }: { children: string; lang?: string }) {
  const [copied, setCopied] = useState(false)
  const copy = () => {
    navigator.clipboard.writeText(children)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }
  return (
    <div className="ig-code">
      {lang && <span className="ig-code-lang">{lang}</span>}
      <button className="ig-code-copy" onClick={copy}>{copied ? 'Copied!' : 'Copy'}</button>
      <pre><code>{children}</code></pre>
    </div>
  )
}

function KeyValue({ field, value }: { field: string; value: string }) {
  return (
    <div className="ig-kv">
      <span className="ig-kv-field">{field}</span>
      <span className="ig-kv-value">{value}</span>
    </div>
  )
}

function StepNumber({ n }: { n: number }) {
  return <span className="ig-step-num">{n}</span>
}

function FaqItem({ question, children }: { question: string; children: React.ReactNode }) {
  const [open, setOpen] = useState(false)
  return (
    <div className={`ig-faq-item ${open ? 'ig-faq-item--open' : ''}`}>
      <button className="ig-faq-q" onClick={() => setOpen(!open)}>
        <span>{question}</span>
        <span className="ig-faq-arrow">{open ? '\u25B2' : '\u25BC'}</span>
      </button>
      {open && <div className="ig-faq-a">{children}</div>}
    </div>
  )
}

function Callout({ type, children }: { type: 'info' | 'warn'; children: React.ReactNode }) {
  return (
    <div className={`ig-callout ig-callout--${type}`}>
      <span className="ig-callout-icon">{type === 'warn' ? '!' : 'i'}</span>
      <div>{children}</div>
    </div>
  )
}

/* ── flow diagram ─────────────────────────────────────────────────────────── */

function FlowDiagram({ steps }: { steps: string[] }) {
  return (
    <div className="ig-flow">
      {steps.map((step, i) => (
        <div key={i} className="ig-flow-item">
          <div className="ig-flow-box">{step}</div>
          {i < steps.length - 1 && <div className="ig-flow-arrow" />}
        </div>
      ))}
    </div>
  )
}

/* ── main page ────────────────────────────────────────────────────────────── */

export default function IntegrationPage() {
  const [activeSection, setActiveSection] = useState('overview')
  const sectionRefs = useRef<Record<string, HTMLElement | null>>({})

  /* scroll to hash on mount (e.g. /integration#notifications) */
  useEffect(() => {
    const hash = window.location.hash.replace('#', '')
    if (hash && sectionRefs.current[hash]) {
      setTimeout(() => {
        sectionRefs.current[hash]?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      }, 100)
    }
  }, [])

  /* scroll spy */
  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            setActiveSection(entry.target.id)
          }
        }
      },
      { rootMargin: '-80px 0px -60% 0px', threshold: 0 },
    )
    for (const s of SECTIONS) {
      const el = sectionRefs.current[s.id]
      if (el) observer.observe(el)
    }
    return () => observer.disconnect()
  }, [])

  const scrollTo = (id: string) => {
    sectionRefs.current[id]?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  const ref = (id: string) => (el: HTMLElement | null) => { sectionRefs.current[id] = el }

  return (
    <div className="page-container">
      <Navbar />
      <div className="ig-layout">
        {/* ── sidebar index ─────────────────────────────────────────────── */}
        <aside className="ig-sidebar">
          <div className="ig-sidebar-inner">
            <h3 className="ig-sidebar-title">Integration Guide</h3>
            <nav className="ig-toc">
              {SECTIONS.map((s) => (
                <button
                  key={s.id}
                  className={`ig-toc-item ${activeSection === s.id ? 'ig-toc-item--active' : ''}`}
                  onClick={() => scrollTo(s.id)}
                >
                  <SectionIcon type={s.icon} />
                  <span>{s.title}</span>
                </button>
              ))}
            </nav>
          </div>
        </aside>

        {/* ── content ───────────────────────────────────────────────────── */}
        <main className="ig-main">

          {/* Overview */}
          <section id="overview" ref={ref('overview')} className="ig-section">
            <h1 className="ig-h1">Integration Guide</h1>
            <p className="ig-lead">
              Connect your AI coding tools to TokenGate in minutes. Pick the scenario that matches your
              team's setup and follow the step-by-step instructions.
            </p>

            <div className="ig-matrix">
              <h3>Supported Scenarios</h3>
              <div className="ig-table-wrap">
                <table className="ig-table">
                  <thead>
                    <tr>
                      <th>#</th>
                      <th>Provider</th>
                      <th>Auth Method</th>
                      <th>Billing</th>
                      <th>Client Tool</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr onClick={() => scrollTo('scenario-1')} className="ig-table-link">
                      <td>1</td><td>Anthropic</td><td>Browser OAuth</td><td>Monthly Subscription</td><td>Claude Code</td>
                    </tr>
                    <tr onClick={() => scrollTo('scenario-2')} className="ig-table-link">
                      <td>2</td><td>Anthropic</td><td>Browser OAuth</td><td>API Usage</td><td>Claude Code</td>
                    </tr>
                    <tr onClick={() => scrollTo('scenario-3')} className="ig-table-link">
                      <td>3</td><td>Anthropic</td><td>BYOK</td><td>API Usage</td><td>curl / SDK</td>
                    </tr>
                    <tr onClick={() => scrollTo('scenario-4')} className="ig-table-link">
                      <td>4</td><td>OpenAI</td><td>Browser OAuth</td><td>Monthly Subscription</td><td>Codex CLI</td>
                    </tr>
                    <tr onClick={() => scrollTo('scenario-5')} className="ig-table-link">
                      <td>5</td><td>OpenAI</td><td>BYOK</td><td>API Usage</td><td>Codex CLI / curl / SDK</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </section>

          {/* ── Scenario 1 ─────────────────────────────────────────────── */}
          <section id="scenario-1" ref={ref('scenario-1')} className="ig-section">
            <h2 className="ig-h2">
              <SectionIcon type="anthropic" />
              Scenario 1: Anthropic &mdash; Browser Auth + Monthly Subscription
            </h2>
            <p className="ig-desc">
              Your developers use <strong>Claude Code</strong> with their own Anthropic subscriptions
              (Pro, Max). The gateway tracks usage for visibility but does not
              bill per token &mdash; costs are covered by each user's existing Anthropic plan.
            </p>

            <FlowDiagram steps={[
              'Developer runs claude',
              'Selects "Claude account with subscription"',
              'Browser opens for Anthropic login',
              'Requests routed via TokenGate',
              'Usage tracked in dashboard',
            ]} />

            <div className="ig-steps">
              <div className="ig-step">
                <h3><StepNumber n={1} /> Create a Gateway API Key</h3>
                <p>Go to the <strong>Management</strong> page and create a key with:</p>
                <div className="ig-kv-group">
                  <KeyValue field="Provider" value="anthropic" />
                  <KeyValue field="Auth Method" value="BROWSER_OAUTH" />
                  <KeyValue field="Billing Mode" value="MONTHLY_SUBSCRIPTION" />
                </div>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={2} /> Developer Setup (each machine)</h3>
                <p>Set the following environment variables:</p>
                <CodeBlock lang="bash">{`export ANTHROPIC_BASE_URL=https://gateway.tokengate.to
export ANTHROPIC_CUSTOM_HEADERS="X-TokenGate-Key:<tokengate-api-key>"`}</CodeBlock>
                <span className="form-hint"><a className="form-hint-link" style={{ cursor: 'pointer' }} onClick={() => scrollTo('faq')}>Don't know how to set environment variables?</a></span>
                <Callout type="warn">
                  Do <strong>NOT</strong> include <code>/v1</code> in <code>ANTHROPIC_BASE_URL</code>. The Anthropic SDK appends <code>/v1/messages</code> automatically.
                </Callout>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={3} /> Run Claude Code</h3>
                <p>Run <code>claude</code> in your terminal. When prompted to choose an authentication method, select:</p>
                <div className="ig-select-option">
                  1. Claude account with subscription &middot; Pro, Max
                </div>
                <p>A browser window will automatically open to complete the Anthropic login. Once authenticated, all requests are routed through the gateway and usage is recorded in your TokenGate dashboard.</p>
              </div>
            </div>
          </section>

          {/* ── Scenario 2 ─────────────────────────────────────────────── */}
          <section id="scenario-2" ref={ref('scenario-2')} className="ig-section">
            <h2 className="ig-h2">
              <SectionIcon type="anthropic" />
              Scenario 2: Anthropic &mdash; Browser Auth + API Usage Billed
            </h2>
            <p className="ig-desc">
              Your developers use <strong>Claude Code</strong> with their own Anthropic Console API keys.
              The gateway tracks usage and bills per token.
            </p>

            <FlowDiagram steps={[
              'Developer runs claude',
              'Selects "Anthropic Console account"',
              'Authenticates with API key',
              'Requests routed via TokenGate',
              'Per-token billing tracked',
            ]} />

            <div className="ig-steps">
              <div className="ig-step">
                <h3><StepNumber n={1} /> Create a Gateway API Key</h3>
                <p>Go to the <strong>Management</strong> page and create a key with:</p>
                <div className="ig-kv-group">
                  <KeyValue field="Provider" value="anthropic" />
                  <KeyValue field="Auth Method" value="BROWSER_OAUTH" />
                  <KeyValue field="Billing Mode" value="API_USAGE" />
                </div>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={2} /> Developer Setup (each machine)</h3>
                <p>Set the following environment variables:</p>
                <CodeBlock lang="bash">{`export ANTHROPIC_BASE_URL=https://gateway.tokengate.to
export ANTHROPIC_CUSTOM_HEADERS="X-TokenGate-Key:<tokengate-api-key>"`}</CodeBlock>
                <span className="form-hint"><a className="form-hint-link" style={{ cursor: 'pointer' }} onClick={() => scrollTo('faq')}>Don't know how to set environment variables?</a></span>
                <Callout type="warn">
                  Do <strong>NOT</strong> include <code>/v1</code> in <code>ANTHROPIC_BASE_URL</code>. The Anthropic SDK appends <code>/v1/messages</code> automatically.
                </Callout>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={3} /> Run Claude Code</h3>
                <p>Run <code>claude</code> in your terminal. When prompted to choose an authentication method, select:</p>
                <div className="ig-select-option">
                  2. Anthropic Console account &middot; API usage billing
                </div>
                <p>Claude Code will add its own Anthropic auth automatically. All requests are routed through the gateway with per-token billing.</p>
              </div>
            </div>
          </section>

          {/* ── Scenario 3 ─────────────────────────────────────────────── */}
          <section id="scenario-3" ref={ref('scenario-3')} className="ig-section">
            <h2 className="ig-h2">
              <SectionIcon type="anthropic" />
              Scenario 3: Anthropic &mdash; BYOK + API Usage Billed
            </h2>
            <p className="ig-desc">
              Your organization stores an Anthropic API key in the TokenGate vault. Developers never
              see the raw key &mdash; the gateway injects it automatically. Usage is billed per token
              with full budget enforcement.
            </p>

            <FlowDiagram steps={[
              'Admin stores Anthropic key in vault',
              'Developer sends request with TokenGate key',
              'Gateway injects provider key',
              'Request forwarded to Anthropic',
              'Per-token billing tracked',
            ]} />

            <div className="ig-steps">
              <div className="ig-step">
                <h3><StepNumber n={1} /> Add a Provider Key (admin, once)</h3>
                <ol className="ig-ol">
                  <li>Go to the <strong>Management</strong> page, find the <strong>Provider Keys</strong> section.</li>
                  <li>Click <strong>Add Provider Key</strong>, select <strong>Anthropic</strong>, and paste your <code>sk-ant-...</code> key.</li>
                  <li>Click <strong>Activate</strong> on the newly created key.</li>
                </ol>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={2} /> Create a Gateway API Key</h3>
                <p>On the same <strong>Management</strong> page, create a key with:</p>
                <div className="ig-kv-group">
                  <KeyValue field="Provider" value="anthropic" />
                  <KeyValue field="Auth Method" value="BYOK" />
                  <KeyValue field="Billing Mode" value="API_USAGE" />
                </div>
                <Callout type="info">
                  Make sure you've added and activated your Anthropic provider key in Step 1 before using this gateway key.
                </Callout>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={3} /> Use the API</h3>
                <p>
                  Include your TokenGate API key in the <code>X-TokenGate-Key</code> header.
                  The base URL is <code>https://gateway.tokengate.to</code> and API paths remain
                  the same as the standard Anthropic API (e.g. <code>/v1/messages</code>).
                </p>
                <p>No Anthropic API key is needed &mdash; the gateway injects it from the vault automatically.</p>
                <CodeBlock lang="bash">{`curl https://gateway.tokengate.to/v1/messages \\
    -H "X-TokenGate-Key: <tokengate-api-key>" \\
    -H "Content-Type: application/json" \\
    -d '{"model":"claude-sonnet-4-6","max_tokens":20,"messages":[{"role":"user","content":"Hello!"}]}'`}</CodeBlock>
              </div>
            </div>
          </section>

          {/* ── Scenario 4 ─────────────────────────────────────────────── */}
          <section id="scenario-4" ref={ref('scenario-4')} className="ig-section">
            <h2 className="ig-h2">
              <SectionIcon type="openai" />
              Scenario 4: OpenAI &mdash; Browser Auth + Monthly Subscription (Codex)
            </h2>
            <p className="ig-desc">
              Your developers use the <strong>OpenAI Codex CLI</strong> with their own ChatGPT/OpenAI
              subscriptions. The gateway tracks usage for visibility but does not bill per token.
            </p>

            <FlowDiagram steps={[
              'Developer runs codex',
              'Authenticates via browser or device code',
              'Requests routed via TokenGate',
              'Usage tracked in dashboard',
            ]} />

            <div className="ig-steps">
              <div className="ig-step">
                <h3><StepNumber n={1} /> Create a Gateway API Key</h3>
                <p>Go to the <strong>Management</strong> page and create a key with:</p>
                <div className="ig-kv-group">
                  <KeyValue field="Provider" value="openai" />
                  <KeyValue field="Auth Method" value="BROWSER_OAUTH" />
                  <KeyValue field="Billing Mode" value="MONTHLY_SUBSCRIPTION" />
                </div>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={2} /> Developer Setup (each machine)</h3>
                <p>Edit (or create) <code>~/.codex/config.toml</code> and paste the following at the top:</p>
                <CodeBlock lang="toml">{`model_provider = "tokengate"

[model_providers.tokengate]
name = "TokenGate Proxy"
base_url = "https://gateway.tokengate.to/v1"
requires_openai_auth = true
wire_api = "responses"
http_headers = {
  "X-Tokengate-Key" = "<tokengate-api-key>"
}`}</CodeBlock>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={3} /> Run Codex</h3>
                <p>Run <code>codex</code> in a code repo. On first launch, Codex will prompt you to authenticate with OpenAI. You will see two options:</p>
                <div className="ig-auth-options">
                  <div className="ig-auth-option">
                    <span className="ig-auth-option-num">1</span>
                    <div>
                      <strong>Browser auth</strong>
                      <p>Opens a browser window for OpenAI login. Recommended if you have a browser available.</p>
                    </div>
                  </div>
                  <div className="ig-auth-option">
                    <span className="ig-auth-option-num">2</span>
                    <div>
                      <strong>Device code auth</strong>
                      <p>Displays a code to enter at openai.com/device. Useful for headless or remote machines.</p>
                    </div>
                  </div>
                </div>
                <p>Pick whichever suits your environment. Both work identically with the gateway.</p>
              </div>
            </div>
          </section>

          {/* ── Scenario 5 ─────────────────────────────────────────────── */}
          <section id="scenario-5" ref={ref('scenario-5')} className="ig-section">
            <h2 className="ig-h2">
              <SectionIcon type="openai" />
              Scenario 5: OpenAI &mdash; BYOK + API Usage Billed
            </h2>
            <p className="ig-desc">
              Your organization stores an OpenAI API key in the TokenGate vault. The gateway injects
              it automatically. Usage is billed per token.
            </p>

            <FlowDiagram steps={[
              'Admin stores OpenAI key in vault',
              'Developer sends request with TokenGate key',
              'Gateway injects provider key',
              'Request forwarded to OpenAI',
              'Per-token billing tracked',
            ]} />

            <div className="ig-steps">
              <div className="ig-step">
                <h3><StepNumber n={1} /> Add a Provider Key (admin, once)</h3>
                <ol className="ig-ol">
                  <li>Go to the <strong>Management</strong> page, find the <strong>Provider Keys</strong> section.</li>
                  <li>Click <strong>Add Provider Key</strong>, select <strong>OpenAI</strong>, and paste your <code>sk-...</code> key.</li>
                  <li>Click <strong>Activate</strong> on the newly created key.</li>
                </ol>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={2} /> Create a Gateway API Key</h3>
                <p>On the same <strong>Management</strong> page, create a key with:</p>
                <div className="ig-kv-group">
                  <KeyValue field="Provider" value="openai" />
                  <KeyValue field="Auth Method" value="BYOK" />
                  <KeyValue field="Billing Mode" value="API_USAGE" />
                </div>
                <Callout type="info">
                  Make sure you've added and activated your OpenAI provider key in Step 1 before using this gateway key.
                </Callout>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={3} /> Use the API</h3>

                <h4 className="ig-h4">Option A: Codex CLI</h4>
                <p>Edit (or create) <code>~/.codex/config.toml</code> and replace its contents with:</p>
                <CodeBlock lang="toml">{`model_provider = "tokengate"

[model_providers.tokengate]
name = "TokenGate Proxy"
base_url = "https://gateway.tokengate.to/v1"
wire_api = "responses"
http_headers = {
  "X-Tokengate-Key" = "<tokengate-api-key>"
}`}</CodeBlock>
                <p>Run <code>codex</code> in a code repo. If prompted, select <strong>"Provide your own API key"</strong> &mdash; otherwise you are good to go. No separate OpenAI key is needed.</p>

                <h4 className="ig-h4">Option B: Direct API Calls (curl / SDK)</h4>
                <p>
                  Use the TokenGate API key in the <code>X-TokenGate-Key</code> header. The base URL
                  is <code>https://gateway.tokengate.to</code> and API paths remain the same as the
                  standard OpenAI API (e.g. <code>/v1/openai/chat/completions</code>).
                </p>
                <CodeBlock lang="bash">{`curl https://gateway.tokengate.to/v1/openai/chat/completions \\
    -H "X-TokenGate-Key: <tokengate-api-key>" \\
    -H "Content-Type: application/json" \\
    -d '{"model":"gpt-4.1","max_tokens":20,"messages":[{"role":"user","content":"Hello!"}]}'`}</CodeBlock>
                <p>Or with environment variables for OpenAI SDK-compatible tools:</p>
                <CodeBlock lang="bash">{`export OPENAI_BASE_URL=https://gateway.tokengate.to/v1/openai
export OPENAI_API_KEY="<tokengate-api-key>"
# No separate OpenAI key needed — the gateway uses your stored provider key`}</CodeBlock>
                <span className="form-hint"><a className="form-hint-link" style={{ cursor: 'pointer' }} onClick={() => scrollTo('faq')}>Don't know how to set environment variables?</a></span>
              </div>
            </div>
          </section>

          {/* ── Endpoints ──────────────────────────────────────────────── */}
          <section id="endpoints" ref={ref('endpoints')} className="ig-section">
            <h2 className="ig-h2">
              <SectionIcon type="api" />
              API Endpoints Reference
            </h2>
            <div className="ig-table-wrap">
              <table className="ig-table">
                <thead>
                  <tr><th>Method</th><th>Path</th><th>Provider</th><th>Description</th></tr>
                </thead>
                <tbody>
                  <tr><td><code>POST</code></td><td><code>/v1/messages</code></td><td>Anthropic</td><td>Messages API proxy</td></tr>
                  <tr><td><code>GET</code></td><td><code>/v1/models</code></td><td>Anthropic</td><td>Model list passthrough</td></tr>
                  <tr><td><code>POST</code></td><td><code>/v1/responses</code></td><td>OpenAI / Anthropic</td><td>Responses API (provider-aware routing)</td></tr>
                  <tr><td><code>ANY</code></td><td><code>/v1/openai/*</code></td><td>OpenAI</td><td>OpenAI API passthrough (e.g. <code>/v1/openai/chat/completions</code>)</td></tr>
                </tbody>
              </table>
            </div>
          </section>

          {/* ── Budget headers ─────────────────────────────────────────── */}
          <section id="budget" ref={ref('budget')} className="ig-section">
            <h2 className="ig-h2">
              <SectionIcon type="budget" />
              Budget &amp; Rate Limit Headers
            </h2>
            <p>When budget alerts are triggered, the gateway adds these response headers:</p>
            <div className="ig-table-wrap">
              <table className="ig-table">
                <thead>
                  <tr><th>Header</th><th>Example</th><th>Description</th></tr>
                </thead>
                <tbody>
                  <tr><td><code>X-Tokengate-Budget-Warning</code></td><td><code>true</code></td><td>Present when at or above alert threshold</td></tr>
                  <tr><td><code>X-Tokengate-Budget-Limit</code></td><td><code>100.0000</code></td><td>Configured limit amount</td></tr>
                  <tr><td><code>X-Tokengate-Budget-Used</code></td><td><code>83.4200</code></td><td>Current spend in the period</td></tr>
                  <tr><td><code>X-Tokengate-Budget-Period</code></td><td><code>monthly</code></td><td><code>monthly</code>, <code>weekly</code>, or <code>daily</code></td></tr>
                  <tr><td><code>X-Tokengate-Budget-Scope</code></td><td><code>account</code></td><td><code>account</code> or <code>api_key</code></td></tr>
                </tbody>
              </table>
            </div>
            <p style={{ marginTop: '1rem' }}>If a blocking limit is exceeded, the gateway returns <strong>HTTP 402</strong>:</p>
            <CodeBlock lang="json">{`{
  "error": "budget_exceeded",
  "message": "Budget limit exceeded for period=monthly. Limit: 100.0000, Current: 105.2300"
}`}</CodeBlock>
          </section>

          {/* ── Notification Setup ─────────────────────────────────────── */}
          <section id="notifications" ref={ref('notifications')} className="ig-section">
            <h2 className="ig-h2">
              <SectionIcon type="notification" />
              Notification Setup
            </h2>
            <p className="ig-desc">
              Get real-time alerts in Slack, email, or a custom webhook when budget limits or rate limits
              are triggered. Below is a step-by-step guide for setting up <strong>Slack incoming webhooks</strong>.
            </p>

            <div className="ig-steps">
              <div className="ig-step">
                <h3><StepNumber n={1} /> Create a Slack App</h3>
                <ol className="ig-ol">
                  <li>Go to <a href="https://api.slack.com/apps" target="_blank" rel="noopener noreferrer">api.slack.com/apps</a> and click <strong>Create New App</strong>.</li>
                  <li>Choose <strong>From scratch</strong>, give it a name (e.g. "TokenGate Alerts"), and select your workspace.</li>
                  <li>Click <strong>Create App</strong>.</li>
                </ol>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={2} /> Enable Incoming Webhooks</h3>
                <ol className="ig-ol">
                  <li>In your app settings, navigate to <strong>Incoming Webhooks</strong> in the left sidebar.</li>
                  <li>Toggle <strong>Activate Incoming Webhooks</strong> to <strong>On</strong>.</li>
                  <li>Click <strong>Add New Webhook to Workspace</strong> at the bottom of the page.</li>
                  <li>Select the channel where you want alerts to appear (e.g. <code>#tokengate-alerts</code>) and click <strong>Allow</strong>.</li>
                </ol>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={3} /> Copy the Webhook URL</h3>
                <p>After authorizing, you'll see a new webhook URL that looks like:</p>
                <CodeBlock>{`https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX`}</CodeBlock>
                <p>Click <strong>Copy</strong> to save this URL to your clipboard.</p>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={4} /> Add the Webhook in TokenGate</h3>
                <ol className="ig-ol">
                  <li>Go to the <a href="/notifications"><strong>Notifications</strong></a> page in your TokenGate dashboard.</li>
                  <li>Click <strong>Add Channel</strong> and select <strong>Slack</strong> as the channel type.</li>
                  <li>Paste the webhook URL you copied in Step 3.</li>
                  <li>Select the event types you want to be notified about (budget blocked, budget warning, rate limit exceeded).</li>
                  <li>Click <strong>Save</strong>, then use the <strong>Test</strong> button to verify the integration.</li>
                </ol>
              </div>
            </div>

            <Callout type="info">
              Each Slack webhook URL is tied to a specific channel. To send alerts to multiple channels,
              create a separate webhook for each and add them as individual notification channels in TokenGate.
            </Callout>
          </section>

          {/* ── Troubleshooting ────────────────────────────────────────── */}
          <section id="troubleshoot" ref={ref('troubleshoot')} className="ig-section">
            <h2 className="ig-h2">
              <SectionIcon type="help" />
              Troubleshooting
            </h2>
            <div className="ig-trouble-list">
              <div className="ig-trouble">
                <div className="ig-trouble-issue"><code>404</code> on Anthropic requests</div>
                <div className="ig-trouble-cause"><code>/v1</code> included in <code>ANTHROPIC_BASE_URL</code></div>
                <div className="ig-trouble-fix">Remove <code>/v1</code> &mdash; use <code>https://gateway.tokengate.to</code> only</div>
              </div>
              <div className="ig-trouble">
                <div className="ig-trouble-issue"><code>401</code> Unauthorized</div>
                <div className="ig-trouble-cause">Invalid or expired gateway key</div>
                <div className="ig-trouble-fix">Check key in Management page, create a new one if needed</div>
              </div>
              <div className="ig-trouble">
                <div className="ig-trouble-issue"><code>402</code> Budget Exceeded</div>
                <div className="ig-trouble-cause">Tenant spend above blocking limit</div>
                <div className="ig-trouble-fix">Increase budget limit on the Limits page</div>
              </div>
              <div className="ig-trouble">
                <div className="ig-trouble-issue"><code>403</code> Forbidden</div>
                <div className="ig-trouble-cause">API key provider does not match the endpoint path</div>
                <div className="ig-trouble-fix">Ensure key provider matches: <code>anthropic</code> keys for <code>/v1/messages</code>, <code>openai</code> keys for <code>/v1/openai/*</code></div>
              </div>
              <div className="ig-trouble">
                <div className="ig-trouble-issue">Codex "no auth" error</div>
                <div className="ig-trouble-cause">Missing <code>requires_openai_auth = true</code> in config</div>
                <div className="ig-trouble-fix">Add the field in <code>~/.codex/config.toml</code> for Browser OAuth + Monthly Subscription scenarios</div>
              </div>
              <div className="ig-trouble">
                <div className="ig-trouble-issue">No usage showing in dashboard</div>
                <div className="ig-trouble-cause">Provider key not activated</div>
                <div className="ig-trouble-fix">Go to Provider Keys on the Management page and click Activate</div>
              </div>
            </div>
          </section>

          {/* ── FAQ ────────────────────────────────────────────────────── */}
          <section id="faq" ref={ref('faq')} className="ig-section">
            <h2 className="ig-h2">
              <SectionIcon type="faq" />
              Frequently Asked Questions
            </h2>

            <div className="ig-faq-list">
              <FaqItem question="How do I set environment variables?">
                <p>TokenGate integration requires setting environment variables like <code>ANTHROPIC_BASE_URL</code> or <code>OPENAI_BASE_URL</code>. Here's how to set them on each operating system:</p>

                <h4 className="ig-h4">macOS</h4>
                <p><strong>Temporary</strong> (current terminal session only):</p>
                <CodeBlock lang="bash">{`export ANTHROPIC_BASE_URL=https://gateway.tokengate.to
export ANTHROPIC_CUSTOM_HEADERS="X-TokenGate-Key:<your-key>"`}</CodeBlock>
                <p><strong>Permanent</strong> — add the export lines to your shell profile:</p>
                <CodeBlock lang="bash">{`# For zsh (default on macOS):
echo 'export ANTHROPIC_BASE_URL=https://gateway.tokengate.to' >> ~/.zshrc
source ~/.zshrc

# For bash:
echo 'export ANTHROPIC_BASE_URL=https://gateway.tokengate.to' >> ~/.bash_profile
source ~/.bash_profile`}</CodeBlock>

                <h4 className="ig-h4">Linux</h4>
                <p><strong>Temporary</strong> (current terminal session only):</p>
                <CodeBlock lang="bash">{`export ANTHROPIC_BASE_URL=https://gateway.tokengate.to
export ANTHROPIC_CUSTOM_HEADERS="X-TokenGate-Key:<your-key>"`}</CodeBlock>
                <p><strong>Permanent</strong> — add the export lines to your shell profile:</p>
                <CodeBlock lang="bash">{`# For bash (most common):
echo 'export ANTHROPIC_BASE_URL=https://gateway.tokengate.to' >> ~/.bashrc
source ~/.bashrc

# For zsh:
echo 'export ANTHROPIC_BASE_URL=https://gateway.tokengate.to' >> ~/.zshrc
source ~/.zshrc`}</CodeBlock>

                <h4 className="ig-h4">Windows</h4>
                <p><strong>Temporary</strong> (current terminal session only):</p>
                <CodeBlock lang="powershell">{`# PowerShell:
$env:ANTHROPIC_BASE_URL = "https://gateway.tokengate.to"
$env:ANTHROPIC_CUSTOM_HEADERS = "X-TokenGate-Key:<your-key>"

# Command Prompt (cmd):
set ANTHROPIC_BASE_URL=https://gateway.tokengate.to
set ANTHROPIC_CUSTOM_HEADERS=X-TokenGate-Key:<your-key>`}</CodeBlock>
                <p><strong>Permanent</strong> — use the System Properties GUI or PowerShell:</p>
                <CodeBlock lang="powershell">{`# PowerShell (permanent for current user):
[System.Environment]::SetEnvironmentVariable("ANTHROPIC_BASE_URL", "https://gateway.tokengate.to", "User")

# Or: Settings > System > About > Advanced system settings
#   > Environment Variables > New (User variable)`}</CodeBlock>

                <Callout type="info">
                  After setting permanent environment variables, restart your terminal (or open a new one) for the changes to take effect.
                </Callout>
              </FaqItem>
            </div>
          </section>
        </main>
      </div>
    </div>
  )
}
