import { useState, useRef, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { SignUp } from '@clerk/clerk-react'

export default function SignUpPage() {
  const [agreed, setAgreed] = useState(false)
  const [showReminder, setShowReminder] = useState(false)
  const clerkRef = useRef<HTMLDivElement>(null)

  // Block Clerk's buttons (Continue, OAuth) when ToS not accepted.
  // Capture-phase listeners fire before Clerk's React handlers.
  useEffect(() => {
    const el = clerkRef.current
    if (!el || agreed) return

    const block = (e: Event) => {
      const target = e.target as HTMLElement
      if (target.closest('button')) {
        e.stopPropagation()
        e.preventDefault()
        setShowReminder(true)
      }
    }

    el.addEventListener('click', block, true)
    el.addEventListener('submit', block, true)
    return () => {
      el.removeEventListener('click', block, true)
      el.removeEventListener('submit', block, true)
    }
  }, [agreed])

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
              and I&apos;m aware my personal data is processed in accordance with our{' '}
              <Link to="/privacy" target="_blank" className="tos-link">
                Privacy Policy
              </Link>
              .
            </span>
          </label>
          {showReminder && (
            <p className="tos-reminder">
              Please agree to the Terms of Service and Privacy Policy to continue.
            </p>
          )}
        </div>

        <div className="signup-clerk-container" ref={clerkRef}>
          <SignUp routing="path" path="/sign-up" />
        </div>
      </div>
    </div>
  )
}
