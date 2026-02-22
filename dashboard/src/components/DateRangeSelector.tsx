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
  const today = new Date().toISOString().slice(0, 10)
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
      <div className="drs-dropdown">
        {presets.map(p => {
          const isSelected = selectedPreset === p.key
          return (
            <button
              key={p.key}
              className={`drs-option${isSelected ? ' drs-option--selected' : ''}${!p.enabled ? ' drs-option--disabled' : ''}`}
              onClick={() => handleSelectPreset(p.key)}
              disabled={!p.enabled}
              title={!p.enabled ? 'Upgrade your plan to access longer history' : undefined}
            >
              {!p.enabled && <span className="drs-lock">🔒</span>}
              {PRESET_LABELS[p.key] ?? p.key}
            </button>
          )
        })}
      </div>

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
