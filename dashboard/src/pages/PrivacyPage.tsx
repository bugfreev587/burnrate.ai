import { Link } from 'react-router-dom'
import LandingFooter from '../components/landing/LandingFooter'
import PrivacyContent from '../components/legal/PrivacyContent'

export default function PrivacyPage() {
  return (
    <div className="min-h-screen flex flex-col bg-white">
      {/* Nav */}
      <nav className="border-b border-gray-100 bg-white">
        <div className="mx-auto max-w-3xl px-4 sm:px-6 py-4 flex items-center justify-between">
          <Link to="/" className="font-bold text-gray-900 text-lg">TokenGate</Link>
          <Link to="/" className="text-sm text-gray-500 hover:text-gray-900 transition-colors">
            &larr; Back to home
          </Link>
        </div>
      </nav>

      {/* Content */}
      <main className="flex-1 mx-auto max-w-3xl px-4 sm:px-6 py-12">
        <h1 className="text-3xl font-bold text-gray-900 mb-2">Privacy Policy</h1>
        <p className="text-sm text-gray-400 mb-10">Last updated: March 1, 2026</p>
        <PrivacyContent />
      </main>

      <LandingFooter />
    </div>
  )
}
