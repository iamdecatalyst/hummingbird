import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { motion } from 'framer-motion'
import {
  LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer
} from 'recharts'

// ── Mock data (replaced by real API when orchestrator is running) ──────────
const MOCK_STATS = {
  todayPnL: 0.847,
  totalPnL: 4.231,
  openPositions: 2,
  winRate: 68,
  totalTrades: 44,
  wins: 30,
  losses: 14,
  paused: false,
}

const MOCK_POSITIONS = [
  { mint: 'PUMP3R8A9x', mode: 'SNIPER', entry: 0.200, score: 87, held: '3m 12s', pnlPct: +42 },
  { mint: '7xKp2mNqW1', mode: 'SCALPER', entry: 0.050, score: 71, held: '1m 08s', pnlPct: +8 },
]

const MOCK_CLOSED = [
  { mint: 'MNSHTf9xVK', mode: 'SNIPER',  entry: 0.200, exit: 1.022, pnl: +0.822, pct: +411, reason: 'TP3' },
  { mint: 'VRTL9mzPQR', mode: 'SCALPER', entry: 0.050, exit: 0.422, pnl: +0.372, pct: +744, reason: 'TP3' },
  { mint: 'Boop7r3KLM', mode: 'SNIPER',  entry: 0.100, exit: 0.194, pnl: +0.094, pct: +94,  reason: 'TP1' },
  { mint: 'RAYLch2KNP', mode: 'SNIPER',  entry: 0.100, exit: 0.078, pnl: -0.022, pct: -22,  reason: 'SL'  },
  { mint: '9mZXq7KpR2', mode: 'SCALPER', entry: 0.050, exit: 0.040, pnl: -0.010, pct: -20,  reason: 'SL'  },
]

const MOCK_CHART = [
  { t: '00:00', pnl: 0 },
  { t: '03:00', pnl: 0.12 },
  { t: '06:00', pnl: 0.08 },
  { t: '09:00', pnl: 0.34 },
  { t: '12:00', pnl: 0.29 },
  { t: '15:00', pnl: 0.61 },
  { t: '18:00', pnl: 0.55 },
  { t: '21:00', pnl: 0.847 },
]

// ── Sub-components ──────────────────────────────────────────────────────────
function StatCard({ label, value, sub, accent }: { label: string; value: string; sub?: string; accent?: boolean }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      className="neu-tile p-5"
    >
      <p className="font-mono text-xs text-[#555] uppercase tracking-widest mb-2">{label}</p>
      <p className={`font-mono text-2xl font-bold ${accent ? 'text-[#00A8FF]' : 'text-white'}`}>{value}</p>
      {sub && <p className="font-mono text-xs text-[#555] mt-1">{sub}</p>}
    </motion.div>
  )
}

function Sidebar({ active }: { active: string }) {
  const navItems = [
    { id: 'overview',  label: 'Overview',   icon: '📊' },
    { id: 'positions', label: 'Positions',  icon: '📍' },
    { id: 'history',   label: 'History',    icon: '🗂' },
    { id: 'config',    label: 'Config',     icon: '⚙️' },
  ]
  return (
    <aside className="w-56 shrink-0 flex flex-col gap-1 pt-4">
      <div className="px-4 mb-6">
        <Link to="/" className="font-mono text-sm font-bold text-white hover:text-[#00A8FF] transition-colors">
          🐦 HUMMINGBIRD
        </Link>
        <div className="flex items-center gap-2 mt-2">
          <span className="status-dot-live" />
          <span className="font-mono text-xs text-[#00A8FF]">LIVE</span>
        </div>
      </div>
      {navItems.map(item => (
        <button
          key={item.id}
          className={`flex items-center gap-3 px-4 py-2.5 rounded-xl text-left font-mono text-sm transition-all duration-200 ${
            active === item.id
              ? 'bg-[#141414] text-white shadow-[3px_3px_8px_rgba(0,0,0,0.7),-3px_-3px_8px_rgba(40,40,40,0.12)]'
              : 'text-[#666] hover:text-[#a0a0a0] hover:bg-[#111]'
          }`}
        >
          <span>{item.icon}</span>
          <span>{item.label}</span>
        </button>
      ))}
      <div className="mt-auto px-4 pb-4 pt-8">
        <div className="neu-card-inset p-3 rounded-xl">
          <p className="font-mono text-xs text-[#555] mb-1">Today P&L</p>
          <p className="font-mono text-lg font-bold text-[#4ADE80]">
            +{MOCK_STATS.todayPnL.toFixed(3)} SOL
          </p>
        </div>
      </div>
    </aside>
  )
}

// ── Main Dashboard ──────────────────────────────────────────────────────────
export default function Dashboard() {
  const [time, setTime] = useState(new Date())
  useEffect(() => {
    const t = setInterval(() => setTime(new Date()), 1000)
    return () => clearInterval(t)
  }, [])

  const stats = MOCK_STATS

  return (
    <div className="min-h-screen bg-[#0d0d0d] flex">
      <Sidebar active="overview" />

      {/* Main content */}
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
            <span className="neu-card-inset px-3 py-1.5 rounded-xl font-mono text-xs text-[#00A8FF]">
              {stats.openPositions} open
            </span>
            <button className="hb-btn text-xs py-2 px-4">
              ⏹ Stop All
            </button>
          </div>
        </div>

        {/* Stat cards */}
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
          <StatCard
            label="Today P&L"
            value={`+${stats.todayPnL.toFixed(3)} SOL`}
            sub="↑ from midnight"
            accent
          />
          <StatCard
            label="Total P&L"
            value={`+${stats.totalPnL.toFixed(3)} SOL`}
            sub={`${stats.totalTrades} trades`}
          />
          <StatCard
            label="Win Rate"
            value={`${stats.winRate}%`}
            sub={`W:${stats.wins}  L:${stats.losses}`}
          />
          <StatCard
            label="Open Positions"
            value={`${stats.openPositions}`}
            sub="max 5 concurrent"
          />
        </div>

        <div className="grid lg:grid-cols-3 gap-6">
          {/* P&L chart */}
          <motion.div
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.2 }}
            className="lg:col-span-2 neu-tile p-5"
          >
            <p className="font-mono text-xs text-[#555] uppercase tracking-widest mb-4">
              Today P&L — SOL
            </p>
            <ResponsiveContainer width="100%" height={180}>
              <LineChart data={MOCK_CHART}>
                <XAxis
                  dataKey="t"
                  tick={{ fontFamily: 'JetBrains Mono', fontSize: 10, fill: '#444' }}
                  axisLine={false}
                  tickLine={false}
                />
                <YAxis
                  tick={{ fontFamily: 'JetBrains Mono', fontSize: 10, fill: '#444' }}
                  axisLine={false}
                  tickLine={false}
                  tickFormatter={v => `${v.toFixed(2)}`}
                />
                <Tooltip
                  contentStyle={{
                    background: '#141414',
                    border: '1px solid rgba(0,168,255,0.15)',
                    borderRadius: 12,
                    fontFamily: 'JetBrains Mono',
                    fontSize: 12,
                    color: '#fff',
                  }}
                  formatter={(v: number) => [`${v.toFixed(3)} SOL`, 'P&L']}
                />
                <Line
                  type="monotone"
                  dataKey="pnl"
                  stroke="#00A8FF"
                  strokeWidth={2}
                  dot={false}
                  strokeDasharray="0"
                />
              </LineChart>
            </ResponsiveContainer>
          </motion.div>

          {/* Open positions */}
          <motion.div
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.3 }}
            className="neu-tile p-5"
          >
            <p className="font-mono text-xs text-[#555] uppercase tracking-widest mb-4">
              Open Positions ({MOCK_POSITIONS.length})
            </p>
            <div className="space-y-3">
              {MOCK_POSITIONS.map(pos => (
                <div key={pos.mint} className="neu-card-inset p-3 rounded-xl">
                  <div className="flex items-start justify-between mb-1">
                    <span className="font-mono text-xs text-white font-bold">{pos.mint.slice(0, 8)}</span>
                    <span className={`font-mono text-xs font-bold ${pos.pnlPct >= 0 ? 'text-[#4ADE80]' : 'text-[#EF4444]'}`}>
                      {pos.pnlPct >= 0 ? '+' : ''}{pos.pnlPct}%
                    </span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="font-mono text-xs text-[#00A8FF]">{pos.mode}</span>
                    <span className="font-mono text-xs text-[#555]">{pos.held}</span>
                  </div>
                  <div className="flex items-center justify-between mt-1">
                    <span className="font-mono text-xs text-[#555]">entry: {pos.entry} SOL</span>
                    <span className="font-mono text-xs text-[#555]">score: {pos.score}</span>
                  </div>
                </div>
              ))}
              {MOCK_POSITIONS.length === 0 && (
                <p className="font-mono text-xs text-[#444] text-center py-4">
                  Scanning for entries...
                </p>
              )}
            </div>
          </motion.div>
        </div>

        {/* Recent trades table */}
        <motion.div
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.4 }}
          className="neu-tile p-5 mt-6"
        >
          <p className="font-mono text-xs text-[#555] uppercase tracking-widest mb-4">
            Recent Trades
          </p>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="text-left border-b border-white/5">
                  {['Token', 'Mode', 'Entry', 'Exit', 'P&L', 'Reason'].map(h => (
                    <th key={h} className="font-mono text-xs text-[#444] uppercase tracking-wider pb-3 pr-6">
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {MOCK_CLOSED.map((t, i) => (
                  <tr key={i} className="border-b border-white/3 hover:bg-white/[0.015] transition-colors">
                    <td className="font-mono text-xs text-white py-3 pr-6">{t.mint.slice(0, 8)}</td>
                    <td className="font-mono text-xs text-[#00A8FF] py-3 pr-6">{t.mode}</td>
                    <td className="font-mono text-xs text-[#a0a0a0] py-3 pr-6">{t.entry.toFixed(3)}</td>
                    <td className="font-mono text-xs text-[#a0a0a0] py-3 pr-6">{t.exit.toFixed(3)}</td>
                    <td className={`font-mono text-xs font-bold py-3 pr-6 ${t.pnl >= 0 ? 'text-[#4ADE80]' : 'text-[#EF4444]'}`}>
                      {t.pnl >= 0 ? '+' : ''}{t.pnl.toFixed(3)} SOL
                      <span className="ml-2 font-normal text-[10px] opacity-70">
                        ({t.pct >= 0 ? '+' : ''}{t.pct}%)
                      </span>
                    </td>
                    <td className="py-3">
                      <span className={`font-mono text-xs px-2 py-0.5 rounded-full ${
                        t.reason.startsWith('TP')
                          ? 'bg-[#4ADE80]/10 text-[#4ADE80]'
                          : 'bg-[#EF4444]/10 text-[#EF4444]'
                      }`}>
                        {t.reason}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </motion.div>

        {/* API notice */}
        <div className="mt-6 neu-card-inset px-4 py-3 rounded-xl flex items-center gap-3">
          <span className="font-mono text-xs text-[#00A8FF]">ℹ</span>
          <span className="font-mono text-xs text-[#555]">
            Dashboard showing mock data. Connect to orchestrator at{' '}
            <code className="text-[#a0a0a0]">localhost:8002</code> for live stats.
          </span>
        </div>
      </main>
    </div>
  )
}
