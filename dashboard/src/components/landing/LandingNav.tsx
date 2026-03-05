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

          <nav aria-label="Main navigation" className="hidden md:flex items-center gap-8 text-[15px] font-semibold text-slate-300">
            <a href="#problem" className="hover:text-white transition-colors">Why</a>
            <a href="#features" className="hover:text-white transition-colors">Features</a>
            <a href="#how-it-works" className="hover:text-white transition-colors">Setup</a>
            <a href="#pricing" className="hover:text-white transition-colors">Pricing</a>
            <a
              href="https://github.com/bugfreev587/TokenGate.to"
              target="_blank"
              rel="noopener noreferrer"
              className="text-slate-400 hover:text-white transition-colors"
              aria-label="GitHub"
            >
              <svg className="h-5 w-5" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                <path fillRule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0 1 12 6.844a9.59 9.59 0 0 1 2.504.337c1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.02 10.02 0 0 0 22 12.017C22 6.484 17.522 2 12 2Z" clipRule="evenodd" />
              </svg>
            </a>
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
