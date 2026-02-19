const plans = [
  {
    name: 'Free',
    price: '$0',
    period: 'forever',
    desc: 'For individual devs getting started.',
    features: [
      'Basic usage dashboard',
      'Token tracking + repo breakdown',
      '7-day history',
      '1 API key',
    ],
    href: 'https://app.tokengate.to/sign-up',
    highlight: false,
  },
  {
    name: 'Pro',
    price: '$9',
    period: '/month',
    desc: 'For power users who need guardrails.',
    features: [
      'Everything in Free',
      'Usage alerts',
      'Runaway protection',
      'Quota simulation (subscription users)',
      '30-day history',
      '5 API keys',
    ],
    href: 'https://app.tokengate.to/sign-up?plan=pro',
    highlight: true,
  },
  {
    name: 'Power',
    price: '$19',
    period: '/month',
    desc: 'For teams and API-heavy workflows.',
    features: [
      'Everything in Pro',
      'Budget caps + hard stops (API users)',
      'Model routing rules',
      'Multi-project policies',
      'Webhooks + advanced analytics',
      'Unlimited API keys',
    ],
    href: 'https://app.tokengate.to/sign-up?plan=power',
    highlight: false,
  },
]

export default function Pricing() {
  return (
    <section id="pricing" aria-labelledby="pricing-heading" className="py-20 sm:py-24 bg-white">
      <div className="mx-auto max-w-6xl px-4 sm:px-6">
        <div className="text-center mb-14">
          <p className="text-sm font-semibold text-blue-600 uppercase tracking-widest mb-3">Pricing</p>
          <h2 id="pricing-heading" className="text-3xl sm:text-4xl font-bold text-gray-900">
            Simple, transparent pricing
          </h2>
          <p className="text-gray-500 mt-3">Start free. Upgrade when you need more.</p>
        </div>

        <div className="grid md:grid-cols-3 gap-6 items-start">
          {plans.map((plan) => (
            <div
              key={plan.name}
              className={`relative rounded-2xl p-8 ${
                plan.highlight
                  ? 'border-2 border-blue-600 bg-white shadow-xl shadow-blue-100'
                  : 'border border-gray-200 bg-white'
              }`}
            >
              {plan.highlight && (
                <div className="absolute -top-3.5 left-1/2 -translate-x-1/2">
                  <span className="rounded-full bg-blue-600 px-4 py-1 text-xs font-bold text-white uppercase tracking-wide">
                    Most Popular
                  </span>
                </div>
              )}

              <div className="mb-6">
                <h3 className="text-lg font-bold text-gray-900 mb-1">{plan.name}</h3>
                <p className="text-gray-500 text-sm mb-4">{plan.desc}</p>
                <div className="flex items-baseline gap-1">
                  <span className="text-4xl font-bold text-gray-900">{plan.price}</span>
                  <span className="text-gray-500 text-sm">{plan.period}</span>
                </div>
              </div>

              <ul className="space-y-3 mb-8" aria-label={`${plan.name} plan features`}>
                {plan.features.map((f) => (
                  <li key={f} className="flex items-start gap-2.5 text-sm text-gray-600">
                    <span aria-hidden="true" className="mt-0.5 shrink-0 text-blue-600">✓</span>
                    <span>{f}</span>
                  </li>
                ))}
              </ul>

              <a
                href={plan.href}
                className={`block w-full rounded-lg py-3 text-center text-sm font-semibold transition-colors ${
                  plan.highlight
                    ? 'bg-blue-600 text-white hover:bg-blue-700'
                    : 'border border-gray-200 text-gray-900 hover:bg-gray-50'
                }`}
              >
                Start Free
              </a>
            </div>
          ))}
        </div>

        <p className="text-center mt-8 text-sm text-gray-400">
          Need enterprise features or custom limits?{' '}
          <a href="mailto:sales@tokengate.to" className="text-blue-600 hover:underline">
            Contact us
          </a>
        </p>
      </div>
    </section>
  )
}
