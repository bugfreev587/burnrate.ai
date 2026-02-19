import { Link } from 'react-router-dom'
import Navbar from '../components/Navbar'
import './HomePage.css'

export default function HomePage() {
  return (
    <div className="page-container">
      <Navbar />
      <div className="hero">
        <div className="hero-content">
          <p className="hero-eyebrow">TokenGate</p>
          <h1 className="hero-title">
            Meter, Control &amp; Govern<br />
            <span className="hero-accent">LLM Usage</span>
          </h1>
          <p className="hero-description">
            BurnRate AI is a gateway that gives you visibility into every token consumed
            by your team's Claude Code sessions — with budgets, alerts, and access control.
          </p>
          <div className="hero-actions">
            <Link to="/sign-up" className="btn btn-primary">Get Started Free</Link>
            <Link to="/sign-in" className="btn btn-secondary">Sign In</Link>
          </div>
        </div>
      </div>

      <div className="features">
        <div className="page-content">
          <div className="features-grid">
            <div className="feature-card card">
              <div className="feature-icon">&#128202;</div>
              <h3>Usage Dashboard</h3>
              <p>Real-time visibility into token usage per user, model, and session.</p>
            </div>
            <div className="feature-card card">
              <div className="feature-icon">&#128272;</div>
              <h3>API Key Management</h3>
              <p>Create, revoke, and scope API keys to control who can use the gateway.</p>
            </div>
            <div className="feature-card card">
              <div className="feature-icon">&#9935;</div>
              <h3>Provider Keys</h3>
              <p>Centrally manage your Anthropic API keys without exposing them to developers.</p>
            </div>
            <div className="feature-card card">
              <div className="feature-icon">&#128100;</div>
              <h3>Team Access Control</h3>
              <p>Role-based access with owner, admin, editor, and viewer tiers.</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
