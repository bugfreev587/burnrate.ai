import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import Navbar from '../components/Navbar'
import { useUserSync, hasPermission } from '../hooks/useUserSync'
import './BillingPage.css'

const API_BASE = import.meta.env.VITE_API_SERVER_URL || 'http://localhost:8080'

// ─── Types ────────────────────────────────────────────────────────────────

interface PaymentMethod {
  brand: string
  last4: string
  exp_month: number
  exp_year: number
}

interface BillingStatus {
  plan: string
  plan_status: string
  has_subscription: boolean
  billing_email: string
  current_period_end: string | null
  stripe_configured: boolean
  cancel_at_period_end?: boolean
  payment_method?: PaymentMethod
}

interface Invoice {
  id: string
  number: string
  status: string
  amount_due: number
  amount_paid: number
  currency: string
  period_start: number
  period_end: number
  created: number
  pdf_url: string
  hosted_url: string
}

// ─── Helpers ──────────────────────────────────────────────────────────────

function planLabel(plan: string): string {
  return plan.charAt(0).toUpperCase() + plan.slice(1) + ' Plan'
}

function formatDate(ts: number | string | null | undefined): string {
  if (!ts) return '—'
  const d = typeof ts === 'number' ? new Date(ts * 1000) : new Date(ts)
  return d.toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' })
}

function formatCurrency(cents: number, currency: string): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: currency.toUpperCase(),
  }).format(cents / 100)
}

function StatusBadge({ status }: { status: string }) {
  return (
    <span className={`billing-status-badge billing-status-${status}`}>
      {status.replace('_', ' ')}
    </span>
  )
}

function PlanBadge({ plan }: { plan: string }) {
  return (
    <span className={`plan-badge plan-badge-${plan}`}>
      {plan.toUpperCase()}
    </span>
  )
}

function InvoiceStatusBadge({ status }: { status: string }) {
  return (
    <span className={`invoice-status invoice-status-${status}`}>
      {status}
    </span>
  )
}

// ─── Main Page ────────────────────────────────────────────────────────────

export default function BillingPage() {
  const { role, userId, isSynced } = useUserSync()
  const [searchParams, setSearchParams] = useSearchParams()
  const [billingStatus, setBillingStatus] = useState<BillingStatus | null>(null)
  const [invoices, setInvoices] = useState<Invoice[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [actionLoading, setActionLoading] = useState<string | null>(null)
  const [flash, setFlash] = useState<{ type: 'success' | 'error'; msg: string } | null>(null)
  const [refreshTick, setRefreshTick] = useState(0)

  const isAdmin = isSynced && hasPermission(role, 'admin')

  // Handle Stripe return query params
  useEffect(() => {
    if (searchParams.get('success') === 'true') {
      setFlash({ type: 'success', msg: 'Subscription activated! Your plan has been updated.' })
      setSearchParams({}, { replace: true })
      setRefreshTick(t => t + 1)
    } else if (searchParams.get('canceled') === 'true') {
      setFlash({ type: 'error', msg: 'Checkout was canceled. No changes were made.' })
      setSearchParams({}, { replace: true })
    }
  }, [searchParams, setSearchParams])

  // Fetch billing data
  useEffect(() => {
    if (!isSynced || !userId) return

    const headers: Record<string, string> = { 'X-User-ID': userId }

    async function load() {
      setLoading(true)
      setError(null)
      try {
        const [statusRes, invoicesRes] = await Promise.all([
          fetch(`${API_BASE}/v1/billing/status`, { headers }),
          fetch(`${API_BASE}/v1/billing/invoices`, { headers }),
        ])

        if (!statusRes.ok) throw new Error(`Billing status fetch failed: HTTP ${statusRes.status}`)

        const [statusData, invoicesData] = await Promise.all([
          statusRes.json(),
          invoicesRes.ok ? invoicesRes.json() : [],
        ])

        setBillingStatus(statusData)
        setInvoices(Array.isArray(invoicesData) ? invoicesData : [])
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load billing data')
      } finally {
        setLoading(false)
      }
    }

    load()
  }, [isSynced, userId, refreshTick])

  async function handleCheckout(plan: string) {
    if (!userId) return
    setActionLoading('checkout')
    setFlash(null)
    try {
      const res = await fetch(`${API_BASE}/v1/billing/checkout`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-User-ID': userId },
        body: JSON.stringify({
          plan,
          success_url: `${window.location.origin}/billing?success=true`,
          cancel_url: `${window.location.origin}/billing?canceled=true`,
        }),
      })
      const data = await res.json()
      if (!res.ok) throw new Error(data.message ?? data.error ?? `HTTP ${res.status}`)
      window.location.href = data.url
    } catch (err) {
      setFlash({ type: 'error', msg: err instanceof Error ? err.message : 'Failed to start checkout' })
    } finally {
      setActionLoading(null)
    }
  }

  async function handlePortal() {
    if (!userId) return
    setActionLoading('portal')
    setFlash(null)
    try {
      const res = await fetch(`${API_BASE}/v1/billing/portal`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-User-ID': userId },
        body: JSON.stringify({
          return_url: `${window.location.origin}/billing`,
        }),
      })
      const data = await res.json()
      if (!res.ok) throw new Error(data.message ?? data.error ?? `HTTP ${res.status}`)
      window.location.href = data.url
    } catch (err) {
      setFlash({ type: 'error', msg: err instanceof Error ? err.message : 'Failed to open billing portal' })
    } finally {
      setActionLoading(null)
    }
  }

  const currentPlan = billingStatus?.plan ?? 'free'
  const hasSubscription = billingStatus?.has_subscription ?? false
  const stripeConfigured = billingStatus?.stripe_configured ?? false

  return (
    <div className="page-container">
      <Navbar />
      <div className="page-content">
        <div className="billing-page-header">
          <h1>Billing</h1>
          {billingStatus && <PlanBadge plan={currentPlan} />}
        </div>

        {loading && (
          <div className="loading-center">
            <div className="spinner" />
          </div>
        )}

        {error && (
          <div className="flash flash-error">
            {error}
            <button
              className="btn btn-secondary"
              style={{ marginLeft: '1rem' }}
              onClick={() => { setError(null); setRefreshTick(t => t + 1) }}
            >
              Retry
            </button>
          </div>
        )}

        {flash && (
          <div className={`flash ${flash.type === 'success' ? 'flash-success' : 'flash-error'}`}
               style={{ marginBottom: '1rem' }}>
            {flash.msg}
          </div>
        )}

        {!loading && !error && billingStatus && (
          <>
            {/* ── Current Plan Card ── */}
            <div className="card billing-section">
              <p className="billing-section-title">Current Plan</p>
              <div className="billing-plan-hero">
                <h2>{planLabel(currentPlan)}</h2>
                <StatusBadge status={billingStatus.plan_status} />
              </div>

              <div className="billing-details">
                {billingStatus.billing_email && (
                  <div className="detail-row">
                    <span className="detail-label">Billing email</span>
                    <span className="detail-value">{billingStatus.billing_email}</span>
                  </div>
                )}
                {billingStatus.current_period_end && (
                  <div className="detail-row">
                    <span className="detail-label">Next billing date</span>
                    <span className="detail-value">{formatDate(billingStatus.current_period_end)}</span>
                  </div>
                )}
                {billingStatus.cancel_at_period_end && (
                  <div className="detail-row">
                    <span className="detail-label">Status</span>
                    <span className="detail-value" style={{ color: '#fb923c' }}>
                      Cancels at end of billing period
                    </span>
                  </div>
                )}
              </div>

              {/* Payment Method */}
              {billingStatus.payment_method && (
                <div className="payment-method-card" style={{ marginBottom: '1rem' }}>
                  <span className="payment-method-brand">{billingStatus.payment_method.brand}</span>
                  <span className="payment-method-details">
                    ending in {billingStatus.payment_method.last4}
                  </span>
                  <span className="payment-method-details">
                    exp {billingStatus.payment_method.exp_month}/{billingStatus.payment_method.exp_year}
                  </span>
                </div>
              )}

              {/* Action Buttons */}
              {isAdmin && stripeConfigured && (
                <div className="billing-actions">
                  {hasSubscription ? (
                    <button
                      className="btn btn-primary"
                      disabled={actionLoading !== null}
                      onClick={handlePortal}
                    >
                      {actionLoading === 'portal' ? 'Loading...' : 'Manage Subscription'}
                    </button>
                  ) : (
                    <>
                      <button
                        className="btn btn-primary"
                        disabled={actionLoading !== null}
                        onClick={() => handleCheckout('pro')}
                      >
                        {actionLoading === 'checkout' ? 'Loading...' : 'Upgrade to Pro — $15/mo'}
                      </button>
                      <button
                        className="btn btn-secondary"
                        disabled={actionLoading !== null}
                        onClick={() => handleCheckout('team')}
                      >
                        {actionLoading === 'checkout' ? 'Loading...' : 'Upgrade to Team — $39/mo'}
                      </button>
                    </>
                  )}
                </div>
              )}
            </div>

            {/* ── Billing History ── */}
            <div className="card billing-section">
              <p className="billing-section-title">Billing History</p>
              {invoices.length === 0 ? (
                <div className="billing-empty">No invoices yet.</div>
              ) : (
                <div className="billing-table-wrapper">
                  <table className="billing-table">
                    <thead>
                      <tr>
                        <th>Date</th>
                        <th>Invoice</th>
                        <th>Amount</th>
                        <th>Status</th>
                        <th></th>
                      </tr>
                    </thead>
                    <tbody>
                      {invoices.map(inv => (
                        <tr key={inv.id}>
                          <td>{formatDate(inv.created)}</td>
                          <td>{inv.number || '—'}</td>
                          <td>{formatCurrency(inv.amount_due, inv.currency || 'usd')}</td>
                          <td><InvoiceStatusBadge status={inv.status} /></td>
                          <td>
                            {inv.pdf_url && (
                              <a
                                href={inv.pdf_url}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="billing-pdf-link"
                              >
                                PDF
                              </a>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>

            {/* ── Note ── */}
            <div className="billing-note">
              This page shows your TokenGate subscription billing. Token usage costs from LLM providers
              (Anthropic, OpenAI, etc.) are billed separately by those providers and are not reflected here.
            </div>

            {/* ── Upgrade CTA for free plan ── */}
            {currentPlan === 'free' && stripeConfigured && isAdmin && (
              <div className="billing-upgrade-cta" style={{ marginTop: '1.5rem' }}>
                <div className="billing-upgrade-cta-text">
                  <p>Unlock more features</p>
                  <p>Upgrade to Pro or Team for more API keys, team members, and advanced budget controls.</p>
                </div>
                <button
                  className="btn btn-primary"
                  disabled={actionLoading !== null}
                  onClick={() => handleCheckout('pro')}
                >
                  Upgrade Now
                </button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
