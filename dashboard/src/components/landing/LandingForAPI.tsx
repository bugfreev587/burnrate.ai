const guardrails = [
  { label: 'Monthly / daily spend caps', icon: '📅' },
  { label: 'Per-project budgets', icon: '📁' },
  { label: 'Per-user / per-key limits', icon: '🔑' },
  { label: 'Hard stop, throttle, downgrade rules', icon: '🛑' },
  { label: 'Alerts before things go wrong', icon: '🔔' },
]

export default function LandingForAPI() {
  return (
    <section aria-labelledby="api-heading" className="py-20 sm:py-24 bg-[#06090f]">
      <div className="mx-auto max-w-7xl px-4 sm:px-6">
        <div className="grid md:grid-cols-2 gap-12 items-center">
          <div aria-label="Example active guardrails" className="rounded-2xl border border-white/15 bg-white/[0.03] p-6 order-2 md:order-1">
            <p className="text-xs font-semibold text-slate-400 uppercase tracking-widest mb-4">
              Active Guardrails
            </p>
            <ul className="space-y-3" aria-label="Guardrail examples">
              {guardrails.map((g) => (
                <li
                  key={g.label}
                  className="flex items-center gap-4 rounded-lg border border-white/10 bg-slate-950/70 px-4 py-3"
                >
                  <span aria-hidden="true" className="text-xl">{g.icon}</span>
                  <span className="text-sm font-medium text-slate-200 flex-1">{g.label}</span>
                  <span className="rounded-full bg-emerald-500/15 px-2.5 py-0.5 text-xs font-semibold text-emerald-300 border border-emerald-400/25">
                    Active
                  </span>
                </li>
              ))}
            </ul>
          </div>

          <div className="order-1 md:order-2">
            <span className="inline-block rounded-lg bg-orange-500/15 px-3 py-1 text-sm font-semibold text-orange-200 mb-5">
              For API Users
            </span>
            <h2 id="api-heading" className="text-3xl sm:text-4xl font-bold text-slate-100 mb-4">
              Add a cost firewall before you get burned.
            </h2>
            <p className="text-slate-300 text-lg leading-relaxed">
              Set hard limits on spend — per project, per user, per key. TokenGate enforces your policies in
              real time, before the bill arrives.
            </p>
          </div>
        </div>
      </div>
    </section>
  )
}
