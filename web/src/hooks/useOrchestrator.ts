import { useState, useEffect, useCallback } from 'react'
import { api, type Stats, type Position, type ClosedPosition } from '../lib/api'

interface OrchestratorState {
  stats:     Stats | null
  positions: Position[]
  closed:    ClosedPosition[]
  online:    boolean
  loading:   boolean
  error:     string | null
  refresh:   () => void
  stop:      () => Promise<void>
  resume:    () => Promise<void>
}

const POLL_MS = 3000

export function useOrchestrator(): OrchestratorState {
  const [stats,     setStats]     = useState<Stats | null>(null)
  const [positions, setPositions] = useState<Position[]>([])
  const [closed,    setClosed]    = useState<ClosedPosition[]>([])
  const [online,    setOnline]    = useState(false)
  const [loading,   setLoading]   = useState(true)
  const [error,     setError]     = useState<string | null>(null)

  const fetchAll = useCallback(async () => {
    try {
      const [s, p, c] = await Promise.all([
        api.stats(),
        api.positions(),
        api.closed(),
      ])
      setStats(s)
      setPositions(p ?? [])
      setClosed(c ?? [])
      setOnline(true)
      setError(null)
    } catch (e) {
      setOnline(false)
      setError(e instanceof Error ? e.message : 'Connection failed')
    } finally {
      setLoading(false)
    }
  }, [])

  // Initial fetch + polling
  useEffect(() => {
    fetchAll()
    const id = setInterval(fetchAll, POLL_MS)
    return () => clearInterval(id)
  }, [fetchAll])

  const stop = useCallback(async () => {
    await api.stop()
    await fetchAll()
  }, [fetchAll])

  const resume = useCallback(async () => {
    await api.resume()
    await fetchAll()
  }, [fetchAll])

  return { stats, positions, closed, online, loading, error, refresh: fetchAll, stop, resume }
}
