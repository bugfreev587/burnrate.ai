import { useEffect, useRef, useState, useCallback } from 'react'
import { createPortal } from 'react-dom'
import { useAuth } from '@clerk/clerk-react'
import './InactivityGuard.css'

const IDLE_TIMEOUT_MS    = 10 * 60 * 1000  // 10 minutes total idle time
const WARNING_DURATION_S = 2 * 60          // 2-minute countdown (seconds)
const WARN_AFTER_MS      = IDLE_TIMEOUT_MS - WARNING_DURATION_S * 1000 // 8 min

const ACTIVITY_EVENTS = [
  'mousemove', 'mousedown', 'keydown', 'scroll', 'touchstart', 'click',
] as const

export default function InactivityGuard() {
  const { isSignedIn, signOut } = useAuth()
  const [showWarning, setShowWarning]   = useState(false)
  const [secondsLeft, setSecondsLeft]   = useState(WARNING_DURATION_S)

  // Use a ref so the stable resetTimer callback can read the current value
  // without being recreated every time the warning state changes.
  const warningActiveRef = useRef(false)
  const mainTimerRef     = useRef<ReturnType<typeof setTimeout>  | null>(null)
  const countdownRef     = useRef<ReturnType<typeof setInterval> | null>(null)

  const cancelCountdown = () => {
    if (countdownRef.current) {
      clearInterval(countdownRef.current)
      countdownRef.current = null
    }
  }

  const startCountdown = useCallback(() => {
    warningActiveRef.current = true
    setShowWarning(true)
    let secs = WARNING_DURATION_S
    setSecondsLeft(secs)
    countdownRef.current = setInterval(() => {
      secs -= 1
      setSecondsLeft(secs)
      if (secs <= 0) {
        cancelCountdown()
        signOut()
      }
    }, 1000)
  }, [signOut])

  // Restart the idle timer from scratch.  Used internally after an explicit
  // cancel or when activity occurs outside the warning phase.
  const restartIdleTimer = useCallback(() => {
    if (mainTimerRef.current) clearTimeout(mainTimerRef.current)
    mainTimerRef.current = setTimeout(startCountdown, WARN_AFTER_MS)
  }, [startCountdown])

  // Called on every user activity event.  During the warning phase activity
  // is deliberately ignored — only the explicit "Stay signed in" button can
  // cancel the countdown.
  const onActivity = useCallback(() => {
    if (warningActiveRef.current) return  // ignore passive activity
    restartIdleTimer()
  }, [restartIdleTimer])

  // Explicit cancel: attached to the "Stay signed in" button only.
  const handleStaySignedIn = useCallback(() => {
    warningActiveRef.current = false
    setShowWarning(false)
    cancelCountdown()
    restartIdleTimer()
  }, [restartIdleTimer])

  useEffect(() => {
    if (!isSignedIn) {
      // User signed out externally — clean up any running timers.
      if (mainTimerRef.current) clearTimeout(mainTimerRef.current)
      cancelCountdown()
      warningActiveRef.current = false
      setShowWarning(false)
      return
    }

    ACTIVITY_EVENTS.forEach(e =>
      window.addEventListener(e, onActivity, { passive: true })
    )
    restartIdleTimer() // start the idle timer on mount / sign-in

    return () => {
      ACTIVITY_EVENTS.forEach(e => window.removeEventListener(e, onActivity))
      if (mainTimerRef.current) clearTimeout(mainTimerRef.current)
      cancelCountdown()
    }
  }, [isSignedIn, onActivity, restartIdleTimer])

  if (!showWarning) return null

  const minutes = Math.floor(secondsLeft / 60)
  const seconds = secondsLeft % 60
  const countdown = `${minutes}:${String(seconds).padStart(2, '0')}`

  return createPortal(
    <div
      className="ia-overlay"
      role="dialog"
      aria-modal="true"
      aria-labelledby="ia-title"
    >
      <div className="ia-modal">
        <div className="ia-header">
          <svg className="ia-clock-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <circle cx="12" cy="12" r="10" />
            <polyline points="12 6 12 12 16 14" />
          </svg>
          <h2 id="ia-title" className="ia-title">Still there?</h2>
        </div>

        <p className="ia-body">
          You've been inactive for a while. For your security, you'll be
          signed out automatically in
        </p>

        <div
          className="ia-countdown"
          aria-live="polite"
          aria-atomic="true"
          {...(secondsLeft <= 30 ? { 'data-urgent': '' } : {})}
        >
          {countdown}
        </div>

        <p className="ia-hint">Click the button below to stay signed in.</p>

        <div className="ia-actions">
          <button className="btn btn-primary ia-btn" onClick={handleStaySignedIn}>
            Stay signed in
          </button>
          <button
            className="btn btn-secondary ia-btn"
            onClick={() => { cancelCountdown(); signOut() }}
          >
            Sign out now
          </button>
        </div>
      </div>
    </div>,
    document.body
  )
}
