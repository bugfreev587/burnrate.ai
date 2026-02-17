import { useUser } from '@clerk/clerk-react'
import Navbar from '../components/Navbar'
import { useUsageData } from '../hooks/useUsageData'
import './Dashboard.css'

export default function Dashboard() {
  const { user } = useUser()
  const { logs, loading, error, refresh } = useUsageData()

  const totalTokens = logs.reduce((sum, l) => sum + l.prompt_tokens + l.completion_tokens, 0)
  const totalCost = logs.reduce((sum, l) => sum + l.cost, 0)

  return (
    <div className="page-container">
      <Navbar />
      <div className="page-content">
        <div className="dash-header">
          <h1>Dashboard</h1>
          {user?.firstName && <p className="dash-welcome">Welcome back, {user.firstName}!</p>}
          <button className="btn btn-secondary refresh-btn" onClick={refresh}>Refresh</button>
        </div>

        {/* Summary Cards */}
        <div className="summary-cards">
          <div className="card summary-card">
            <p className="summary-label">Total Requests</p>
            <p className="summary-value">{logs.length.toLocaleString()}</p>
          </div>
          <div className="card summary-card">
            <p className="summary-label">Total Tokens</p>
            <p className="summary-value">{totalTokens.toLocaleString()}</p>
          </div>
          <div className="card summary-card">
            <p className="summary-label">Total Cost</p>
            <p className="summary-value">${totalCost.toFixed(4)}</p>
          </div>
        </div>

        {/* Usage Table */}
        <div className="card">
          <h2 className="section-title">Recent Usage</h2>

          {loading && (
            <div className="loading-center">
              <div className="spinner" />
              <p>Loading usage data...</p>
            </div>
          )}

          {error && (
            <div className="error-state">
              <p>Failed to load usage data: {error}</p>
              <button className="btn btn-secondary" onClick={refresh} style={{ marginTop: '1rem' }}>
                Try Again
              </button>
            </div>
          )}

          {!loading && !error && logs.length === 0 && (
            <div className="empty-state">
              <p>No usage recorded yet.</p>
              <p className="empty-hint">
                Configure your Claude Code client to report usage via the gateway API.
              </p>
            </div>
          )}

          {!loading && !error && logs.length > 0 && (
            <div className="table-wrapper">
              <table className="usage-table">
                <thead>
                  <tr>
                    <th>Date</th>
                    <th>Model</th>
                    <th>Provider</th>
                    <th>Input Tokens</th>
                    <th>Output Tokens</th>
                    <th>Cost (USD)</th>
                  </tr>
                </thead>
                <tbody>
                  {logs.map(log => (
                    <tr key={log.id}>
                      <td>{new Date(log.created_at).toLocaleString()}</td>
                      <td><code>{log.model}</code></td>
                      <td>{log.provider}</td>
                      <td>{log.prompt_tokens.toLocaleString()}</td>
                      <td>{log.completion_tokens.toLocaleString()}</td>
                      <td>${log.cost.toFixed(4)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
