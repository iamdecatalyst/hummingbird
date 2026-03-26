// API client for the Hummingbird orchestrator.
// Base URL falls back to localhost:8002 in dev; set VITE_API_URL in production.

const BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8002'

export interface Stats {
  open_positions: number
  total_trades:   number
  wins:           number
  losses:         number
  win_rate:       number
  today_pnl:      number
  total_pnl:      number
  paused:         boolean
  pause_reason:   string
}

export interface Position {
  id:               string
  mint:             string
  wallet_id:        string
  entry_price_sol:  number
  entry_amount_sol: number
  token_balance:    number
  score:            number
  opened_at:        string  // ISO timestamp
  peak_price_sol:   number
  take_profit_level: number
}

export interface ClosedPosition extends Position {
  exit_price_sol:  number
  exit_amount_sol: number
  pnl_sol:         number
  pnl_percent:     number
  reason:          string
  closed_at:       string
  tx_hash:         string
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`)
  if (!res.ok) throw new Error(`${path} → ${res.status}`)
  return res.json() as Promise<T>
}

async function post(path: string): Promise<void> {
  const res = await fetch(`${BASE}${path}`, { method: 'POST' })
  if (!res.ok) throw new Error(`${path} → ${res.status}`)
}

export const api = {
  stats():                    Promise<Stats>           { return get('/stats') },
  positions():                Promise<Position[]>      { return get('/positions') },
  closed():                   Promise<ClosedPosition[]> { return get('/closed') },
  health():                   Promise<{ status: string }> { return get('/health') },
  stop():                     Promise<void>            { return post('/stop') },
  resume():                   Promise<void>            { return post('/resume') },
}
