import { useState, useEffect } from 'react'
import { Routes, Route } from 'react-router-dom'
import Landing from './pages/Landing'
import Dashboard from './pages/Dashboard'
import Login from './pages/Login'
import { useAuth } from './hooks/useAuth'
import { api } from './lib/api'

// Wraps /dashboard — shows Login when multi-tenant + not signed in
function DashboardRoute() {
  const [multiTenant, setMultiTenant] = useState<boolean | null>(null)
  const { token, me, loading, signin, logout } = useAuth()

  useEffect(() => {
    api.mode()
      .then(r => setMultiTenant(r.multi_tenant))
      .catch(() => setMultiTenant(false)) // default single-tenant
  }, [])

  // Still detecting mode
  if (multiTenant === null || loading) {
    return (
      <div className="min-h-screen bg-[#0d0d0d] flex items-center justify-center">
        <img
          src="/logo.png"
          alt="Hummingbird"
          className="w-14 h-14 object-contain animate-pulse"
          style={{ filter: 'drop-shadow(0 0 16px rgba(0,168,255,0.5))' }}
        />
      </div>
    )
  }

  // Multi-tenant + not signed in → show Login
  if (multiTenant && !token) {
    return <Login onSignin={signin} />
  }

  // Single-tenant or already authenticated
  return <Dashboard onLogout={multiTenant ? logout : undefined} walletId={me?.wallet_id} />
}

export default function App() {
  return (
    <Routes>
      <Route path="/"          element={<Landing />} />
      <Route path="/dashboard" element={<DashboardRoute />} />
    </Routes>
  )
}
