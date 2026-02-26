type FeatureItem = { text: string; comingSoon?: boolean }

const categories: { label: string; tag: string; tagColor: string; items: FeatureItem[] }[] = [
  {
    label: 'Visibility',
    tag: 'Everyone',
    tagColor: 'bg-blue-50 text-blue-700',
    items: [
      { text: 'Tokens, requests, latency' },
      { text: 'Usage by model / repo / project' },
      { text: 'Cost equivalent for subscription plans' },
      { text: 'History + time series' },
    ],
  },
  {
    label: 'Guardrails',
    tag: 'API + Subscription',
    tagColor: 'bg-purple-50 text-purple-700',
    items: [
      { text: 'Max tokens per session' },
      { text: 'Max stream duration (SSE)' },
      { text: 'Max requests per minute' },
      { text: 'Runaway loop detection', comingSoon: true },
      { text: '"Kill switch" for bad sessions', comingSoon: true },
    ],
  },
  {
    label: 'Cost Control',
    tag: 'API',
    tagColor: 'bg-orange-50 text-orange-700',
    items: [
      { text: 'Monthly budget cap' },
      { text: 'Per-model caps' },
      { text: 'Auto downgrade (Opus → Sonnet)', comingSoon: true },
      { text: 'Block expensive endpoints', comingSoon: true },
    ],
  },
  {
    label: 'Efficiency Insights',
    tag: 'Pro+',
    tagColor: 'bg-emerald-50 text-emerald-700',
    items: [
      { text: 'Cache hit rate & savings', comingSoon: true },
      { text: 'Usage cap forecasting', comingSoon: true },
      { text: 'Cost per session breakdown', comingSoon: true },
      { text: 'Model cost-efficiency scoring', comingSoon: true },
      { text: 'Wasted spend detection', comingSoon: true },
      { text: 'Cost attribution by project / repo', comingSoon: true },
      { text: 'Per-member efficiency benchmarks', comingSoon: true },
      { text: 'Peak usage heatmap', comingSoon: true },
    ],
  },
]

export default function LandingFeatures() {
  return (
    <section id="features" aria-labelledby="features-heading" className="py-20 sm:py-24 bg-white">
      <div className="mx-auto max-w-6xl px-4 sm:px-6">
        <div className="text-center mb-14">
          <p className="text-sm font-semibold text-blue-600 uppercase tracking-widest mb-3">Features</p>
          <h2 id="features-heading" className="text-3xl sm:text-4xl font-bold text-gray-900">
            Everything you need to stay in control
          </h2>
          <p className="text-gray-500 mt-3 max-w-xl mx-auto">
            From raw visibility to hard spending limits — all in one gateway.
          </p>
        </div>

        <div className="grid sm:grid-cols-2 gap-6">
          {categories.map((cat) => (
            <div
              key={cat.label}
              className="rounded-2xl border border-gray-200 p-7 hover:border-gray-300 transition-colors"
            >
              <div className="flex items-center gap-3 mb-5">
                <h3 className="text-lg font-bold text-gray-900">{cat.label}</h3>
                <span className={`rounded-full px-2.5 py-0.5 text-xs font-semibold ${cat.tagColor}`}>
                  {cat.tag}
                </span>
              </div>
              <ul className="space-y-2.5" aria-label={`${cat.label} features`}>
                {cat.items.map((item) => (
                  <li key={item.text} className="flex items-start gap-2.5 text-sm text-gray-600">
                    <span aria-hidden="true" className="mt-0.5 shrink-0 text-blue-600">✓</span>
                    <span>
                      {item.text}
                      {item.comingSoon && (
                        <span className="ml-1.5 inline-flex items-center rounded-full bg-amber-50 px-2 py-0.5 text-[10px] font-semibold text-amber-600 border border-amber-200">
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
