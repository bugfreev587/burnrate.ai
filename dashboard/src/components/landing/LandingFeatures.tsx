type FeatureItem = { text: string; comingSoon?: boolean }

const categories: { label: string; tag: string; tagColor: string; items: FeatureItem[] }[] = [
  {
    label: 'Visibility',
    tag: 'Everyone',
    tagColor: 'bg-blue-500/15 text-blue-200 border border-blue-400/20',
    items: [
      { text: 'Per-request token & cost tracking' },
      { text: 'Breakdown by model & API key' },
      { text: 'Daily cost & token trend charts' },
      { text: 'Projected monthly spend' },
      { text: 'Multi-provider unified dashboard' },
      { text: 'CLI status line with live budget progress bars' },
      { text: 'Cost equivalent: subscription vs API', comingSoon: true },
      { text: 'Usage by project / repository', comingSoon: true },
      { text: 'Latency & response time tracking', comingSoon: true },
    ],
  },
  {
    label: 'Guardrails',
    tag: 'Pro+',
    tagColor: 'bg-violet-500/15 text-violet-200 border border-violet-400/20',
    items: [
      { text: 'Rate limiting (RPM / ITPM / OTPM)' },
      { text: 'Per-key & per-model rate limits' },
      { text: 'Hard budget block (HTTP 402)' },
      { text: 'Per-provider budget scoping' },
      { text: 'Model allowlists / blocklists'},
      { text: 'Max input tokens per request', comingSoon: true },
      { text: 'Runaway loop detection', comingSoon: true },
      { text: 'Session kill switch', comingSoon: true },
    ],
  },
  {
    label: 'Cost Control',
    tag: 'Pro+',
    tagColor: 'bg-orange-500/15 text-orange-200 border border-orange-400/20',
    items: [
      { text: 'Monthly, weekly & daily budget caps' },
      { text: 'Hard stop — auto-block at limit' },
      { text: 'Per-API-key budget scoping' },
      { text: 'Alert thresholds + warning headers' },
      { text: 'Per-model budget caps', comingSoon: true },
      { text: 'Spend velocity alerts', comingSoon: true },
      { text: 'Auto downgrade (Opus → Sonnet)', comingSoon: true },
      { text: 'Block expensive endpoints', comingSoon: true },
    ],
  },
  {
    label: 'Efficiency Insights',
    tag: 'Pro+',
    tagColor: 'bg-emerald-500/15 text-emerald-200 border border-emerald-400/20',
    items: [
      { text: 'Cache hit rate & savings', comingSoon: true },
      { text: 'Usage cap forecasting', comingSoon: true },
      { text: 'Cost per session breakdown', comingSoon: true },
      { text: 'Model cost-efficiency scoring', comingSoon: true },
      { text: 'Wasted spend detection', comingSoon: true },
      { text: 'Cost attribution by project / repo' },
      { text: 'Per-member efficiency benchmarks', comingSoon: true },
      { text: 'Peak usage heatmap', comingSoon: true },
    ],
  },
]

export default function LandingFeatures() {
  return (
    <section id="features" aria-labelledby="features-heading" className="py-20 sm:py-24 bg-[#070d18]">
      <div className="mx-auto max-w-7xl px-4 sm:px-6">
        <div className="text-center mb-14">
          <p className="text-sm font-semibold text-blue-400 uppercase tracking-widest mb-3">Features</p>
          <h2 id="features-heading" className="text-3xl sm:text-4xl font-bold text-slate-100">
            Everything you need to stay in control
          </h2>
          <p className="text-slate-400 mt-3 max-w-xl mx-auto">
            From raw visibility to hard spending limits — all in one gateway.
          </p>
        </div>

        <div className="grid sm:grid-cols-2 gap-6">
          {categories.map((cat) => (
            <div
              key={cat.label}
              className="rounded-2xl border border-white/15 bg-white/[0.03] p-7 hover:border-white/30 transition-colors"
            >
              <div className="flex items-center gap-3 mb-5">
                <h3 className="text-lg font-bold text-slate-100">{cat.label}</h3>
                <span className={`rounded-full px-2.5 py-0.5 text-xs font-semibold ${cat.tagColor}`}>
                  {cat.tag}
                </span>
              </div>
              <ul className="space-y-2.5" aria-label={`${cat.label} features`}>
                {cat.items.map((item) => (
                  <li key={item.text} className="flex items-start gap-2.5 text-sm text-slate-300">
                    <span aria-hidden="true" className="mt-0.5 shrink-0 text-blue-400">✓</span>
                    <span>
                      {item.text}
                      {item.comingSoon && (
                        <span className="ml-1.5 inline-flex items-center rounded-full bg-emerald-950/60 px-2 py-0.5 text-[10px] font-semibold text-emerald-400 border border-emerald-800/50">
                          Coming Soon
                        </span>
                      )}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
