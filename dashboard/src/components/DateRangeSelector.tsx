import type { DashboardConfig } from '../hooks/useDashboardConfig'
import type { DateRange } from '../hooks/useUsageData'
import './DateRangeSelector.css'

interface Props {
  config: DashboardConfig | null
  value: DateRange
  onChange: (range: DateRange) => void
}

const PRESET_LABELS: Record<string, string> = {
  '1d': 'Last 24 hours',
  '3d': 'Last 3 days',
  '7d': 'Last 7 days',
  '14d': 'Last 14 days',
  '30d': 'Last 30 days',
  '90d': 'Last 90 days',
  'custom': 'Custom range',
}

function retentionHint(config: DashboardConfig | null): string {
  if (!config) return ''
  const { retention, effective } = config
  if (retention.type === 'UNLIMITED') {
    return `Retention: unlimited (data available since ${effective.min_start_date})`
  }
  const planLabels: Record<string, string> = {
    free: 'last 7 days',
    pro: 'last 90 days',
    team: 'last 180 days',
    business: 'unlimited',
  }
  const label = planLabels[config.plan] ?? `last ${retention.max_days} days`
  return `Retention: ${label}`
}

export default function DateRangeSelector({ config, value, onChange }: Props) {
  const _now = new Date()
  const today = _now.getFullYear() + '-' + String(_now.getMonth() + 1).padStart(2, '0') + '-' + String(_now.getDate()).padStart(2, '0')
  const effectiveMin = config?.effective.min_start_date ?? '2026-01-01'
  const selectedPreset = value.preset ?? '30d'
  const showCustomPicker = selectedPreset === 'custom'

  const presets = config?.preset_options ?? [
    { key: '1d', days: 1, enabled: true },
    { key: '3d', days: 3, enabled: true },
    { key: '7d', days: 7, enabled: true },
    { key: '14d', days: 14, enabled: false },
    { key: '30d', days: 30, enabled: false },
    { key: '90d', days: 90, enabled: false },
    { key: 'custom', enabled: false },
  ]

  function handleSelectPreset(key: string) {
    const preset = presets.find(p => p.key === key)
    if (!preset || !preset.enabled) return
    onChange({ preset: key })
  }

  function handleStartDate(date: string) {
    onChange({ ...value, startDate: date })
  }

  function handleEndDate(date: string) {
    onChange({ ...value, endDate: date })
  }

  return (
    <div className="drs-root">
      <select
        value={selectedPreset}
        onChange={e => handleSelectPreset(e.target.value)}
        style={{
          fontSize: '0.75rem',
          background: 'var(--color-bg)',
          color: 'var(--color-text-muted)',
          border: '1px solid var(--color-border)',
          borderRadius: '4px',
          padding: '0.25rem 0.5rem',
        }}
      >
        {presets.map(p => (
          <option key={p.key} value={p.key} disabled={!p.enabled}>
            {!p.enabled ? '🔒 ' : ''}{PRESET_LABELS[p.key] ?? p.key}
          </option>
        ))}
      </select>

      {config && (
        <p className="drs-hint">{retentionHint(config)}</p>
      )}

      {showCustomPicker && presets.find(p => p.key === 'custom')?.enabled && (
        <div className="drs-custom">
          <div className="drs-custom-row">
            <label className="drs-custom-label">
              Start
              <input
                type="date"
                className="drs-date-input"
                value={value.startDate ?? ''}
                min={effectiveMin}
                max={value.endDate ?? today}
                onChange={e => handleStartDate(e.target.value)}
              />
            </label>
            <label className="drs-custom-label">
              End
              <input
                type="date"
                className="drs-date-input"
                value={value.endDate ?? ''}
                min={value.startDate ?? effectiveMin}
                max={today}
                onChange={e => handleEndDate(e.target.value)}
              />
            </label>
          </div>
          <p className="drs-hint">Data available from {effectiveMin} to today</p>
        </div>
      )}
    </div>
  )
}
