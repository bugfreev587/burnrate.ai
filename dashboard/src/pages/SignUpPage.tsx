import { useState } from 'react'
import { SignUp } from '@clerk/clerk-react'

export default function SignUpPage() {
  const [agreed, setAgreed] = useState(false)

  return (
    <div className="auth-container">
      <div className="mb-4 flex items-start gap-2 max-w-[400px] mx-auto">
        <input
          id="consent"
          type="checkbox"
          checked={agreed}
          onChange={(e) => setAgreed(e.target.checked)}
          className="mt-1 h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500 cursor-pointer"
        />
        <label htmlFor="consent" className="text-sm text-gray-600 cursor-pointer select-none">
          I agree to the{' '}
          <a
            href="/terms"
            target="_blank"
            rel="noopener noreferrer"
            className="text-blue-600 hover:text-blue-800 underline"
          >
            Terms of Service
          </a>{' '}
          and{' '}
          <a
            href="/privacy"
            target="_blank"
            rel="noopener noreferrer"
            className="text-blue-600 hover:text-blue-800 underline"
          >
            Privacy Policy
          </a>
        </label>
      </div>

      <div
        style={{
          pointerEvents: agreed ? 'auto' : 'none',
          opacity: agreed ? 1 : 0.5,
          transition: 'opacity 0.2s ease',
        }}
      >
        <SignUp routing="path" path="/sign-up" />
      </div>
    </div>
  )
}
