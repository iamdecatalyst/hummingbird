import { useState, useEffect } from 'react'
import { Routes, Route, useNavigate, Navigate } from 'react-router-dom'
import { NexusCallback, getAccessToken } from '@vylth/nexus-react'
import Landing from './pages/Landing'
import Dashboard from './pages/Dashboard'
import Login from './pages/Login'
import SignetSetup from './pages/SignetSetup'
import CLIAuth from './pages/CLIAuth'
import { useAuth } from './hooks/useAuth'
import { api } from './lib/api'

// Handles /auth/callback — exchanges Nexus token for Hummingbird JWT
function AuthCallback() {
  const navigate = useNavigate()
  const { nexusSignin } = useAuth()

  return (
    <NexusCallback
      onSuccess={async () => {
        try {
          const nexusToken = getAccessToken()
          if (!nexusToken) throw new Error('no token')
          const { has_signet } = await nexusSignin(nexusToken)
          window.umami?.track('signup', { product: 'hummingbird', plan: 'default' })
          navigate(has_signet ? '/dashboard' : '/dashboard/setup', { replace: true })
        } catch {
          navigate('/', { replace: true })
        }
      }}
      onError={() => navigate('/', { replace: true })}
    />
  )
}

// Loading spinner
function Spinner() {
  return (
    <div className="min-h-screen bg-[#0d0d0d] flex items-center justify-center">
      <img
        src="/logo.png"
        alt=""
        className="w-14 h-14 object-contain animate-pulse"
        style={{ filter: 'drop-shadow(0 0 16px rgba(0,168,255,0.5))' }}
      />
    </div>
  )
}

// Wraps /dashboard — handles mode detection + auth gate
function DashboardRoute() {
  const [multiTenant, setMultiTenant] = useState<boolean | null>(null)
  const { token, me, loading, logout, refreshMe } = useAuth()
  const navigate = useNavigate()

  useEffect(() => {
    api.mode()
      .then(r => setMultiTenant(r.multi_tenant))
      .catch(() => setMultiTenant(false))
  }, [])

  if (multiTenant === null || loading) return <Spinner />

  // Single-tenant: no auth needed
  if (!multiTenant) {
    return <Dashboard />
  }

  // Multi-tenant: not logged in → redirect to /login
  if (!token) return <Navigate to="/login" replace />

  // Logged in but no Signet key yet → Setup
  if (me && !me.has_signet) {
    return (
      <SignetSetup
        firstName={me.first_name || 'there'}
        onComplete={refreshMe}
      />
    )
  }

  useEffect(() => {
    window.umami?.track('dashboard_visit', { product: 'hummingbird' })
  }, [])

  return (
    <Dashboard
      onLogout={() => { logout(); navigate('/login', { replace: true }) }}
      walletId={me?.wallet_id}
      userName={me ? `${me.first_name} ${me.last_name}`.trim() : undefined}
      userUsername={me?.username}
      userAvatar={me?.avatar}
      signetKeyPrefix={me?.signet_key_prefix}
      hasSignet={me?.has_signet}
      mainWalletId={me?.main_wallet_id}
      telegramChatId={me?.telegram_chat_id}
      onCredentialsSaved={refreshMe}
    />
  )
}

// /login — redirect to /dashboard if already logged in
function LoginRoute() {
  const [multiTenant, setMultiTenant] = useState<boolean | null>(null)
  const { token, loading } = useAuth()

  useEffect(() => {
    api.mode()
      .then(r => setMultiTenant(r.multi_tenant))
      .catch(() => setMultiTenant(false))
  }, [])

  if (multiTenant === null || loading) return <Spinner />
  if (!multiTenant) return <Navigate to="/dashboard" replace />
  if (token) return <Navigate to="/dashboard" replace />
  return <Login />
}

export default function App() {
  return (
    <Routes>
      <Route path="/"                element={<Landing />} />
      <Route path="/login"           element={<LoginRoute />} />
      <Route path="/auth/callback"   element={<AuthCallback />} />
      <Route path="/dashboard"       element={<DashboardRoute />} />
      <Route path="/dashboard/setup" element={<DashboardRoute />} />
      <Route path="/cli/auth"         element={<CLIAuth />} />
    </Routes>
  )
}
