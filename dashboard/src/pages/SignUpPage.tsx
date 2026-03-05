import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useSignUp } from '@clerk/clerk-react'
import './SignUpPage.css'

export default function SignUpPage() {
  const { isLoaded, signUp, setActive } = useSignUp()
  const navigate = useNavigate()

  // Form state
  const [firstName, setFirstName] = useState('')
  const [lastName, setLastName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [agreed, setAgreed] = useState(false)
  const [tosError, setTosError] = useState(false)

  // Flow state
  const [step, setStep] = useState<'start' | 'verify'>('start')
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  if (!isLoaded) {
    return (
      <div className="auth-container">
        <div className="loading-center"><div className="spinner" /></div>
      </div>
    )
  }

  // ── OAuth ────────────────────────────────────────────────────────────────────
  async function handleOAuth(strategy: 'oauth_google' | 'oauth_github') {
    if (!agreed) {
      setTosError(true)
      return
    }
    setTosError(false)
    setError('')
    try {
      await signUp!.authenticateWithRedirect({
        strategy,
        redirectUrl: '/sso-callback',
        redirectUrlComplete: '/sign-up',
        legalAccepted: true,
      })
    } catch (err: unknown) {
      setError(clerkErrorMessage(err))
    }
  }

  // ── Email / password ─────────────────────────────────────────────────────────
  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!agreed) {
      setTosError(true)
      return
    }
    setTosError(false)
    setError('')
    setLoading(true)
    try {
      await signUp!.create({
        firstName,
        lastName,
        emailAddress: email,
        password,
        legalAccepted: true,
      })
      await signUp!.prepareEmailAddressVerification({ strategy: 'email_code' })
      setStep('verify')
    } catch (err: unknown) {
      setError(clerkErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }

  // ── Verification ─────────────────────────────────────────────────────────────
  async function handleVerify(e: FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const result = await signUp!.attemptEmailAddressVerification({ code })
      if (result.status === 'complete' && result.createdSessionId) {
        await setActive!({ session: result.createdSessionId })
        navigate('/sign-up')
      } else {
        setError('Verification incomplete. Please try again.')
      }
    } catch (err: unknown) {
      setError(clerkErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }

  // ── Start step ───────────────────────────────────────────────────────────────
  if (step === 'start') {
    return (
      <div className="auth-container">
        <div className="signup-card">
          <h1 className="signup-title">Create your account</h1>
          <p className="signup-subtitle">Welcome! Please fill in the details to get started.</p>

          {/* OAuth */}
          <div className="oauth-row">
            <button type="button" className="oauth-btn" onClick={() => handleOAuth('oauth_google')}>
              <GoogleIcon />
              Google
            </button>
            <button type="button" className="oauth-btn" onClick={() => handleOAuth('oauth_github')}>
              <GitHubIcon />
              GitHub
            </button>
          </div>

          <div className="divider"><span>or</span></div>

          {/* Form */}
          <form onSubmit={handleSubmit}>
            <div className="field-row">
              <label className="field">
                <span className="field-label">First name</span>
                <input
                  type="text"
                  value={firstName}
                  onChange={(e) => setFirstName(e.target.value)}
                  placeholder="First name"
                />
              </label>
              <label className="field">
                <span className="field-label">Last name</span>
                <input
                  type="text"
                  value={lastName}
                  onChange={(e) => setLastName(e.target.value)}
                  placeholder="Last name"
                />
              </label>
            </div>

            <label className="field">
              <span className="field-label">Email address</span>
              <input
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@example.com"
              />
            </label>

            <label className="field">
              <span className="field-label">Password</span>
              <input
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Enter a password"
              />
            </label>

            {/* ToS checkbox */}
            <label className="tos-label">
              <input
                type="checkbox"
                checked={agreed}
                onChange={(e) => {
                  setAgreed(e.target.checked)
                  if (e.target.checked) setTosError(false)
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
            {tosError && (
              <p className="tos-reminder">
                Please agree to the Terms of Service and Privacy Policy to continue.
              </p>
            )}

            {error && <p className="form-error">{error}</p>}

            <button type="submit" className="submit-btn" disabled={loading}>
              {loading ? 'Creating account…' : 'Continue'}
            </button>
          </form>

          <p className="signin-link">
            Already have an account?{' '}
            <Link to="/sign-in">Sign in</Link>
          </p>
        </div>
      </div>
    )
  }

  // ── Verify step ──────────────────────────────────────────────────────────────
  return (
    <div className="auth-container">
      <div className="signup-card">
        <h1 className="signup-title">Verify your email</h1>
        <p className="verify-msg">
          We sent a verification code to <strong>{email}</strong>
        </p>

        <form onSubmit={handleVerify}>
          <label className="field">
            <span className="field-label">Verification code</span>
            <input
              type="text"
              inputMode="numeric"
              autoComplete="one-time-code"
              required
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder="Enter 6-digit code"
            />
          </label>

          {error && <p className="form-error">{error}</p>}

          <button type="submit" className="submit-btn" disabled={loading}>
            {loading ? 'Verifying…' : 'Verify'}
          </button>
        </form>

        <button
          type="button"
          className="back-link"
          onClick={() => { setStep('start'); setError(''); setCode('') }}
        >
          &larr; Back
        </button>
      </div>
    </div>
  )
}

// ── Helpers ──────────────────────────────────────────────────────────────────

function clerkErrorMessage(err: unknown): string {
  if (
    typeof err === 'object' && err !== null && 'errors' in err &&
    Array.isArray((err as { errors: unknown[] }).errors)
  ) {
    const first = (err as { errors: { longMessage?: string; message?: string }[] }).errors[0]
    return first?.longMessage || first?.message || 'Something went wrong.'
  }
  return (err as Error)?.message || 'Something went wrong.'
}

function GoogleIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24">
      <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z"/>
      <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
      <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
      <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
    </svg>
  )
}

function GitHubIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z"/>
    </svg>
  )
}
