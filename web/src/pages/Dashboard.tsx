import React, { useState, useEffect, useRef } from 'react'
import { QRCodeSVG } from 'qrcode.react'
import MobileBottomNav from '../components/ui/MobileBottomNav'
import { Link } from 'react-router-dom'
import { motion } from 'framer-motion'
import {
  AreaChart, Area, BarChart, Bar, Cell,
  XAxis, YAxis, Tooltip, ResponsiveContainer,
} from 'recharts'
import {
  ChartBar, ArrowsLeftRight, Terminal, Play, Stop,
  Pulse, SignOut, Copy, Check, Wallet, Key,
  ArrowUp, ArrowDown, Lightning, Warning, Info,
  Eye, EyeSlash, X, Plus, QrCode, PaperPlaneTilt, CaretDown,
  TelegramLogo, SlidersHorizontal, Spinner, SquaresFour, DownloadSimple, ArrowsClockwise, ShareNetwork,
} from '@phosphor-icons/react'
import type { UserConfig } from '../lib/api'
import type { WalletEntry } from '../lib/api'
import { useOrchestrator } from '../hooks/useOrchestrator'
import { api } from '../lib/api'
import type { ClosedPosition, Position, LogEntry } from '../lib/api'

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
      <p className="font-mono text-xs text-[#888] uppercase tracking-widest mb-1.5">{label}</p>
      <div className="flex items-center gap-2 neu-card-inset rounded-xl px-3 py-2.5">
        <span className="font-mono text-xs text-[#a0a0a0] truncate flex-1">{value}</span>
        <button onClick={copy} className="text-[#666] hover:text-white transition-colors shrink-0">
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
      <p className="font-mono text-xs text-[#888] uppercase tracking-widest mb-2">{label}</p>
      <p className={`font-mono text-2xl font-bold ${
        accent ? 'text-[#00A8FF]' :
        positive === true  ? 'text-[#4ADE80]' :
        positive === false ? 'text-[#EF4444]' :
        'text-white'
      }`}>{value}</p>
      {sub && <p className="font-mono text-xs text-[#888] mt-1">{sub}</p>}
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
        <span className="font-mono text-xs text-[#888]">{held(pos.opened_at)}</span>
      </div>
      <div className="grid grid-cols-3 gap-3">
        <div>
          <p className="font-mono text-[10px] text-[#666] mb-0.5">ENTRY</p>
          <p className="font-mono text-xs text-[#a0a0a0]">{pos.entry_amount_sol.toFixed(3)} SOL</p>
        </div>
        <div>
          <p className="font-mono text-[10px] text-[#666] mb-0.5">SCORE</p>
          <p className="font-mono text-xs text-[#00A8FF]">{pos.score}</p>
        </div>
        {pnlPct && (
          <div>
            <p className="font-mono text-[10px] text-[#666] mb-0.5">PEAK</p>
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
  { id: 'accounts',  label: 'Accounts',  icon: <SquaresFour size={14} weight="duotone" /> },
  { id: 'logs',      label: 'Logs',      icon: <Terminal size={14} weight="duotone" /> },
]

function TopNav({ tab, setTab, paused, online, onStop, onResume, onLogout, onOpenCredentials, onOpenTelegram, onOpenConfig, telegramConnected, userName, userUsername, userAvatar }: {
  tab: string
  setTab: (t: string) => void
  paused: boolean
  online: boolean
  onStop: () => void
  onResume: () => void
  onLogout?: () => void
  onOpenCredentials?: () => void
  onOpenTelegram?: () => void
  onOpenConfig?: () => void
  telegramConnected?: boolean
  userName?: string
  userUsername?: string
  userAvatar?: string
}) {
  const [userMenuOpen, setUserMenuOpen] = useState(false)

  return (
    <nav className="sticky top-0 z-20 bg-[#0d0d0d]/90 backdrop-blur-md border-b border-white/5 px-3 sm:px-6 h-14 flex items-center gap-2 sm:gap-4">
      {/* Logo */}
      <Link to="/" className="flex items-center gap-2 shrink-0 group mr-1 sm:mr-2">
        <img src="/logo.png" alt="" className="w-6 h-6 object-contain"
          style={{ filter: 'drop-shadow(0 0 6px rgba(0,168,255,0.4))' }} />
        <span className="font-mono text-sm font-bold text-white group-hover:text-[#00A8FF] transition-colors hidden sm:block">
          HUMMINGBIRD
        </span>
      </Link>

      {/* Tabs — hidden on mobile; MobileBottomNav handles tab switching below lg */}
      <div className="hidden lg:flex items-center gap-1 flex-1 min-w-0">
        {TABS.map(t => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg font-mono text-xs transition-all duration-150 ${
              tab === t.id
                ? 'bg-white/8 text-white'
                : 'text-[#888] hover:text-[#a0a0a0] hover:bg-white/4'
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

        {/* Icon buttons — Wallets + Credentials + Telegram. Hidden on mobile; available in the bottom-nav menu sheet. */}
        {onOpenConfig && (
          <button onClick={onOpenConfig} title="Config"
            className="hidden lg:flex w-8 h-8 rounded-lg items-center justify-center text-[#888] hover:text-white hover:bg-white/5 transition-colors">
            <SlidersHorizontal size={15} />
          </button>
        )}
        {onOpenCredentials && (
          <button onClick={onOpenCredentials} title="API Credentials"
            className="hidden lg:flex w-8 h-8 rounded-lg items-center justify-center text-[#888] hover:text-white hover:bg-white/5 transition-colors">
            <Key size={15} />
          </button>
        )}
        {onOpenTelegram && (
          <button onClick={onOpenTelegram} title={telegramConnected ? 'Telegram connected' : 'Connect Telegram'}
            className="hidden lg:flex w-8 h-8 rounded-lg items-center justify-center transition-colors hover:bg-white/5"
            style={{ color: telegramConnected ? '#24A1DE' : '#555' }}>
            <TelegramLogo size={15} weight={telegramConnected ? 'fill' : 'regular'} />
          </button>
        )}

        {/* User avatar / sign out */}
        {onLogout && (
          <div className="relative">
            <button
              onClick={() => setUserMenuOpen(v => !v)}
              className="flex items-center gap-2 pl-1.5 pr-2.5 py-1 rounded-lg hover:bg-white/5 transition-colors"
            >
              {userAvatar && userAvatar.startsWith('https://')
                ? <img src={userAvatar} className="w-6 h-6 rounded-full object-cover ring-1 ring-white/10" alt="" />
                : <div className="w-6 h-6 rounded-full bg-[#1a1a1a] border border-white/10 flex items-center justify-center">
                    <span className="font-mono text-[10px] text-[#666]">{(userName || 'U')[0].toUpperCase()}</span>
                  </div>
              }
              <span className="font-mono text-xs text-[#666] hidden md:block max-w-[100px] truncate">
                {userUsername ? `@${userUsername}` : (userName || 'User')}
              </span>
            </button>

            {userMenuOpen && (
              <>
                <div className="fixed inset-0 z-10" onClick={() => setUserMenuOpen(false)} />
                <div className="absolute right-0 top-full mt-1.5 z-20 w-40 neu-card rounded-xl py-1 shadow-2xl">
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

// ── Modals ────────────────────────────────────────────────────────────────────

function Modal({ onClose, children, maxWidth = 480 }: { onClose: () => void; children: React.ReactNode; maxWidth?: number }) {
  return (
    <>
      <div
        onClick={onClose}
        style={{
          position: 'fixed', inset: 0, zIndex: 100,
          background: 'rgba(0,0,0,0.65)',
          backdropFilter: 'blur(6px)',
          animation: 'hb-fade-in 0.18s ease-out',
        }}
      />
      <div style={{
        position: 'fixed', top: '50%', left: '50%',
        transform: 'translate(-50%, -50%)',
        zIndex: 101,
        width: '92vw', maxWidth, maxHeight: '85vh',
        background: '#111',
        borderRadius: 20,
        boxShadow: '8px 8px 24px #050505, -4px -4px 16px rgba(255,255,255,0.03)',
        overflowY: 'auto',
        animation: 'hb-slide-up 0.22s ease-out',
      }}>
        {children}
      </div>
      <style>{`
        @keyframes hb-fade-in  { from { opacity:0 } to { opacity:1 } }
        @keyframes hb-slide-up { from { opacity:0; transform:translate(-50%,-48%) } to { opacity:1; transform:translate(-50%,-50%) } }
      `}</style>
    </>
  )
}

function ModalHeader({ icon, title, sub, onClose }: {
  icon: React.ReactNode; title: string; sub?: string; onClose: () => void
}) {
  return (
    <div style={{
      position: 'sticky', top: 0, zIndex: 1,
      display: 'flex', alignItems: 'center', justifyContent: 'space-between',
      padding: '18px 20px 14px',
      background: '#111',
      borderBottom: '1px solid rgba(255,255,255,0.06)',
    }}>
      <div className="flex items-center gap-3">
        <div className="neu-card-inset w-9 h-9 rounded-xl flex items-center justify-center shrink-0">
          {icon}
        </div>
        <div>
          <h2 className="font-mono text-xs font-bold text-white uppercase tracking-[3px]">{title}</h2>
          {sub && <p className="font-mono text-[10px] text-[#888] mt-0.5">{sub}</p>}
        </div>
      </div>
      <button
        onClick={onClose}
        className="w-8 h-8 neu-card-inset rounded-xl flex items-center justify-center text-[#888] hover:text-white transition-colors"
      >
        <X size={14} />
      </button>
    </div>
  )
}

// ── Credentials modal ─────────────────────────────────────────────────────────

function CredentialsModal({ signetKeyPrefix, hasSignet, telegramChatId, onClose, onSaved }: {
  signetKeyPrefix?: string
  hasSignet?: boolean
  telegramChatId?: string
  onClose: () => void
  onSaved?: () => void
}) {
  const [apiKey,     setApiKey]     = useState('')
  const [apiSecret,  setApiSecret]  = useState('')
  const [showKey,    setShowKey]    = useState(false)
  const [showSecret, setShowSecret] = useState(false)
  const [saving,       setSaving]       = useState(false)
  const [deleting,     setDeleting]     = useState(false)
  const [error,        setError]        = useState('')
  const [saved,        setSaved]        = useState(false)
  const [tgLinking,    setTgLinking]    = useState(false)

  const handleSave = async () => {
    if (!apiKey.trim() || !apiSecret.trim()) { setError('Both fields required'); return }
    setSaving(true); setError('')
    try {
      await api.setupSignet(apiKey.trim(), apiSecret.trim())
      setSaved(true)
      setTimeout(() => { onClose(); onSaved?.() }, 900)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Invalid credentials')
    } finally { setSaving(false) }
  }

  const handleDelete = async () => {
    // Destructive: stops the bot, erases encrypted Signet creds. No undo.
    if (!window.confirm('Remove Signet credentials?\n\nThis stops the bot and deletes your stored API key. You will need to re-enter it to trade again.')) {
      return
    }
    setDeleting(true)
    try { await api.deleteSignet(); onClose(); onSaved?.() }
    catch (e: unknown) {
      window.alert(e instanceof Error ? e.message : 'Failed to remove credentials')
    } finally { setDeleting(false) }
  }

  const handleConnectTelegram = async () => {
    setTgLinking(true)
    try {
      const { token, bot_username } = await api.telegramToken()
      window.open(`https://t.me/${bot_username}?start=${token}`, '_blank')
    } catch { /* ignore */ } finally { setTgLinking(false) }
  }

  return (
    <Modal onClose={onClose}>
      <ModalHeader
        icon={<Key size={16} className="text-[#00A8FF]" />}
        title="API Credentials"
        sub="Signet API key — encrypted at rest"
        onClose={onClose}
      />
      <div className="p-5 space-y-4">
        {/* Saved key card */}
        {(hasSignet || signetKeyPrefix) && (
          <div style={{
            display: 'flex', alignItems: 'center', gap: 12,
            padding: '14px 16px', borderRadius: 14,
            background: '#0d0d0d',
            boxShadow: 'inset 2px 2px 6px #070707, inset -1px -1px 4px rgba(255,255,255,0.02)',
            borderLeft: '3px solid #4ADE80',
          }}>
            <div className="w-8 h-8 rounded-xl flex items-center justify-center shrink-0" style={{ background: 'rgba(74,222,128,0.08)' }}>
              <Check size={14} className="text-[#4ADE80]" />
            </div>
            <div className="flex-1 min-w-0">
              <p className="font-mono text-[10px] text-[#888] uppercase tracking-widest mb-0.5">Active Key</p>
              <p className="font-mono text-xs text-[#a0a0a0]">{signetKeyPrefix || 'Key saved'}</p>
            </div>
            <button
              onClick={handleDelete}
              disabled={deleting}
              title="Remove credentials"
              className="w-8 h-8 rounded-xl flex items-center justify-center transition-colors disabled:opacity-40 shrink-0"
              style={{ background: 'rgba(239,68,68,0.08)', color: '#EF4444' }}
            >
              <X size={13} />
            </button>
          </div>
        )}

        <p className="font-mono text-[10px] text-[#888] uppercase tracking-widest">
          {(hasSignet || signetKeyPrefix) ? 'Replace with new credentials' : 'Enter Signet credentials'}
        </p>

        {/* API Key */}
        <div>
          <label className="font-mono text-[10px] text-[#888] uppercase tracking-wider block mb-1.5">API Key</label>
          <div className="flex items-center neu-card-inset rounded-xl overflow-hidden">
            <input
              type={showKey ? 'text' : 'password'}
              value={apiKey}
              onChange={e => setApiKey(e.target.value)}
              placeholder="sgn_••••••••••••••••"
              className="flex-1 px-3 py-2.5 bg-transparent font-mono text-xs text-white placeholder-[#333] outline-none"
            />
            <button
              onClick={() => setShowKey(v => !v)}
              className="w-9 h-9 flex items-center justify-center text-[#666] hover:text-[#a0a0a0] transition-colors shrink-0"
            >
              {showKey ? <EyeSlash size={13} /> : <Eye size={13} />}
            </button>
          </div>
        </div>

        {/* API Secret */}
        <div>
          <label className="font-mono text-[10px] text-[#888] uppercase tracking-wider block mb-1.5">API Secret</label>
          <div className="flex items-center neu-card-inset rounded-xl overflow-hidden">
            <input
              type={showSecret ? 'text' : 'password'}
              value={apiSecret}
              onChange={e => setApiSecret(e.target.value)}
              placeholder="••••••••••••••••••••••••••"
              className="flex-1 px-3 py-2.5 bg-transparent font-mono text-xs text-white placeholder-[#333] outline-none"
            />
            <button
              onClick={() => setShowSecret(v => !v)}
              className="w-9 h-9 flex items-center justify-center text-[#666] hover:text-[#a0a0a0] transition-colors shrink-0"
            >
              {showSecret ? <EyeSlash size={13} /> : <Eye size={13} />}
            </button>
          </div>
        </div>

        {error && <p className="font-mono text-xs text-[#EF4444]">{error}</p>}

        <button
          onClick={handleSave}
          disabled={saving || saved}
          className="w-full py-3 rounded-xl font-mono text-xs font-bold transition-all"
          style={{
            background: saved ? 'rgba(74,222,128,0.12)' : 'rgba(0,168,255,0.1)',
            color: saved ? '#4ADE80' : '#00A8FF',
            opacity: saving ? 0.6 : 1,
          }}
        >
          {saved ? '✓ Saved' : saving ? 'Verifying…' : (hasSignet || signetKeyPrefix) ? 'Update Credentials' : 'Save Credentials'}
        </button>

        <p className="font-mono text-[10px] text-[#333] text-center">
          Get your keys at <a href="https://signet.vylth.com" target="_blank" rel="noopener noreferrer" className="text-[#00A8FF] hover:underline">signet.vylth.com</a>
        </p>

        {/* Telegram connect */}
        <div style={{ borderTop: '1px solid rgba(255,255,255,0.06)', paddingTop: 16 }}>
          <p className="font-mono text-[10px] text-[#888] uppercase tracking-widest mb-3">Telegram Alerts</p>
          {telegramChatId ? (
            <div style={{
              display: 'flex', alignItems: 'center', gap: 12,
              padding: '12px 16px', borderRadius: 14,
              background: '#0d0d0d',
              boxShadow: 'inset 2px 2px 6px #070707, inset -1px -1px 4px rgba(255,255,255,0.02)',
              borderLeft: '3px solid #4ADE80',
            }}>
              <div className="w-8 h-8 rounded-xl flex items-center justify-center shrink-0" style={{ background: 'rgba(74,222,128,0.08)' }}>
                <TelegramLogo size={15} className="text-[#4ADE80]" />
              </div>
              <div className="flex-1 min-w-0">
                <p className="font-mono text-[10px] text-[#888] uppercase tracking-widest mb-0.5">Connected</p>
                <p className="font-mono text-xs text-[#a0a0a0]">Trade alerts active</p>
              </div>
            </div>
          ) : (
            <button
              onClick={handleConnectTelegram}
              disabled={tgLinking}
              className="w-full py-3 rounded-xl font-mono text-xs font-bold flex items-center justify-center gap-2 transition-all disabled:opacity-50"
              style={{ background: 'rgba(36,161,222,0.1)', color: '#24A1DE' }}
            >
              <TelegramLogo size={14} weight="fill" />
              {tgLinking ? 'Opening Telegram…' : 'Connect Telegram'}
            </button>
          )}
        </div>
      </div>
    </Modal>
  )
}

// ── Wallets tab ───────────────────────────────────────────────────────────────

function TabWallets({ mainWalletId, onMainWalletSet }: { mainWalletId?: string; onMainWalletSet?: () => void }) {
  const [wallets,     setWallets]     = useState<WalletEntry[]>([])
  const [loading,     setLoading]     = useState(true)
  const [creating,    setCreating]    = useState(false)
  const [createLabel, setCreateLabel] = useState('')
  const [showCreate,  setShowCreate]  = useState(false)
  const [activeWallet, setActiveWallet] = useState<string | null>(null)
  const [dropOpen,    setDropOpen]    = useState(false)

  const load = () => {
    setLoading(true)
    api.wallets().then(ws => {
      setWallets(ws)
      setActiveWallet(prev => prev ?? (ws.length > 0 ? (ws.find(w => w.id === mainWalletId)?.id ?? ws[0].id) : null))
    }).catch(() => {}).finally(() => setLoading(false))
  }
  useEffect(() => { load() }, [])

  const handleCreate = async () => {
    if (creating) return
    setCreating(true)
    try {
      await api.createWallet(createLabel.trim() || undefined)
      setCreateLabel(''); setShowCreate(false); load()
    } catch { /* ignore */ } finally { setCreating(false) }
  }

  const active = wallets.find(w => w.id === activeWallet)

  return (
    <div className="p-6 max-w-lg mx-auto">
      <div className="flex items-center justify-between mb-5">
        <div>
          <h2 className="font-mono font-bold text-lg text-white">Wallets</h2>
          <p className="font-mono text-xs text-[#888] mt-0.5">Powered by Signet KMS</p>
        </div>
        <button
          onClick={() => setShowCreate(v => !v)}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl font-mono text-xs transition-colors"
          style={{ background: 'rgba(0,168,255,0.08)', color: '#00A8FF' }}
        >
          <Plus size={12} /><span>New</span>
        </button>
      </div>

      {showCreate && (
        <div className="neu-tile p-4 mb-4 flex items-center gap-2">
          <input
            autoFocus
            type="text"
            placeholder="Wallet label (optional)…"
            value={createLabel}
            onChange={e => setCreateLabel(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && handleCreate()}
            className="flex-1 bg-transparent font-mono text-xs text-white placeholder-[#444] outline-none"
          />
          <button onClick={handleCreate} disabled={creating}
            className="px-3 py-1.5 rounded-lg font-mono text-[10px] disabled:opacity-40 shrink-0"
            style={{ background: 'rgba(0,168,255,0.12)', color: '#00A8FF' }}>
            {creating ? '…' : 'Create'}
          </button>
          <button onClick={() => setShowCreate(false)} className="text-[#666] hover:text-white transition-colors"><X size={13} /></button>
        </div>
      )}

      {loading ? (
        <div className="neu-tile p-10 text-center">
          <Spinner size={18} className="text-[#333] animate-spin mx-auto" />
        </div>
      ) : wallets.length === 0 ? (
        <div className="neu-tile p-10 text-center">
          <p className="font-mono text-xs text-[#666]">No wallets yet — create one above.</p>
        </div>
      ) : (
        <>
          {/* Wallet selector */}
          {wallets.length > 1 && (
            <div className="relative mb-4">
              <button
                onClick={() => setDropOpen(v => !v)}
                className="w-full flex items-center gap-3 px-4 py-3 rounded-xl transition-all"
                style={{ background: 'rgba(255,255,255,0.04)', border: '1px solid rgba(255,255,255,0.06)' }}
              >
                <div className="w-7 h-7 rounded-full flex items-center justify-center shrink-0" style={{ background: 'rgba(153,69,255,0.15)' }}>
                  <SolanaIcon size={13} />
                </div>
                <div className="flex-1 text-left min-w-0">
                  <div className="flex items-center gap-1.5">
                    <p className="font-mono text-xs text-white font-bold truncate">{active?.label || 'Wallet'}</p>
                    {mainWalletId === activeWallet && (
                      <span className="font-mono text-[9px] px-1.5 py-0.5 rounded-full shrink-0"
                        style={{ background: 'rgba(74,222,128,0.12)', color: '#4ADE80' }}>main</span>
                    )}
                  </div>
                  <p className="font-mono text-[10px] text-[#888]">{active?.balance_sol.toFixed(4)} SOL</p>
                </div>
                <CaretDown size={13} className="text-[#888] shrink-0" style={{ transform: dropOpen ? 'rotate(180deg)' : 'none', transition: 'transform 0.15s' }} />
              </button>
              {dropOpen && (
                <>
                  <div className="fixed inset-0 z-10" onClick={() => setDropOpen(false)} />
                  <div className="absolute left-0 right-0 top-full mt-1.5 z-20 rounded-xl overflow-hidden"
                    style={{ background: '#111', border: '1px solid rgba(255,255,255,0.08)', boxShadow: '0 12px 40px rgba(0,0,0,0.7)' }}>
                    {wallets.map(w => (
                      <button key={w.id} onClick={() => { setActiveWallet(w.id); setDropOpen(false) }}
                        className="w-full flex items-center gap-3 px-4 py-3 transition-colors hover:bg-white/4"
                        style={{ borderBottom: '1px solid rgba(255,255,255,0.04)' }}>
                        <div className="w-6 h-6 rounded-full flex items-center justify-center shrink-0" style={{ background: 'rgba(153,69,255,0.12)' }}>
                          <SolanaIcon size={11} />
                        </div>
                        <div className="flex-1 text-left min-w-0">
                          <div className="flex items-center gap-1.5">
                            <p className="font-mono text-xs text-white truncate">{w.label || 'Wallet'}</p>
                            {mainWalletId === w.id && (
                              <span className="font-mono text-[9px] px-1.5 py-0.5 rounded-full shrink-0"
                                style={{ background: 'rgba(74,222,128,0.12)', color: '#4ADE80' }}>main</span>
                            )}
                          </div>
                          <p className="font-mono text-[10px] text-[#888]">{w.balance_sol.toFixed(4)} SOL</p>
                        </div>
                        {activeWallet === w.id && <Check size={13} className="text-[#00A8FF] shrink-0" />}
                      </button>
                    ))}
                  </div>
                </>
              )}
            </div>
          )}

          {active && <WalletDetail wal={active} onRefresh={load} mainWalletId={mainWalletId} onMainWalletSet={onMainWalletSet} />}
        </>
      )}
    </div>
  )
}

// ── Wallets modal ─────────────────────────────────────────────────────────────

// Solana logo inline SVG
const SolanaIcon = ({ size = 16 }: { size?: number }) => (
  <svg width={size} height={size} viewBox="0 0 397.7 311.7" fill="none" xmlns="http://www.w3.org/2000/svg">
    <linearGradient id="sol-a" x1="360.879" y1="351.455" x2="141.213" y2="-69.294" gradientUnits="userSpaceOnUse">
      <stop offset="0" stopColor="#9945ff"/><stop offset=".91" stopColor="#14f195"/>
    </linearGradient>
    <linearGradient id="sol-b" x1="264.829" y1="401.601" x2="45.163" y2="-19.148" gradientUnits="userSpaceOnUse">
      <stop offset="0" stopColor="#9945ff"/><stop offset=".91" stopColor="#14f195"/>
    </linearGradient>
    <linearGradient id="sol-c" x1="312.548" y1="376.688" x2="92.882" y2="-44.061" gradientUnits="userSpaceOnUse">
      <stop offset="0" stopColor="#9945ff"/><stop offset=".91" stopColor="#14f195"/>
    </linearGradient>
    <path d="M64.6 237.9a11 11 0 017.7-3.2h317.6c4.9 0 7.3 5.9 3.9 9.4l-62.7 62.7a11 11 0 01-7.7 3.2H5.8c-4.9 0-7.3-5.9-3.9-9.4l62.7-62.7z" fill="url(#sol-a)"/>
    <path d="M64.6 3.2A11.2 11.2 0 0172.3 0h317.6c4.9 0 7.3 5.9 3.9 9.4L331.1 72a11 11 0 01-7.7 3.2H5.8C.9 75.2-1.5 69.3 1.9 65.9L64.6 3.2z" fill="url(#sol-b)"/>
    <path d="M333.1 120.1a11 11 0 00-7.7-3.2H5.8c-4.9 0-7.3 5.9-3.9 9.4l62.7 62.7a11 11 0 007.7 3.2h317.6c4.9 0 7.3-5.9 3.9-9.4l-62.7-62.7z" fill="url(#sol-c)"/>
  </svg>
)


type WalletView = 'overview' | 'deposit' | 'withdraw'

// Backend enforces 10 SOL per-call + 50 SOL/day/user (see orchestrator main.go
// maxPerWithdrawSOL / maxDailyWithdrawSOL). Surface them here so users don't
// hit a 400 with no context.
const WITHDRAW_MAX_PER_CALL_SOL = 10
const WITHDRAW_MAX_DAILY_SOL    = 50

// MAX button must truncate, not round — `(0.00009).toFixed(4) === "0.0001"`
// used to be a footgun that overspent the wallet by the rounded-up delta.
function truncToDecimals(n: number, digits: number): string {
  const f = Math.pow(10, digits)
  return (Math.floor(n * f) / f).toFixed(digits)
}

// validateWithdraw returns an error string or '' if the inputs are OK.
// Client-side validation is defense-in-depth — the backend validates again.
function validateWithdraw(to: string, amountStr: string, balance: number): string {
  const addr = to.trim()
  // base58 alphabet (no 0/O/I/l), Solana pubkeys are typically 32-44 chars
  if (!/^[1-9A-HJ-NP-Za-km-z]{32,44}$/.test(addr)) {
    return 'Recipient is not a valid Solana address'
  }
  // strict decimal: "1", "0.5", "1.2345" — no scientific, no commas, no negatives
  if (!/^\d+(\.\d+)?$/.test(amountStr.trim())) {
    return 'Amount must be a decimal number (e.g. 0.5)'
  }
  const amount = Number.parseFloat(amountStr)
  if (!Number.isFinite(amount) || amount <= 0) return 'Amount must be greater than 0'
  if (amount > balance)                        return `Amount exceeds balance (${balance.toFixed(4)} SOL)`
  if (amount > WITHDRAW_MAX_PER_CALL_SOL)      return `Per-call limit is ${WITHDRAW_MAX_PER_CALL_SOL} SOL`
  return ''
}

function WalletDetail({ wal, onRefresh, mainWalletId, onMainWalletSet }: {
  wal: WalletEntry
  onRefresh: () => void
  mainWalletId?: string
  onMainWalletSet?: () => void
}) {
  const [view, setView]         = useState<WalletView>('overview')
  const [addrCopied, setAddrCopied] = useState(false)
  const [sendTo,    setSendTo]   = useState('')
  const [sendAmt,   setSendAmt]  = useState('')
  const [sending,   setSending]  = useState(false)
  const [sendErr,   setSendErr]  = useState('')
  const [txHash,    setTxHash]   = useState('')
  const [settingMain, setSettingMain] = useState(false)
  const prevId = useRef(wal.id)

  // reset when wallet switches
  useEffect(() => {
    if (prevId.current !== wal.id) {
      setView('overview'); setSendTo(''); setSendAmt(''); setSendErr(''); setTxHash('')
      prevId.current = wal.id
    }
  }, [wal.id])

  const copyAddr = () => {
    navigator.clipboard.writeText(wal.address)
    setAddrCopied(true)
    setTimeout(() => setAddrCopied(false), 1500)
  }

  const handleWithdraw = async () => {
    const err = validateWithdraw(sendTo, sendAmt, wal.balance_sol)
    if (err) { setSendErr(err); return }
    // Final confirm — address is irreversible and users paste from exchanges.
    const full = sendTo.trim()
    const preview = `${full.slice(0, 8)}…${full.slice(-8)}`
    if (!window.confirm(`Send ${sendAmt} SOL to ${preview}?\n\nThis cannot be undone.`)) return
    setSending(true); setSendErr(''); setTxHash('')
    try {
      const res = await api.withdraw(wal.id, full, sendAmt.trim())
      setTxHash(res.tx_hash)
      setSendTo(''); setSendAmt('')
      onRefresh()
    } catch (e: unknown) {
      setSendErr(e instanceof Error ? e.message : 'Transaction failed')
    } finally { setSending(false) }
  }

  const isMain = mainWalletId === wal.id
  const handleSetMain = async () => {
    if (isMain || settingMain) return
    setSettingMain(true)
    try { await api.setMainWallet(wal.id); onMainWalletSet?.(); onRefresh() }
    catch { /* ignore */ } finally { setSettingMain(false) }
  }

  const VIEWS: { id: WalletView; label: string; icon: React.ReactNode }[] = [
    { id: 'overview', label: 'Overview', icon: <Wallet size={13} /> },
    { id: 'deposit',  label: 'Deposit',  icon: <QrCode size={13} /> },
    { id: 'withdraw', label: 'Withdraw', icon: <PaperPlaneTilt size={13} /> },
  ]

  return (
    <div>
      {/* Balance card */}
      <div style={{
        borderRadius: 16,
        background: 'linear-gradient(135deg, #0f1318 0%, #111827 100%)',
        border: '1px solid rgba(255,255,255,0.06)',
        padding: '20px 20px 18px',
        marginBottom: 16,
      }}>
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-full flex items-center justify-center" style={{ background: 'rgba(153,69,255,0.15)' }}>
              <SolanaIcon size={14} />
            </div>
            <span className="font-mono text-xs text-[#666]">{wal.label || 'Wallet'}</span>
            {isMain && (
              <span className="font-mono text-[9px] px-1.5 py-0.5 rounded-full uppercase tracking-widest"
                style={{ background: 'rgba(0,168,255,0.12)', color: '#00A8FF' }}>
                Trading
              </span>
            )}
          </div>
          <div className="flex items-center gap-2">
            {!isMain && (
              <button
                onClick={handleSetMain}
                disabled={settingMain}
                className="font-mono text-[10px] px-2.5 py-1 rounded-lg transition-colors disabled:opacity-40"
                style={{ background: 'rgba(255,255,255,0.05)', color: '#555' }}
                onMouseEnter={e => { (e.currentTarget as HTMLButtonElement).style.color = '#fff' }}
                onMouseLeave={e => { (e.currentTarget as HTMLButtonElement).style.color = '#555' }}
              >
                {settingMain ? '…' : 'Set as main'}
              </button>
            )}
            <span className="font-mono text-[10px] text-[#666]">SOLANA MAINNET</span>
          </div>
        </div>
        <div className="mb-1">
          <span className="font-mono text-3xl font-bold text-white">{wal.balance_sol.toFixed(4)}</span>
          <span className="font-mono text-sm text-[#888] ml-2">SOL</span>
        </div>
        <div className="flex items-center gap-2 mt-3">
          <span className="font-mono text-[10px] text-[#666] flex-1 truncate">
            {wal.address.slice(0, 12)}…{wal.address.slice(-8)}
          </span>
          <button onClick={copyAddr} className="flex items-center gap-1 text-[#666] hover:text-white transition-colors shrink-0">
            {addrCopied ? <Check size={11} className="text-[#4ADE80]" /> : <Copy size={11} />}
          </button>
        </div>
      </div>

      {/* View pill nav */}
      <div className="flex gap-1 mb-5 p-1 rounded-xl" style={{ background: 'rgba(255,255,255,0.04)' }}>
        {VIEWS.map(v => (
          <button
            key={v.id}
            onClick={() => setView(v.id)}
            className="flex-1 flex items-center justify-center gap-1.5 py-2 rounded-lg font-mono text-xs transition-all"
            style={{
              background: view === v.id ? '#111' : 'transparent',
              color: view === v.id ? '#00A8FF' : '#555',
              boxShadow: view === v.id ? '0 2px 8px rgba(0,0,0,0.4)' : 'none',
              fontWeight: view === v.id ? 600 : 400,
            }}
          >
            {v.icon} {v.label}
          </button>
        ))}
      </div>

      {/* ── Overview ── */}
      {view === 'overview' && (
        <div style={{
          padding: '14px 16px', borderRadius: 14,
          background: '#0d0d0d',
          boxShadow: 'inset 2px 2px 6px #070707, inset -1px -1px 4px rgba(255,255,255,0.02)',
        }}>
          <p className="font-mono text-[10px] text-[#888] uppercase tracking-widest mb-3">Wallet Address</p>
          <div className="flex items-center gap-2">
            <span className="font-mono text-xs text-[#a0a0a0] flex-1 break-all leading-relaxed">{wal.address}</span>
            <button onClick={copyAddr} className="text-[#666] hover:text-white transition-colors shrink-0 ml-1">
              {addrCopied ? <Check size={13} className="text-[#4ADE80]" /> : <Copy size={13} />}
            </button>
          </div>
        </div>
      )}

      {/* ── Deposit ── */}
      {view === 'deposit' && (
        <div className="text-center">
          <div className="inline-flex p-4 rounded-2xl mb-4" style={{ background: '#fff' }}>
            <QRCodeSVG value={wal.address} size={180} bgColor="#ffffff" fgColor="#0a0a0a" level="M" />
          </div>
          <p className="font-mono text-[10px] text-[#888] uppercase tracking-widest mb-1">Scan to deposit SOL</p>
          <p className="font-mono text-[10px] text-[#333] mb-4">Solana network only</p>
          <div className="flex items-center gap-2 p-3 rounded-xl text-left" style={{ background: '#0d0d0d', boxShadow: 'inset 2px 2px 6px #070707' }}>
            <span className="font-mono text-[10px] text-[#666] flex-1 break-all leading-relaxed">{wal.address}</span>
            <button onClick={copyAddr} className="text-[#666] hover:text-white transition-colors shrink-0">
              {addrCopied ? <Check size={13} className="text-[#4ADE80]" /> : <Copy size={13} />}
            </button>
          </div>
        </div>
      )}

      {/* ── Withdraw ── */}
      {view === 'withdraw' && (
        <div className="space-y-3">
          {/* Available */}
          <div className="flex items-center justify-between px-3 py-2.5 rounded-xl" style={{ background: '#0d0d0d', boxShadow: 'inset 2px 2px 6px #070707' }}>
            <span className="font-mono text-xs text-[#888]">Available</span>
            <div className="flex items-center gap-1.5">
              <SolanaIcon size={12} />
              <span className="font-mono text-xs font-bold text-white">{wal.balance_sol.toFixed(4)} SOL</span>
            </div>
          </div>

          {/* Recipient */}
          <div>
            <label className="font-mono text-[10px] text-[#888] uppercase tracking-wider block mb-1.5">Recipient Address</label>
            <input
              type="text"
              value={sendTo}
              onChange={e => setSendTo(e.target.value)}
              placeholder="Solana address (base58)"
              className="w-full neu-card-inset rounded-xl px-3 py-2.5 font-mono text-xs text-white placeholder-[#333] outline-none"
            />
          </div>

          {/* Amount */}
          <div>
            <label className="font-mono text-[10px] text-[#888] uppercase tracking-wider block mb-1.5">Amount (SOL)</label>
            <div className="flex items-center neu-card-inset rounded-xl overflow-hidden">
              <input
                type="text"
                inputMode="decimal"
                value={sendAmt}
                onChange={e => setSendAmt(e.target.value)}
                placeholder="0.000"
                className="flex-1 px-3 py-2.5 bg-transparent font-mono text-sm text-white placeholder-[#333] outline-none"
              />
              <button
                onClick={() => setSendAmt(truncToDecimals(Math.min(wal.balance_sol, WITHDRAW_MAX_PER_CALL_SOL), 4))}
                className="px-3 py-1.5 mr-1.5 rounded-lg font-mono text-[10px] font-bold tracking-widest"
                style={{ background: 'rgba(0,168,255,0.1)', color: '#00A8FF' }}
              >
                MAX
              </button>
            </div>
            <p className="font-mono text-[10px] text-[#555] mt-1.5">
              Limits: {WITHDRAW_MAX_PER_CALL_SOL} SOL per withdrawal · {WITHDRAW_MAX_DAILY_SOL} SOL per day
            </p>
          </div>

          {sendErr && <p className="font-mono text-xs text-[#EF4444]">{sendErr}</p>}

          {txHash && (
            <div className="flex items-center gap-2 p-3 rounded-xl" style={{ background: 'rgba(74,222,128,0.06)', border: '1px solid rgba(74,222,128,0.15)' }}>
              <Check size={13} className="text-[#4ADE80] shrink-0" />
              <a
                href={`https://solscan.io/tx/${txHash}`}
                target="_blank"
                rel="noopener noreferrer"
                className="font-mono text-[10px] text-[#4ADE80] flex-1 truncate underline decoration-dotted underline-offset-2"
              >
                Sent · {txHash.slice(0, 16)}… ↗
              </a>
            </div>
          )}

          <button
            onClick={handleWithdraw}
            disabled={sending || !sendTo || !sendAmt}
            className="w-full py-3 rounded-xl font-mono text-xs font-bold flex items-center justify-center gap-2 transition-all disabled:opacity-40"
            style={{ background: 'rgba(0,168,255,0.12)', color: '#00A8FF' }}
          >
            <PaperPlaneTilt size={13} weight="fill" />
            {sending ? 'Sending…' : 'Send SOL'}
          </button>

          {/* Warning */}
          <div className="flex items-start gap-2 p-3 rounded-xl" style={{ background: 'rgba(251,191,36,0.05)', border: '1px solid rgba(251,191,36,0.1)' }}>
            <Warning size={12} className="text-[#F59E0B] shrink-0 mt-0.5" weight="fill" />
            <span className="font-mono text-[10px] text-[#888] leading-relaxed">
              Double-check the address. Blockchain transactions are irreversible.
            </span>
          </div>
        </div>
      )}
    </div>
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
          <p className="font-mono text-xs text-[#888] mt-0.5">{time.toISOString().replace('T', ' ').slice(0, 19)} UTC</p>
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
            <p className="font-mono text-xs text-[#888] uppercase tracking-widest mb-3">{card.label}</p>
            <p className="font-mono font-bold leading-none" style={{ fontSize: 28, color: card.color }}>
              {card.value}
              <span className="text-base ml-1 opacity-60">{card.unit}</span>
            </p>
            <p className="font-mono text-xs text-[#666] mt-2">{card.sub}</p>
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
            <p className="font-mono text-xs text-[#888] uppercase tracking-widest">P&L Today — SOL</p>
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
          <p className="font-mono text-xs text-[#888] uppercase tracking-widest mb-4">7-Day Trades</p>
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
          <p className="font-mono text-xs text-[#888] uppercase tracking-widest mb-4">
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
          <p className="font-mono text-xs text-[#888] uppercase tracking-widest mb-4">Recent Trades</p>
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

const LOG_META: Record<string, { color: string; bg: string; icon: React.ReactNode }> = {
  ENTER: { color: '#4ADE80', bg: 'bg-[#4ADE80]/10', icon: <ArrowUp size={12} weight="bold" /> },
  EXIT:  { color: '#00A8FF', bg: 'bg-[#00A8FF]/10', icon: <ArrowDown size={12} weight="bold" /> },
  START: { color: '#4ADE80', bg: 'bg-[#4ADE80]/10', icon: <Lightning size={12} weight="fill" /> },
  STOP:  { color: '#F59E0B', bg: 'bg-[#F59E0B]/10', icon: <Stop size={12} weight="fill" /> },
  ALERT: { color: '#EF4444', bg: 'bg-[#EF4444]/10', icon: <Warning size={12} weight="fill" /> },
  INFO:  { color: '#555',    bg: 'bg-white/5',       icon: <Info size={12} weight="fill" /> },
}

function ClosedTradeCard({ t }: { t: ClosedPosition }) {
  const isWin = t.pnl_sol >= 0
  const isGreenReason = t.reason.startsWith('take') || t.reason === 'scalp'
  const [sharing, setSharing] = useState(false)

  const handleShare = async () => {
    setSharing(true)
    try {
      await api.downloadCard(t.mint)
      window.umami?.track('pnl_card_shared', { product: 'hummingbird' })
    } catch {
      // fallback: wkhtmltoimage may not be installed yet
    } finally {
      setSharing(false)
    }
  }

  return (
    <div className="flex items-center gap-3 px-4 py-3 border-b border-white/[0.04] last:border-0 hover:bg-white/[0.015] transition-colors group">
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-0.5">
          <span className="font-mono text-xs text-white truncate">{shortMint(t.mint)}</span>
          <span className="font-mono text-[9px] px-1.5 py-0.5 rounded-full bg-[#00A8FF]/10 text-[#00A8FF] shrink-0">
            {t.score >= 75 ? 'SNP' : 'SCP'}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <span className={`font-mono text-[10px] px-1.5 py-0.5 rounded-full ${isGreenReason ? 'bg-[#4ADE80]/10 text-[#4ADE80]' : 'bg-[#EF4444]/10 text-[#EF4444]'}`}>
            {t.reason.replace('_', ' ').toUpperCase()}
          </span>
          <span className="font-mono text-[10px] text-[#333]">{new Date(t.closed_at).toLocaleTimeString()}</span>
        </div>
      </div>
      <div className="flex items-center gap-3 shrink-0">
        <button
          onClick={handleShare}
          disabled={sharing}
          title="Download PnL card"
          className="transition-opacity text-[#888] hover:text-white p-1.5 rounded border border-white/10 hover:border-white/30 disabled:opacity-40 flex items-center"
        >
          {sharing ? <Spinner size={12} className="animate-spin" /> : <ShareNetwork size={12} />}
        </button>
        <div className="text-right">
          <p className={`font-mono text-sm font-bold ${isWin ? 'text-[#4ADE80]' : 'text-[#EF4444]'}`}>
            {isWin ? '+' : ''}{t.pnl_sol.toFixed(3)}
          </p>
          <p className={`font-mono text-[10px] ${isWin ? 'text-[#4ADE80]' : 'text-[#EF4444]'} opacity-70`}>
            {t.pnl_percent >= 0 ? '+' : ''}{t.pnl_percent.toFixed(1)}%
          </p>
        </div>
      </div>
    </div>
  )
}

function ActivityItem({ log }: { log: LogEntry }) {
  const meta = LOG_META[log.type] ?? LOG_META.INFO
  return (
    <div className="flex items-start gap-3 px-4 py-2.5 border-b border-white/[0.04] last:border-0 hover:bg-white/[0.015] transition-colors">
      <span className={`shrink-0 mt-0.5 w-5 h-5 rounded-full flex items-center justify-center ${meta.bg}`} style={{ color: meta.color }}>
        {meta.icon}
      </span>
      <div className="flex-1 min-w-0">
        <p className="font-mono text-[11px] text-[#a0a0a0] leading-relaxed break-words">{log.message}</p>
        {log.type === 'EXIT' && log.pnl_sol !== undefined && (
          <span className={`font-mono text-[10px] font-bold ${log.pnl_sol >= 0 ? 'text-[#4ADE80]' : 'text-[#EF4444]'}`}>
            {log.pnl_sol >= 0 ? '+' : ''}{log.pnl_sol.toFixed(4)} SOL
          </span>
        )}
      </div>
      <span className="shrink-0 font-mono text-[10px] text-[#333] mt-0.5">{new Date(log.time).toLocaleTimeString()}</span>
    </div>
  )
}

function DepositDialog({ wal, onClose }: { wal: WalletEntry; onClose: () => void }) {
  const [copied, setCopied] = useState(false)
  const copy = () => {
    navigator.clipboard.writeText(wal.address)
    setCopied(true); setTimeout(() => setCopied(false), 1500)
  }
  return (
    <Modal onClose={onClose} maxWidth={400}>
      <ModalHeader icon={<QrCode size={16} />} title="Deposit SOL" sub={wal.label || 'Wallet'} onClose={onClose} />
      <div className="p-5 text-center">
        <div className="inline-flex p-4 rounded-2xl mb-4" style={{ background: '#fff' }}>
          <QRCodeSVG value={wal.address} size={180} bgColor="#ffffff" fgColor="#0a0a0a" level="M" />
        </div>
        <p className="font-mono text-[10px] text-[#888] uppercase tracking-widest mb-1">Scan to deposit SOL</p>
        <p className="font-mono text-[10px] text-[#333] mb-4">Solana network only</p>
        <div className="flex items-center gap-2 p-3 rounded-xl text-left" style={{ background: '#0d0d0d', boxShadow: 'inset 2px 2px 6px #070707' }}>
          <span className="font-mono text-[10px] text-[#666] flex-1 break-all leading-relaxed">{wal.address}</span>
          <button onClick={copy} className="text-[#666] hover:text-white transition-colors shrink-0">
            {copied ? <Check size={13} className="text-[#4ADE80]" /> : <Copy size={13} />}
          </button>
        </div>
      </div>
    </Modal>
  )
}

function WithdrawDialog({ wal, onClose, onDone }: { wal: WalletEntry; onClose: () => void; onDone: () => void }) {
  const [sendTo,  setSendTo]  = useState('')
  const [sendAmt, setSendAmt] = useState('')
  const [sending, setSending] = useState(false)
  const [sendErr, setSendErr] = useState('')
  const [txHash,  setTxHash]  = useState('')

  const handleWithdraw = async () => {
    const err = validateWithdraw(sendTo, sendAmt, wal.balance_sol)
    if (err) { setSendErr(err); return }
    const full = sendTo.trim()
    const preview = `${full.slice(0, 8)}…${full.slice(-8)}`
    if (!window.confirm(`Send ${sendAmt} SOL to ${preview}?\n\nThis cannot be undone.`)) return
    setSending(true); setSendErr(''); setTxHash('')
    try {
      const res = await api.withdraw(wal.id, full, sendAmt.trim())
      setTxHash(res.tx_hash); setSendTo(''); setSendAmt('')
      onDone()
    } catch (e: unknown) {
      setSendErr(e instanceof Error ? e.message : 'Transaction failed')
    } finally { setSending(false) }
  }

  return (
    <Modal onClose={onClose} maxWidth={420}>
      <ModalHeader icon={<PaperPlaneTilt size={16} />} title="Send SOL" sub={`${wal.balance_sol.toFixed(4)} SOL available`} onClose={onClose} />
      <div className="p-5 space-y-3">
        <div className="flex items-center justify-between px-3 py-2.5 rounded-xl" style={{ background: '#0d0d0d', boxShadow: 'inset 2px 2px 6px #070707' }}>
          <span className="font-mono text-xs text-[#888]">Available</span>
          <div className="flex items-center gap-1.5">
            <SolanaIcon size={12} />
            <span className="font-mono text-xs font-bold text-white">{wal.balance_sol.toFixed(4)} SOL</span>
          </div>
        </div>
        <div>
          <label className="font-mono text-[10px] text-[#888] uppercase tracking-wider block mb-1.5">Recipient Address</label>
          <input type="text" value={sendTo} onChange={e => setSendTo(e.target.value)}
            placeholder="Solana address (base58)"
            className="w-full neu-card-inset rounded-xl px-3 py-2.5 font-mono text-xs text-white placeholder-[#333] outline-none" />
        </div>
        <div>
          <label className="font-mono text-[10px] text-[#888] uppercase tracking-wider block mb-1.5">Amount (SOL)</label>
          <div className="flex items-center neu-card-inset rounded-xl overflow-hidden">
            <input type="text" inputMode="decimal" value={sendAmt} onChange={e => setSendAmt(e.target.value)}
              placeholder="0.000"
              className="flex-1 px-3 py-2.5 bg-transparent font-mono text-sm text-white placeholder-[#333] outline-none" />
            <button onClick={() => setSendAmt(truncToDecimals(Math.min(wal.balance_sol, WITHDRAW_MAX_PER_CALL_SOL), 4))}
              className="px-3 py-1.5 mr-1.5 rounded-lg font-mono text-[10px] font-bold tracking-widest"
              style={{ background: 'rgba(0,168,255,0.1)', color: '#00A8FF' }}>MAX</button>
          </div>
          <p className="font-mono text-[10px] text-[#555] mt-1.5">
            Limits: {WITHDRAW_MAX_PER_CALL_SOL} SOL per withdrawal · {WITHDRAW_MAX_DAILY_SOL} SOL per day
          </p>
        </div>
        {sendErr && <p className="font-mono text-xs text-[#EF4444]">{sendErr}</p>}
        {txHash && (
          <div className="flex items-center gap-2 p-3 rounded-xl" style={{ background: 'rgba(74,222,128,0.06)', border: '1px solid rgba(74,222,128,0.15)' }}>
            <Check size={13} className="text-[#4ADE80] shrink-0" />
            <span className="font-mono text-[10px] text-[#4ADE80] flex-1 truncate">Sent · {txHash.slice(0, 16)}…</span>
          </div>
        )}
        <button onClick={handleWithdraw} disabled={sending || !sendTo || !sendAmt}
          className="w-full py-3 rounded-xl font-mono text-xs font-bold flex items-center justify-center gap-2 transition-all disabled:opacity-40"
          style={{ background: 'rgba(0,168,255,0.12)', color: '#00A8FF' }}>
          <PaperPlaneTilt size={13} weight="fill" />
          {sending ? 'Sending…' : 'Send SOL'}
        </button>
        <div className="flex items-start gap-2 p-3 rounded-xl" style={{ background: 'rgba(251,191,36,0.05)', border: '1px solid rgba(251,191,36,0.1)' }}>
          <Warning size={12} className="text-[#F59E0B] shrink-0 mt-0.5" weight="fill" />
          <span className="font-mono text-[10px] text-[#888] leading-relaxed">Double-check the address. Blockchain transactions are irreversible.</span>
        </div>
      </div>
    </Modal>
  )
}

function HoldingsList({ holdings, onSold }: {
  holdings: { mint: string; ui_amount: number }[]
  onSold: () => void
}) {
  const [selling, setSelling] = useState<string | null>(null)
  const [result,  setResult]  = useState<Record<string, 'ok' | 'err'>>({})

  const sell = async (mint: string) => {
    if (selling) return
    setSelling(mint)
    try {
      await api.forceSell(mint)
      setResult(r => ({ ...r, [mint]: 'ok' }))
      setTimeout(onSold, 2000) // refresh holdings after a short delay
    } catch {
      setResult(r => ({ ...r, [mint]: 'err' }))
    } finally {
      setSelling(null)
    }
  }

  return (
    <div>
      <p className="font-mono text-[10px] text-[#888] uppercase tracking-widest mb-2">Token Holdings</p>
      <div className="flex flex-col gap-1">
        {holdings.map(h => (
          <div key={h.mint} className="flex items-center gap-2 px-3 py-2 rounded-lg" style={{ background: 'rgba(255,255,255,0.03)' }}>
            <a
              href={`https://solscan.io/token/${h.mint}`}
              target="_blank"
              rel="noreferrer"
              className="font-mono text-[10px] text-[#00A8FF] hover:text-white transition-colors flex-1 truncate"
            >
              {h.mint.slice(0, 8)}…{h.mint.slice(-6)}
            </a>
            <span className="font-mono text-[10px] text-[#aaa] shrink-0 mr-1">
              {h.ui_amount.toLocaleString(undefined, { maximumFractionDigits: 0 })}
            </span>
            {result[h.mint] === 'ok' ? (
              <span className="font-mono text-[10px] text-[#4ADE80]">sold</span>
            ) : result[h.mint] === 'err' ? (
              <span className="font-mono text-[10px] text-[#EF4444]">failed</span>
            ) : (
              <button
                onClick={() => sell(h.mint)}
                disabled={!!selling}
                className="font-mono text-[10px] px-2 py-0.5 rounded transition-colors disabled:opacity-40 shrink-0"
                style={{ background: 'rgba(239,68,68,0.1)', color: '#EF4444', border: '1px solid rgba(239,68,68,0.2)' }}
              >
                {selling === h.mint ? <Spinner size={10} className="animate-spin" /> : 'Sell'}
              </button>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

function WalletCard({ mainWalletId, onMainWalletSet }: { mainWalletId?: string; onMainWalletSet?: () => void }) {
  const [wallets,      setWallets]      = useState<WalletEntry[]>([])
  const [loading,      setLoading]      = useState(true)
  const [creating,     setCreating]     = useState(false)
  const [createLabel,  setCreateLabel]  = useState('')
  const [showCreate,   setShowCreate]   = useState(false)
  const [activeWallet, setActiveWallet] = useState<string | null>(null)
  const [dropOpen,     setDropOpen]     = useState(false)
  const [showDeposit,  setShowDeposit]  = useState(false)
  const [showWithdraw, setShowWithdraw] = useState(false)
  const [settingMain,  setSettingMain]  = useState(false)
  const [addrCopied,   setAddrCopied]   = useState(false)

  const [refreshing, setRefreshing] = useState(false)
  const [holdings,   setHoldings]   = useState<{ mint: string; ui_amount: number }[]>([])

  const load = (silent = false) => {
    if (!silent) setLoading(true)
    else setRefreshing(true)
    api.wallets().then(ws => {
      setWallets(ws)
      setActiveWallet(prev => prev ?? (ws.find(w => w.id === mainWalletId)?.id ?? ws[0]?.id ?? null))
    }).catch(() => {}).finally(() => { setLoading(false); setRefreshing(false) })
    api.holdings().then(setHoldings).catch(() => {})
  }
  useEffect(() => {
    load()
    const id = setInterval(() => load(true), 5 * 60 * 1000) // refresh every 5 min
    return () => clearInterval(id)
  }, [])

  const handleCreate = async () => {
    if (creating) return
    setCreating(true)
    try {
      await api.createWallet(createLabel.trim() || undefined)
      setCreateLabel(''); setShowCreate(false); load()
    } catch { /* ignore */ } finally { setCreating(false) }
  }

  const handleSetMain = async (walId: string) => {
    if (settingMain) return
    setSettingMain(true)
    try { await api.setMainWallet(walId); onMainWalletSet?.(); load() }
    catch { /* ignore */ } finally { setSettingMain(false) }
  }

  const copyAddr = (addr: string) => {
    navigator.clipboard.writeText(addr)
    setAddrCopied(true); setTimeout(() => setAddrCopied(false), 1500)
  }

  const active = wallets.find(w => w.id === activeWallet)
  const isMain = activeWallet === mainWalletId

  return (
    <>
      <div className="neu-tile flex flex-col overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-white/[0.04] shrink-0">
          <div className="flex items-center gap-2.5">
            <div className="w-5 h-5 rounded-full flex items-center justify-center shrink-0" style={{ background: 'rgba(153,69,255,0.15)' }}>
              <SolanaIcon size={11} />
            </div>
            <span className="font-mono text-xs font-bold text-white">Solana Wallets</span>
          </div>
          <button onClick={() => load(true)} disabled={refreshing}
            className="text-[#666] hover:text-white transition-colors disabled:opacity-40"
            title="Refresh balance">
            <ArrowsClockwise size={13} className={refreshing ? 'animate-spin' : ''} />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-4 flex flex-col gap-4">
          {loading ? (
            <p className="font-mono text-xs text-[#333] text-center py-8">Loading…</p>
          ) : wallets.length === 0 ? (
            <div className="text-center py-8">
              <p className="font-mono text-xs text-[#666] mb-4">No wallets yet.</p>
              {showCreate ? (
                <div className="flex gap-2 justify-center">
                  <input autoFocus type="text" placeholder="Label (optional)"
                    value={createLabel} onChange={e => setCreateLabel(e.target.value)}
                    onKeyDown={e => e.key === 'Enter' && handleCreate()}
                    className="neu-card-inset rounded-xl px-3 py-2 font-mono text-xs text-white placeholder-[#444] outline-none w-40" />
                  <button onClick={handleCreate} disabled={creating}
                    className="px-4 py-2 rounded-xl font-mono text-xs disabled:opacity-40"
                    style={{ background: 'rgba(0,168,255,0.1)', color: '#00A8FF' }}>
                    {creating ? '…' : 'Create'}
                  </button>
                </div>
              ) : (
                <button onClick={() => setShowCreate(true)}
                  className="flex items-center gap-2 mx-auto px-4 py-2.5 rounded-xl font-mono text-xs text-[#888] hover:text-white transition-colors"
                  style={{ background: 'rgba(255,255,255,0.04)' }}>
                  <Plus size={13} /> Create first wallet
                </button>
              )}
            </div>
          ) : (
            <>
              {/* Wallet switcher */}
              <div className="relative">
                <button onClick={() => setDropOpen(v => !v)}
                  className="w-full flex items-center gap-3 px-4 py-3 rounded-xl transition-all"
                  style={{ background: 'rgba(255,255,255,0.04)', border: '1px solid rgba(255,255,255,0.06)' }}>
                  <div className="w-7 h-7 rounded-full flex items-center justify-center shrink-0" style={{ background: 'rgba(153,69,255,0.15)' }}>
                    <SolanaIcon size={13} />
                  </div>
                  <div className="flex-1 text-left min-w-0">
                    <div className="flex items-center gap-1.5">
                      <p className="font-mono text-xs text-white font-bold truncate">{active?.label || 'Wallet'}</p>
                      {isMain && (
                        <span className="font-mono text-[9px] px-1.5 py-0.5 rounded-full shrink-0"
                          style={{ background: 'rgba(74,222,128,0.12)', color: '#4ADE80' }}>main</span>
                      )}
                    </div>
                    <p className="font-mono text-[10px] text-[#888]">{active?.balance_sol.toFixed(4)} SOL</p>
                  </div>
                  <CaretDown size={13} className="text-[#888] shrink-0" style={{ transform: dropOpen ? 'rotate(180deg)' : 'none', transition: 'transform 0.15s' }} />
                </button>

                {dropOpen && (
                  <>
                    <div className="fixed inset-0 z-10" onClick={() => setDropOpen(false)} />
                    <div className="absolute left-0 right-0 top-full mt-1.5 z-20 rounded-xl overflow-hidden"
                      style={{ background: '#111', border: '1px solid rgba(255,255,255,0.08)', boxShadow: '0 12px 40px rgba(0,0,0,0.7)' }}>
                      {wallets.map(w => (
                        <button key={w.id} onClick={() => { setActiveWallet(w.id); setDropOpen(false) }}
                          className="w-full flex items-center gap-3 px-4 py-3 transition-colors hover:bg-white/[0.04]"
                          style={{ borderBottom: '1px solid rgba(255,255,255,0.04)' }}>
                          <div className="w-6 h-6 rounded-full flex items-center justify-center shrink-0" style={{ background: 'rgba(153,69,255,0.12)' }}>
                            <SolanaIcon size={11} />
                          </div>
                          <div className="flex-1 text-left min-w-0">
                            <div className="flex items-center gap-1.5">
                              <p className="font-mono text-xs text-white truncate">{w.label || 'Wallet'}</p>
                              {mainWalletId === w.id && (
                                <span className="font-mono text-[9px] px-1.5 py-0.5 rounded-full shrink-0"
                                  style={{ background: 'rgba(74,222,128,0.12)', color: '#4ADE80' }}>main</span>
                              )}
                            </div>
                            <p className="font-mono text-[10px] text-[#888]">{w.balance_sol.toFixed(4)} SOL</p>
                          </div>
                          {activeWallet === w.id && <Check size={13} className="text-[#00A8FF] shrink-0" />}
                        </button>
                      ))}
                      {showCreate ? (
                        <div className="flex items-center gap-2 px-3 py-2.5">
                          <input autoFocus type="text" placeholder="Label (optional)…"
                            value={createLabel} onChange={e => setCreateLabel(e.target.value)}
                            onKeyDown={e => e.key === 'Enter' && handleCreate()}
                            className="flex-1 bg-[#1a1a1a] rounded-lg px-3 py-1.5 font-mono text-xs text-white placeholder-[#444] outline-none" />
                          <button onClick={handleCreate} disabled={creating}
                            className="px-3 py-1.5 rounded-lg font-mono text-[10px] disabled:opacity-40 shrink-0"
                            style={{ background: 'rgba(0,168,255,0.1)', color: '#00A8FF' }}>
                            {creating ? '…' : 'Create'}
                          </button>
                        </div>
                      ) : (
                        <button onClick={() => setShowCreate(true)}
                          className="w-full flex items-center gap-2 px-4 py-3 text-[#888] hover:text-white transition-colors">
                          <Plus size={13} /><span className="font-mono text-xs">New wallet</span>
                        </button>
                      )}
                    </div>
                  </>
                )}
              </div>

              {/* Balance + actions — compact row */}
              {active && (
                <div style={{ borderRadius: 14, background: 'linear-gradient(135deg, #0f1318 0%, #111827 100%)', border: '1px solid rgba(255,255,255,0.06)', padding: '12px 14px' }}>
                  <div className="flex items-center justify-between mb-1.5">
                    <div>
                      <span className="font-mono text-2xl font-bold text-white">{active.balance_sol.toFixed(4)}</span>
                      <span className="font-mono text-xs text-[#888] ml-1.5">SOL</span>
                      {isMain
                        ? <span className="ml-2 font-mono text-[9px] px-1.5 py-0.5 rounded-full" style={{ background: 'rgba(0,168,255,0.12)', color: '#00A8FF' }}>Trading</span>
                        : <button onClick={() => handleSetMain(active.id)} disabled={settingMain}
                            className="ml-2 font-mono text-[9px] px-1.5 py-0.5 rounded-full transition-colors disabled:opacity-40"
                            style={{ background: 'rgba(255,255,255,0.05)', color: '#888' }}>
                            {settingMain ? '…' : 'Set main'}
                          </button>
                      }
                    </div>
                    <div className="flex gap-1.5">
                      <button onClick={() => setShowDeposit(true)}
                        className="flex items-center gap-1 px-2.5 py-1.5 rounded-lg font-mono text-[10px] transition-all"
                        style={{ background: 'rgba(74,222,128,0.08)', color: '#4ADE80', border: '1px solid rgba(74,222,128,0.12)' }}>
                        <QrCode size={11} /> Deposit
                      </button>
                      <button onClick={() => setShowWithdraw(true)}
                        className="flex items-center gap-1 px-2.5 py-1.5 rounded-lg font-mono text-[10px] transition-all"
                        style={{ background: 'rgba(0,168,255,0.08)', color: '#00A8FF', border: '1px solid rgba(0,168,255,0.12)' }}>
                        <PaperPlaneTilt size={11} /> Send
                      </button>
                    </div>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <span className="font-mono text-[10px] text-[#666] flex-1 truncate">
                      {active.address.slice(0, 14)}…{active.address.slice(-8)}
                    </span>
                    <button onClick={() => copyAddr(active.address)} className="text-[#666] hover:text-white transition-colors shrink-0">
                      {addrCopied ? <Check size={10} className="text-[#4ADE80]" /> : <Copy size={10} />}
                    </button>
                  </div>
                </div>
              )}

              {/* Token Holdings */}
              {holdings.length > 0 && (
                <HoldingsList holdings={holdings} onSold={load} />
              )}
            </>
          )}
        </div>
      </div>

      {showDeposit  && active && <DepositDialog  wal={active} onClose={() => setShowDeposit(false)} />}
      {showWithdraw && active && <WithdrawDialog wal={active} onClose={() => setShowWithdraw(false)} onDone={() => { load(); setShowWithdraw(false) }} />}
    </>
  )
}

function TabAccounts({ positions, closed, mainWalletId, onMainWalletSet }: {
  positions: Position[]
  closed: ClosedPosition[]
  mainWalletId?: string
  onMainWalletSet?: () => void
}) {
  const [txLogs, setTxLogs] = useState<LogEntry[]>([])

  useEffect(() => {
    const fetchLogs = () => api.logs().then(all => {
      setTxLogs(all.filter(l => l.type === 'INFO'))
    }).catch(() => {})
    fetchLogs()
    const id = setInterval(fetchLogs, 8000)
    return () => clearInterval(id)
  }, [])

  const exportTxJSON = () => {
    const data = JSON.stringify(txLogs, null, 2)
    const blob = new Blob([data], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `hummingbird-transactions-${new Date().toISOString().slice(0,10)}.json`
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    // Mobile: stack four tiles vertically and let the page scroll naturally.
    // Desktop (md+): original 2x2 quadrant layout that fills the viewport.
    <div className="grid grid-cols-1 md:grid-cols-2 md:grid-rows-2 gap-3 p-3 md:overflow-hidden md:h-[calc(100vh-3.5rem)]">

      {/* Top-left — Open Positions */}
      <div className="neu-tile flex flex-col overflow-hidden min-h-[280px] md:min-h-0">
        <div className="flex items-center justify-between px-4 py-3 border-b border-white/[0.04] shrink-0">
          <span className="font-mono text-xs font-bold text-white">Open Positions</span>
          <span className="font-mono text-[10px] px-2 py-0.5 rounded-full bg-[#00A8FF]/10 text-[#00A8FF]">{positions.length}</span>
        </div>
        <div className="flex-1 overflow-y-auto">
          {positions.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-full gap-1.5">
              <p className="font-mono text-[11px] text-[#333]">No open positions.</p>
              <p className="font-mono text-[10px] text-[#222]">Bot is scanning for tokens.</p>
            </div>
          ) : (
            <div className="p-3 space-y-2">
              {positions.map(pos => <PositionCard key={pos.id} pos={pos} />)}
            </div>
          )}
        </div>
      </div>

      {/* Top-right — Wallet */}
      <WalletCard mainWalletId={mainWalletId} onMainWalletSet={onMainWalletSet} />

      {/* Bottom-left — Trade History */}
      <div className="neu-tile flex flex-col overflow-hidden min-h-[280px] md:min-h-0">
        <div className="flex items-center justify-between px-4 py-3 border-b border-white/[0.04] shrink-0">
          <span className="font-mono text-xs font-bold text-white">Trade History</span>
          <span className="font-mono text-[10px] px-2 py-0.5 rounded-full bg-white/5 text-[#888]">{closed.length}</span>
        </div>
        <div className="flex-1 overflow-y-auto">
          {closed.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-full gap-1.5">
              <p className="font-mono text-[11px] text-[#333]">No closed trades yet.</p>
            </div>
          ) : (
            <div>{closed.map((t, i) => <ClosedTradeCard key={i} t={t} />)}</div>
          )}
        </div>
      </div>

      {/* Bottom-right — Transaction History (deposits/withdrawals) */}
      <div className="neu-tile flex flex-col overflow-hidden min-h-[280px] md:min-h-0">
        <div className="flex items-center justify-between px-4 py-3 border-b border-white/[0.04] shrink-0">
          <span className="font-mono text-xs font-bold text-white">Transaction History</span>
          <button
            onClick={exportTxJSON}
            title="Export as JSON"
            className="flex items-center gap-1 text-[#888] hover:text-white px-2 py-0.5 rounded border border-white/10 hover:border-white/30 transition-colors"
          >
            <DownloadSimple size={12} />
            <span className="font-mono text-[10px]">Export</span>
          </button>
        </div>
        <div className="flex-1 overflow-y-auto">
          {txLogs.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-full gap-1.5">
              <p className="font-mono text-[11px] text-[#333]">No deposits or withdrawals yet.</p>
            </div>
          ) : (
            <div>
              {txLogs.map((log, i) => {
                const isDeposit = log.message.toLowerCase().includes('deposit')
                return (
                  <div key={i} className="flex items-center gap-3 px-4 py-2.5 border-b border-white/[0.04] last:border-0 hover:bg-white/[0.015] transition-colors">
                    <span className={`font-mono text-[10px] w-16 shrink-0 ${isDeposit ? 'text-[#4ADE80]' : 'text-[#888]'}`}>
                      {isDeposit ? '↓ IN' : '↑ OUT'}
                    </span>
                    <span className="font-mono text-[10px] text-[#aaa] flex-1 truncate">{log.message}</span>
                    {log.tx_hash && (
                      <a
                        href={`https://solscan.io/tx/${log.tx_hash}`}
                        target="_blank"
                        rel="noreferrer"
                        className="font-mono text-[10px] text-[#00A8FF] hover:text-white transition-colors shrink-0"
                      >
                        {log.tx_hash.slice(0, 6)}…{log.tx_hash.slice(-6)}
                      </a>
                    )}
                    <span className="font-mono text-[10px] text-[#888] shrink-0">
                      {new Date(log.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                    </span>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </div>

    </div>
  )
}

function TabLogs() {
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetch = () => api.logs().then(setLogs).catch(() => {}).finally(() => setLoading(false))
    fetch()
    const id = setInterval(fetch, 3000)
    return () => clearInterval(id)
  }, [])

  const exportLogs = () => {
    const blob = new Blob([JSON.stringify(logs, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `hummingbird-logs-${new Date().toISOString().slice(0, 19).replace(/:/g, '-')}.json`
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="font-mono font-bold text-xl text-white">Logs</h1>
          <p className="font-mono text-xs text-[#888] mt-0.5">Live bot activity — last 200 events</p>
        </div>
        <div className="flex items-center gap-3">
          {logs.length > 0 && (
            <button onClick={exportLogs}
              className="flex items-center gap-1.5 font-mono text-xs px-3 py-1.5 rounded-lg transition-colors"
              style={{ background: 'rgba(255,255,255,0.04)', color: '#555', border: '1px solid rgba(255,255,255,0.06)' }}
              onMouseEnter={e => (e.currentTarget.style.color = '#fff')}
              onMouseLeave={e => (e.currentTarget.style.color = '#555')}>
              <DownloadSimple size={13} /> Export JSON
            </button>
          )}
          <span className="flex items-center gap-1.5 font-mono text-xs text-[#4ADE80]">
            <span className="w-1.5 h-1.5 rounded-full bg-[#4ADE80] animate-pulse" />
            live
          </span>
        </div>
      </div>

      <div className="neu-tile overflow-hidden">
        {loading ? (
          <p className="font-mono text-xs text-[#333] text-center py-12">Loading...</p>
        ) : logs.length === 0 ? (
          <div className="flex flex-col items-center gap-2 py-16">
            <Terminal size={24} className="text-[#222]" />
            <p className="font-mono text-xs text-[#333]">No activity yet. Start the bot to see events here.</p>
          </div>
        ) : (
          <div className="divide-y divide-white/[0.03]">
            {logs.map((log, i) => {
              const meta = LOG_META[log.type] ?? LOG_META.INFO
              return (
                <div key={i} className="flex items-start gap-4 px-5 py-3 hover:bg-white/[0.015] transition-colors">
                  {/* Type badge */}
                  <span className={`shrink-0 mt-0.5 flex items-center gap-1 font-mono text-[10px] px-2 py-0.5 rounded-full ${meta.bg}`} style={{ color: meta.color }}>
                    {meta.icon}
                    {log.type}
                  </span>

                  {/* Message */}
                  <span className="font-mono text-xs text-[#a0a0a0] flex-1 leading-relaxed">{log.message}</span>

                  {/* P&L inline badge for EXIT events */}
                  {log.type === 'EXIT' && log.pnl_sol !== undefined && (
                    <span className={`shrink-0 font-mono text-xs font-bold ${log.pnl_sol >= 0 ? 'text-[#4ADE80]' : 'text-[#EF4444]'}`}>
                      {log.pnl_sol >= 0 ? '+' : ''}{log.pnl_sol.toFixed(4)} SOL
                      {log.pnl_pct !== undefined && (
                        <span className="ml-1 opacity-60 font-normal text-[10px]">({log.pnl_pct >= 0 ? '+' : ''}{log.pnl_pct.toFixed(1)}%)</span>
                      )}
                    </span>
                  )}

                  {/* Timestamp */}
                  <span className="shrink-0 font-mono text-[10px] text-[#333]">
                    {new Date(log.time).toLocaleTimeString()}
                  </span>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}

// ── Config tab ────────────────────────────────────────────────────────────────

function ConfigRow({ label, sub, value, onDec, onInc, display }: {
  label: string; sub?: string; value?: number | boolean | string
  onDec?: () => void; onInc?: () => void; display?: string
}) {
  return (
    <div className="flex items-center justify-between py-3 border-b border-white/[0.04] last:border-0">
      <div>
        <p className="font-mono text-xs text-white">{label}</p>
        {sub && <p className="font-mono text-[10px] text-[#666] mt-0.5">{sub}</p>}
      </div>
      {onDec && onInc ? (
        <div className="flex items-center gap-2">
          <button onClick={onDec}
            className="w-7 h-7 rounded-lg flex items-center justify-center font-mono text-sm text-[#666] hover:text-white hover:bg-white/8 transition-colors">
            −
          </button>
          <span className="font-mono text-xs text-[#00A8FF] min-w-[72px] text-center">{display ?? String(value)}</span>
          <button onClick={onInc}
            className="w-7 h-7 rounded-lg flex items-center justify-center font-mono text-sm text-[#666] hover:text-white hover:bg-white/8 transition-colors">
            +
          </button>
        </div>
      ) : (
        <span className="font-mono text-xs text-[#666]">{display ?? String(value)}</span>
      )}
    </div>
  )
}

function ToggleRow({ label, sub, value, onToggle }: {
  label: string; sub?: string; value: boolean; onToggle: () => void
}) {
  return (
    <div className="flex items-center justify-between py-3 border-b border-white/[0.04] last:border-0">
      <div>
        <p className="font-mono text-xs text-white">{label}</p>
        {sub && <p className="font-mono text-[10px] text-[#666] mt-0.5">{sub}</p>}
      </div>
      <button
        onClick={onToggle}
        className={`relative w-10 h-5 rounded-full transition-colors ${value ? 'bg-[#00A8FF]/30' : 'bg-white/8'}`}
      >
        <span className={`absolute top-0.5 w-4 h-4 rounded-full transition-all ${
          value ? 'left-[22px] bg-[#00A8FF]' : 'left-0.5 bg-[#444]'
        }`} />
      </button>
    </div>
  )
}

// ── Config modal ──────────────────────────────────────────────────────────────

function ConfigModal({ onClose }: { onClose: () => void }) {
  const [cfg, setCfg]         = useState<UserConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving,  setSaving]  = useState(false)
  const [saved,   setSaved]   = useState(false)
  const [error,   setError]   = useState('')

  useEffect(() => {
    api.config().then(setCfg).catch(() => {}).finally(() => setLoading(false))
  }, [])

  const update = (patch: Partial<UserConfig>) => {
    setCfg(prev => prev ? { ...prev, ...patch } : prev)
    setSaved(false)
  }

  const step = (field: keyof UserConfig, delta: number, min: number, max: number, decimals = 2) => {
    if (!cfg) return
    const raw = (cfg[field] as number) + delta
    const clamped = Math.max(min, Math.min(max, raw))
    update({ [field]: parseFloat(clamped.toFixed(decimals)) })
  }

  const handleSave = async () => {
    if (!cfg) return
    setSaving(true); setError('')
    try {
      // eslint-disable-next-line @typescript-eslint/no-unused-vars
      const { wallet_id: _, ...body } = cfg
      await api.saveConfig(body)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Save failed')
    } finally { setSaving(false) }
  }

  return (
    <Modal onClose={onClose} maxWidth={540}>
      <ModalHeader
        icon={<SlidersHorizontal size={16} className="text-[#00A8FF]" />}
        title="Config"
        sub="Per-trade and portfolio settings"
        onClose={onClose}
      />

      {loading || !cfg ? (
        <div className="flex items-center justify-center h-40">
          <Spinner size={20} className="text-[#333] animate-spin" />
        </div>
      ) : (
        <div className="p-5 space-y-3">
          {error && <p className="font-mono text-xs text-[#EF4444]">{error}</p>}

          {/* Modes */}
          <div className="neu-tile p-4">
            <p className="font-mono text-[10px] text-[#888] uppercase tracking-widest mb-3">Trading Modes</p>
            <ToggleRow label="Sniper" sub="High-score tokens (≥75) — larger position" value={cfg.sniper_enabled} onToggle={() => update({ sniper_enabled: !cfg.sniper_enabled })} />
            <ToggleRow label="Scalper" sub="Lower-score tokens — smaller position, quick exits" value={cfg.scalper_enabled} onToggle={() => update({ scalper_enabled: !cfg.scalper_enabled })} />
          </div>

          {/* Position sizing */}
          <div className="neu-tile p-4">
            <p className="font-mono text-[10px] text-[#888] uppercase tracking-widest mb-3">Position Sizing</p>
            <ConfigRow label="Max position size" sub="SOL per trade"
              value={cfg.max_position_sol} display={`${cfg.max_position_sol.toFixed(2)} SOL`}
              onDec={() => step('max_position_sol', -0.05, 0.01, 5.0, 2)}
              onInc={() => step('max_position_sol', +0.05, 0.01, 5.0, 2)} />
            <ConfigRow label="Max concurrent positions" sub="Open trades at once"
              value={cfg.max_positions} display={`${cfg.max_positions}`}
              onDec={() => step('max_positions', -1, 1, 20, 0)}
              onInc={() => step('max_positions', +1, 1, 20, 0)} />
            <ConfigRow label="Min wallet balance" sub="Stop trading below this"
              value={cfg.min_balance_sol} display={`${cfg.min_balance_sol.toFixed(2)} SOL`}
              onDec={() => step('min_balance_sol', -0.1, 0.0, 10.0, 2)}
              onInc={() => step('min_balance_sol', +0.1, 0.0, 10.0, 2)} />
          </div>

          {/* Risk */}
          <div className="neu-tile p-4">
            <p className="font-mono text-[10px] text-[#888] uppercase tracking-widest mb-3">Risk Controls</p>
            <ConfigRow label="Stop loss" sub="Exit immediately below this"
              value={cfg.stop_loss_pct} display={`−${(cfg.stop_loss_pct * 100).toFixed(0)}%`}
              onDec={() => step('stop_loss_pct', -0.05, 0.05, 0.90, 2)}
              onInc={() => step('stop_loss_pct', +0.05, 0.05, 0.90, 2)} />
            <ConfigRow label="Daily loss limit" sub="Pause bot when portfolio drops this much"
              value={cfg.daily_loss_limit} display={`−${(cfg.daily_loss_limit * 100).toFixed(0)}%`}
              onDec={() => step('daily_loss_limit', -0.05, 0.05, 0.90, 2)}
              onInc={() => step('daily_loss_limit', +0.05, 0.05, 0.90, 2)} />
            <ConfigRow label="Position timeout" sub="Force-exit after this long"
              value={cfg.timeout_minutes} display={`${cfg.timeout_minutes} min`}
              onDec={() => step('timeout_minutes', -1, 1, 60, 0)}
              onInc={() => step('timeout_minutes', +1, 1, 60, 0)} />
          </div>

          {/* Take profit */}
          <div className="neu-tile p-4">
            <p className="font-mono text-[10px] text-[#888] uppercase tracking-widest mb-3">Take Profit — Staged Exits</p>
            <ConfigRow label="TP1 — sell 40%" sub="First partial exit at this price multiple"
              value={cfg.take_profit_1x} display={`${cfg.take_profit_1x.toFixed(1)}x`}
              onDec={() => step('take_profit_1x', -0.5, 1.2, 10.0, 1)}
              onInc={() => step('take_profit_1x', +0.5, 1.2, 10.0, 1)} />
            <ConfigRow label="TP2 — sell 40%" sub="Second partial exit"
              value={cfg.take_profit_2x} display={`${cfg.take_profit_2x.toFixed(1)}x`}
              onDec={() => step('take_profit_2x', -0.5, 1.5, 20.0, 1)}
              onInc={() => step('take_profit_2x', +0.5, 1.5, 20.0, 1)} />
            <ConfigRow label="TP3 — sell rest" sub="Full exit at this multiple"
              value={cfg.take_profit_3x} display={`${cfg.take_profit_3x.toFixed(1)}x`}
              onDec={() => step('take_profit_3x', -1.0, 2.0, 50.0, 1)}
              onInc={() => step('take_profit_3x', +1.0, 2.0, 50.0, 1)} />
          </div>

          <button
            onClick={handleSave}
            disabled={saving}
            className="w-full flex items-center justify-center gap-2 py-2.5 rounded-xl font-mono text-xs font-bold transition-all disabled:opacity-50"
            style={{ background: saved ? 'rgba(74,222,128,0.12)' : 'rgba(0,168,255,0.12)', color: saved ? '#4ADE80' : '#00A8FF' }}
          >
            {saving && <Spinner size={12} className="animate-spin" />}
            {saved ? '✓ Saved' : saving ? 'Saving…' : 'Save Changes'}
          </button>
        </div>
      )}
    </Modal>
  )
}


// ── Main ──────────────────────────────────────────────────────────────────────

interface DashboardProps {
  onLogout?:           () => void
  walletId?:           string
  userName?:           string
  userUsername?:       string
  userAvatar?:         string
  signetKeyPrefix?:    string
  hasSignet?:          boolean
  mainWalletId?:       string
  telegramChatId?:     string
  onCredentialsSaved?: () => void
}

export default function Dashboard({ onLogout, walletId, userName, userUsername, userAvatar, signetKeyPrefix, hasSignet, mainWalletId, telegramChatId, onCredentialsSaved }: DashboardProps) {
  const { stats, positions, closed, online, loading, error, stop, resume } = useOrchestrator()
  const [tab, setTab] = useState('overview')
  const [showCredentials, setShowCredentials] = useState(false)
  const [showConfig,      setShowConfig]      = useState(false)

  if (loading) {
    return (
      <div className="min-h-screen bg-[#0d0d0d] flex items-center justify-center">
        <div className="text-center">
          <img src="/logo.png" alt="Hummingbird" className="w-16 h-16 object-contain mx-auto mb-4 animate-pulse"
            style={{ filter: 'drop-shadow(0 0 16px rgba(0,168,255,0.5))' }} />
          <p className="font-mono text-[#888] text-sm animate-pulse">Connecting to orchestrator...</p>
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
        onOpenCredentials={() => setShowCredentials(true)}
        onOpenTelegram={() => setShowCredentials(true)}
        onOpenConfig={() => setShowConfig(true)}
        telegramConnected={!!telegramChatId}
        userName={userName}
        userUsername={userUsername}
        userAvatar={userAvatar}
      />

      {tab === 'overview'  && <TabOverview stats={stats} positions={positions} closed={closed} online={online} error={error} />}
      {tab === 'accounts'  && <TabAccounts positions={positions} closed={closed} mainWalletId={mainWalletId} onMainWalletSet={onCredentialsSaved} />}
      {tab === 'logs'      && <TabLogs />}

      <MobileBottomNav
        tab={tab}
        setTab={setTab}
        paused={s.paused}
        onStop={stop}
        onResume={resume}
        onOpenConfig={() => setShowConfig(true)}
        onOpenCredentials={() => setShowCredentials(true)}
        telegramConnected={!!telegramChatId}
        userName={userName}
        userAvatar={userAvatar}
        onLogout={onLogout}
      />

      {showConfig      && <ConfigModal onClose={() => setShowConfig(false)} />}
      {showCredentials && (
        <CredentialsModal
          signetKeyPrefix={signetKeyPrefix}
          hasSignet={hasSignet}
          telegramChatId={telegramChatId}
          onClose={() => setShowCredentials(false)}
          onSaved={onCredentialsSaved}
        />
      )}
    </div>
  )
}
