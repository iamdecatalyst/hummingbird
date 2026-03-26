import { useState, useEffect } from 'react'
import { api, type MeResponse } from '../lib/api'

export interface AuthState {
  token:    string | null
  me:       MeResponse | null
  loading:  boolean
  signin:   (apiKey: string, apiSecret: string) => Promise<void>
  logout:   () => void
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

  const signin = async (apiKey: string, apiSecret: string) => {
    const res = await api.signin(apiKey, apiSecret)
    localStorage.setItem('hb_token', res.token)
    setToken(res.token)
  }

  const logout = () => {
    localStorage.removeItem('hb_token')
    setToken(null)
    setMe(null)
  }

  return { token, me, loading, signin, logout }
}
