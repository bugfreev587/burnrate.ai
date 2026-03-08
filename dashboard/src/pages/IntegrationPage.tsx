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
  { id: 'statusline',     title: 'CLI Status Line',                              icon: 'statusline' },
  { id: 'endpoints',      title: 'API Endpoints',                               icon: 'api' },
  { id: 'budget',         title: 'Budget Headers',                              icon: 'budget' },
  { id: 'notifications',  title: 'Notification Setup',                          icon: 'notification' },
  { id: 'smart-routing',  title: 'Smart Routing',                                icon: 'routing' },
  { id: 'troubleshoot',   title: 'Troubleshooting',                             icon: 'help' },
  { id: 'faq',            title: 'FAQ',                                          icon: 'faq' },
  { id: 'rbac',           title: 'Roles & Permissions',                          icon: 'rbac' },
]

/* ── icon map ─────────────────────────────────────────────────────────────── */

function SectionIcon({ type }: { type: string }) {
  switch (type) {
    case 'anthropic':
      return <span className="ig-icon ig-icon--anthropic">A</span>
    case 'openai':
      return <span className="ig-icon ig-icon--openai">O</span>
    case 'statusline':
      return <span className="ig-icon ig-icon--statusline">&#x2581;</span>
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
    case 'routing':
      return <span className="ig-icon ig-icon--routing">&harr;</span>
    case 'rbac':
      return <span className="ig-icon ig-icon--rbac">&#x1f6e1;</span>
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

function FaqItem({ question, children, id }: { question: string; children: React.ReactNode; id?: string }) {
  const [open, setOpen] = useState(() => {
    if (id && typeof window !== 'undefined') {
      return window.location.hash === `#${id}`
    }
    return false
  })
  return (
    <div id={id} className={`ig-faq-item ${open ? 'ig-faq-item--open' : ''}`}>
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

  /* scroll to hash on mount (e.g. /integration#notifications or /integration#faq-env-vars) */
  useEffect(() => {
    const hash = window.location.hash.replace('#', '')
    if (!hash) return
    setTimeout(() => {
      const el = sectionRefs.current[hash] || document.getElementById(hash)
      if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }, 100)
  }, [])

  /* scroll spy */
  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            setActiveSection(entry.target.id)
            window.history.replaceState(null, '', `#${entry.target.id}`)
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
    window.history.replaceState(null, '', `#${id}`)
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
                      <td>1</td><td>Anthropic</td><td>Browser OAuth</td><td>Monthly Subscription</td><td>Claude Code CLI / VS Code Extension</td>
                    </tr>
                    <tr onClick={() => scrollTo('scenario-2')} className="ig-table-link">
                      <td>2</td><td>Anthropic</td><td>Browser OAuth</td><td>API Usage</td><td>Claude Code CLI / VS Code Extension</td>
                    </tr>
                    <tr onClick={() => scrollTo('scenario-3')} className="ig-table-link">
                      <td>3</td><td>Anthropic</td><td>BYOK</td><td>API Usage</td><td>curl / SDK</td>
                    </tr>
                    <tr onClick={() => scrollTo('scenario-4')} className="ig-table-link">
                      <td>4</td><td>OpenAI</td><td>Browser OAuth</td><td>Monthly Subscription</td><td>Codex CLI / App / VS Code extension</td>
                    </tr>
                    <tr onClick={() => scrollTo('scenario-5')} className="ig-table-link">
                      <td>5</td><td>OpenAI</td><td>BYOK</td><td>API Usage</td><td>Codex CLI / App / VS Code extension / curl / SDK</td>
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

            <div className="ig-video">
              <iframe
                width="100%"
                height="400"
                src="https://www.youtube.com/embed/0DlF6scMDx0"
                title="Scenario 1: Anthropic — Browser Auth + Monthly Subscription"
                frameBorder="0"
                allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
                allowFullScreen
              />
            </div>

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

                <h4>Option A: CLI</h4>
                <p>Set the following environment variables:</p>
                <CodeBlock lang="bash">{`export ANTHROPIC_BASE_URL=https://gateway.tokengate.to
export ANTHROPIC_CUSTOM_HEADERS="X-TokenGate-Key:<tokengate-api-key>"`}</CodeBlock>
                <span className="form-hint"><a className="form-hint-link" style={{ cursor: 'pointer' }} onClick={() => scrollTo('faq')}>Don't know how to set environment variables?</a></span>
                <Callout type="warn">
                  Do <strong>NOT</strong> include <code>/v1</code> in <code>ANTHROPIC_BASE_URL</code>. The Anthropic SDK appends <code>/v1/messages</code> automatically.
                </Callout>

                <h4>Option B: VS Code Extension</h4>
                <p>Open VS Code Settings &rarr; search <strong>Claude Code</strong> &rarr; edit <code>settings.json</code> and add:</p>
                <CodeBlock lang="json">{`"claudeCode.environmentVariables": [
  { "name": "ANTHROPIC_BASE_URL", "value": "https://gateway.tokengate.to" },
  { "name": "ANTHROPIC_CUSTOM_HEADERS", "value": "X-TokenGate-Key:<tokengate-api-key>" }
]`}</CodeBlock>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={3} /> Run Claude Code</h3>
                <p>Run <code>claude</code> in your terminal (CLI) or open the Claude Code panel in VS Code (Extension). When prompted to choose an authentication method, select:</p>
                <div className="ig-select-option">
                  1. Claude account with subscription &middot; Pro, Max
                </div>
                <p>A browser window will automatically open to complete the Anthropic login. Once authenticated, all requests are routed through the gateway and usage is recorded in your TokenGate dashboard.</p>
                <Callout type="info">
                  In the VS Code Extension, you can run <code>/login</code> in the chat panel to trigger the authentication flow.
                </Callout>
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

            <div className="ig-video">
              <iframe
                width="100%"
                height="400"
                src="https://www.youtube.com/embed/knhhcrzB30Q"
                title="Scenario 2: Anthropic — Browser Auth + API Usage Billed"
                frameBorder="0"
                allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
                allowFullScreen
              />
            </div>

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

                <h4>Option A: CLI</h4>
                <p>Set the following environment variables:</p>
                <CodeBlock lang="bash">{`export ANTHROPIC_BASE_URL=https://gateway.tokengate.to
export ANTHROPIC_CUSTOM_HEADERS="X-TokenGate-Key:<tokengate-api-key>"`}</CodeBlock>
                <span className="form-hint"><a className="form-hint-link" style={{ cursor: 'pointer' }} onClick={() => scrollTo('faq')}>Don't know how to set environment variables?</a></span>
                <Callout type="warn">
                  Do <strong>NOT</strong> include <code>/v1</code> in <code>ANTHROPIC_BASE_URL</code>. The Anthropic SDK appends <code>/v1/messages</code> automatically.
                </Callout>

                <h4>Option B: VS Code Extension</h4>
                <p>Open VS Code Settings &rarr; search <strong>Claude Code</strong> &rarr; edit <code>settings.json</code> and add:</p>
                <CodeBlock lang="json">{`"claudeCode.environmentVariables": [
  { "name": "ANTHROPIC_BASE_URL", "value": "https://gateway.tokengate.to" },
  { "name": "ANTHROPIC_CUSTOM_HEADERS", "value": "X-TokenGate-Key:<tokengate-api-key>" }
]`}</CodeBlock>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={3} /> Run Claude Code</h3>
                <p>Run <code>claude</code> in your terminal (CLI) or open the Claude Code panel in VS Code (Extension). When prompted to choose an authentication method, select:</p>
                <div className="ig-select-option">
                  2. Anthropic Console account &middot; API usage billing
                </div>
                <p>Claude Code will add its own Anthropic auth automatically. All requests are routed through the gateway with per-token billing.</p>
                <Callout type="info">
                  In the VS Code Extension, you can run <code>/login</code> in the chat panel to trigger the authentication flow.
                </Callout>
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

            <div className="ig-video">
              <iframe
                width="100%"
                height="400"
                src="https://www.youtube.com/embed/-BOCS_YboR0"
                title="Scenario 3: Anthropic — BYOK + API Usage Billed"
                frameBorder="0"
                allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
                allowFullScreen
              />
            </div>

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
                  <li>Go to the <a href="/management"><strong>Management</strong></a> page, then open the <strong>Provider Keys</strong> section.</li>
                  <li>Click <strong>Add Provider Key</strong>, select <strong>Anthropic</strong>, and paste your <code>sk-ant-...</code> key. <a className="form-hint-link" href="/integration#faq-provider-api-keys">Where do I find my Anthropic/OpenAI API keys?</a></li>
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

            <div className="ig-video">
              <iframe
                width="100%"
                height="400"
                src="https://www.youtube.com/embed/WzxcJ1M2vZk"
                title="Scenario 4: OpenAI — Browser Auth + Monthly Subscription (Codex)"
                frameBorder="0"
                allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
                allowFullScreen
              />
            </div>

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

            <div className="ig-video">
              <iframe
                width="100%"
                height="400"
                src="https://www.youtube.com/embed/sv-cYS3QzzI"
                title="Scenario 5: OpenAI — BYOK + API Usage Billed"
                frameBorder="0"
                allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
                allowFullScreen
              />
            </div>

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
                  <li>Go to the <a href="/management"><strong>Management</strong></a> page, then open the <strong>Provider Keys</strong> section.</li>
                  <li>Click <strong>Add Provider Key</strong>, select <strong>OpenAI</strong>, and paste your <code>sk-...</code> key. <a className="form-hint-link" href="/integration#faq-provider-api-keys">Where do I find my Anthropic/OpenAI API keys?</a></li>
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

          {/* ── CLI Status Line ──────────────────────────────────────── */}
          <section id="statusline" ref={ref('statusline')} className="ig-section">
            <h2 className="ig-h2">
              <SectionIcon type="statusline" />
              CLI Status Line Integration
            </h2>
            <p className="ig-desc">
              See your budget, cost, and usage data directly in your Claude Code terminal &mdash; no context switching required.
              TokenGate provides a drop-in status line script that polls the gateway and renders live progress bars.
            </p>

            {/* Terminal preview */}
            <div className="ig-terminal-preview" style={{
              background: '#0d1117',
              borderRadius: '12px',
              border: '1px solid rgba(255,255,255,0.1)',
              overflow: 'hidden',
              marginBottom: '2rem',
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '6px', padding: '10px 14px', borderBottom: '1px solid rgba(255,255,255,0.06)', background: 'rgba(255,255,255,0.02)' }}>
                <span style={{ width: 12, height: 12, borderRadius: '50%', background: '#ff5f57' }} />
                <span style={{ width: 12, height: 12, borderRadius: '50%', background: '#febc2e' }} />
                <span style={{ width: 12, height: 12, borderRadius: '50%', background: '#28c840' }} />
                <span style={{ marginLeft: 8, fontSize: '12px', color: '#6e7681' }}>Claude Code &mdash; tokengate-statusline.sh</span>
              </div>
              <div style={{ padding: '16px 20px', fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace', fontSize: '13px', lineHeight: '1.6', overflowX: 'auto' }}>
                <div style={{ whiteSpace: 'nowrap' }}>
                  <span style={{ color: '#0099ff' }}>Opus 4.6</span>
                  <span style={{ color: '#484f58' }}> | </span>
                  <span style={{ color: '#2e9599' }}>Desktop</span>
                  <span style={{ color: '#484f58' }}> | </span>
                  <span style={{ color: '#ffb055' }}>0/200k</span>
                  <span style={{ color: '#484f58' }}> | </span>
                  <span style={{ color: '#c878ff' }}>$2.43 today</span>
                  <span style={{ color: '#484f58' }}> | </span>
                  <span style={{ color: '#2e9599' }}>Month</span>
                  {' '}
                  <span style={{ color: '#ffb055' }}>$83/$100</span>
                  {' '}
                  <span style={{ color: '#ffb055' }}>[■■■■■■□□]</span>
                  {' '}
                  <span style={{ color: '#ffb055' }}>83%</span>
                  <span style={{ color: '#484f58' }}> | </span>
                  <span style={{ color: '#2e9599' }}>Day</span>
                  {' '}
                  <span style={{ color: '#ffb055' }}>$4/$10</span>
                  {' '}
                  <span style={{ color: '#e6c800' }}>[■■■□□□□□]</span>
                  {' '}
                  <span style={{ color: '#e6c800' }}>40%</span>
                </div>
              </div>
            </div>

            <div className="ig-steps">
              <div className="ig-step">
                <h3><StepNumber n={1} /> Install the status line script</h3>
                <p>Copy the TokenGate status line script to your Claude config directory:</p>
                <CodeBlock lang="bash">{`# Download the script
curl -o ~/.claude/tokengate-statusline.sh \\
  https://raw.githubusercontent.com/bugfreev587/TokenGate.to/main/statusline/tokengate-statusline.sh

# Make it executable
chmod +x ~/.claude/tokengate-statusline.sh`}</CodeBlock>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={2} /> Configure Claude Code</h3>
                <p>Tell Claude Code to use the TokenGate status line. You can either:</p>
                <p><strong>Option A:</strong> Run the <code>/statusline</code> command inside Claude Code and point it to the script.</p>
                <p><strong>Option B:</strong> Add it manually to <code>~/.claude/settings.json</code>:</p>
                <CodeBlock lang="json">{`{
  "statusLine": {
    "type": "command",
    "command": "~/.claude/tokengate-statusline.sh"
  }
}`}</CodeBlock>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={3} /> Verify</h3>
                <p>
                  Start a new Claude Code session. You should see the status line at the bottom of the terminal
                  showing your model, context window, and TokenGate budget/cost data.
                </p>
                <Callout type="info">
                  The status line requires <code>ANTHROPIC_BASE_URL</code> and <code>ANTHROPIC_API_KEY</code> (or <code>TOKENGATE_API_KEY</code>)
                  to be set. If these are already configured for gateway routing, the status line works automatically.
                </Callout>
              </div>
            </div>

            <h3 className="ig-h3" style={{ marginTop: '2rem' }}>What&apos;s displayed</h3>
            <div className="ig-table-wrap">
              <table className="ig-table">
                <thead>
                  <tr><th>Segment</th><th>Color</th><th>Description</th></tr>
                </thead>
                <tbody>
                  <tr><td><strong>Model</strong></td><td>Blue</td><td>Active Claude model (e.g. Opus 4.6)</td></tr>
                  <tr><td><strong>Directory</strong></td><td>Cyan</td><td>Current working directory name</td></tr>
                  <tr><td><strong>Context</strong></td><td>Orange</td><td>Token usage vs context window (e.g. 45k/200k)</td></tr>
                  <tr><td><strong>Cost today</strong></td><td>Magenta</td><td>API cost today (only shown for API_USAGE billing mode)</td></tr>
                  <tr><td><strong>Month budget</strong></td><td>Color-coded</td><td>Monthly spend vs limit with progress bar</td></tr>
                  <tr><td><strong>Day budget</strong></td><td>Color-coded</td><td>Daily spend vs limit with progress bar</td></tr>
                </tbody>
              </table>
            </div>

            <h3 className="ig-h3" style={{ marginTop: '2rem' }}>Progress bar colors</h3>
            <div className="ig-table-wrap">
              <table className="ig-table">
                <thead>
                  <tr><th>Usage</th><th>Color</th><th>Example</th></tr>
                </thead>
                <tbody>
                  <tr><td>&lt; 40%</td><td style={{ color: '#00a000' }}>Green</td><td style={{ fontFamily: 'monospace' }}>[■■■□□□□□] 30%</td></tr>
                  <tr><td>40 &ndash; 59%</td><td style={{ color: '#e6c800' }}>Yellow</td><td style={{ fontFamily: 'monospace' }}>[■■■■□□□□] 50%</td></tr>
                  <tr><td>60 &ndash; 84%</td><td style={{ color: '#ffb055' }}>Orange</td><td style={{ fontFamily: 'monospace' }}>[■■■■■■□□] 75%</td></tr>
                  <tr><td>&ge; 85%</td><td style={{ color: '#ff5555' }}>Red</td><td style={{ fontFamily: 'monospace' }}>[■■■■■■■□] 90%</td></tr>
                </tbody>
              </table>
            </div>

            <h3 className="ig-h3" style={{ marginTop: '2rem' }}>Configuration options</h3>
            <div className="ig-table-wrap">
              <table className="ig-table">
                <thead>
                  <tr><th>Environment Variable</th><th>Default</th><th>Description</th></tr>
                </thead>
                <tbody>
                  <tr><td><code>TOKENGATE_API_KEY</code></td><td><code>ANTHROPIC_API_KEY</code></td><td>Explicit TokenGate key (overrides ANTHROPIC_API_KEY)</td></tr>
                  <tr><td><code>TOKENGATE_STATUSLINE_POLL</code></td><td><code>5</code></td><td>Cache TTL in seconds between API polls</td></tr>
                  <tr><td><code>TOKENGATE_STATUSLINE_BARS</code></td><td><code>8</code></td><td>Number of blocks in progress bars</td></tr>
                </tbody>
              </table>
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

          {/* ── Smart Routing Guide ──────────────────────────────────── */}
          <section id="smart-routing" ref={ref('smart-routing')} className="ig-section">
            <h2 className="ig-h2">
              <SectionIcon type="routing" />
              Smart Routing &mdash; User Guide
            </h2>
            <p className="ig-desc">
              Smart Routing lets you send your AI requests to <strong>multiple providers</strong> (like Anthropic, OpenAI, DeepSeek, or Mistral)
              through a single endpoint. Instead of being locked in to one provider, you can set up automatic failover,
              load balancing, or cost optimization &mdash; all without changing any code on the developer side.
            </p>

            <Callout type="info">
              <strong>Who is this for?</strong> Smart Routing is useful if you want to:
              reduce downtime by having backup providers,
              spread traffic across multiple models,
              route to the cheapest available model automatically,
              or test different providers without changing developer configurations.
            </Callout>

            {/* ── Key Concepts ──────────────────────────────────────── */}
            <h3 className="ig-h3" style={{ marginTop: '2rem' }}>Key Concepts</h3>
            <p className="ig-desc" style={{ marginBottom: '1rem' }}>
              Before you get started, here are the terms you will see on the Smart Routing page:
            </p>
            <div className="ig-table-wrap">
              <table className="ig-table">
                <thead>
                  <tr><th>Term</th><th>What It Means</th></tr>
                </thead>
                <tbody>
                  <tr>
                    <td><strong>Model Group</strong></td>
                    <td>
                      A virtual model name that maps to one or more real AI models across different providers.
                      For example, you could create a model group called <code>claude-ha</code> that sends requests
                      to Anthropic Claude first, and falls back to OpenAI GPT if Claude is down.
                    </td>
                  </tr>
                  <tr>
                    <td><strong>Deployment</strong></td>
                    <td>
                      A single entry inside a model group. Each deployment points to one specific model at one specific provider.
                      A model group contains one or more deployments. Think of deployments as the &ldquo;candidates&rdquo; that the
                      routing strategy chooses from.
                    </td>
                  </tr>
                  <tr>
                    <td><strong>Strategy</strong></td>
                    <td>
                      The rule that decides <em>which</em> deployment gets each incoming request.
                      There are four strategies to choose from (explained in detail below).
                    </td>
                  </tr>
                  <tr>
                    <td><strong>Provider Key</strong></td>
                    <td>
                      The API key for a specific AI provider (e.g. your Anthropic API key, or your OpenAI API key).
                      You manage these on the <strong>Management</strong> page under &ldquo;Provider Keys.&rdquo;
                      Each deployment can use a specific provider key, or &ldquo;Auto&rdquo; to use the tenant&rsquo;s default key for that provider.
                    </td>
                  </tr>
                  <tr>
                    <td><strong>Priority</strong></td>
                    <td>
                      A number that determines the order in which deployments are tried.
                      <strong>Lower numbers are tried first.</strong> Two deployments with the same priority are treated as equal.
                    </td>
                  </tr>
                  <tr>
                    <td><strong>Weight</strong></td>
                    <td>
                      Used by the Round Robin strategy to distribute traffic unevenly.
                      A deployment with weight 2 gets twice as many requests as one with weight 1.
                      If you don&rsquo;t need this, leave it at the default of 1.
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>

            {/* ── Routing Strategies Explained ──────────────────────── */}
            <h3 className="ig-h3" style={{ marginTop: '2rem' }}>Routing Strategies Explained</h3>
            <p className="ig-desc" style={{ marginBottom: '1rem' }}>
              When you create a model group, you choose one of four strategies. Here is what each one does in plain language:
            </p>

            <div className="ig-sr-strategy-cards">
              <div className="ig-sr-strategy-card">
                <div className="ig-sr-strategy-name">Fallback</div>
                <div className="ig-sr-strategy-subtitle">Best for: Reliability &amp; uptime</div>
                <p>
                  Requests go to the <strong>highest-priority</strong> (lowest number) deployment first.
                  If that deployment fails or is unhealthy, the system automatically tries the next one in line.
                  This is the most common strategy &mdash; use it when you have a preferred provider but want a backup.
                </p>
                <div className="ig-sr-strategy-example">
                  <strong>Example:</strong> You prefer Anthropic Claude but want OpenAI GPT as a backup.
                  Set Claude as priority 0 and GPT as priority 1. If Claude goes down, requests automatically
                  go to GPT without any developer action needed.
                </div>
              </div>

              <div className="ig-sr-strategy-card">
                <div className="ig-sr-strategy-name">Round Robin</div>
                <div className="ig-sr-strategy-subtitle">Best for: Load balancing</div>
                <p>
                  Requests are distributed evenly across all healthy deployments.
                  Each deployment takes turns handling requests. You can use the <strong>Weight</strong> field
                  to send more traffic to certain deployments (e.g. weight 3 gets 3x the requests of weight 1).
                </p>
                <div className="ig-sr-strategy-example">
                  <strong>Example:</strong> You have two OpenAI API keys with separate rate limits.
                  Add both as deployments with weight 1 each. Requests alternate between them, effectively doubling
                  your available throughput.
                </div>
              </div>

              <div className="ig-sr-strategy-card">
                <div className="ig-sr-strategy-name">Lowest Latency</div>
                <div className="ig-sr-strategy-subtitle">Best for: Speed</div>
                <p>
                  The system tracks the average response time of each deployment and automatically
                  routes requests to whichever one has been responding the fastest. Latency is measured
                  continuously, so if a provider gets slower, traffic shifts away from it.
                </p>
                <div className="ig-sr-strategy-example">
                  <strong>Example:</strong> You have deployments on Anthropic and DeepSeek.
                  During peak hours DeepSeek responds faster &mdash; the system automatically sends more traffic there.
                  When Anthropic speeds up again, traffic shifts back.
                </div>
              </div>

              <div className="ig-sr-strategy-card">
                <div className="ig-sr-strategy-name">Cost Optimized</div>
                <div className="ig-sr-strategy-subtitle">Best for: Budget control</div>
                <p>
                  Routes requests to the <strong>cheapest healthy deployment</strong>, based on the
                  cost-per-1K-tokens values you enter for each deployment. If the cheapest option becomes
                  unhealthy, the system falls back to the next cheapest.
                </p>
                <div className="ig-sr-strategy-example">
                  <strong>Example:</strong> You have DeepSeek at $0.001/1K input and Anthropic at $0.003/1K input.
                  Normally all traffic goes to DeepSeek. If DeepSeek has an outage, traffic automatically
                  switches to Anthropic until DeepSeek recovers.
                </div>
              </div>
            </div>

            {/* ── Step-by-Step: Creating Your First Model Group ───── */}
            <h3 className="ig-h3" style={{ marginTop: '2rem' }}>Step-by-Step: Creating Your First Model Group</h3>

            <div className="ig-steps">
              <div className="ig-step">
                <h3><StepNumber n={1} /> Make sure you have Provider Keys</h3>
                <p>
                  Before creating a model group, you need at least one <strong>Provider Key</strong> for each AI provider
                  you want to use. Go to the <strong>Management</strong> page, scroll to the <strong>Provider Keys</strong> section,
                  and add keys for each provider (e.g. Anthropic, OpenAI).
                </p>
                <Callout type="info">
                  If you are using <strong>Browser OAuth</strong> (subscription mode), you may not need provider keys for every
                  deployment &mdash; the developer&rsquo;s own credentials are used for OAuth-based providers. Provider keys are required
                  for <strong>BYOK</strong> and <strong>API Usage</strong> billing modes.
                </Callout>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={2} /> Navigate to the Smart Routing page</h3>
                <p>
                  Click <strong>Smart Routing</strong> in the top navigation bar. You will see the Model Groups table.
                  If this is your first time, it will be empty with a message inviting you to create your first group.
                </p>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={3} /> Click &ldquo;Create Model Group&rdquo;</h3>
                <p>
                  A form will appear. Fill in these fields:
                </p>
                <div className="ig-table-wrap" style={{ marginTop: '0.75rem' }}>
                  <table className="ig-table">
                    <thead>
                      <tr><th>Field</th><th>What to Enter</th></tr>
                    </thead>
                    <tbody>
                      <tr>
                        <td><strong>Name</strong></td>
                        <td>
                          A short, descriptive name for this model group. This becomes the virtual model name that
                          developers can use. For example: <code>claude-ha</code>, <code>fast-model</code>, or <code>budget-model</code>.
                        </td>
                      </tr>
                      <tr>
                        <td><strong>Strategy</strong></td>
                        <td>
                          Click the dropdown and choose one of the four strategies described above.
                          If you are unsure, start with <strong>Fallback</strong> &mdash; it is the simplest and most common choice.
                        </td>
                      </tr>
                      <tr>
                        <td><strong>Description</strong></td>
                        <td>
                          Optional. A note for yourself or your team about what this group is for.
                          For example: &ldquo;Primary Claude with GPT fallback.&rdquo;
                        </td>
                      </tr>
                      <tr>
                        <td><strong>Enabled</strong></td>
                        <td>
                          Check this box to make the model group active. If unchecked, the group exists but won&rsquo;t receive any traffic.
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={4} /> Add Deployments</h3>
                <p>
                  Below the main fields you will see a <strong>Deployments</strong> section with at least one deployment card.
                  Each card represents one provider + model combination. Fill in:
                </p>
                <div className="ig-table-wrap" style={{ marginTop: '0.75rem' }}>
                  <table className="ig-table">
                    <thead>
                      <tr><th>Field</th><th>What to Enter</th></tr>
                    </thead>
                    <tbody>
                      <tr>
                        <td><strong>Provider</strong></td>
                        <td>Select the AI provider: Anthropic, OpenAI, DeepSeek, or Mistral.</td>
                      </tr>
                      <tr>
                        <td><strong>Model</strong></td>
                        <td>
                          Type the exact model name the provider expects. For example:
                          <code>claude-sonnet-4-20250514</code> for Anthropic,
                          <code>gpt-4o</code> for OpenAI,
                          <code>deepseek-chat</code> for DeepSeek.
                        </td>
                      </tr>
                      <tr>
                        <td><strong>Provider Key</strong></td>
                        <td>
                          Choose which API key to use for this deployment. Select &ldquo;Auto (default)&rdquo; to use your
                          organization&rsquo;s default key for that provider. Or pick a specific key from the dropdown
                          (keys are managed on the Management page).
                        </td>
                      </tr>
                      <tr>
                        <td><strong>Priority</strong></td>
                        <td>
                          A number that controls the order. <strong>0 = highest priority</strong> (tried first).
                          Only matters for Fallback and Cost Optimized strategies.
                          For Round Robin and Lowest Latency, all deployments are equally eligible.
                        </td>
                      </tr>
                      <tr>
                        <td><strong>Weight</strong></td>
                        <td>
                          Controls traffic distribution in Round Robin mode. Default is 1.
                          A deployment with weight 2 gets double the traffic of one with weight 1.
                          For other strategies, you can leave this at 1.
                        </td>
                      </tr>
                      <tr>
                        <td><strong>Cost/1K input &amp; Cost/1K output</strong></td>
                        <td>
                          The cost (in dollars) per 1,000 tokens for this model. Only used by the <strong>Cost Optimized</strong> strategy
                          to decide which deployment is cheapest. If you are not using cost optimization, leave these at 0.
                        </td>
                      </tr>
                      <tr>
                        <td><strong>Enabled</strong></td>
                        <td>
                          Uncheck to temporarily remove this deployment from the rotation without deleting it.
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <p style={{ marginTop: '0.75rem' }}>
                  Click <strong>+ Add</strong> to add more deployments. You can add as many as you need.
                  Click <strong>Remove</strong> on a card to delete it (you must keep at least one).
                </p>
              </div>

              <div className="ig-step">
                <h3><StepNumber n={5} /> Save the Model Group</h3>
                <p>
                  Click <strong>Create</strong> (or <strong>Save Changes</strong> if editing). The model group will appear
                  in the table. It is now active and ready to route requests.
                </p>
              </div>
            </div>

            {/* ── How to Use Model Groups in Practice ──────────────── */}
            <h3 className="ig-h3" style={{ marginTop: '2rem' }}>How to Use Model Groups in Practice</h3>
            <p className="ig-desc" style={{ marginBottom: '1rem' }}>
              Once you have created a model group, developers can send requests to it by using the
              model group <strong>name</strong> as the model name in their API call. The gateway will
              automatically route the request based on the strategy you configured.
            </p>

            <FlowDiagram steps={[
              'Developer sends request with model = "claude-ha"',
              'Gateway finds model group "claude-ha"',
              'Strategy picks the best deployment',
              'Request forwarded to the chosen provider',
              'Response returned to developer',
            ]} />

            <Callout type="info">
              <strong>No code changes needed for developers!</strong> Once you set up the model group,
              developers just use the model group name (e.g. <code>claude-ha</code>) in place of a
              real model name. They do not need to know which provider or model is actually serving
              their request. Everything is handled by the gateway.
            </Callout>

            <p style={{ marginTop: '1rem' }}>
              For example, if a developer normally sends requests to <code>claude-sonnet-4-20250514</code>,
              you can tell them to use <code>claude-ha</code> instead. Behind the scenes, the gateway
              will try Anthropic Claude first. If it is down, the gateway will automatically fall back
              to whichever backup deployment you configured.
            </p>

            {/* ── Managing Model Groups ────────────────────────────── */}
            <h3 className="ig-h3" style={{ marginTop: '2rem' }}>Managing Model Groups</h3>
            <p className="ig-desc" style={{ marginBottom: '1rem' }}>
              After creating a model group, you can manage it from the Smart Routing table:
            </p>
            <div className="ig-table-wrap">
              <table className="ig-table">
                <thead>
                  <tr><th>Action</th><th>How to Do It</th><th>What Happens</th></tr>
                </thead>
                <tbody>
                  <tr>
                    <td><strong>Edit</strong></td>
                    <td>Click the <strong>Edit</strong> button on a row</td>
                    <td>
                      Opens the same form used to create the group. You can change the name, strategy,
                      description, enabled/disabled status, and add, edit, or remove deployments.
                      Click <strong>Save Changes</strong> when done.
                    </td>
                  </tr>
                  <tr>
                    <td><strong>Health Check</strong></td>
                    <td>Click the <strong>Health</strong> button on a row</td>
                    <td>
                      Opens a popup showing the real-time health of each deployment:
                      whether it is <span style={{ color: '#86efac' }}>Healthy</span> or <span style={{ color: '#fca5a5' }}>Unhealthy</span>,
                      and its average response time in milliseconds. Use this to verify your deployments are working.
                    </td>
                  </tr>
                  <tr>
                    <td><strong>Delete</strong></td>
                    <td>Click the <strong>Delete</strong> button on a row</td>
                    <td>
                      A confirmation dialog asks if you are sure. If you confirm, the model group and all its
                      deployments are permanently removed. Any requests using that model group name will
                      start failing. This cannot be undone.
                    </td>
                  </tr>
                  <tr>
                    <td><strong>Disable/Enable</strong></td>
                    <td>Click <strong>Edit</strong>, then uncheck/check the <strong>Enabled</strong> checkbox</td>
                    <td>
                      A disabled model group still exists in the table but does not accept traffic.
                      This is useful for temporarily pausing routing without deleting the configuration.
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>

            {/* ── Example Scenarios ───────────────────────────────── */}
            <h3 className="ig-h3" style={{ marginTop: '2rem' }}>Common Scenarios</h3>

            <div className="ig-sr-strategy-cards">
              <div className="ig-sr-strategy-card">
                <div className="ig-sr-strategy-name">High Availability Setup</div>
                <p>
                  <strong>Goal:</strong> Keep AI coding tools working even if one provider has an outage.<br />
                  <strong>Strategy:</strong> Fallback<br />
                  <strong>Setup:</strong> Deployment 1 = Anthropic Claude (priority 0), Deployment 2 = OpenAI GPT-4o (priority 1).<br />
                  <strong>Result:</strong> Developers always get a response. If Anthropic is down, traffic automatically goes to OpenAI.
                </p>
              </div>

              <div className="ig-sr-strategy-card">
                <div className="ig-sr-strategy-name">Rate Limit Doubling</div>
                <p>
                  <strong>Goal:</strong> Handle more concurrent requests than a single API key allows.<br />
                  <strong>Strategy:</strong> Round Robin<br />
                  <strong>Setup:</strong> Two deployments, both pointing to the same model (e.g. <code>claude-sonnet-4-20250514</code>),
                  each using a different provider key.<br />
                  <strong>Result:</strong> Requests alternate between the two keys, effectively doubling your rate limit.
                </p>
              </div>

              <div className="ig-sr-strategy-card">
                <div className="ig-sr-strategy-name">Budget-Friendly Routing</div>
                <p>
                  <strong>Goal:</strong> Minimize costs by using the cheapest available model.<br />
                  <strong>Strategy:</strong> Cost Optimized<br />
                  <strong>Setup:</strong> Deployment 1 = DeepSeek (cost: $0.001/1K input), Deployment 2 = Anthropic Claude (cost: $0.003/1K input).<br />
                  <strong>Result:</strong> All traffic goes to DeepSeek. If DeepSeek is unhealthy, Claude takes over automatically.
                </p>
              </div>
            </div>

            {/* ── Smart Routing FAQ ────────────────────────────────── */}
            <h3 className="ig-h3" style={{ marginTop: '2rem' }}>Frequently Asked Questions</h3>
            <div className="ig-faq-list">
              <FaqItem id="faq-sr-who" question="Who can create and manage model groups?">
                <p>
                  You need at least the <strong>Editor</strong> role in your organization to see the Smart Routing page
                  and manage model groups. Viewers cannot access this feature. Owners and Admins always have full access.
                </p>
              </FaqItem>

              <FaqItem id="faq-sr-model-name" question="What model name should I give developers?">
                <p>
                  Tell them to use the <strong>model group name</strong> as their model. For example, if your
                  model group is called <code>claude-ha</code>, developers should set <code>model: "claude-ha"</code>
                  in their configuration. The gateway will handle the rest.
                </p>
              </FaqItem>

              <FaqItem id="faq-sr-existing" question="Do I need to change my existing gateway API keys?">
                <p>
                  No. Your existing gateway API keys work the same as before. Smart Routing only activates
                  when a request uses a model group name instead of a real model name. Requests with regular
                  model names (like <code>claude-sonnet-4-20250514</code>) are sent directly to the provider
                  as they always were.
                </p>
              </FaqItem>

              <FaqItem id="faq-sr-health" question="How does the health check work?">
                <p>
                  The gateway continuously monitors each deployment&rsquo;s health by tracking response statuses
                  and latency. If a deployment returns errors or becomes slow, it is marked as unhealthy.
                  The Health button on the Smart Routing page shows you the current status of all deployments
                  in a model group, including their average response time.
                </p>
              </FaqItem>

              <FaqItem id="faq-sr-failover" question="What happens when a deployment fails?">
                <p>
                  It depends on the strategy. With <strong>Fallback</strong>, the system tries the next deployment
                  in priority order. With <strong>Round Robin</strong> or <strong>Lowest Latency</strong>,
                  the unhealthy deployment is skipped and traffic goes to the remaining healthy ones.
                  With <strong>Cost Optimized</strong>, the next cheapest healthy deployment is chosen.
                  In all cases, the failover happens automatically and instantly &mdash; the developer does not notice.
                </p>
              </FaqItem>

              <FaqItem id="faq-sr-cost-fields" question="Do I need to fill in the cost fields?">
                <p>
                  Only if you are using the <strong>Cost Optimized</strong> strategy. The cost fields tell
                  the system which deployment is cheaper. For other strategies (Fallback, Round Robin, Lowest Latency),
                  you can leave them at 0.
                </p>
              </FaqItem>

              <FaqItem id="faq-sr-weight" question="When does the Weight field matter?">
                <p>
                  Weight only affects the <strong>Round Robin</strong> strategy. A deployment with weight 2
                  receives twice as many requests as one with weight 1. For all other strategies, weight is ignored.
                  If you are using Round Robin with equal distribution, leave all weights at 1.
                </p>
              </FaqItem>

              <FaqItem id="faq-sr-priority" question="When does the Priority field matter?">
                <p>
                  Priority is used by the <strong>Fallback</strong> strategy to determine which deployment
                  to try first (lower number = higher priority = tried first). It also influences
                  <strong> Cost Optimized</strong> as a tiebreaker when two deployments have the same cost.
                  For Round Robin and Lowest Latency, priority is not used.
                </p>
              </FaqItem>

              <FaqItem id="faq-sr-disable" question="How do I temporarily stop routing without deleting?">
                <p>
                  You have two options: <strong>(1)</strong> Edit the model group and uncheck <strong>Enabled</strong> to
                  disable the entire group. <strong>(2)</strong> Edit an individual deployment and uncheck
                  its <strong>Enabled</strong> checkbox to remove just that one deployment from the rotation.
                </p>
              </FaqItem>

              <FaqItem id="faq-sr-auto-key" question="What does 'Auto (default)' mean for Provider Key?">
                <p>
                  When you select &ldquo;Auto (default)&rdquo; for a deployment&rsquo;s provider key, the system
                  uses your organization&rsquo;s default active key for that provider. This is the most common
                  choice. You only need to select a specific key if you have multiple keys for the same
                  provider and want to control which one is used for a particular deployment.
                </p>
              </FaqItem>
            </div>

          </section>

          {/* ── Troubleshooting ────────────────────────────────────────── */}
          <section id="troubleshoot" ref={ref('troubleshoot')} className="ig-section">
            <h2 className="ig-h2">
              <SectionIcon type="help" />
              Troubleshooting
            </h2>
            <div className="ig-trouble-list">
              <div id="trouble-404" className="ig-trouble">
                <div className="ig-trouble-issue"><code>404</code> on Anthropic requests</div>
                <div className="ig-trouble-cause"><code>/v1</code> included in <code>ANTHROPIC_BASE_URL</code></div>
                <div className="ig-trouble-fix">Remove <code>/v1</code> &mdash; use <code>https://gateway.tokengate.to</code> only</div>
              </div>
              <div id="trouble-401" className="ig-trouble">
                <div className="ig-trouble-issue"><code>401</code> Unauthorized</div>
                <div className="ig-trouble-cause">Invalid or expired gateway key</div>
                <div className="ig-trouble-fix">Check key in Management page, create a new one if needed</div>
              </div>
              <div id="trouble-402" className="ig-trouble">
                <div className="ig-trouble-issue"><code>402</code> Budget Exceeded</div>
                <div className="ig-trouble-cause">Tenant spend above blocking limit</div>
                <div className="ig-trouble-fix">Increase budget limit on the Limits page</div>
              </div>
              <div id="trouble-403" className="ig-trouble">
                <div className="ig-trouble-issue"><code>403</code> Forbidden</div>
                <div className="ig-trouble-cause">API key provider does not match the endpoint path</div>
                <div className="ig-trouble-fix">Ensure key provider matches: <code>anthropic</code> keys for <code>/v1/messages</code>, <code>openai</code> keys for <code>/v1/openai/*</code></div>
              </div>
              <div id="trouble-codex-no-auth" className="ig-trouble">
                <div className="ig-trouble-issue">Codex "no auth" error</div>
                <div className="ig-trouble-cause">Missing <code>requires_openai_auth = true</code> in config</div>
                <div className="ig-trouble-fix">Add the field in <code>~/.codex/config.toml</code> for Browser OAuth + Monthly Subscription scenarios</div>
              </div>
              <div id="trouble-no-usage" className="ig-trouble">
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
              <FaqItem id="faq-env-vars" question="How do I set environment variables?">
                <p>TokenGate integration requires setting environment variables like <code>ANTHROPIC_BASE_URL</code> or <code>OPENAI_BASE_URL</code>. Here's how to set them on each operating system:</p>

                <h4 className="ig-h4">macOS</h4>
                <p><strong>Temporary</strong> (current terminal session only):</p>
                <CodeBlock lang="bash">{`export ANTHROPIC_BASE_URL=https://gateway.tokengate.to
export ANTHROPIC_CUSTOM_HEADERS="X-TokenGate-Key:<your-api-key>"`}</CodeBlock>
                <p><strong>Permanent</strong> — add the export lines to your shell profile:</p>
                <CodeBlock lang="bash">{`# For zsh (default on macOS):
echo 'export ANTHROPIC_BASE_URL=https://gateway.tokengate.to
export ANTHROPIC_CUSTOM_HEADERS="X-TokenGate-Key:<your-api-key>"' >> ~/.zshrc
source ~/.zshrc

# For bash:
echo 'export ANTHROPIC_BASE_URL=https://gateway.tokengate.to
export ANTHROPIC_CUSTOM_HEADERS="X-TokenGate-Key:<your-api-key>"' >> ~/.bash_profile
source ~/.bash_profile`}</CodeBlock>

                <h4 className="ig-h4">Linux</h4>
                <p><strong>Temporary</strong> (current terminal session only):</p>
                <CodeBlock lang="bash">{`export ANTHROPIC_BASE_URL=https://gateway.tokengate.to
export ANTHROPIC_CUSTOM_HEADERS="X-TokenGate-Key:<your-api-key>""`}</CodeBlock>
                <p><strong>Permanent</strong> — add the export lines to your shell profile:</p>
                <CodeBlock lang="bash">{`# For bash (most common):
echo 'export ANTHROPIC_BASE_URL=https://gateway.tokengate.to
export ANTHROPIC_CUSTOM_HEADERS="X-TokenGate-Key:<your-api-key>"' >> ~/.bashrc
source ~/.bashrc

# For zsh:
echo 'export ANTHROPIC_BASE_URL=https://gateway.tokengate.to
export ANTHROPIC_CUSTOM_HEADERS="X-TokenGate-Key:<your-api-key>"' >> ~/.zshrc
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

              <FaqItem id="faq-provider-api-keys" question="Where do I find my Anthropic/OpenAI API keys?">
                <p>You can create and manage provider API keys from the official provider consoles:</p>
                <h4 className="ig-h4">Anthropic</h4>
                <ol className="ig-ol">
                  <li>Go to <a href="https://console.anthropic.com/settings/keys" target="_blank" rel="noopener noreferrer">console.anthropic.com/settings/keys</a>.</li>
                  <li>Click <strong>Create Key</strong>.</li>
                  <li>Give it a name (e.g. "TokenGate") and click <strong>Create Key</strong>.</li>
                  <li>Copy the key (starts with <code>sk-ant-...</code>) — you won't be able to see it again.</li>
                </ol>
                <h4 className="ig-h4">OpenAI</h4>
                <ol className="ig-ol">
                  <li>Go to <a href="https://platform.openai.com/api-keys" target="_blank" rel="noopener noreferrer">platform.openai.com/api-keys</a>.</li>
                  <li>Click <strong>Create new secret key</strong>.</li>
                  <li>Give it a name (e.g. "TokenGate") and click <strong>Create secret key</strong>.</li>
                  <li>Copy the key (starts with <code>sk-...</code>) — you won't be able to see it again.</li>
                </ol>
                <Callout type="warn">
                  Store provider keys securely. If you lose a key, create a new one. In TokenGate, add it from <a href="/management"><strong>Management → Provider Keys</strong></a> using <strong>Add Provider Key</strong>.
                </Callout>
              </FaqItem>
            </div>
          </section>

          {/* ── Roles & Permissions (RBAC) ────────────────────────────── */}
          <section id="rbac" ref={ref('rbac')} className="ig-section">
            <h2 className="ig-h2">
              <SectionIcon type="rbac" />
              Roles &amp; Permissions
            </h2>
            <p className="ig-desc">
              A complete reference for understanding who can do what in your TokenGate organization.
            </p>

            <Callout type="info">
              TokenGate uses a <strong>two-level role system</strong>: <strong>Organization roles</strong> control
              tenant-wide access (members, billing, provider keys), while <strong>Project roles</strong> control
              access within individual projects (API keys, limits). Org Owners and Admins automatically have
              full access to all projects.
            </Callout>

            {/* ── Organization Roles ──────────────────────────────────── */}
            <h3 className="ig-h3" style={{ marginTop: '2rem' }}>Organization Roles</h3>
            <div className="ig-table-wrap">
              <table className="ig-table">
                <thead>
                  <tr><th>Role</th><th>Description</th><th>Key Capabilities</th></tr>
                </thead>
                <tbody>
                  <tr>
                    <td><strong>Owner</strong></td>
                    <td>Full control of the organization</td>
                    <td>All permissions including delete org, manage billing, and cancel subscription</td>
                  </tr>
                  <tr>
                    <td><strong>Admin</strong></td>
                    <td>Day-to-day organization management</td>
                    <td>Everything except deleting the org, updating the billing plan, and cancelling the subscription</td>
                  </tr>
                  <tr>
                    <td><strong>Editor</strong></td>
                    <td>Project-level contributor</td>
                    <td>List projects and view usage. Must be added to individual projects for deeper access</td>
                  </tr>
                  <tr>
                    <td><strong>Viewer</strong></td>
                    <td>Read-only observer</td>
                    <td>List projects, view usage, and view plan status. Must be added to projects for project access</td>
                  </tr>
                </tbody>
              </table>
            </div>

            {/* ── Organization Permission Matrix ─────────────────────── */}
            <h3 className="ig-h3" style={{ marginTop: '2rem' }}>Organization Permission Matrix</h3>
            <div className="ig-table-wrap">
              <table className="ig-table ig-permission-matrix">
                <thead>
                  <tr>
                    <th>Action</th>
                    <th>Owner</th>
                    <th>Admin</th>
                    <th>Editor</th>
                    <th>Viewer</th>
                  </tr>
                </thead>
                <tbody>
                  <tr><td colSpan={5} className="ig-matrix-group">Organization</td></tr>
                  <tr><td>View org settings</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Update org settings</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Delete organization</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>

                  <tr><td colSpan={5} className="ig-matrix-group">Members</td></tr>
                  <tr><td>List members</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Invite members</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Update member roles</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Remove members</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>

                  <tr><td colSpan={5} className="ig-matrix-group">Projects</td></tr>
                  <tr><td>List projects</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td></tr>
                  <tr><td>Create projects</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>

                  <tr><td colSpan={5} className="ig-matrix-group">Provider Keys</td></tr>
                  <tr><td>List / view provider keys</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Create provider keys</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Update provider keys</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Delete provider keys</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>

                  <tr><td colSpan={5} className="ig-matrix-group">Usage &amp; Reports</td></tr>
                  <tr><td>View usage</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td></tr>
                  <tr><td>Generate reports</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>

                  <tr><td colSpan={5} className="ig-matrix-group">Audit</td></tr>
                  <tr><td>View audit logs</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>

                  <tr><td colSpan={5} className="ig-matrix-group">Billing</td></tr>
                  <tr><td>View billing / plan status</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-yes">&#x2713;</td></tr>
                  <tr><td>Update billing plan</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Cancel subscription</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Check downgrade impact</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                </tbody>
              </table>
            </div>

            {/* ── Project Roles ───────────────────────────────────────── */}
            <h3 className="ig-h3" style={{ marginTop: '2rem' }}>Project Roles</h3>
            <div className="ig-table-wrap">
              <table className="ig-table">
                <thead>
                  <tr><th>Role</th><th>Description</th><th>Key Capabilities</th></tr>
                </thead>
                <tbody>
                  <tr>
                    <td><strong>Project Admin</strong></td>
                    <td>Full control of the project</td>
                    <td>Manage project settings, members, API keys, and limits (including create &amp; delete)</td>
                  </tr>
                  <tr>
                    <td><strong>Project Editor</strong></td>
                    <td>Manage API keys within the project</td>
                    <td>View &amp; update project settings, create/update/revoke API keys, view limits</td>
                  </tr>
                  <tr>
                    <td><strong>Project Viewer</strong></td>
                    <td>Read-only project access</td>
                    <td>View project settings, list API keys, and view limits</td>
                  </tr>
                </tbody>
              </table>
            </div>

            {/* ── Project Permission Matrix ───────────────────────────── */}
            <h3 className="ig-h3" style={{ marginTop: '2rem' }}>Project Permission Matrix</h3>
            <div className="ig-table-wrap">
              <table className="ig-table ig-permission-matrix">
                <thead>
                  <tr>
                    <th>Action</th>
                    <th>Project Admin</th>
                    <th>Project Editor</th>
                    <th>Project Viewer</th>
                  </tr>
                </thead>
                <tbody>
                  <tr><td colSpan={4} className="ig-matrix-group">Project Settings</td></tr>
                  <tr><td>View project</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td></tr>
                  <tr><td>Update project</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Delete project</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>

                  <tr><td colSpan={4} className="ig-matrix-group">Project Members</td></tr>
                  <tr><td>List members</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Add members</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Update member roles</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Remove members</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>

                  <tr><td colSpan={4} className="ig-matrix-group">API Keys</td></tr>
                  <tr><td>List / view API keys</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td></tr>
                  <tr><td>Create API keys</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Update API keys</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Revoke API keys</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td></tr>

                  <tr><td colSpan={4} className="ig-matrix-group">Limits</td></tr>
                  <tr><td>List / view limits</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td></tr>
                  <tr><td>Create limits</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Update limits</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Delete limits</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td><td className="ig-perm-no">&mdash;</td></tr>
                </tbody>
              </table>
            </div>

            {/* ── How Roles Interact ──────────────────────────────────── */}
            <Callout type="info">
              <strong>How roles interact:</strong> Org <strong>Owners</strong> and <strong>Admins</strong> automatically
              bypass project-level role checks &mdash; they have full access to every project without needing explicit
              membership. Org <strong>Editors</strong> and <strong>Viewers</strong> must be explicitly added to a project
              and assigned a project role to access it.
            </Callout>

            {/* ── Dashboard Navigation by Role ────────────────────────── */}
            <h3 className="ig-h3" style={{ marginTop: '2rem' }}>Dashboard Navigation by Role</h3>
            <p className="ig-desc" style={{ marginBottom: '1rem' }}>
              The pages visible in the navigation bar depend on the user's org role.
            </p>
            <div className="ig-table-wrap">
              <table className="ig-table ig-permission-matrix">
                <thead>
                  <tr>
                    <th>Page</th>
                    <th>Owner</th>
                    <th>Admin</th>
                    <th>Editor</th>
                    <th>Viewer</th>
                  </tr>
                </thead>
                <tbody>
                  <tr><td>Dashboard</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td></tr>
                  <tr><td>Management</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Smart Routing</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Limits</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Notifications</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Pricing Config</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-no">&mdash;</td></tr>
                  <tr><td>Integration</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td></tr>
                  <tr><td>Audit</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td><td className="ig-perm-yes">&#x2713;</td></tr>
                </tbody>
              </table>
            </div>

            {/* ── RBAC FAQ ────────────────────────────────────────────── */}
            <h3 className="ig-h3" style={{ marginTop: '2rem' }}>Common Questions</h3>
            <div className="ig-faq-list">
              <FaqItem id="faq-invite-member" question="How do I invite a new team member?">
                <p>Go to the <strong>Management</strong> page and open the <strong>Members</strong> section. Click <strong>Invite Member</strong>, enter their email, and select an org role. Only <strong>Owners</strong> and <strong>Admins</strong> can invite members.</p>
              </FaqItem>

              <FaqItem id="faq-invite-admin" question="Can I invite someone directly as an Admin?">
                <p>Yes. When inviting a member, you can assign any org role (Owner, Admin, Editor, or Viewer). Owners and Admins can set the role at invite time.</p>
              </FaqItem>

              <FaqItem id="faq-user-suspended" question="What happens when a user is suspended?">
                <p>Suspended users cannot log in or make API requests. Their membership record is preserved so they can be reinstated later. Only Owners and Admins can suspend or reinstate members.</p>
              </FaqItem>

              <FaqItem id="faq-default-project" question="What is the Default project?">
                <p>Every organization has a <strong>Default</strong> project created automatically. All org members have access to the Default project. It cannot be deleted but can be renamed. New API keys are assigned to the Default project unless you choose a different one.</p>
              </FaqItem>

              <FaqItem id="faq-multi-user-plan" question="Which plan supports multi-user teams?">
                <p>Multi-user teams with role-based access control are available on the <strong>Team</strong> plan and above. The Free plan is limited to a single user. Visit the <a href="/pricing"><strong>Pricing</strong></a> page for plan details.</p>
              </FaqItem>
            </div>
          </section>
        </main>
      </div>
    </div>
  )
}
