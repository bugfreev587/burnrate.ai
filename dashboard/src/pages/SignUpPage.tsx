import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { SignUp } from '@clerk/clerk-react'
import TermsContent from '../components/legal/TermsContent'
import PrivacyContent from '../components/legal/PrivacyContent'

type Tab = 'terms' | 'privacy'

export default function SignUpPage() {
  const [step, setStep] = useState<'terms' | 'signup'>('terms')
  const [activeTab, setActiveTab] = useState<Tab>('terms')
  const [agreed, setAgreed] = useState(false)
  const navigate = useNavigate()

  if (step === 'terms') {
    return (
      <div className="min-h-screen bg-gray-50 flex items-start justify-center px-4 py-12">
        <div className="w-full max-w-2xl bg-white rounded-2xl shadow-lg border border-gray-200 overflow-hidden">
          {/* Header */}
          <div className="px-6 pt-6 pb-4 border-b border-gray-100">
            <h1 className="text-xl font-bold text-gray-900">Terms of Service & Privacy Policy</h1>
            <p className="text-sm text-gray-500 mt-1">
              Please review our terms before creating your account.
            </p>
          </div>

          {/* Tabs */}
          <div className="flex border-b border-gray-200">
            <button
              onClick={() => setActiveTab('terms')}
              className={`flex-1 py-3 text-sm font-medium text-center transition-colors ${
                activeTab === 'terms'
                  ? 'text-blue-600 border-b-2 border-blue-600'
                  : 'text-gray-500 hover:text-gray-700'
              }`}
            >
              Terms of Service
            </button>
            <button
              onClick={() => setActiveTab('privacy')}
              className={`flex-1 py-3 text-sm font-medium text-center transition-colors ${
                activeTab === 'privacy'
                  ? 'text-blue-600 border-b-2 border-blue-600'
                  : 'text-gray-500 hover:text-gray-700'
              }`}
            >
              Privacy Policy
            </button>
          </div>

          {/* Scrollable content */}
          <div className="px-6 py-6 max-h-[400px] overflow-y-auto">
            {activeTab === 'terms' ? <TermsContent /> : <PrivacyContent />}
          </div>

          {/* Footer: checkbox + buttons */}
          <div className="px-6 py-4 border-t border-gray-200 bg-gray-50">
            <div className="flex items-start gap-2 mb-4">
              <input
                id="consent"
                type="checkbox"
                checked={agreed}
                onChange={(e) => setAgreed(e.target.checked)}
                className="mt-1 h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500 cursor-pointer"
              />
              <label htmlFor="consent" className="text-sm text-gray-600 cursor-pointer select-none">
                I have read and agree to the Terms of Service and Privacy Policy.
              </label>
            </div>

            <div className="flex gap-3 justify-end">
              <button
                onClick={() => navigate('/')}
                className="px-5 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-lg hover:bg-gray-50 transition-colors"
              >
                Cancel
              </button>
              <button
                disabled={!agreed}
                onClick={() => setStep('signup')}
                className={`px-5 py-2 text-sm font-medium rounded-lg transition-colors ${
                  agreed
                    ? 'bg-blue-600 text-white hover:bg-blue-700'
                    : 'bg-gray-200 text-gray-400 cursor-not-allowed'
                }`}
              >
                Continue
              </button>
            </div>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="auth-container">
      <SignUp routing="path" path="/sign-up" />
    </div>
  )
}
