import Navbar from '../components/Navbar'

export default function ManagementPage() {
  return (
    <div className="page-container">
      <Navbar />
      <div className="page-content">
        <h1 style={{ marginBottom: '2rem' }}>Management</h1>
        <div className="card">
          <p style={{ color: 'var(--color-text-muted)' }}>
            Team management, API key administration, and provider key configuration coming soon.
          </p>
        </div>
      </div>
    </div>
  )
}
