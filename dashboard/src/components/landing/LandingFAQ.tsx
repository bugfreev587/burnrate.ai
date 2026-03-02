import { useState } from 'react'

const faqs = [
  {
    q: 'What problem does TokenGate solve?',
    a: 'TokenGate gives you visibility and control over AI tool usage. Whether your team uses Claude Code, VS Code extensions, or API keys directly — TokenGate tracks every request, shows real-time cost and token usage, and enforces budgets and rate limits before bills or caps surprise you.',
  },
  {
    q: 'How is TokenGate different from a simple proxy?',
    a: 'A proxy just forwards requests. TokenGate is a control plane: it tracks per-request costs, enforces spend and rate limits, provides per-key and per-model analytics, sends alerts, and gives teams role-based governance — all in real time. You get a full dashboard, not just a passthrough.',
  },
  {
    q: 'Does TokenGate work with Claude Code and the VS Code extension?',
    a: 'Yes. For Claude Code (CLI), set two environment variables (ANTHROPIC_BASE_URL and ANTHROPIC_API_KEY). For the VS Code extension, add the same variables in your settings.json under claudeCode.environmentVariables. Setup takes under 30 seconds for either.',
  },
  {
    q: 'Can TokenGate work with browser-based OAuth tools like claude.ai or ChatGPT?',
    a: 'Not directly. TokenGate works with tools that let you configure a custom API endpoint — like Claude Code, the VS Code extension, or OpenAI Codex CLI. Browser-based OAuth tools (claude.ai, chatgpt.com) don\'t support custom base URLs, so they can\'t be routed through TokenGate.',
  },
  {
    q: 'What billing models does TokenGate support?',
    a: 'TokenGate supports two billing models. Monthly subscription: pay a flat fee for the TokenGate platform (Free, Pro, Team, or Business). API usage-based: if you bring your own LLM provider keys, you pay the provider directly for token usage — TokenGate just tracks and controls it.',
  },
  {
    q: 'Can I use TokenGate without changing my developer workflow?',
    a: 'Yes. You set environment variables or update a config file once — then use Claude Code, VS Code, or Codex exactly as before. Same commands, same editor, same speed. TokenGate adds under 5ms of latency on average and streams responses in real time.',
  },
  {
    q: 'What governance controls does TokenGate provide?',
    a: 'TokenGate supports monthly/daily spend caps, per-key and per-model rate limits (RPM, input/output tokens per minute), hard budget blocks (HTTP 402), alert thresholds, and model allowlists/blocklists. You choose whether to block, throttle, or alert when limits are hit.',
  },
  {
    q: 'Is TokenGate suitable for teams and enterprises?',
    a: 'Yes. The Team plan adds multi-user support, role-based access control (Owner / Admin / Member / Viewer), per-key budgets, webhook alerts, and 180-day data retention. The Business plan adds SSO, unlimited members, advanced RBAC, dedicated onboarding, and 1+ year retention.',
  },
  {
    q: 'What providers are supported today, and what\'s coming next?',
    a: 'Today TokenGate supports Anthropic (Claude) and OpenAI as LLM providers. Google Gemini and other providers are on the roadmap. On the tool side, Claude Code, the VS Code extension, and OpenAI Codex CLI are fully supported.',
  },
  {
    q: 'How does TokenGate handle security and API keys?',
    a: 'Your LLM provider API keys are encrypted at rest and never exposed to the frontend or request logs. TokenGate issues its own gateway keys (tg_xxxxx) that your tools use — your real provider keys stay server-side. All requests are proxied securely with full audit logging.',
  },
  {
    q: 'Will TokenGate slow down my AI sessions?',
    a: 'No. TokenGate adds under 5ms of latency on average. Streaming responses are passed through in real time with no buffering, so your Claude Code or Codex sessions feel exactly the same.',
  },
]

export default function LandingFAQ() {
  const [open, setOpen] = useState<number | null>(null)

  return (
    <section id="faq" aria-labelledby="faq-heading" className="py-20 sm:py-24 bg-[#070d18]">
      <div className="mx-auto max-w-2xl px-4 sm:px-6">
        <div className="text-center mb-12">
          <p className="text-sm font-semibold text-blue-400 uppercase tracking-widest mb-3">FAQ</p>
          <h2 id="faq-heading" className="text-3xl sm:text-4xl font-bold text-slate-100">
            Common questions
          </h2>
        </div>

        <div className="divide-y divide-white/10 rounded-2xl border border-white/15 bg-slate-950 overflow-hidden">
          {faqs.map((faq, i) => (
            <div key={i}>
              <button
                className="flex w-full items-center justify-between gap-4 bg-slate-950 px-6 py-5 text-left hover:bg-slate-900 transition-colors"
                onClick={() => setOpen(open === i ? null : i)}
                aria-expanded={open === i}
                aria-controls={`faq-answer-${i}`}
              >
                <span className="text-sm font-semibold text-white">{faq.q}</span>
                <span
                  aria-hidden="true"
                  className={`shrink-0 text-xl text-slate-400 transition-transform duration-200 ${
                    open === i ? 'rotate-45' : ''
                  }`}
                >
                  +
                </span>
              </button>
              {open === i && (
                <div id={`faq-answer-${i}`} className="px-6 pb-5">
                  <p className="text-sm text-slate-300 leading-relaxed">{faq.a}</p>
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
