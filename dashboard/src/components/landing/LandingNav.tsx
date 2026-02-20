import { Link } from 'react-router-dom'

export default function LandingNav() {
  return (
    <header className="fixed top-0 left-0 right-0 z-50 border-b border-gray-100 bg-white/90 backdrop-blur-md">
      <div className="mx-auto max-w-6xl px-4 sm:px-6">
        <div className="flex h-14 items-center justify-between">
          <Link to="/" className="flex items-center gap-2">
            <span className="font-bold text-gray-900 text-lg tracking-tight">TokenGate</span>
            <span className="hidden sm:inline rounded bg-blue-600 px-1.5 py-0.5 text-[10px] font-bold text-white uppercase tracking-wide">
              Beta
            </span>
          </Link>

          <nav aria-label="Main navigation" className="hidden md:flex items-center gap-6 text-sm text-gray-600">
            <a href="#problem" className="hover:text-gray-900 transition-colors">Why</a>
            <a href="#features" className="hover:text-gray-900 transition-colors">Features</a>
            <a href="#how-it-works" className="hover:text-gray-900 transition-colors">Setup</a>
            <a href="#pricing" className="hover:text-gray-900 transition-colors">Pricing</a>
          </nav>

          <div className="flex items-center gap-3">
            <Link
              to="/sign-in"
              className="hidden md:inline-flex text-sm font-medium text-gray-600 hover:text-gray-900 transition-colors"
            >
              Sign In
            </Link>
            <Link
              to="/sign-up"
              className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-semibold text-white hover:bg-blue-700 transition-colors"
            >
              Start Free
            </Link>
          </div>
        </div>
      </div>
    </header>
  )
}
