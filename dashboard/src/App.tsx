import { useState, useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { useAuth } from '@clerk/clerk-react'
import { useUserSync } from './hooks/useUserSync'
import APIKeyModal from './components/APIKeyModal'
import InactivityGuard from './components/InactivityGuard'
import LandingPage from './pages/LandingPage'
import SignInPage from './pages/SignInPage'
import SignUpPage from './pages/SignUpPage'
import Dashboard from './pages/Dashboard'
import ProfilePage from './pages/ProfilePage'
import ManagementPage from './pages/ManagementPage'
import PricingConfigPage from './pages/PricingConfigPage'
import PlanPage from './pages/PlanPage'
import BillingPage from './pages/BillingPage'
import LimitsPage from './pages/LimitsPage'
import IntegrationPage from './pages/IntegrationPage'
import SettingsPage from './pages/SettingsPage'
import PublicPricingPage from './pages/PublicPricingPage'

// ─── Providers ──────────────────────────────────────────────────────────────

function UserSyncProvider({ children }: { children: React.ReactNode }) {
  const { isSyncing, error, isNewUser, apiKey } = useUserSync()
  const [showAPIKeyModal, setShowAPIKeyModal] = useState(false)
  const [displayedKey, setDisplayedKey] = useState<string | null>(null)

  useEffect(() => {
    if (isNewUser && apiKey) {
      setDisplayedKey(apiKey)
      setShowAPIKeyModal(true)
    }
  }, [isNewUser, apiKey])

  if (error) console.warn('User sync error:', error)
  if (isSyncing) { /* could show a global loading indicator */ }

  return (
    <>
      {children}
      {showAPIKeyModal && displayedKey && (
        <APIKeyModal
          apiKey={displayedKey}
          onClose={() => {
            setShowAPIKeyModal(false)
            setDisplayedKey(null)
          }}
        />
      )}
      <InactivityGuard />
    </>
  )
}

// ─── Route guards ────────────────────────────────────────────────────────────

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isLoaded, isSignedIn } = useAuth()
  if (!isLoaded) return <div className="loading-center"><div className="spinner" /></div>
  if (!isSignedIn) return <Navigate to="/sign-in" replace />
  return <>{children}</>
}

function PublicOnlyRoute({ children }: { children: React.ReactNode }) {
  const { isLoaded, isSignedIn } = useAuth()
  if (!isLoaded) return <div className="loading-center"><div className="spinner" /></div>
  if (isSignedIn) return <Navigate to="/dashboard" replace />
  return <>{children}</>
}

// ─── App ─────────────────────────────────────────────────────────────────────

export default function App() {
  return (
    <UserSyncProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<LandingPage />} />
          <Route path="/sign-in/*" element={<PublicOnlyRoute><SignInPage /></PublicOnlyRoute>} />
          <Route path="/sign-up/*" element={<PublicOnlyRoute><SignUpPage /></PublicOnlyRoute>} />
          <Route path="/dashboard" element={<ProtectedRoute><Dashboard /></ProtectedRoute>} />
          <Route path="/profile/*" element={<ProtectedRoute><ProfilePage /></ProtectedRoute>} />
          <Route path="/management" element={<ProtectedRoute><ManagementPage /></ProtectedRoute>} />
          <Route path="/pricing" element={<ProtectedRoute><PricingConfigPage /></ProtectedRoute>} />
          <Route path="/limits" element={<ProtectedRoute><LimitsPage /></ProtectedRoute>} />
          <Route path="/integration" element={<ProtectedRoute><IntegrationPage /></ProtectedRoute>} />
          <Route path="/settings" element={<ProtectedRoute><SettingsPage /></ProtectedRoute>} />
          <Route path="/plan" element={<ProtectedRoute><PlanPage /></ProtectedRoute>} />
          <Route path="/billing" element={<ProtectedRoute><BillingPage /></ProtectedRoute>} />
          <Route path="/plans" element={<PublicPricingPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </UserSyncProvider>
  )
}
