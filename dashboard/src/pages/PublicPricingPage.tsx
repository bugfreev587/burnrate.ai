import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '@clerk/clerk-react'
import Navbar from '../components/Navbar'
import { useUserSync } from '../hooks/useUserSync'
import './PublicPricingPage.css'

const API_BASE = import.meta.env.VITE_API_SERVER_URL || 'http://localhost:8080'

// ─── Types ────────────────────────────────────────────────────────────────

type PlanKey = 'free' | 'pro' | 'team' | 'business'

interface PlanCard {
  key: PlanKey
  label: string
  price: string
  period: string
  description: string
  features: string[]
  popular?: boolean
  contactSales?: boolean
}

// ─── Plan definitions ─────────────────────────────────────────────────────

const PLAN_CARDS: PlanCard[] = [
  {
    key: 'free',
    label: 'Free',
    price: '0',
    period: '/month',
    description: 'Perfect for individuals exploring AI cost monitoring.',
    features: [
      '1 API key',
      '1 team member',
      'Monthly budgets',
      '30-day data retention',
      'Basic usage tracking',
    ],
  },
  {
    key: 'pro',
    label: 'Pro',
    price: '15',
    period: '/month',
    description: 'Ideal for power users who need fine-grained spend control.',
    features: [
      '5 API keys',
      '1 team member',
      'Daily, weekly & monthly budgets',
      'Hard spend block (HTTP 402)',
      '90-day data retention',
    ],
  },
  {
    key: 'team',
    label: 'Team',
    price: '29',
    period: '/month',
    description: 'Built for teams collaborating on AI spending.',
    features: [
      'Unlimited API keys',
      'Up to 10 team members',
      'All budget periods',
      'Hard spend block',
      'Per-key budgets',
      '1-year data retention',
    ],
    popular: true,
  },
  {
    key: 'business',
    label: 'Business',
    price: 'Custom',
    period: '',
    description: 'Enterprise-grade for large teams with custom needs.',
    features: [
      'Unlimited API keys',
      'Unlimited team members',
      'All budget periods',
      'Hard spend block',
      'Per-key budgets',
      'Unlimited data retention',
      'Dedicated support',
      'SLA guarantee',
    ],
    contactSales: true,
  },
]

// ─── Page ─────────────────────────────────────────────────────────────────

export default function PublicPricingPage() {
  const navigate = useNavigate()
  const { isSignedIn } = useAuth()
  const { userId, role, isSynced } = useUserSync()
  const [currentPlan, setCurrentPlan] = useState<PlanKey | null>(null)
  const [switching, setSwitching] = useState<PlanKey | null>(null)
  const [flash, setFlash] = useState<{ type: 'success' | 'error'; msg: string } | null>(null)

  const isOwner = isSynced && role === 'owner'

  // Fetch current plan for signed-in owners
  useEffect(() => {
    if (!isOwner || !userId) return
    fetch(`${API_BASE}/v1/owner/settings`, { headers: { 'X-User-ID': userId } })
      .then(r => r.json())
      .then(d => setCurrentPlan(d.plan ?? null))
      .catch(() => {})
  }, [isOwner, userId])

  async function handleSelectPlan(plan: PlanKey) {
    if (!isSignedIn) {
      navigate('/sign-up')
      return
    }
    if (!isOwner || !userId) return
    if (plan === currentPlan) return

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
      setCurrentPlan(data.plan ?? plan)
      setFlash({ type: 'success', msg: `Successfully switched to the ${(data.plan ?? plan).charAt(0).toUpperCase() + (data.plan ?? plan).slice(1)} plan!` })
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
    if (card.contactSales) {
      window.location.href = 'mailto:sales@tokengate.to'
      return
    }
    if (!isSignedIn) {
      navigate('/sign-up')
      return
    }
    if (isOwner && card.key !== currentPlan) {
      handleSelectPlan(card.key)
    }
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
        <div className="pub-pricing-header">
          <h1>Simple, Transparent Pricing</h1>
          <p>Choose the plan that's right for your AI spending needs.</p>
          {isSignedIn && currentPlan && (
            <p className="pub-pricing-current-note">
              You're currently on the <strong>
                {currentPlan.charAt(0).toUpperCase() + currentPlan.slice(1)}
              </strong> plan.
            </p>
          )}
        </div>

        {flash && (
          <div className={`flash ${flash.type === 'success' ? 'flash-success' : 'flash-error'} pub-pricing-flash`}>
            {flash.msg}
          </div>
        )}

        <div className="pub-pricing-grid">
          {PLAN_CARDS.map(card => {
            const isCurrent = isOwner && card.key === currentPlan
            return (
              <div
                key={card.key}
                className={[
                  'pub-pricing-card',
                  card.popular ? 'pub-pricing-card-popular' : '',
                  isCurrent ? 'pub-pricing-card-current' : '',
                ].join(' ').trim()}
              >
                {card.popular && !isCurrent && (
                  <div className="pub-pricing-badge">Most Popular</div>
                )}
                {isCurrent && (
                  <div className="pub-pricing-badge pub-pricing-badge-current">Current Plan</div>
                )}

                <div className="pub-pricing-card-header">
                  <h2 className="pub-pricing-plan-name">{card.label}</h2>
                  <p className="pub-pricing-description">{card.description}</p>
                </div>

                <div className="pub-pricing-price">
                  {card.price === 'Custom' ? (
                    <span className="pub-pricing-amount">Custom</span>
                  ) : (
                    <>
                      <span className="pub-pricing-currency">$</span>
                      <span className="pub-pricing-amount">{card.price}</span>
                      <span className="pub-pricing-period">{card.period}</span>
                    </>
                  )}
                </div>

                <ul className="pub-pricing-features">
                  {card.features.map((f, i) => (
                    <li key={i} className="pub-pricing-feature">
                      <span className="pub-pricing-check">✓</span>
                      {f}
                    </li>
                  ))}
                </ul>

                <button
                  className={`pub-pricing-btn ${card.popular ? 'pub-pricing-btn-primary' : 'pub-pricing-btn-secondary'}`}
                  onClick={() => handleClick(card)}
                  disabled={isDisabled(card)}
                >
                  {buttonLabel(card)}
                </button>
              </div>
            )
          })}
        </div>

        <div className="pub-pricing-footer">
          <p>Free plan is free forever. No credit card required.</p>
          {isSignedIn && !isOwner && (
            <p>Contact your account owner to change your plan.</p>
          )}
        </div>
      </div>
    </div>
  )
}
