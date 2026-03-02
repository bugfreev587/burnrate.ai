import { Link } from 'react-router-dom'
import { useAuth, useUser } from '@clerk/clerk-react'
import logoLight from '../../assets/logo-light.svg'

export default function LandingNav() {
  const { isSignedIn, isLoaded } = useAuth()
  const { user } = useUser()

  return (
    <header className="fixed top-0 left-0 right-0 z-50 border-b border-white/10 bg-slate-950/80 backdrop-blur-md">
      <div className="mx-auto max-w-7xl px-4 sm:px-6">
        <div className="flex h-14 items-center justify-between">
          <Link to="/" className="flex items-center gap-2">
            <img src={logoLight} alt="TokenGate" className="h-8 w-8 shrink-0" />
            <span className="font-bold text-slate-100 text-lg tracking-tight">TokenGate</span>
            <span className="hidden sm:inline rounded bg-blue-600 px-1.5 py-0.5 text-[10px] font-bold text-white uppercase tracking-wide">
              Beta
            </span>
          </Link>

          <nav aria-label="Main navigation" className="hidden md:flex items-center gap-6 text-sm text-slate-400">
            <a href="#problem" className="hover:text-slate-100 transition-colors">Why</a>
            <a href="#features" className="hover:text-slate-100 transition-colors">Features</a>
            <a href="#how-it-works" className="hover:text-slate-100 transition-colors">Setup</a>
            <a href="#pricing" className="hover:text-slate-100 transition-colors">Pricing</a>
          </nav>

          {/* Right-side actions — wait for Clerk to load before rendering */}
          {isLoaded && (
            <div className="flex items-center gap-3">
              {isSignedIn ? (
                <>
                  <Link
                    to="/dashboard"
                    className="hidden md:inline-flex rounded-lg border border-white/15 px-4 py-2 text-sm font-semibold text-slate-200 hover:bg-white/5 transition-colors"
                  >
                    Dashboard
                  </Link>
                  <Link to="/dashboard" aria-label="Go to dashboard">
                    {user?.imageUrl ? (
                      <img
                        src={user.imageUrl}
                        alt={user.fullName ?? 'Profile'}
                        className="h-8 w-8 rounded-full object-cover ring-2 ring-white/20 hover:ring-blue-500 transition-all"
                      />
                    ) : (
                      <div className="h-8 w-8 rounded-full bg-blue-600 flex items-center justify-center text-white text-xs font-bold ring-2 ring-white/20 hover:ring-blue-500 transition-all">
                        {(user?.firstName?.[0] ?? user?.emailAddresses?.[0]?.emailAddress?.[0] ?? '?').toUpperCase()}
                      </div>
                    )}
                  </Link>
                </>
              ) : (
                <>
                  <Link
                    to="/sign-in"
                    className="hidden md:inline-flex text-sm font-medium text-slate-400 hover:text-slate-100 transition-colors"
                  >
                    Sign In
                  </Link>
                  <Link
                    to="/sign-up"
                    className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-semibold text-white hover:bg-blue-700 transition-colors"
                  >
                    Start Free
                  </Link>
                </>
              )}
            </div>
          )}
        </div>
      </div>
    </header>
  )
}
