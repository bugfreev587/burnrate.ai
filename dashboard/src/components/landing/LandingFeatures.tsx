const categories = [
  {
    label: 'Visibility',
    tag: 'Everyone',
    tagColor: 'bg-blue-50 text-blue-700',
    items: [
      'Tokens, requests, latency',
      'Usage by model / repo / project',
      'Cost equivalent for subscription plans',
      'History + time series',
    ],
  },
  {
    label: 'Guardrails',
    tag: 'API + Subscription',
    tagColor: 'bg-purple-50 text-purple-700',
    items: [
      'Max tokens per session',
      'Max stream duration (SSE)',
      'Max requests per minute',
      'Runaway loop detection',
      '"Kill switch" for bad sessions',
    ],
  },
  {
    label: 'Cost Control',
    tag: 'API',
    tagColor: 'bg-orange-50 text-orange-700',
    items: [
      'Monthly budget cap',
      'Per-model caps',
      'Auto downgrade (Opus → Sonnet)',
      'Block expensive endpoints',
    ],
  },
  {
    label: 'Efficiency Insights',
    tag: 'Subscription-first',
    tagColor: 'bg-emerald-50 text-emerald-700',
    items: [
      'Most expensive prompts',
      'Context bloat detection',
      'Repeated calls / waste patterns',
      'Recommendations to reduce caps',
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
                  <li key={item} className="flex items-start gap-2.5 text-sm text-gray-600">
                    <span aria-hidden="true" className="mt-0.5 shrink-0 text-blue-600">✓</span>
                    <span>{item}</span>
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
