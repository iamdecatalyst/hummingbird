import { useState, useEffect } from 'react'
import { api, type MeResponse } from '../lib/api'

export interface AuthState {
  token:     string | null
  me:        MeResponse | null
  loading:   boolean
  nexusSignin: (accessToken: string) => Promise<{ has_signet: boolean; user: MeResponse }>
  logout:    () => void
  refreshMe: () => Promise<void>
}

export function useAuth(): AuthState {
  const [token,   setToken]   = useState<string | null>(() => localStorage.getItem('hb_token'))
  const [me,      setMe]      = useState<MeResponse | null>(null)
  const [loading, setLoading] = useState(!!token)

  useEffect(() => {
    if (!token) { setLoading(false); return }
    api.me()
      .then(setMe)
      .catch(() => { localStorage.removeItem('hb_token'); setToken(null) })
      .finally(() => setLoading(false))
  }, [token])

  const nexusSignin = async (accessToken: string) => {
    const res = await api.nexusSignin(accessToken)
    localStorage.setItem('hb_token', res.token)
    setToken(res.token)
    const meData = res.user as unknown as MeResponse
    meData.has_signet = res.has_signet
    setMe(meData)
    return { has_signet: res.has_signet, user: meData }
  }

  const logout = () => {
    // Clear Hummingbird JWT + any Nexus SSO artifacts. Without wiping the nx_*
    // keys, "Sign out" on a shared device leaves the Nexus refresh token behind
    // and the next visit auto-signs back in.
    localStorage.removeItem('hb_token')
    Object.keys(localStorage)
      .filter(k => k.startsWith('nx_') || k.startsWith('nexus_'))
      .forEach(k => localStorage.removeItem(k))
    sessionStorage.clear()
    setToken(null)
    setMe(null)
  }

  const refreshMe = async () => {
    if (!token) return
    try {
      const updated = await api.me()
      setMe(updated)
    } catch { /* ignore */ }
  }

  return { token, me, loading, nexusSignin, logout, refreshMe }
}
