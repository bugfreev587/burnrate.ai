import { useState } from 'react'

const faqs = [
  {
    q: 'Does this work with Claude Pro subscriptions?',
    a: 'Yes. TokenGate tracks token usage and patterns for subscription plans, showing you real usage and the "API cost equivalent" so you understand exactly where your quota is going.',
  },
  {
    q: 'Does it work with Anthropic or OpenAI API keys?',
    a: 'Yes. Set ANTHROPIC_BASE_URL to gateway.tokengate.to and TokenGate proxies your requests, tracking costs and enforcing budgets in real time.',
  },
  {
    q: 'Will it slow down my Claude Code sessions?',
    a: 'No. TokenGate adds under 5ms of latency on average. Streaming responses are passed through in real time with no buffering.',
  },
  {
    q: 'What happens when I hit a budget cap?',
    a: 'Depending on your configured policy: requests can be hard-blocked, throttled, or auto-downgraded to a cheaper model (e.g., Opus → Sonnet). You choose the behavior.',
  },
  {
    q: 'Is my API key secure?',
    a: 'Yes. Your Anthropic API key is encrypted at rest and never exposed to the frontend or request logs. TokenGate acts as a secure, audited proxy.',
  },
  {
    q: 'Can I use this for my whole team?',
    a: 'Yes. The Power plan supports multiple members with per-user budgets, per-key limits, and role-based access control.',
  },
]

export default function LandingFAQ() {
  const [open, setOpen] = useState<number | null>(null)

  return (
    <section id="faq" aria-labelledby="faq-heading" className="py-20 sm:py-24 bg-gray-50">
      <div className="mx-auto max-w-2xl px-4 sm:px-6">
        <div className="text-center mb-12">
          <p className="text-sm font-semibold text-blue-600 uppercase tracking-widest mb-3">FAQ</p>
          <h2 id="faq-heading" className="text-3xl sm:text-4xl font-bold text-gray-900">
            Common questions
          </h2>
        </div>

        <div className="divide-y divide-gray-200 rounded-2xl border border-gray-200 bg-white overflow-hidden">
          {faqs.map((faq, i) => (
            <div key={i}>
              <button
                className="flex w-full items-center justify-between gap-4 px-6 py-5 text-left"
                onClick={() => setOpen(open === i ? null : i)}
                aria-expanded={open === i}
                aria-controls={`faq-answer-${i}`}
              >
                <span className="text-sm font-semibold text-gray-900">{faq.q}</span>
                <span
                  aria-hidden="true"
                  className={`shrink-0 text-xl text-gray-400 transition-transform duration-200 ${
                    open === i ? 'rotate-45' : ''
                  }`}
                >
                  +
                </span>
              </button>
              {open === i && (
                <div id={`faq-answer-${i}`} className="px-6 pb-5">
                  <p className="text-sm text-gray-600 leading-relaxed">{faq.a}</p>
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
