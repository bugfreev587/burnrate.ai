import { Fragment, useState } from 'react'
import { Link } from 'react-router-dom'

// ─── Plan definitions ────────────────────────────────────────────────────────

type PlanFeature = { text: string; comingSoon?: boolean }
type PlanKey = 'free' | 'pro' | 'team' | 'business'
type ComparisonValue = boolean | string
type ComparisonRow = {
  feature: string
  values: Record<PlanKey, ComparisonValue>
}
type ComparisonCategory = {
  category: string
  rows: ComparisonRow[]
}

const plans: { key: PlanKey; name: string; tagline: string; monthlyPrice: number | null; annualMonthly: number | null; annualTotal: number | null; annualSaving?: number | null; desc: string; features: PlanFeature[]; limit: string | null; cta: string; to: string; highlight: boolean; contactSales: boolean }[] = [
  {
    key: 'free',
    name: 'Free',
    tagline: 'Visibility',
    monthlyPrice: 0,
    annualMonthly: 0,
    annualTotal: 0,
    desc: 'For individual developers who want basic visibility into their AI usage.',
    features: [
      { text: 'Single user' },
      { text: 'Single gateway api-key' },
      { text: 'Single LLM provider' },
      { text: 'Cost and token usage overview' },
      { text: 'Per-request token & cost tracking' },
      { text: 'Breakdown by model, API key and provider' },
      { text: 'Daily cost & token trend charts' },
      { text: 'Single spend limit' },
      { text: 'Single rate limit or token limit' },
      { text: 'Alert threshold + warning header' },
      { text: 'Single Slack or email notifications' },
      { text: '7-day data retention' },
    ],
    limit: null,
    cta: 'Start for free',
    to: '/sign-up',
    highlight: false,
    contactSales: false,
  },
  {
    key: 'pro',
    name: 'Pro',
    tagline: 'Personal Control',
    monthlyPrice: 15,
    annualMonthly: 10,
    annualTotal: 120,
    annualSaving: 60,
    desc: 'For power users who actively use Claude Code and want real cost control.',
    features: [
      { text: 'Everything in Free' },
      { text: 'Multiple LLM providers (Anthropic, OpenAI, ...)' },
      { text: 'Multiple gateway API keys' },
      { text: 'Per-provider budget scoping' },
      { text: 'Multiple Rate limiting (RPM / ITPM / OTPM)' },
      { text: 'Hard budget / rate enforcement — auto-block at limit' },
      { text: '90-day data retention' },
      { text: 'Audit report export' },
      { text: 'Cache hit rate & savings', comingSoon: true },
      { text: 'Usage cap forecasting', comingSoon: true },
      { text: 'Model cost-efficiency scoring', comingSoon: true },
    ],
    limit: null,
    cta: 'Get started',
    to: '/sign-up?plan=pro',
    highlight: false,
    contactSales: false,
  },
  {
    key: 'team',
    name: 'Team',
    tagline: 'Shared Governance',
    monthlyPrice: 39,
    annualMonthly: 33,
    annualTotal: 400,
    annualSaving: 68,
    desc: 'For small teams sharing AI usage and budgets across projects.',
    features: [
      { text: 'Everything in Pro' },
      { text: 'Multiple team members' },
      { text: 'Role-based access (Owner / Admin / Member / Viewer)' },
      { text: 'Per-key & per-model rate limits' },
      { text: 'More spend & rate limits' },
      { text: 'Webhook support (budget alerts)'},
      { text: '180-day data retention' },
      { text: 'Audit logs', comingSoon: true },
      { text: 'Wasted spend detection', comingSoon: true },
      { text: 'Cost attribution by project / repo', comingSoon: true },
      { text: 'Per-member efficiency benchmarks', comingSoon: true },
      { text: 'Peak usage heatmap', comingSoon: true },
    ],
    limit: null,
    cta: 'Get started',
    to: '/sign-up?plan=team',
    highlight: true,
    contactSales: false,
  },
  {
    key: 'business',
    name: 'Business',
    tagline: 'Enterprise Policy & Compliance',
    monthlyPrice: null,
    annualMonthly: null,
    annualTotal: null,
    annualSaving: null,
    desc: 'For companies that need governance, compliance, and enterprise-grade scale.',
    features: [
      { text: 'Everything in Team' },
      { text: 'Unlimited team members' },
      { text: 'Unlimited gateway api-keys' },
      { text: 'Unlimited spend & rate limits' },
      { text: 'Key rotation tracking & full audit logs' },
      { text: '1+ year data retention' },
      { text: 'Priority support + SLA' },
      { text: 'SSO (Google / GitHub)' },
      { text: 'SAML SSO', comingSoon: true },
      { text: 'Dedicated onboarding' },
      { text: 'Advanced RBAC & fine-grained permissions' },
      { text: 'Model allowlists / blocklists', comingSoon: true },
      { text: 'Spend velocity alerts', comingSoon: true },
    ],
    limit: null,
    cta: 'Contact Sales',
    to: 'mailto:sales@tokengate.to',
    highlight: false,
    contactSales: true,
  },
]

const comparisonCategories: ComparisonCategory[] = [
  {
    category: 'Core',
    rows: [
      { feature: 'API Gateway access', values: { free: false, pro: true, team: true, business: true } },
      { feature: 'Claude Code support', values: { free: true, pro: true, team: true, business: true } },
      { feature: 'VS Code extension support', values: { free: true, pro: true, team: true, business: true } },
      { feature: 'OpenAI / Anthropic support', values: { free: false, pro: true, team: true, business: true } },
    ],
  },
  {
    category: 'Governance',
    rows: [
      { feature: 'Spend limits', values: { free: true, pro: true, team: true, business: true } },
      { feature: 'Rate limits', values: { free: true, pro: true, team: true, business: true } },
      { feature: 'Project-level isolation', values: { free: false, pro: false, team: true, business: true } },
      { feature: 'Model allowlist', values: { free: false, pro: false, team: false, business: true } },
      { feature: 'API key-level budgets', values: { free: false, pro: false, team: true, business: true } },
    ],
  },
  {
    category: 'Team & Security',
    rows: [
      { feature: 'Multi-user support', values: { free: false, pro: false, team: true, business: true } },
      { feature: 'Role-based access control (RBAC)', values: { free: false, pro: false, team: true, business: true } },
      { feature: 'Audit logs', values: { free: false, pro: false, team: true, business: true } },
      { feature: 'Data retention period', values: { free: '7 days', pro: '90 days', team: '180 days', business: '1+ year' } },
      { feature: 'SSO', values: { free: false, pro: false, team: false, business: true } },
    ],
  },
  {
    category: 'Billing',
    rows: [
      { feature: 'Monthly subscription', values: { free: true, pro: true, team: true, business: true } },
      { feature: 'API usage-based billing', values: { free: true, pro: true, team: true, business: true } },
      { feature: 'Invoice & statement download', values: { free: false, pro: true, team: true, business: true } },
    ],
  },
  {
    category: 'Limits',
    rows: [
      { feature: 'Max API keys', values: { free: '1', pro: '5', team: '50', business: 'Unlimited' } },
      { feature: 'Max provider keys', values: { free: '1', pro: '3', team: '10', business: 'Unlimited' } },
      { feature: 'Max projects', values: { free: '1', pro: '5', team: '20', business: 'Unlimited' } },
      { feature: 'Max spend / rate limit rules', values: { free: '1', pro: '5', team: '20', business: 'Unlimited' } },
    ],
  },
]

// ─── Component ───────────────────────────────────────────────────────────────

export default function LandingPricing() {
  const [annual, setAnnual] = useState(false)

  return (
    <section id="pricing" aria-labelledby="pricing-heading" className="py-20 sm:py-24 bg-gray-50">
      <div className="mx-auto max-w-7xl px-4 sm:px-6">

        {/* Header */}
        <div className="text-center mb-12">
          <p className="text-sm font-semibold text-blue-600 uppercase tracking-widest mb-3">Pricing</p>
          <h2 id="pricing-heading" className="text-3xl sm:text-4xl font-bold text-gray-900">
            Simple, transparent pricing
          </h2>
          <p className="text-gray-500 mt-3 text-lg">
            Start free. Upgrade when you need more control.
          </p>

          {/* Positioning tagline */}
          <p className="mt-4 text-sm text-gray-400 font-medium tracking-wide">
            Free&nbsp;→&nbsp;Visibility&nbsp;&nbsp;·&nbsp;&nbsp;
            Pro&nbsp;→&nbsp;Personal Control&nbsp;&nbsp;·&nbsp;&nbsp;
            Team&nbsp;→&nbsp;Shared Governance&nbsp;&nbsp;·&nbsp;&nbsp;
            Business&nbsp;→&nbsp;Enterprise Policy
          </p>

          {/* Billing toggle */}
          <div className="inline-flex items-center gap-3 mt-8 bg-white border border-gray-200 rounded-full px-4 py-2 shadow-sm">
            <button
              onClick={() => setAnnual(false)}
              className={`text-sm font-semibold px-3 py-1 rounded-full transition-colors ${
                !annual ? 'bg-blue-600 text-white' : 'text-gray-500 hover:text-gray-800'
              }`}
            >
              Monthly
            </button>
            <button
              onClick={() => setAnnual(true)}
              className={`text-sm font-semibold px-3 py-1 rounded-full transition-colors ${
                annual ? 'bg-blue-600 text-white' : 'text-gray-500 hover:text-gray-800'
              }`}
            >
              Annual
              <span className="ml-1.5 text-[10px] font-bold bg-green-100 text-green-700 rounded px-1.5 py-0.5 uppercase tracking-wide">
                Save up to 33%
              </span>
            </button>
          </div>
        </div>

        {/* Cards */}
        <div className="grid md:grid-cols-2 xl:grid-cols-4 gap-6 items-stretch">
          {plans.map((plan) => (
            <div
              key={plan.key}
              className={`relative flex h-full flex-col rounded-2xl p-7 transition-shadow ${
                plan.highlight
                  ? 'border-2 border-blue-600 bg-white shadow-2xl shadow-blue-100 ring-1 ring-blue-600/10'
                  : 'border border-gray-200 bg-white shadow-sm hover:shadow-md'
              }`}
            >
              {/* Most Popular badge */}
              {plan.highlight && (
                <div className="absolute -top-3.5 left-1/2 -translate-x-1/2">
                  <span className="rounded-full bg-blue-600 px-4 py-1 text-xs font-bold text-white uppercase tracking-wide shadow-sm">
                    Most Popular
                  </span>
                </div>
              )}

              {/* Plan name + tagline */}
              <div className="mb-5 min-h-[128px]">
                <h3 className="text-xl font-bold text-gray-900">{plan.name}</h3>
                <p className="text-xs font-semibold text-blue-600 uppercase tracking-widest mt-0.5">
                  {plan.tagline}
                </p>
                <p className="text-gray-500 text-sm mt-2 leading-relaxed">{plan.desc}</p>
              </div>

              {/* Price */}
              <div className="mb-6 pb-6 border-b border-gray-100">
                {plan.contactSales ? (
                  <div>
                    <div className="text-3xl font-bold text-gray-900">Contact Sales</div>
                    <p className="text-xs text-gray-400 mt-1">Starting at $199 / month</p>
                  </div>
                ) : plan.monthlyPrice === 0 ? (
                  <div>
                    <div className="flex items-baseline gap-1">
                      <span className="text-4xl font-bold text-gray-900">$0</span>
                      <span className="text-gray-400 text-sm">/ month</span>
                    </div>
                    <p className="text-xs text-gray-400 mt-1">Free forever</p>
                  </div>
                ) : (
                  <div>
                    <div className="flex items-baseline gap-1">
                      <span className="text-4xl font-bold text-gray-900">
                        ${annual ? plan.annualMonthly : plan.monthlyPrice}
                      </span>
                      <span className="text-gray-400 text-sm">/ mo</span>
                    </div>
                    {annual && plan.annualTotal ? (
                      <p className="text-xs text-gray-400 mt-1">
                        Billed ${plan.annualTotal} / year&nbsp;
                        <span className="text-green-600 font-semibold">
                          · Save ${plan.annualSaving}
                        </span>
                      </p>
                    ) : (
                      <p className="text-xs text-gray-400 mt-1">
                        Billed monthly
                      </p>
                    )}
                  </div>
                )}
              </div>

              {/* Features */}
              <ul className="space-y-2.5 mb-8 min-h-[320px] flex-grow" aria-label={`${plan.name} plan features`}>
                {plan.features.map((f) => (
                  <li key={f.text} className="flex items-start gap-2 text-sm text-gray-600">
                    <svg className="mt-0.5 shrink-0 h-4 w-4 text-blue-500" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                      <path d="M3 8l3.5 3.5L13 4.5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
                    </svg>
                    <span>
                      {f.text}
                      {f.comingSoon && (
                        <span className="ml-1.5 inline-flex items-center rounded-full bg-amber-50 px-2 py-0.5 text-[10px] font-semibold text-amber-600 border border-amber-200">
                          Coming Soon
                        </span>
                      )}
                    </span>
                  </li>
                ))}
                {plan.limit && (
                  <li className="flex items-start gap-2 text-sm text-gray-400 mt-1 pt-2 border-t border-gray-100">
                    <svg className="mt-0.5 shrink-0 h-4 w-4" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                      <circle cx="8" cy="8" r="6" stroke="currentColor" strokeWidth="1.5"/>
                      <path d="M6 10l4-4M6 6l4 4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
                    </svg>
                    <span>{plan.limit}</span>
                  </li>
                )}
              </ul>

              {/* CTA */}
              <div className="mt-auto">
                {plan.contactSales ? (
                  <a
                    href={plan.to}
                    className="block w-full rounded-xl py-3 text-center text-sm font-semibold border border-gray-200 text-gray-900 hover:bg-gray-50 transition-colors"
                  >
                    {plan.cta}
                  </a>
                ) : (
                  <Link
                    to={plan.to}
                    className={`block w-full rounded-xl py-3 text-center text-sm font-semibold transition-colors ${
                      plan.highlight
                        ? 'bg-blue-600 text-white hover:bg-blue-700 shadow-sm'
                        : plan.monthlyPrice === 0
                        ? 'border border-gray-200 text-gray-900 hover:bg-gray-50'
                        : 'border border-blue-200 text-blue-700 bg-blue-50 hover:bg-blue-100'
                    }`}
                  >
                    {plan.cta}
                  </Link>
                )}
              </div>
            </div>
          ))}
        </div>

        {/* Comparison table */}
        <div className="mt-14 rounded-2xl border border-gray-200 bg-white shadow-sm overflow-hidden">
          <div className="border-b border-gray-100 px-6 py-5">
            <h3 className="text-xl font-bold text-gray-900">Compare all plan features</h3>
            <p className="mt-1 text-sm text-gray-500">Quickly scan what is included across Free, Pro, Team, and Business.</p>
          </div>

          <div className="overflow-x-auto">
            <table className="w-full min-w-[760px] border-collapse">
              <thead className="bg-gray-50">
                <tr>
                  <th className="sticky left-0 z-10 bg-gray-50 px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Feature</th>
                  <th className="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wide text-gray-500">Free</th>
                  <th className="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wide text-gray-500">Pro</th>
                  <th className="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wide text-gray-500">Team</th>
                  <th className="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wide text-gray-500">Business</th>
                </tr>
              </thead>
              <tbody>
                {comparisonCategories.map((section) => (
                  <Fragment key={section.category}>
                    <tr className="bg-blue-50/50">
                      <th colSpan={5} className="px-4 py-2.5 text-left text-xs font-semibold uppercase tracking-wide text-blue-700">
                        {section.category}
                      </th>
                    </tr>
                    {section.rows.map((row) => (
                      <tr key={`${section.category}-${row.feature}`} className="border-t border-gray-100">
                        <th className="sticky left-0 bg-white px-4 py-3 text-left text-sm font-medium text-gray-700">
                          {row.feature}
                        </th>
                        {(['free', 'pro', 'team', 'business'] as const).map((planKey) => {
                          const value = row.values[planKey]
                          return (
                            <td key={`${row.feature}-${planKey}`} className="px-4 py-3 text-center text-sm text-gray-700">
                              {typeof value === 'boolean' ? (
                                value ? (
                                  <span className="inline-flex h-6 w-6 items-center justify-center rounded-full bg-emerald-50 text-emerald-600" aria-label="Included">
                                    ✓
                                  </span>
                                ) : (
                                  <span className="text-gray-300" aria-label="Not included">—</span>
                                )
                              ) : (
                                <span className="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-700">
                                  {value}
                                </span>
                              )}
                            </td>
                          )
                        })}
                      </tr>
                    ))}
                  </Fragment>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        {/* Footer note */}
        <p className="text-center mt-10 text-sm text-gray-400">
          Free plan is free forever. No credit card required.{' '}
          <a href="mailto:sales@tokengate.to" className="text-blue-600 hover:underline">
            Questions? Talk to us.
          </a>
        </p>
      </div>
    </section>
  )
}
