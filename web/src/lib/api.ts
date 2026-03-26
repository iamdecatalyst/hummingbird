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
  configured:     boolean
}

export interface Position {
  id:               string
  mint:             string
  wallet_id:        string
  entry_price_sol:  number
  entry_amount_sol: number
  token_balance:    number
  score:            number
  opened_at:        string
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

export interface MeResponse {
  id:                string
  username:          string
  first_name:        string
  last_name:         string
  email:             string
  avatar:            string
  has_signet:        boolean
  signet_key_prefix: string
  wallet_id:         string
  bot_active:        boolean
}

export interface WalletEntry {
  id:          string
  address:     string
  label:       string
  balance_sol: number
}

function getToken(): string | null {
  return localStorage.getItem('hb_token')
}

function authHeaders(): HeadersInit {
  const token = getToken()
  return token ? { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` }
                : { 'Content-Type': 'application/json' }
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`, { headers: authHeaders() })
  if (!res.ok) throw new Error(`${path} → ${res.status}`)
  return res.json() as Promise<T>
}

async function post<T = void>(path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: 'POST',
    headers: authHeaders(),
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(text || `${path} → ${res.status}`)
  }
  return res.json() as Promise<T>
}

export interface LogEntry {
  time:        string
  type:        string   // ENTER EXIT START STOP ALERT INFO
  token?:      string
  amount_sol?: number
  pnl_sol?:    number
  pnl_pct?:    number
  reason?:     string
  message:     string
}

export const api = {
  // Mode detection
  mode(): Promise<{ multi_tenant: boolean }> { return get('/mode') },

  // Stats / trading
  stats():     Promise<Stats>                { return get('/stats') },
  positions(): Promise<Position[]>           { return get('/positions') },
  closed():    Promise<ClosedPosition[]>      { return get('/closed') },
  logs():      Promise<LogEntry[]>            { return get('/logs') },
  stop():      Promise<void>                 { return post('/stop') },
  resume():    Promise<void>                 { return post('/resume') },
  startBot():  Promise<void>                 { return post('/start') },

  // Multi-tenant auth — Nexus SSO
  nexusSignin(access_token: string) {
    return post<{ token: string; has_signet: boolean; user: { id: string; first_name: string; last_name: string; email: string; avatar: string } }>('/auth/nexus', { access_token })
  },
  me(): Promise<MeResponse> { return get('/auth/me') },
  setupSignet(api_key: string, api_secret: string) {
    return post<{ status: string; bot_active: boolean }>('/auth/setup-signet', { api_key, api_secret })
  },

  // Wallets
  wallets():                           Promise<WalletEntry[]> { return get('/wallets') },
  createWallet(label?: string):        Promise<{ id: string; address: string }> {
    return post('/wallets', { label: label ?? 'hummingbird' })
  },
  withdraw(walletId: string, to: string, amount: string): Promise<{ tx_hash: string }> {
    return post(`/wallets/${walletId}/withdraw`, { to, amount })
  },
}
