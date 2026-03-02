import { useState } from 'react'
import { Link } from 'react-router-dom'
import { SignUp } from '@clerk/clerk-react'

export default function SignUpPage() {
  const [agreed, setAgreed] = useState(false)
  const [showReminder, setShowReminder] = useState(false)

  return (
    <div className="auth-container">
      <div className="signup-wrapper">
        <div className="tos-consent">
          <label className="tos-label">
            <input
              type="checkbox"
              checked={agreed}
              onChange={(e) => {
                setAgreed(e.target.checked)
                if (e.target.checked) setShowReminder(false)
              }}
              className="tos-checkbox"
            />
            <span className="tos-text">
              I agree to the TokenGate{' '}
              <Link to="/terms" target="_blank" className="tos-link">
                Terms of Service
              </Link>{' '}
              and I'm aware my personal data is processed in accordance with our{' '}
              <Link to="/privacy" target="_blank" className="tos-link">
                Privacy Policy
              </Link>
              . Please read it carefully.
            </span>
          </label>
          {showReminder && (
            <p className="tos-reminder">
              Please agree to the Terms of Service and Privacy Policy to continue.
            </p>
          )}
        </div>

        <div className="signup-clerk-container">
          {!agreed && (
            <div
              className="signup-overlay"
              onClick={() => setShowReminder(true)}
              aria-hidden="true"
            />
          )}
          <SignUp routing="path" path="/sign-up" />
        </div>
      </div>
    </div>
  )
}
