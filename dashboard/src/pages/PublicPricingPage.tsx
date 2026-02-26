import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '@clerk/clerk-react'
import Navbar from '../components/Navbar'
import { useUserSync } from '../hooks/useUserSync'
import './PublicPricingPage.css'

const API_BASE = import.meta.env.VITE_API_SERVER_URL || 'http://localhost:8080'

// ─── Types ────────────────────────────────────────────────────────────────────

type PlanKey = 'free' | 'pro' | 'team' | 'business'

interface PlanFeature {
  text: string
  comingSoon?: boolean
}

interface PlanCard {
  key: PlanKey
  label: string
  tagline: string
  monthlyPrice: number | null   // null = contact sales
  annualMonthly: number | null  // monthly-equivalent when billed annually
  annualTotal: number | null    // total annual charge
  annualSaving: number | null
  description: string
  features: PlanFeature[]
  limits: string[]              // shown with a muted "not included" style
  bestFor: string[]
  popular?: boolean
  contactSales?: boolean
}

// ─── Plan definitions ─────────────────────────────────────────────────────────

const PLAN_CARDS: PlanCard[] = [
  {
    key: 'free',
    label: 'Free',
    tagline: 'Visibility',
    monthlyPrice: 0,
    annualMonthly: 0,
    annualTotal: 0,
    annualSaving: null,
    description: 'For individual developers who want basic visibility into their AI usage.',
    features: [
      { text: 'Single user' },
      { text: 'Anthropic provider' },
      { text: 'Per-request token & cost tracking' },
      { text: 'Breakdown by model & API key' },
      { text: 'Daily cost & token trend charts' },
      { text: 'Projected monthly spend' },
      { text: 'Soft budget alerts' },
      { text: '7-day data retention' },
    ],
    limits: [
      'Up to $200 monitored spend / month',
      'No hard budget enforcement',
      'No API access',
      'No team support',
    ],
    bestFor: ['Solo developers', 'Getting started'],
  },
  {
    key: 'pro',
    label: 'Pro',
    tagline: 'Personal Control',
    monthlyPrice: 15,
    annualMonthly: 10,
    annualTotal: 120,
    annualSaving: 60,
    description: 'For power users who actively use Claude Code and want real cost control.',
    features: [
      { text: 'Everything in Free, plus:' },
      { text: 'Multiple providers (Anthropic, OpenAI)' },
      { text: 'Multiple API keys' },
      { text: 'Hard budget enforcement — auto-block at limit' },
      { text: 'Daily, weekly & monthly budget caps' },
      { text: 'Per-provider budget scoping' },
      { text: 'Rate limiting (RPM / ITPM / OTPM)' },
      { text: 'Alert thresholds + warning headers' },
      { text: 'Cache hit rate & savings', comingSoon: true },
      { text: 'Usage cap forecasting', comingSoon: true },
      { text: 'Cost per session breakdown', comingSoon: true },
      { text: 'Model cost-efficiency scoring', comingSoon: true },
      { text: 'Slack notifications', comingSoon: true },
      { text: '90-day data retention' },
      { text: 'CSV export' },
    ],
    limits: [],
    bestFor: ['Heavy Claude Code users', 'Indie developers', 'AI tool builders'],
  },
  {
    key: 'team',
    label: 'Team',
    tagline: 'Shared Governance',
    monthlyPrice: 39,
    annualMonthly: 33,
    annualTotal: 400,
    annualSaving: 68,
    description: 'For small teams sharing AI usage and budgets across projects.',
    features: [
      { text: 'Everything in Pro, plus:' },
      { text: 'Up to 10 team members' },
      { text: 'Role-based access (Owner / Admin / Member / Viewer)' },
      { text: 'Per-API-key budget scoping' },
      { text: 'Per-key & per-model rate limits' },
      { text: 'Audit logs' },
      { text: '180-day data retention' },
      { text: 'Wasted spend detection', comingSoon: true },
      { text: 'Cost attribution by project / repo', comingSoon: true },
      { text: 'Per-member efficiency benchmarks', comingSoon: true },
      { text: 'Peak usage heatmap', comingSoon: true },
      { text: 'Read-only usage API', comingSoon: true },
      { text: 'Webhook support (budget alerts)', comingSoon: true },
    ],
    limits: [],
    bestFor: ['Startup teams', 'AI-native product teams', 'Shared API key environments'],
    popular: true,
  },
  {
    key: 'business',
    label: 'Business',
    tagline: 'Enterprise Policy & Compliance',
    monthlyPrice: null,
    annualMonthly: null,
    annualTotal: null,
    annualSaving: null,
    description: 'For companies that need governance, compliance, and enterprise-grade scale.',
    features: [
      { text: 'Everything in Team, plus:' },
      { text: 'Unlimited team members' },
      { text: 'Advanced RBAC & fine-grained permissions', comingSoon: true },
      { text: 'Model allowlists / blocklists', comingSoon: true },
      { text: 'Spend velocity alerts', comingSoon: true },
      { text: 'Key rotation tracking & full audit logs' },
      { text: '1+ year data retention' },
      { text: 'Priority support + SLA' },
      { text: 'SSO (Google / SAML)' },
      { text: 'Dedicated onboarding' },
    ],
    limits: [],
    bestFor: ['AI product companies', 'Multi-team enterprises', 'Compliance-sensitive environments'],
    contactSales: true,
  },
]

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function PublicPricingPage() {
  const navigate = useNavigate()
  const { isSignedIn } = useAuth()
  const { userId, role, isSynced } = useUserSync()
  const [currentPlan, setCurrentPlan] = useState<PlanKey | null>(null)
  const [switching, setSwitching]     = useState<PlanKey | null>(null)
  const [flash, setFlash]             = useState<{ type: 'success' | 'error'; msg: string } | null>(null)
  const [annual, setAnnual]           = useState(false)

  const isOwner = isSynced && role === 'owner'

  useEffect(() => {
    if (!isOwner || !userId) return
    fetch(`${API_BASE}/v1/owner/settings`, { headers: { 'X-User-ID': userId } })
      .then(r => r.json())
      .then(d => setCurrentPlan(d.plan ?? null))
      .catch(() => {})
  }, [isOwner, userId])

  async function handleSelectPlan(plan: PlanKey) {
    if (!isSignedIn) { navigate('/sign-up'); return }
    if (!isOwner || !userId || plan === currentPlan) return

    setSwitching(plan)
    setFlash(null)
    try {
      const res = await fetch(`${API_BASE}/v1/owner/plan`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json', 'X-User-ID': userId },
        body: JSON.stringify({ plan }),
      })
      const data = await res.json()
      if (!res.ok) throw new Error(data.message ?? data.error ?? `HTTP ${res.status}`)
      const next = (data.plan ?? plan) as PlanKey
      setCurrentPlan(next)
      setFlash({ type: 'success', msg: `Successfully switched to the ${next.charAt(0).toUpperCase() + next.slice(1)} plan!` })
    } catch (err) {
      setFlash({ type: 'error', msg: err instanceof Error ? err.message : 'Failed to switch plan' })
    } finally {
      setSwitching(null)
    }
  }

  function buttonLabel(card: PlanCard): string {
    if (card.contactSales) return 'Contact Sales'
    if (!isSignedIn) return card.key === 'free' ? 'Get Started Free' : 'Get Started'
    if (isOwner && card.key === currentPlan) return 'Current Plan'
    if (switching === card.key) return 'Switching…'
    if (isOwner) return `Switch to ${card.label}`
    return card.key === 'free' ? 'Get Started Free' : 'Get Started'
  }

  function handleClick(card: PlanCard) {
    if (card.contactSales) { window.location.href = 'mailto:sales@tokengate.to'; return }
    if (!isSignedIn) { navigate('/sign-up'); return }
    if (isOwner && card.key !== currentPlan) handleSelectPlan(card.key)
  }

  function isDisabled(card: PlanCard): boolean {
    if (card.contactSales) return false
    if (!isSignedIn) return false
    if (!isOwner) return true
    if (card.key === currentPlan) return true
    return switching !== null
  }

  return (
    <div className="page-container">
      <Navbar />
      <div className="pub-pricing-wrapper">

        {/* Header */}
        <div className="pub-pricing-header">
          <p className="pub-pricing-eyebrow">Pricing</p>
          <h1>Simple, Transparent Pricing</h1>
          <p>Start free. Upgrade when your team or usage demands it.</p>

          {/* Positioning tagline */}
          <p className="pub-pricing-positioning">
            <span>Free → Visibility</span>
            <span className="pub-pricing-dot" aria-hidden="true">·</span>
            <span>Pro → Personal Control</span>
            <span className="pub-pricing-dot" aria-hidden="true">·</span>
            <span>Team → Shared Governance</span>
            <span className="pub-pricing-dot" aria-hidden="true">·</span>
            <span>Business → Enterprise Policy</span>
          </p>

          {isSignedIn && currentPlan && (
            <p className="pub-pricing-current-note">
              You're currently on the{' '}
              <strong>{currentPlan.charAt(0).toUpperCase() + currentPlan.slice(1)}</strong> plan.
            </p>
          )}

          {/* Billing toggle */}
          <div className="pub-billing-toggle">
            <button
              className={`pub-billing-btn ${!annual ? 'pub-billing-btn-active' : ''}`}
              onClick={() => setAnnual(false)}
            >
              Monthly
            </button>
            <button
              className={`pub-billing-btn ${annual ? 'pub-billing-btn-active' : ''}`}
              onClick={() => setAnnual(true)}
            >
              Annual
              <span className="pub-billing-save-badge">Save up to 33%</span>
            </button>
          </div>
        </div>

        {flash && (
          <div className={`flash ${flash.type === 'success' ? 'flash-success' : 'flash-error'} pub-pricing-flash`}>
            {flash.msg}
          </div>
        )}

        {/* Cards grid */}
        <div className="pub-pricing-grid">
          {PLAN_CARDS.map(card => {
            const isCurrent = isOwner && card.key === currentPlan
            return (
              <div
                key={card.key}
                className={[
                  'pub-pricing-card',
                  card.popular   ? 'pub-pricing-card-popular'  : '',
                  isCurrent      ? 'pub-pricing-card-current'  : '',
                  card.contactSales ? 'pub-pricing-card-business' : '',
                ].join(' ').trim()}
              >
                {/* Badge */}
                {card.popular && !isCurrent && (
                  <div className="pub-pricing-badge">Most Popular</div>
                )}
                {isCurrent && (
                  <div className="pub-pricing-badge pub-pricing-badge-current">Current Plan</div>
                )}

                {/* Plan name + tagline */}
                <div className="pub-pricing-card-header">
                  <h2 className="pub-pricing-plan-name">{card.label}</h2>
                  <p className="pub-pricing-tagline">{card.tagline}</p>
                  <p className="pub-pricing-description">{card.description}</p>
                </div>

                {/* Price */}
                <div className="pub-pricing-price">
                  {card.contactSales ? (
                    <>
                      <span className="pub-pricing-amount">Contact Sales</span>
                      <span className="pub-pricing-starting">Starting at $199 / month</span>
                    </>
                  ) : card.monthlyPrice === 0 ? (
                    <>
                      <div className="pub-pricing-amount-row">
                        <span className="pub-pricing-currency">$</span>
                        <span className="pub-pricing-amount">0</span>
                        <span className="pub-pricing-period">/ month</span>
                      </div>
                      <span className="pub-pricing-sub">Free forever</span>
                    </>
                  ) : (
                    <>
                      <div className="pub-pricing-amount-row">
                        <span className="pub-pricing-currency">$</span>
                        <span className="pub-pricing-amount">
                          {annual ? card.annualMonthly : card.monthlyPrice}
                        </span>
                        <span className="pub-pricing-period">/ mo</span>
                      </div>
                      {annual && card.annualTotal ? (
                        <span className="pub-pricing-sub pub-pricing-annual-note">
                          Billed ${card.annualTotal} / year
                          <span className="pub-pricing-saving">· Save ${card.annualSaving}</span>
                        </span>
                      ) : (
                        <span className="pub-pricing-sub">Billed monthly</span>
                      )}
                    </>
                  )}
                </div>

                {/* Features */}
                <ul className="pub-pricing-features">
                  {card.features.map((f, i) => (
                    <li
                      key={i}
                      className={`pub-pricing-feature ${i === 0 && f.text.includes('plus:') ? 'pub-pricing-feature-inherits' : ''}`}
                    >
                      {!(i === 0 && f.text.includes('plus:')) && (
                        <svg className="pub-pricing-check-icon" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                          <path d="M3 8l3.5 3.5L13 4.5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
                        </svg>
                      )}
                      <span>
                        {f.text}
                        {f.comingSoon && (
                          <span className="pub-pricing-coming-soon">Coming Soon</span>
                        )}
                      </span>
                    </li>
                  ))}

                  {/* Limits (muted) */}
                  {card.limits.map((l, i) => (
                    <li key={`limit-${i}`} className="pub-pricing-feature pub-pricing-limit">
                      <svg className="pub-pricing-limit-icon" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                        <circle cx="8" cy="8" r="5.5" stroke="currentColor" strokeWidth="1.5"/>
                        <path d="M6 10l4-4M6 6l4 4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
                      </svg>
                      <span>{l}</span>
                    </li>
                  ))}
                </ul>

                {/* CTA */}
                <button
                  className={`pub-pricing-btn ${
                    card.popular || isCurrent ? 'pub-pricing-btn-primary' :
                    card.contactSales         ? 'pub-pricing-btn-outline'  :
                                               'pub-pricing-btn-secondary'
                  }`}
                  onClick={() => handleClick(card)}
                  disabled={isDisabled(card)}
                >
                  {buttonLabel(card)}
                </button>

                {/* Best for */}
                {card.bestFor.length > 0 && (
                  <div className="pub-pricing-best-for">
                    <span className="pub-pricing-best-for-label">Best for</span>
                    <div className="pub-pricing-best-for-chips">
                      {card.bestFor.map(b => (
                        <span key={b} className="pub-pricing-chip">{b}</span>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>

        {/* Footer */}
        <div className="pub-pricing-footer">
          <p>Free plan is free forever. No credit card required.</p>
          {isSignedIn && !isOwner && (
            <p>Contact your account owner to change your plan.</p>
          )}
          <p>
            Questions?{' '}
            <a href="mailto:sales@tokengate.to" className="pub-pricing-contact-link">
              Talk to us
            </a>
          </p>
        </div>
      </div>
    </div>
  )
}
