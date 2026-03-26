import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { motion } from 'framer-motion'
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts'
import { ChartBar, MapPin, ClockCounterClockwise, Gear, Play, Stop, ArrowLeft, Pulse, SignOut } from '@phosphor-icons/react'
import { useOrchestrator } from '../hooks/useOrchestrator'
import type { ClosedPosition, Position } from '../lib/api'

// ── Helpers ──────────────────────────────────────────────────────────────────

function held(openedAt: string): string {
  const ms = Date.now() - new Date(openedAt).getTime()
  const s  = Math.floor(ms / 1000)
  const m  = Math.floor(s / 60)
  const h  = Math.floor(m / 60)
  if (h > 0) return `${h}h ${m % 60}m`
  if (m > 0) return `${m}m ${s % 60}s`
  return `${s}s`
}

function shortMint(mint: string) { return mint.slice(0, 8) }

// ── Sub-components ────────────────────────────────────────────────────────────

function StatCard({
  label, value, sub, positive, accent,
}: { label: string; value: string; sub?: string; positive?: boolean; accent?: boolean }) {
  return (
    <motion.div initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} className="neu-tile p-5">
      <p className="font-mono text-xs text-[#555] uppercase tracking-widest mb-2">{label}</p>
      <p className={`font-mono text-2xl font-bold ${
        accent    ? 'text-[#00A8FF]' :
        positive === true  ? 'text-[#4ADE80]' :
        positive === false ? 'text-[#EF4444]' :
        'text-white'
      }`}>
        {value}
      </p>
      {sub && <p className="font-mono text-xs text-[#555] mt-1">{sub}</p>}
    </motion.div>
  )
}

function PositionCard({ pos }: { pos: Position }) {
  const heldStr = held(pos.opened_at)
  return (
    <div className="neu-card-inset p-3 rounded-xl">
      <div className="flex items-start justify-between mb-1">
        <span className="font-mono text-xs text-white font-bold">{shortMint(pos.mint)}</span>
        <span className="font-mono text-xs text-[#00A8FF]">score: {pos.score}</span>
      </div>
      <div className="flex items-center justify-between">
        <span className="font-mono text-xs text-[#00A8FF]">
          {pos.score >= 75 ? 'SNIPER' : 'SCALPER'}
        </span>
        <span className="font-mono text-xs text-[#555]">{heldStr}</span>
      </div>
      <div className="font-mono text-xs text-[#555] mt-1">
        entry: {pos.entry_amount_sol.toFixed(3)} SOL
      </div>
    </div>
  )
}

function Sidebar({
  active, paused, onStop, onResume, onLogout, walletId, userName, userAvatar,
}: { active: string; paused: boolean; onStop: () => void; onResume: () => void; onLogout?: () => void; walletId?: string; userName?: string; userAvatar?: string }) {
  const navItems = [
    { id: 'overview',  label: 'Overview',  icon: <ChartBar size={16} weight="duotone" /> },
    { id: 'positions', label: 'Positions', icon: <MapPin size={16} weight="duotone" /> },
    { id: 'history',   label: 'History',   icon: <ClockCounterClockwise size={16} weight="duotone" /> },
    { id: 'config',    label: 'Config',    icon: <Gear size={16} weight="duotone" /> },
  ]
  return (
    <aside className="w-56 shrink-0 flex flex-col gap-1 pt-4 border-r border-white/5">
      <div className="px-4 mb-6">
        <Link to="/" className="flex items-center gap-2 group">
          <img src="/logo.png" alt="" className="w-6 h-6 object-contain"
            style={{ filter: 'drop-shadow(0 0 6px rgba(0,168,255,0.4))' }} />
          <span className="font-mono text-sm font-bold text-white group-hover:text-[#00A8FF] transition-colors">
            HUMMINGBIRD
          </span>
        </Link>
        <div className="flex items-center gap-2 mt-2">
          {paused
            ? <><span className="w-2 h-2 rounded-full bg-[#F59E0B]" /><span className="font-mono text-xs text-[#F59E0B]">PAUSED</span></>
            : <><span className="status-dot-live" /><span className="font-mono text-xs text-[#00A8FF]">LIVE</span></>
          }
        </div>
      </div>

      {navItems.map(item => (
        <button key={item.id}
          className={`flex items-center gap-3 px-4 py-2.5 rounded-xl text-left font-mono text-sm transition-all duration-200 ${
            active === item.id
              ? 'bg-[#141414] text-white shadow-[3px_3px_8px_rgba(0,0,0,0.7),-3px_-3px_8px_rgba(40,40,40,0.12)]'
              : 'text-[#666] hover:text-[#a0a0a0] hover:bg-[#111]'
          }`}
        >
          <span>{item.icon}</span><span>{item.label}</span>
        </button>
      ))}

      <div className="mt-auto px-4 pb-4 pt-8 flex flex-col gap-2">
        {paused
          ? <button onClick={onResume} className="hb-btn text-xs py-2 justify-center gap-1.5"><Play size={13} weight="fill" /> Resume</button>
          : <button onClick={onStop}   className="neu-btn-ghost text-xs py-2 justify-center gap-1.5"><Stop size={13} weight="fill" /> Stop All</button>
        }
        {onLogout && (
          <div className="border-t border-white/5 pt-3 mt-1">
            <div className="flex items-center gap-2 mb-2">
              {userAvatar
                ? <img src={userAvatar} className="w-5 h-5 rounded-full object-cover" alt="" />
                : <div className="w-5 h-5 rounded-full bg-[#1a1a1a] border border-white/10" />
              }
              <span className="font-mono text-[10px] text-[#444] truncate">
                {userName || (walletId ? `${walletId.slice(0,6)}…` : 'User')}
              </span>
            </div>
            <button
              onClick={onLogout}
              className="flex items-center gap-2 text-[#444] hover:text-[#EF4444] font-mono text-xs transition-colors w-full"
            >
              <SignOut size={13} /> Sign out
            </button>
          </div>
        )}
      </div>
    </aside>
  )
}

// Build chart from closed positions (cumulative P&L by hour)
function buildChart(closed: ClosedPosition[]) {
  if (closed.length === 0) return []
  const today = new Date().toDateString()
  const todayTrades = closed.filter(c => new Date(c.closed_at).toDateString() === today)
  const byHour: Record<number, number> = {}
  for (const t of todayTrades) {
    const h = new Date(t.closed_at).getHours()
    byHour[h] = (byHour[h] ?? 0) + t.pnl_sol
  }
  let cum = 0
  return Array.from({ length: 24 }, (_, h) => {
    cum += byHour[h] ?? 0
    return { t: `${String(h).padStart(2, '0')}:00`, pnl: parseFloat(cum.toFixed(4)) }
  }).filter((_, h) => h <= new Date().getHours())
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
  const [time, setTime] = useState(new Date())

  useEffect(() => {
    const t = setInterval(() => setTime(new Date()), 1000)
    return () => clearInterval(t)
  }, [])

  const chartData = buildChart(closed)

  // Loading spinner
  if (loading) {
    return (
      <div className="min-h-screen bg-[#0d0d0d] flex items-center justify-center">
        <div className="text-center">
          <img
            src="/logo.png"
            alt="Hummingbird"
            className="w-16 h-16 object-contain mx-auto mb-4 animate-pulse"
            style={{ filter: 'drop-shadow(0 0 16px rgba(0,168,255,0.5))' }}
          />
          <p className="font-mono text-[#555] text-sm animate-pulse">Connecting to orchestrator...</p>
        </div>
      </div>
    )
  }

  // Unconfigured — show setup instructions
  if (stats && stats.configured === false) {
    return (
      <div className="min-h-screen bg-[#0d0d0d] flex items-center justify-center px-6">
        <div className="max-w-lg w-full text-center">
          <img
            src="/logo.png"
            alt="Hummingbird"
            className="w-20 h-20 object-contain mx-auto mb-6"
            style={{ filter: 'drop-shadow(0 0 20px rgba(0,168,255,0.4))' }}
          />
          <h1 className="font-mono font-bold text-white text-2xl mb-2">Not configured</h1>
          <p className="text-[#666] text-sm mb-8">
            The orchestrator is running but Signet credentials are missing.
            Set them in your <code className="text-[#00A8FF] bg-white/5 px-1.5 py-0.5 rounded">.env</code> and restart.
          </p>
          <div className="neu-tile p-5 text-left font-mono text-xs leading-relaxed mb-6">
            <div className="text-[#555] mb-2"># /opt/hummingbird/.env</div>
            <div><span className="text-[#00A8FF]">SIGNET_API_KEY</span>=<span className="text-[#4ADE80]">your_api_key</span></div>
            <div><span className="text-[#00A8FF]">SIGNET_API_SECRET</span>=<span className="text-[#4ADE80]">your_api_secret</span></div>
          </div>
          <div className="flex gap-3 justify-center">
            <a
              href="https://signet.vylth.com"
              target="_blank"
              rel="noopener noreferrer"
              className="hb-btn text-sm"
            >
              Get Signet API key →
            </a>
            <a href="https://github.com/iamdecatalyst/hummingbird" target="_blank" rel="noopener noreferrer" className="neu-btn-ghost text-sm">
              View docs
            </a>
          </div>
        </div>
      </div>
    )
  }

  // Show offline banner but still render with zeros
  const s = stats ?? {
    open_positions: 0, total_trades: 0, wins: 0, losses: 0,
    win_rate: 0, today_pnl: 0, total_pnl: 0, paused: false, pause_reason: '', configured: false,
  }

  return (
    <div className="min-h-screen bg-[#0d0d0d] flex">
      <Sidebar
        active="overview"
        paused={s.paused}
        onStop={stop}
        onResume={resume}
        onLogout={onLogout}
        walletId={walletId}
        userName={userName}
        userAvatar={userAvatar}
      />

      <main className="flex-1 overflow-y-auto p-6">
        {/* Header */}
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="font-mono font-bold text-2xl text-white">Overview</h1>
            <p className="font-mono text-xs text-[#555] mt-1">
              {time.toISOString().replace('T', ' ').slice(0, 19)} UTC
            </p>
          </div>
          <div className="flex items-center gap-3">
            {/* Connection status */}
            <span className={`flex items-center gap-1.5 neu-card-inset px-3 py-1.5 rounded-xl font-mono text-xs ${
              online ? 'text-[#4ADE80]' : 'text-[#EF4444]'
            }`}>
              <Pulse size={13} weight="bold" />
              {online ? 'live' : 'offline'}
            </span>
            <span className="neu-card-inset px-3 py-1.5 rounded-xl font-mono text-xs text-[#00A8FF]">
              {s.open_positions} open
            </span>
          </div>
        </div>

        {/* Offline error banner */}
        {error && (
          <div className="mb-6 neu-card-inset px-4 py-3 rounded-xl flex items-center gap-3">
            <span className="font-mono text-xs text-[#EF4444]">⚠</span>
            <span className="font-mono text-xs text-[#EF4444]">
              Orchestrator offline — {error}. Showing last known data.
            </span>
          </div>
        )}

        {/* Stat cards */}
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
          <StatCard
            label="Today P&L"
            value={`${s.today_pnl >= 0 ? '+' : ''}${s.today_pnl.toFixed(3)} SOL`}
            sub="↑ from midnight"
            positive={s.today_pnl >= 0 ? true : false}
          />
          <StatCard
            label="Total P&L"
            value={`${s.total_pnl >= 0 ? '+' : ''}${s.total_pnl.toFixed(3)} SOL`}
            sub={`${s.total_trades} trades`}
            positive={s.total_pnl >= 0 ? true : false}
          />
          <StatCard
            label="Win Rate"
            value={`${s.win_rate.toFixed(0)}%`}
            sub={`W:${s.wins}  L:${s.losses}`}
            accent
          />
          <StatCard
            label="Open Positions"
            value={`${s.open_positions}`}
            sub="max 5 concurrent"
          />
        </div>

        <div className="grid lg:grid-cols-3 gap-6">
          {/* P&L chart */}
          <motion.div
            initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.2 }}
            className="lg:col-span-2 neu-tile p-5"
          >
            <p className="font-mono text-xs text-[#555] uppercase tracking-widest mb-4">
              Today P&L — SOL
            </p>
            {chartData.length > 1 ? (
              <ResponsiveContainer width="100%" height={180}>
                <LineChart data={chartData}>
                  <XAxis dataKey="t" tick={{ fontFamily: 'JetBrains Mono', fontSize: 10, fill: '#444' }} axisLine={false} tickLine={false} />
                  <YAxis tick={{ fontFamily: 'JetBrains Mono', fontSize: 10, fill: '#444' }} axisLine={false} tickLine={false} tickFormatter={v => v.toFixed(3)} />
                  <Tooltip
                    contentStyle={{ background: '#141414', border: '1px solid rgba(0,168,255,0.15)', borderRadius: 12, fontFamily: 'JetBrains Mono', fontSize: 12, color: '#fff' }}
                    formatter={(v: number) => [`${v.toFixed(3)} SOL`, 'P&L']}
                  />
                  <Line type="monotone" dataKey="pnl" stroke="#00A8FF" strokeWidth={2} dot={false} />
                </LineChart>
              </ResponsiveContainer>
            ) : (
              <div className="h-[180px] flex items-center justify-center">
                <p className="font-mono text-xs text-[#444]">No trades today yet.</p>
              </div>
            )}
          </motion.div>

          {/* Open positions */}
          <motion.div
            initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.3 }}
            className="neu-tile p-5"
          >
            <p className="font-mono text-xs text-[#555] uppercase tracking-widest mb-4">
              Open Positions ({positions.length})
            </p>
            <div className="space-y-3">
              {positions.map(pos => <PositionCard key={pos.id} pos={pos} />)}
              {positions.length === 0 && (
                <p className="font-mono text-xs text-[#444] text-center py-6">
                  Scanning for entries...
                </p>
              )}
            </div>
          </motion.div>
        </div>

        {/* Recent trades table */}
        <motion.div
          initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.4 }}
          className="neu-tile p-5 mt-6"
        >
          <p className="font-mono text-xs text-[#555] uppercase tracking-widest mb-4">
            Recent Trades
          </p>
          {closed.length === 0 ? (
            <p className="font-mono text-xs text-[#444] text-center py-6">No trades yet.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="text-left border-b border-white/5">
                    {['Token', 'Mode', 'Entry', 'Exit', 'P&L', 'Reason'].map(h => (
                      <th key={h} className="font-mono text-xs text-[#444] uppercase tracking-wider pb-3 pr-6">{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {closed.slice(0, 20).map((t, i) => (
                    <tr key={i} className="border-b border-white/[0.03] hover:bg-white/[0.015] transition-colors">
                      <td className="font-mono text-xs text-white py-3 pr-6">{shortMint(t.mint)}</td>
                      <td className="font-mono text-xs text-[#00A8FF] py-3 pr-6">
                        {t.score >= 75 ? 'SNIPER' : 'SCALPER'}
                      </td>
                      <td className="font-mono text-xs text-[#a0a0a0] py-3 pr-6">{t.entry_amount_sol.toFixed(3)}</td>
                      <td className="font-mono text-xs text-[#a0a0a0] py-3 pr-6">{t.exit_amount_sol.toFixed(3)}</td>
                      <td className={`font-mono text-xs font-bold py-3 pr-6 ${t.pnl_sol >= 0 ? 'text-[#4ADE80]' : 'text-[#EF4444]'}`}>
                        {t.pnl_sol >= 0 ? '+' : ''}{t.pnl_sol.toFixed(3)} SOL
                        <span className="ml-2 font-normal text-[10px] opacity-70">
                          ({t.pnl_percent >= 0 ? '+' : ''}{t.pnl_percent.toFixed(0)}%)
                        </span>
                      </td>
                      <td className="py-3">
                        <span className={`font-mono text-xs px-2 py-0.5 rounded-full ${
                          t.reason.startsWith('take') || t.reason === 'scalp'
                            ? 'bg-[#4ADE80]/10 text-[#4ADE80]'
                            : 'bg-[#EF4444]/10 text-[#EF4444]'
                        }`}>
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

        {/* Footer note */}
        <div className="mt-4 font-mono text-xs text-[#333] text-center">
          Polling every 3s · orchestrator at {import.meta.env.VITE_API_URL ?? 'localhost:8002'}
        </div>
      </main>
    </div>
  )
}
