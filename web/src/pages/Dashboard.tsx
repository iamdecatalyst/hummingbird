import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { motion } from 'framer-motion'
import {
  AreaChart, Area, BarChart, Bar, Cell,
  XAxis, YAxis, Tooltip, ResponsiveContainer,
} from 'recharts'
import {
  ChartBar, MapPin, ClockCounterClockwise, Gear, Play, Stop,
  Pulse, SignOut, Copy, Check, Wallet, Key,
} from '@phosphor-icons/react'
import { useOrchestrator } from '../hooks/useOrchestrator'
import type { ClosedPosition, Position } from '../lib/api'

// ── Helpers ───────────────────────────────────────────────────────────────────

function held(openedAt: string): string {
  const ms = Date.now() - new Date(openedAt).getTime()
  const s  = Math.floor(ms / 1000)
  const m  = Math.floor(s / 60)
  const h  = Math.floor(m / 60)
  if (h > 0) return `${h}h ${m % 60}m`
  if (m > 0) return `${m}m ${s % 60}s`
  return `${s}s`
}

function shortMint(mint: string) { return mint.slice(0, 8) + '…' }

function CopyField({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false)
  const copy = () => {
    navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }
  return (
    <div>
      <p className="font-mono text-xs text-[#555] uppercase tracking-widest mb-1.5">{label}</p>
      <div className="flex items-center gap-2 neu-card-inset rounded-xl px-3 py-2.5">
        <span className="font-mono text-xs text-[#a0a0a0] truncate flex-1">{value}</span>
        <button onClick={copy} className="text-[#444] hover:text-white transition-colors shrink-0">
          {copied ? <Check size={13} className="text-[#4ADE80]" /> : <Copy size={13} />}
        </button>
      </div>
    </div>
  )
}

// ── Stat card ─────────────────────────────────────────────────────────────────

function StatCard({ label, value, sub, positive, accent }: {
  label: string; value: string; sub?: string; positive?: boolean; accent?: boolean
}) {
  return (
    <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="neu-tile p-5">
      <p className="font-mono text-xs text-[#555] uppercase tracking-widest mb-2">{label}</p>
      <p className={`font-mono text-2xl font-bold ${
        accent ? 'text-[#00A8FF]' :
        positive === true  ? 'text-[#4ADE80]' :
        positive === false ? 'text-[#EF4444]' :
        'text-white'
      }`}>{value}</p>
      {sub && <p className="font-mono text-xs text-[#555] mt-1">{sub}</p>}
    </motion.div>
  )
}

// ── Position card ─────────────────────────────────────────────────────────────

function PositionCard({ pos }: { pos: Position }) {
  const pnlPct = pos.peak_price_sol > 0
    ? ((pos.peak_price_sol - pos.entry_price_sol) / pos.entry_price_sol * 100).toFixed(1)
    : null
  return (
    <div className="neu-card-inset p-4 rounded-xl">
      <div className="flex items-start justify-between mb-2">
        <div>
          <span className="font-mono text-sm text-white font-bold">{shortMint(pos.mint)}</span>
          <span className="ml-2 font-mono text-xs text-[#00A8FF]">{pos.score >= 75 ? 'SNIPER' : 'SCALPER'}</span>
        </div>
        <span className="font-mono text-xs text-[#555]">{held(pos.opened_at)}</span>
      </div>
      <div className="grid grid-cols-3 gap-3">
        <div>
          <p className="font-mono text-[10px] text-[#444] mb-0.5">ENTRY</p>
          <p className="font-mono text-xs text-[#a0a0a0]">{pos.entry_amount_sol.toFixed(3)} SOL</p>
        </div>
        <div>
          <p className="font-mono text-[10px] text-[#444] mb-0.5">SCORE</p>
          <p className="font-mono text-xs text-[#00A8FF]">{pos.score}</p>
        </div>
        {pnlPct && (
          <div>
            <p className="font-mono text-[10px] text-[#444] mb-0.5">PEAK</p>
            <p className="font-mono text-xs text-[#4ADE80]">+{pnlPct}%</p>
          </div>
        )}
      </div>
    </div>
  )
}

// ── Top nav ───────────────────────────────────────────────────────────────────

const TABS = [
  { id: 'overview',  label: 'Overview',  icon: <ChartBar size={14} weight="duotone" /> },
  { id: 'positions', label: 'Positions', icon: <MapPin size={14} weight="duotone" /> },
  { id: 'history',   label: 'History',   icon: <ClockCounterClockwise size={14} weight="duotone" /> },
  { id: 'config',    label: 'Config',    icon: <Gear size={14} weight="duotone" /> },
]

function TopNav({ tab, setTab, paused, online, onStop, onResume, onLogout, walletId, userName, userAvatar }: {
  tab: string
  setTab: (t: string) => void
  paused: boolean
  online: boolean
  onStop: () => void
  onResume: () => void
  onLogout?: () => void
  walletId?: string
  userName?: string
  userAvatar?: string
}) {
  const [userMenuOpen, setUserMenuOpen] = useState(false)

  return (
    <nav className="sticky top-0 z-20 bg-[#0d0d0d]/90 backdrop-blur-md border-b border-white/5 px-6 h-14 flex items-center gap-4">
      {/* Logo */}
      <Link to="/" className="flex items-center gap-2 shrink-0 group mr-2">
        <img src="/logo.png" alt="" className="w-6 h-6 object-contain"
          style={{ filter: 'drop-shadow(0 0 6px rgba(0,168,255,0.4))' }} />
        <span className="font-mono text-sm font-bold text-white group-hover:text-[#00A8FF] transition-colors hidden sm:block">
          HUMMINGBIRD
        </span>
      </Link>

      {/* Tabs */}
      <div className="flex items-center gap-1 flex-1">
        {TABS.map(t => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg font-mono text-xs transition-all duration-150 ${
              tab === t.id
                ? 'bg-white/8 text-white'
                : 'text-[#555] hover:text-[#a0a0a0] hover:bg-white/4'
            }`}
          >
            {t.icon}
            <span className="hidden sm:block">{t.label}</span>
          </button>
        ))}
      </div>

      {/* Right side */}
      <div className="flex items-center gap-2 shrink-0">
        {/* Live status */}
        <span className={`flex items-center gap-1.5 font-mono text-xs px-2.5 py-1 rounded-lg ${
          online ? 'text-[#4ADE80]' : 'text-[#EF4444]'
        }`}>
          <Pulse size={12} weight="bold" />
          <span className="hidden sm:block">{online ? 'live' : 'offline'}</span>
        </span>

        {/* Stop / Resume */}
        {paused
          ? <button onClick={onResume} className="flex items-center gap-1.5 font-mono text-xs px-3 py-1.5 rounded-lg bg-[#4ADE80]/10 text-[#4ADE80] hover:bg-[#4ADE80]/20 transition-colors">
              <Play size={12} weight="fill" /><span className="hidden sm:block">Resume</span>
            </button>
          : <button onClick={onStop} className="flex items-center gap-1.5 font-mono text-xs px-3 py-1.5 rounded-lg bg-white/5 text-[#666] hover:bg-white/8 hover:text-[#a0a0a0] transition-colors">
              <Stop size={12} weight="fill" /><span className="hidden sm:block">Stop</span>
            </button>
        }

        {/* User avatar */}
        {onLogout && (
          <div className="relative">
            <button
              onClick={() => setUserMenuOpen(v => !v)}
              className="flex items-center gap-2 pl-2 pr-3 py-1 rounded-lg hover:bg-white/5 transition-colors"
            >
              {userAvatar
                ? <img src={userAvatar} className="w-6 h-6 rounded-full object-cover ring-1 ring-white/10" alt="" />
                : <div className="w-6 h-6 rounded-full bg-[#1a1a1a] border border-white/10 flex items-center justify-center">
                    <span className="font-mono text-[10px] text-[#666]">{(userName || 'U')[0].toUpperCase()}</span>
                  </div>
              }
              <span className="font-mono text-xs text-[#666] hidden md:block max-w-[120px] truncate">
                {userName || (walletId ? `${walletId.slice(0, 6)}…` : 'User')}
              </span>
            </button>

            {userMenuOpen && (
              <>
                <div className="fixed inset-0 z-10" onClick={() => setUserMenuOpen(false)} />
                <div className="absolute right-0 top-full mt-1.5 z-20 w-44 neu-card rounded-xl py-1 shadow-2xl">
                  <button
                    onClick={() => { setUserMenuOpen(false); onLogout() }}
                    className="flex items-center gap-2 w-full px-4 py-2.5 font-mono text-xs text-[#666] hover:text-[#EF4444] hover:bg-white/4 transition-colors"
                  >
                    <SignOut size={13} /> Sign out
                  </button>
                </div>
              </>
            )}
          </div>
        )}
      </div>
    </nav>
  )
}

// ── Tab views ─────────────────────────────────────────────────────────────────

function buildPnlChart(closed: ClosedPosition[]) {
  const today = new Date().toDateString()
  const todayTrades = closed.filter(c => new Date(c.closed_at).toDateString() === today)
  const byHour: Record<number, number> = {}
  for (const t of todayTrades) {
    const h = new Date(t.closed_at).getHours()
    byHour[h] = (byHour[h] ?? 0) + t.pnl_sol
  }
  let cum = 0
  return Array.from({ length: new Date().getHours() + 1 }, (_, h) => {
    cum += byHour[h] ?? 0
    return { t: `${String(h).padStart(2, '0')}:00`, pnl: parseFloat(cum.toFixed(4)) }
  })
}

function buildBarChart(closed: ClosedPosition[]) {
  // Last 7 days wins/losses by day
  const days: Record<string, { label: string; wins: number; losses: number }> = {}
  for (let i = 6; i >= 0; i--) {
    const d = new Date()
    d.setDate(d.getDate() - i)
    const key = d.toDateString()
    const label = d.toLocaleDateString('en', { weekday: 'short' })
    days[key] = { label, wins: 0, losses: 0 }
  }
  for (const t of closed) {
    const key = new Date(t.closed_at).toDateString()
    if (days[key]) {
      if (t.pnl_sol >= 0) days[key].wins++
      else days[key].losses++
    }
  }
  return Object.values(days)
}

const tooltipStyle = {
  background: '#111',
  border: '1px solid rgba(255,255,255,0.06)',
  borderRadius: 12,
  fontFamily: 'JetBrains Mono',
  fontSize: 11,
  color: '#fff',
  boxShadow: '0 8px 32px rgba(0,0,0,0.6)',
}

function TabOverview({ stats, positions, closed, online, error }: {
  stats: ReturnType<typeof useOrchestrator>['stats']
  positions: Position[]
  closed: ClosedPosition[]
  online: boolean
  error: string | null
}) {
  const [time, setTime] = useState(new Date())
  useEffect(() => {
    const id = setInterval(() => setTime(new Date()), 1000)
    return () => clearInterval(id)
  }, [])

  const s = stats ?? { open_positions: 0, total_trades: 0, wins: 0, losses: 0, win_rate: 0, today_pnl: 0, total_pnl: 0, paused: false, pause_reason: '', configured: true }
  const pnlData = buildPnlChart(closed)
  const barData = buildBarChart(closed)
  const pnlPositive = s.today_pnl >= 0

  return (
    <div className="p-6 space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="font-mono font-bold text-xl text-white">Overview</h1>
          <p className="font-mono text-xs text-[#555] mt-0.5">{time.toISOString().replace('T', ' ').slice(0, 19)} UTC</p>
        </div>
        {!online && error && (
          <div className="neu-card-inset px-4 py-2 rounded-xl">
            <span className="font-mono text-xs text-[#EF4444]">⚠ {error}</span>
          </div>
        )}
      </div>

      {/* Stat cards row */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {[
          {
            label: 'Today P&L',
            value: `${s.today_pnl >= 0 ? '+' : ''}${s.today_pnl.toFixed(3)}`,
            unit: 'SOL',
            sub: 'from midnight',
            color: s.today_pnl >= 0 ? '#4ADE80' : '#EF4444',
          },
          {
            label: 'Total P&L',
            value: `${s.total_pnl >= 0 ? '+' : ''}${s.total_pnl.toFixed(3)}`,
            unit: 'SOL',
            sub: `${s.total_trades} trades`,
            color: s.total_pnl >= 0 ? '#4ADE80' : '#EF4444',
          },
          {
            label: 'Win Rate',
            value: `${s.win_rate.toFixed(0)}`,
            unit: '%',
            sub: `${s.wins}W · ${s.losses}L`,
            color: '#00A8FF',
          },
          {
            label: 'Open Positions',
            value: `${s.open_positions}`,
            unit: '',
            sub: 'active now',
            color: '#fff',
          },
        ].map((card, i) => (
          <motion.div
            key={card.label}
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: i * 0.05 }}
            className="neu-tile p-5 relative overflow-hidden"
          >
            {/* Subtle glow blob */}
            <div className="absolute -top-6 -right-6 w-20 h-20 rounded-full opacity-10 blur-2xl"
              style={{ background: card.color }} />
            <p className="font-mono text-xs text-[#555] uppercase tracking-widest mb-3">{card.label}</p>
            <p className="font-mono font-bold leading-none" style={{ fontSize: 28, color: card.color }}>
              {card.value}
              <span className="text-base ml-1 opacity-60">{card.unit}</span>
            </p>
            <p className="font-mono text-xs text-[#444] mt-2">{card.sub}</p>
          </motion.div>
        ))}
      </div>

      {/* Charts row */}
      <div className="grid lg:grid-cols-5 gap-5">

        {/* P&L area chart — spans 3 cols */}
        <motion.div
          initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.15 }}
          className="lg:col-span-3 neu-tile p-5"
        >
          <div className="flex items-center justify-between mb-4">
            <p className="font-mono text-xs text-[#555] uppercase tracking-widest">P&L Today — SOL</p>
            <span className={`font-mono text-xs px-2.5 py-1 rounded-full ${pnlPositive ? 'bg-[#4ADE80]/10 text-[#4ADE80]' : 'bg-[#EF4444]/10 text-[#EF4444]'}`}>
              {s.today_pnl >= 0 ? '+' : ''}{s.today_pnl.toFixed(4)} SOL
            </span>
          </div>
          {pnlData.length > 1 ? (
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={pnlData}>
                <defs>
                  <linearGradient id="pnlGrad" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor={pnlPositive ? '#4ADE80' : '#EF4444'} stopOpacity={0.25} />
                    <stop offset="100%" stopColor={pnlPositive ? '#4ADE80' : '#EF4444'} stopOpacity={0} />
                  </linearGradient>
                </defs>
                <XAxis dataKey="t" tick={{ fontFamily: 'JetBrains Mono', fontSize: 10, fill: '#333' }} axisLine={false} tickLine={false} interval="preserveStartEnd" />
                <YAxis tick={{ fontFamily: 'JetBrains Mono', fontSize: 10, fill: '#333' }} axisLine={false} tickLine={false} tickFormatter={v => v.toFixed(2)} width={48} />
                <Tooltip contentStyle={tooltipStyle} formatter={(v: number) => [`${v.toFixed(4)} SOL`, 'P&L']} />
                <Area
                  type="monotone" dataKey="pnl"
                  stroke={pnlPositive ? '#4ADE80' : '#EF4444'} strokeWidth={2.5}
                  fill="url(#pnlGrad)" dot={false}
                />
              </AreaChart>
            </ResponsiveContainer>
          ) : (
            <div className="h-[200px] flex items-center justify-center">
              <p className="font-mono text-xs text-[#333]">No trades today yet.</p>
            </div>
          )}
        </motion.div>

        {/* Win/Loss bar chart — spans 2 cols */}
        <motion.div
          initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.2 }}
          className="lg:col-span-2 neu-tile p-5"
        >
          <p className="font-mono text-xs text-[#555] uppercase tracking-widest mb-4">7-Day Trades</p>
          <ResponsiveContainer width="100%" height={200}>
            <BarChart data={barData} barGap={3} barCategoryGap="30%">
              <defs>
                <linearGradient id="winGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#4ADE80" stopOpacity={0.9} />
                  <stop offset="100%" stopColor="#4ADE80" stopOpacity={0.3} />
                </linearGradient>
                <linearGradient id="lossGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#EF4444" stopOpacity={0.9} />
                  <stop offset="100%" stopColor="#EF4444" stopOpacity={0.3} />
                </linearGradient>
              </defs>
              <XAxis dataKey="label" tick={{ fontFamily: 'JetBrains Mono', fontSize: 10, fill: '#333' }} axisLine={false} tickLine={false} />
              <YAxis tick={{ fontFamily: 'JetBrains Mono', fontSize: 10, fill: '#333' }} axisLine={false} tickLine={false} allowDecimals={false} width={24} />
              <Tooltip contentStyle={tooltipStyle} />
              <Bar dataKey="wins"   name="Wins"   fill="url(#winGrad)"  radius={[4, 4, 0, 0]} />
              <Bar dataKey="losses" name="Losses" fill="url(#lossGrad)" radius={[4, 4, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        </motion.div>
      </div>

      {/* Bottom row: open positions + recent trades */}
      <div className="grid lg:grid-cols-5 gap-5">

        {/* Open positions */}
        <motion.div
          initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.25 }}
          className="lg:col-span-2 neu-tile p-5"
        >
          <p className="font-mono text-xs text-[#555] uppercase tracking-widest mb-4">
            Open Positions <span className="text-white ml-1">{positions.length}</span>
          </p>
          <div className="space-y-3 max-h-[280px] overflow-y-auto pr-1">
            {positions.map(pos => <PositionCard key={pos.id} pos={pos} />)}
            {positions.length === 0 && (
              <div className="flex flex-col items-center justify-center py-10 gap-2">
                <div className="w-8 h-8 rounded-full bg-white/4 flex items-center justify-center">
                  <span className="text-[#333] text-xs">—</span>
                </div>
                <p className="font-mono text-xs text-[#333]">Scanning for entries...</p>
              </div>
            )}
          </div>
        </motion.div>

        {/* Recent trades */}
        <motion.div
          initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.3 }}
          className="lg:col-span-3 neu-tile p-5"
        >
          <p className="font-mono text-xs text-[#555] uppercase tracking-widest mb-4">Recent Trades</p>
          {closed.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-10 gap-2">
              <p className="font-mono text-xs text-[#333]">No trades yet.</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="text-left border-b border-white/5">
                    {['Token', 'Mode', 'Entry', 'Exit', 'P&L', 'Reason'].map(h => (
                      <th key={h} className="font-mono text-[10px] text-[#333] uppercase tracking-wider pb-3 pr-5">{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {closed.slice(0, 8).map((t, i) => (
                    <tr key={i} className="border-b border-white/[0.03] hover:bg-white/[0.02] transition-colors">
                      <td className="font-mono text-xs text-white py-2.5 pr-5">{shortMint(t.mint)}</td>
                      <td className="font-mono text-xs text-[#00A8FF] py-2.5 pr-5">{t.score >= 75 ? 'SNIPER' : 'SCALPER'}</td>
                      <td className="font-mono text-xs text-[#666] py-2.5 pr-5">{t.entry_amount_sol.toFixed(3)}</td>
                      <td className="font-mono text-xs text-[#666] py-2.5 pr-5">{t.exit_amount_sol.toFixed(3)}</td>
                      <td className={`font-mono text-xs font-bold py-2.5 pr-5 ${t.pnl_sol >= 0 ? 'text-[#4ADE80]' : 'text-[#EF4444]'}`}>
                        {t.pnl_sol >= 0 ? '+' : ''}{t.pnl_sol.toFixed(3)}
                        <span className="ml-1 font-normal text-[10px] opacity-60">({t.pnl_percent >= 0 ? '+' : ''}{t.pnl_percent.toFixed(0)}%)</span>
                      </td>
                      <td className="py-2.5">
                        <span className={`font-mono text-[10px] px-2 py-0.5 rounded-full ${t.reason.startsWith('take') || t.reason === 'scalp' ? 'bg-[#4ADE80]/10 text-[#4ADE80]' : 'bg-[#EF4444]/10 text-[#EF4444]'}`}>
                          {t.reason.replace('_', ' ').toUpperCase()}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </motion.div>

      </div>
    </div>
  )
}

function TabPositions({ positions }: { positions: Position[] }) {
  return (
    <div className="p-6">
      <div className="mb-6">
        <h1 className="font-mono font-bold text-xl text-white">Positions</h1>
        <p className="font-mono text-xs text-[#555] mt-0.5">{positions.length} open</p>
      </div>
      {positions.length === 0 ? (
        <div className="neu-tile p-16 text-center">
          <p className="font-mono text-[#444] text-sm">No open positions.</p>
          <p className="font-mono text-[#333] text-xs mt-1">The bot is scanning for entries.</p>
        </div>
      ) : (
        <div className="grid md:grid-cols-2 lg:grid-cols-3 gap-4">
          {positions.map(pos => <PositionCard key={pos.id} pos={pos} />)}
        </div>
      )}
    </div>
  )
}

function TabHistory({ closed }: { closed: ClosedPosition[] }) {
  return (
    <div className="p-6">
      <div className="mb-6">
        <h1 className="font-mono font-bold text-xl text-white">History</h1>
        <p className="font-mono text-xs text-[#555] mt-0.5">{closed.length} trades total</p>
      </div>
      <div className="neu-tile p-5">
        {closed.length === 0 ? (
          <p className="font-mono text-xs text-[#444] text-center py-12">No closed trades yet.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="text-left border-b border-white/5">
                  {['Token', 'Mode', 'Entry SOL', 'Exit SOL', 'P&L', '%', 'Reason', 'Closed'].map(h => (
                    <th key={h} className="font-mono text-xs text-[#444] uppercase tracking-wider pb-3 pr-5">{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {closed.map((t, i) => (
                  <tr key={i} className="border-b border-white/[0.03] hover:bg-white/[0.015] transition-colors">
                    <td className="font-mono text-xs text-white py-3 pr-5">{shortMint(t.mint)}</td>
                    <td className="font-mono text-xs text-[#00A8FF] py-3 pr-5">{t.score >= 75 ? 'SNIPER' : 'SCALPER'}</td>
                    <td className="font-mono text-xs text-[#a0a0a0] py-3 pr-5">{t.entry_amount_sol.toFixed(3)}</td>
                    <td className="font-mono text-xs text-[#a0a0a0] py-3 pr-5">{t.exit_amount_sol.toFixed(3)}</td>
                    <td className={`font-mono text-xs font-bold py-3 pr-5 ${t.pnl_sol >= 0 ? 'text-[#4ADE80]' : 'text-[#EF4444]'}`}>
                      {t.pnl_sol >= 0 ? '+' : ''}{t.pnl_sol.toFixed(3)}
                    </td>
                    <td className={`font-mono text-xs py-3 pr-5 ${t.pnl_percent >= 0 ? 'text-[#4ADE80]' : 'text-[#EF4444]'}`}>
                      {t.pnl_percent >= 0 ? '+' : ''}{t.pnl_percent.toFixed(1)}%
                    </td>
                    <td className="py-3 pr-5">
                      <span className={`font-mono text-xs px-2 py-0.5 rounded-full ${t.reason.startsWith('take') || t.reason === 'scalp' ? 'bg-[#4ADE80]/10 text-[#4ADE80]' : 'bg-[#EF4444]/10 text-[#EF4444]'}`}>
                        {t.reason.replace('_', ' ').toUpperCase()}
                      </span>
                    </td>
                    <td className="font-mono text-[10px] text-[#444] py-3">
                      {new Date(t.closed_at).toLocaleTimeString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

function TabConfig({ walletId, userName, userAvatar, botActive }: {
  walletId?: string; userName?: string; userAvatar?: string; botActive?: boolean
}) {
  return (
    <div className="p-6">
      <div className="mb-6">
        <h1 className="font-mono font-bold text-xl text-white">Config</h1>
        <p className="font-mono text-xs text-[#555] mt-0.5">Account & bot configuration</p>
      </div>

      <div className="grid md:grid-cols-2 gap-6">
        <div className="neu-tile p-6 space-y-4">
          <div className="flex items-center gap-3 pb-4 border-b border-white/5">
            {userAvatar
              ? <img src={userAvatar} className="w-10 h-10 rounded-full object-cover ring-1 ring-white/10" alt="" />
              : <div className="w-10 h-10 rounded-full bg-[#141414] border border-white/10 flex items-center justify-center">
                  <span className="font-mono text-sm text-[#666]">{(userName || 'U')[0].toUpperCase()}</span>
                </div>
            }
            <div>
              <p className="font-mono text-sm text-white font-bold">{userName || 'User'}</p>
              <p className="font-mono text-xs text-[#555]">VYLTH Nexus account</p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Wallet size={14} className="text-[#555] shrink-0" />
            <p className="font-mono text-xs text-[#555] uppercase tracking-widest">Signet Wallet</p>
          </div>
          {walletId
            ? <CopyField label="Wallet Address" value={walletId} />
            : <p className="font-mono text-xs text-[#444]">No wallet connected.</p>
          }
        </div>

        <div className="neu-tile p-6 space-y-4">
          <p className="font-mono text-xs text-[#555] uppercase tracking-widest pb-4 border-b border-white/5">Bot Status</p>
          <div className="flex items-center justify-between">
            <span className="font-mono text-xs text-[#666]">Status</span>
            <span className={`font-mono text-xs px-2.5 py-1 rounded-full ${botActive ? 'bg-[#4ADE80]/10 text-[#4ADE80]' : 'bg-[#EF4444]/10 text-[#EF4444]'}`}>
              {botActive ? 'Running' : 'Stopped'}
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="font-mono text-xs text-[#666]">Network</span>
            <span className="font-mono text-xs text-[#a0a0a0]">Solana Mainnet</span>
          </div>
          <div className="flex items-center justify-between">
            <span className="font-mono text-xs text-[#666]">DEX</span>
            <span className="font-mono text-xs text-[#a0a0a0]">pump.fun / Raydium</span>
          </div>
          <div className="flex items-center gap-2 pt-4 border-t border-white/5">
            <Key size={14} className="text-[#555] shrink-0" />
            <p className="font-mono text-xs text-[#555]">Signet credentials encrypted at rest</p>
          </div>
        </div>
      </div>
    </div>
  )
}

// ── Main ──────────────────────────────────────────────────────────────────────

interface DashboardProps {
  onLogout?:   () => void
  walletId?:   string
  userName?:   string
  userAvatar?: string
}

export default function Dashboard({ onLogout, walletId, userName, userAvatar }: DashboardProps) {
  const { stats, positions, closed, online, loading, error, stop, resume } = useOrchestrator()
  const [tab, setTab] = useState('overview')

  if (loading) {
    return (
      <div className="min-h-screen bg-[#0d0d0d] flex items-center justify-center">
        <div className="text-center">
          <img src="/logo.png" alt="Hummingbird" className="w-16 h-16 object-contain mx-auto mb-4 animate-pulse"
            style={{ filter: 'drop-shadow(0 0 16px rgba(0,168,255,0.5))' }} />
          <p className="font-mono text-[#555] text-sm animate-pulse">Connecting to orchestrator...</p>
        </div>
      </div>
    )
  }

  if (stats && stats.configured === false) {
    return (
      <div className="min-h-screen bg-[#0d0d0d] flex items-center justify-center px-6">
        <div className="max-w-lg w-full text-center">
          <img src="/logo.png" alt="Hummingbird" className="w-20 h-20 object-contain mx-auto mb-6"
            style={{ filter: 'drop-shadow(0 0 20px rgba(0,168,255,0.4))' }} />
          <h1 className="font-mono font-bold text-white text-2xl mb-2">Not configured</h1>
          <p className="text-[#666] text-sm mb-8">Signet credentials missing.</p>
          <a href="https://signet.vylth.com" target="_blank" rel="noopener noreferrer" className="hb-btn text-sm">
            Get Signet API key →
          </a>
        </div>
      </div>
    )
  }

  const s = stats ?? { open_positions: 0, total_trades: 0, wins: 0, losses: 0, win_rate: 0, today_pnl: 0, total_pnl: 0, paused: false, pause_reason: '', configured: true }

  return (
    <div className="min-h-screen bg-[#0d0d0d]">
      <TopNav
        tab={tab}
        setTab={setTab}
        paused={s.paused}
        online={online}
        onStop={stop}
        onResume={resume}
        onLogout={onLogout}
        walletId={walletId}
        userName={userName}
        userAvatar={userAvatar}
      />

      {tab === 'overview'  && <TabOverview stats={stats} positions={positions} closed={closed} online={online} error={error} />}
      {tab === 'positions' && <TabPositions positions={positions} />}
      {tab === 'history'   && <TabHistory closed={closed} />}
      {tab === 'config'    && <TabConfig walletId={walletId} userName={userName} userAvatar={userAvatar} botActive={stats?.configured} />}
    </div>
  )
}
