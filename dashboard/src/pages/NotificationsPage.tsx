import { useState, useEffect } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { useUserSync, hasPermission } from '../hooks/useUserSync'
import { useNotifications } from '../hooks/useNotifications'
import { useDashboardConfig } from '../hooks/useDashboardConfig'
import type { CreateNotificationChannelReq } from '../hooks/useNotifications'
import Navbar from '../components/Navbar'
import './LimitsPage.css'
import './ManagementPage.css'

const CHANNEL_TYPES = [
  { value: 'email', label: 'Email' },
  { value: 'slack', label: 'Slack' },
  { value: 'webhook', label: 'Webhook' },
]

const EVENT_TYPE_OPTIONS = [
  { value: 'budget_blocked', label: 'Budget Blocked', description: 'When a blocking budget limit is exceeded' },
  { value: 'budget_warning', label: 'Budget Warning', description: 'When spend reaches the alert threshold' },
  { value: 'rate_limit_exceeded', label: 'Rate Limit Exceeded', description: 'When a rate limit is hit' },
]

function channelTypeLabel(t: string) {
  return CHANNEL_TYPES.find(c => c.value === t)?.label ?? t
}

function parseConfig(configStr: string): Record<string, string> {
  try { return JSON.parse(configStr) } catch { return {} }
}

function configSummary(channelType: string, configStr: string): string {
  const cfg = parseConfig(configStr)
  switch (channelType) {
    case 'email': return cfg.email || 'No email set'
    case 'slack': return cfg.slack_webhook_url ? 'Webhook configured' : 'No webhook URL'
    case 'webhook': return cfg.webhook_url || 'No URL set'
    default: return ''
  }
}

export default function NotificationsPage() {
  const navigate = useNavigate()
  const { role, isSynced } = useUserSync()
  const { channels, loading, error, createChannel, updateChannel, deleteChannel, testChannel } = useNotifications()
  const { config } = useDashboardConfig()
  const planLimits = config?.plan_limits
  const channelCapped = planLimits != null && planLimits.max_notification_channels !== -1

  // Modal state
  const [showModal, setShowModal] = useState(false)
  const [channelType, setChannelType] = useState('slack')
  const [name, setName] = useState('')
  const [configEmail, setConfigEmail] = useState('')
  const [configSlackUrl, setConfigSlackUrl] = useState('')
  const [configWebhookUrl, setConfigWebhookUrl] = useState('')
  const [configSigningSecret, setConfigSigningSecret] = useState('')
  const [eventTypes, setEventTypes] = useState<string[]>(['budget_blocked', 'budget_warning', 'rate_limit_exceeded'])
  const [formError, setFormError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  // Shared state
  const [successMsg, setSuccessMsg] = useState<string | null>(null)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)
  const [testingId, setTestingId] = useState<number | null>(null)

  const canAccess = isSynced && hasPermission(role, 'editor')

  useEffect(() => {
    if (isSynced && !canAccess) navigate('/dashboard')
  }, [isSynced, canAccess, navigate])

  const showSuccess = (msg: string) => { setSuccessMsg(msg); setTimeout(() => setSuccessMsg(null), 3000) }
  const showError = (msg: string) => { setErrorMsg(msg); setTimeout(() => setErrorMsg(null), 5000) }

  const resetForm = () => {
    setChannelType('slack'); setName(''); setConfigEmail(''); setConfigSlackUrl('')
    setConfigWebhookUrl(''); setConfigSigningSecret('')
    setEventTypes(['budget_blocked', 'budget_warning', 'rate_limit_exceeded']); setFormError(null)
  }

  const handleSave = async () => {
    if (eventTypes.length === 0) { setFormError('Select at least one event type'); return }
    if (!name.trim()) { setFormError('Name is required'); return }

    let config: Record<string, string> = {}
    switch (channelType) {
      case 'email':
        if (!configEmail.trim()) { setFormError('Email address is required'); return }
        config = { email: configEmail.trim() }
        break
      case 'slack':
        if (!configSlackUrl.trim()) { setFormError('Slack webhook URL is required'); return }
        config = { slack_webhook_url: configSlackUrl.trim() }
        break
      case 'webhook':
        if (!configWebhookUrl.trim()) { setFormError('Webhook URL is required'); return }
        config = { webhook_url: configWebhookUrl.trim() }
        if (configSigningSecret.trim()) config.signing_secret = configSigningSecret.trim()
        break
    }

    setSaving(true); setFormError(null)
    try {
      const req: CreateNotificationChannelReq = {
        channel_type: channelType,
        name: name.trim(),
        config,
        event_types: eventTypes,
        enabled: true,
      }
      await createChannel(req)
      showSuccess('Notification channel created'); setShowModal(false); resetForm()
    } catch (e) { setFormError(e instanceof Error ? e.message : 'Failed to save') }
    finally { setSaving(false) }
  }

  const handleToggle = async (id: number, currentEnabled: boolean) => {
    try {
      await updateChannel(id, { enabled: !currentEnabled })
      showSuccess(`Channel ${currentEnabled ? 'disabled' : 'enabled'}`)
    } catch (e) { showError(e instanceof Error ? e.message : 'Failed to toggle') }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('Delete this notification channel?')) return
    try { await deleteChannel(id); showSuccess('Channel deleted') }
    catch (e) { showError(e instanceof Error ? e.message : 'Failed to delete') }
  }

  const handleTest = async (id: number) => {
    setTestingId(id)
    try {
      const result = await testChannel(id)
      if (result.success) {
        showSuccess('Test notification sent successfully')
      } else {
        showError(`Test failed: ${result.error || 'Unknown error'}`)
      }
    } catch (e) { showError(e instanceof Error ? e.message : 'Test failed') }
    finally { setTestingId(null) }
  }

  if (!isSynced || loading) {
    return (
      <div className="page-container">
        <Navbar />
        <div className="page-content">
          <div className="loading-center"><div className="spinner" /></div>
        </div>
      </div>
    )
  }

  return (
    <div className="page-container">
      <Navbar />
      <div className="page-content">
        <div className="limits-container">

          <div className="limits-header">
            <h1>Notifications</h1>
          </div>

          {successMsg && <div className="flash flash-success">{successMsg}</div>}
          {errorMsg && <div className="flash flash-error">{errorMsg}</div>}
          {error && <div className="flash flash-error">{error}</div>}

          <section className="mgmt-section">
            <div className="section-hdr">
              <div>
                <h2>Notification Channels</h2>
                <span className="section-count">
                  {channels.length} / {channelCapped ? planLimits!.max_notification_channels : 'Unlimited'}
                </span>
                <p className="section-desc">
                  Configure Email, Slack, or Webhook channels to receive real-time alerts when budget limits
                  or rate limits are triggered. Notifications are debounced (5 min cooldown per event type).
                  {' '}<Link to="/integration#notifications" className="form-hint-link">Need help setting up a Slack webhook?</Link>
                </p>
              </div>
              <button className="btn btn-primary" onClick={() => { resetForm(); setShowModal(true) }} disabled={channelCapped && channels.length >= planLimits!.max_notification_channels} title={channelCapped && channels.length >= planLimits!.max_notification_channels ? 'Limit reached — upgrade to add more' : undefined}>
                Add Channel
              </button>
            </div>

            <div className="table-scroll">
              <table className="mgmt-table">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Type</th>
                    <th>Destination</th>
                    <th>Events</th>
                    <th>Status</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {channels.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="empty-cell">
                        <div className="empty-cta">
                          <p>No notification channels configured yet.</p>
                          <button className="btn btn-primary" onClick={() => { resetForm(); setShowModal(true) }}>
                            Add Your First Channel
                          </button>
                        </div>
                      </td>
                    </tr>
                  ) : channels.map(ch => (
                    <tr key={ch.id}>
                      <td>{ch.name}</td>
                      <td><span className="provider-badge">{channelTypeLabel(ch.channel_type)}</span></td>
                      <td><span className="text-muted">{configSummary(ch.channel_type, ch.config)}</span></td>
                      <td>
                        {(ch.event_types || []).map(et => (
                          <span key={et} className="metric-badge" style={{ marginRight: '0.3rem', marginBottom: '0.2rem' }}>
                            {et.replace(/_/g, ' ')}
                          </span>
                        ))}
                      </td>
                      <td>
                        <button
                          className={`btn btn-small ${ch.enabled ? 'btn-secondary' : 'btn-primary'}`}
                          onClick={() => handleToggle(ch.id, ch.enabled)}
                          style={{ minWidth: '5rem' }}
                        >
                          <span className={`enabled-dot ${ch.enabled ? 'on' : 'off'}`} />
                          {ch.enabled ? 'Enabled' : 'Disabled'}
                        </button>
                      </td>
                      <td>
                        <div style={{ display: 'flex', gap: '0.4rem' }}>
                          <button
                            className="btn btn-small btn-secondary"
                            onClick={() => handleTest(ch.id)}
                            disabled={testingId === ch.id}
                          >
                            {testingId === ch.id ? 'Testing...' : 'Test'}
                          </button>
                          <button className="btn btn-small btn-danger" onClick={() => handleDelete(ch.id)}>
                            Delete
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        </div>
      </div>

      {/* ── Add Channel Modal ────────────────────────────────────────────────── */}
      {showModal && (
        <div className="modal-overlay" onClick={() => { setShowModal(false); setFormError(null) }}>
          <div className="modal-box" onClick={e => e.stopPropagation()}>
            <div className="modal-hdr">
              <h2>Add Notification Channel</h2>
            </div>
            <div className="modal-body">
              <p className="modal-hint">
                Configure a channel to receive alerts when budget or rate limit events fire.
              </p>

              <div className="form-group">
                <label>Name <span className="required">*</span></label>
                <input
                  type="text" value={name}
                  onChange={e => setName(e.target.value)}
                  placeholder="e.g. Engineering Alerts" autoFocus
                />
              </div>

              <div className="form-group">
                <label>Channel Type <span className="required">*</span></label>
                <select value={channelType} onChange={e => setChannelType(e.target.value)}>
                  {CHANNEL_TYPES.map(t => (
                    <option key={t.value} value={t.value}>{t.label}</option>
                  ))}
                </select>
              </div>

              {channelType === 'email' && (
                <div className="form-group">
                  <label>Email Address <span className="required">*</span></label>
                  <input
                    type="email" value={configEmail}
                    onChange={e => setConfigEmail(e.target.value)}
                    placeholder="alerts@yourcompany.com"
                  />
                </div>
              )}

              {channelType === 'slack' && (
                <div className="form-group">
                  <label>Slack Webhook URL <span className="required">*</span></label>
                  <input
                    type="url" value={configSlackUrl}
                    onChange={e => setConfigSlackUrl(e.target.value)}
                    placeholder="https://hooks.slack.com/services/..."
                  />
                  <span className="form-hint">
                    Create an incoming webhook in your Slack workspace settings.
                    {' '}<Link to="/integration#notifications" className="form-hint-link" onClick={() => setShowModal(false)}>Don't know where to find the webhook URL?</Link>
                  </span>
                </div>
              )}

              {channelType === 'webhook' && (
                <>
                  <div className="form-group">
                    <label>Webhook URL <span className="required">*</span></label>
                    <input
                      type="url" value={configWebhookUrl}
                      onChange={e => setConfigWebhookUrl(e.target.value)}
                      placeholder="https://your-server.com/webhook"
                    />
                  </div>
                  <div className="form-group">
                    <label>Signing Secret (optional)</label>
                    <input
                      type="text" value={configSigningSecret}
                      onChange={e => setConfigSigningSecret(e.target.value)}
                      placeholder="Optional HMAC signing secret"
                    />
                    <span className="form-hint">If set, payloads will include an X-TokenGate-Signature HMAC-SHA256 header.</span>
                  </div>
                </>
              )}

              <div className="form-group">
                <label>Event Types <span className="required">*</span></label>
                <div className="role-select">
                  {EVENT_TYPE_OPTIONS.map(et => {
                    const checked = eventTypes.includes(et.value)
                    return (
                      <label key={et.value} className={`role-option ${checked ? 'selected' : ''}`}>
                        <input
                          type="checkbox" value={et.value}
                          checked={checked}
                          onChange={() => setEventTypes(prev =>
                            prev.includes(et.value) ? prev.filter(v => v !== et.value) : [...prev, et.value]
                          )}
                        />
                        <div>
                          <strong>{et.label}</strong>
                          <span className="role-desc">{et.description}</span>
                        </div>
                      </label>
                    )
                  })}
                </div>
              </div>
            </div>

            {formError && <div className="flash flash-error modal-flash">{formError}</div>}

            <div className="modal-ftr">
              <button className="btn btn-secondary" onClick={() => { setShowModal(false); setFormError(null) }}
                disabled={saving}>Cancel</button>
              <button className="btn btn-primary" onClick={handleSave}
                disabled={saving}>
                {saving ? 'Saving...' : 'Save'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
