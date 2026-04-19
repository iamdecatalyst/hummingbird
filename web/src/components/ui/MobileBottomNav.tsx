// Mobile-only bottom nav. Mirrors the Vylth Flow pattern (see
// vylth-flow/apps/flow/src/components/layout/MobileBottomNav.tsx): four-slot
// fixed bar with three primary tabs + a "Menu" button that opens a bottom
// sheet containing less-common actions (Config, Credentials, Telegram,
// Stop/Resume, Sign out). Hidden on lg+ where the regular TopNav handles it.
import { useEffect, useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import {
  ChartBar, SquaresFour, Terminal, List, X, SlidersHorizontal, Key,
  TelegramLogo, Stop, Play, SignOut,
} from '@phosphor-icons/react'

interface Tab {
  id:    string
  label: string
  icon:  React.ComponentType<{ size?: number; weight?: 'duotone' | 'fill' | 'regular' | 'bold' }>
}

const TABS: Tab[] = [
  { id: 'overview', label: 'Overview', icon: ChartBar },
  { id: 'accounts', label: 'Accounts', icon: SquaresFour },
  { id: 'logs',     label: 'Logs',     icon: Terminal  },
]

interface MobileBottomNavProps {
  tab:                string
  setTab:             (t: string) => void
  paused:             boolean
  onStop:             () => void
  onResume:           () => void
  onOpenConfig:       () => void
  onOpenCredentials:  () => void
  telegramConnected?: boolean
  userName?:          string
  userAvatar?:        string
  onLogout?:          () => void
}

export default function MobileBottomNav({
  tab, setTab, paused, onStop, onResume, onOpenConfig, onOpenCredentials,
  telegramConnected, userName, userAvatar, onLogout,
}: MobileBottomNavProps) {
  const [menuOpen, setMenuOpen] = useState(false)

  // Lock background scroll while the sheet is up — otherwise two scrollbars race.
  useEffect(() => {
    if (!menuOpen) return
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => { document.body.style.overflow = prev }
  }, [menuOpen])

  // Only render the sheet body when the sheet is already known-safe to show —
  // the validation of userAvatar matches the TopNav's convention.
  const safeAvatar = userAvatar && userAvatar.startsWith('https://') ? userAvatar : undefined

  return (
    <>
      {/* Fixed bottom bar */}
      <nav className="lg:hidden fixed bottom-0 left-0 right-0 z-40 bg-[#0d0d0d]/95 backdrop-blur-md border-t border-white/5">
        <div className="flex items-center justify-around h-16 px-2 pb-[env(safe-area-inset-bottom)]">
          {TABS.map(t => {
            const Icon = t.icon
            const active = tab === t.id
            return (
              <button
                key={t.id}
                onClick={() => setTab(t.id)}
                className="flex-1 flex flex-col items-center justify-center gap-1 py-2 rounded-lg transition-colors"
                style={{ color: active ? '#00A8FF' : '#666' }}
              >
                <Icon size={22} weight={active ? 'fill' : 'regular'} />
                <span className="font-mono text-[10px]" style={{ fontWeight: active ? 700 : 500 }}>
                  {t.label}
                </span>
              </button>
            )
          })}
          <button
            onClick={() => setMenuOpen(true)}
            className="flex-1 flex flex-col items-center justify-center gap-1 py-2 rounded-lg"
            style={{ color: menuOpen ? '#00A8FF' : '#666' }}
          >
            <List size={22} />
            <span className="font-mono text-[10px]">Menu</span>
          </button>
        </div>
      </nav>

      {/* Bottom sheet */}
      <AnimatePresence>
        {menuOpen && (
          <div className="lg:hidden fixed inset-0 z-50">
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.2 }}
              className="absolute inset-0 backdrop-blur-sm"
              style={{ background: 'rgba(0,0,0,0.6)' }}
              onClick={() => setMenuOpen(false)}
            />
            <motion.div
              initial={{ y: '100%' }}
              animate={{ y: 0 }}
              exit={{ y: '100%' }}
              transition={{ type: 'spring', damping: 30, stiffness: 300 }}
              className="absolute bottom-0 left-0 right-0 rounded-t-3xl overflow-hidden"
              style={{ background: '#0d0d0d', maxHeight: '85vh', borderTop: '1px solid rgba(255,255,255,0.06)' }}
            >
              <div className="flex justify-center pt-3 pb-1">
                <div className="w-10 h-1 rounded-full bg-white/20" />
              </div>

              <div className="flex items-center justify-between px-5 pt-2 pb-3">
                <div className="flex items-center gap-2">
                  <img src="/logo.png" alt="" className="w-6 h-6 object-contain"
                    style={{ filter: 'drop-shadow(0 0 6px rgba(0,168,255,0.4))' }} />
                  <span className="font-mono font-bold text-sm text-white tracking-widest">HUMMINGBIRD</span>
                </div>
                <button
                  onClick={() => setMenuOpen(false)}
                  className="w-9 h-9 rounded-xl flex items-center justify-center neu-card-inset text-[#888]"
                >
                  <X size={16} />
                </button>
              </div>

              {/* Scrollable content */}
              <div className="overflow-y-auto px-5 pb-8" style={{ maxHeight: 'calc(85vh - 80px)' }}>
                {/* Greeting */}
                <div className="mb-5 p-4 rounded-2xl neu-card-inset flex items-center gap-3">
                  {safeAvatar
                    ? <img src={safeAvatar} alt="" className="w-11 h-11 rounded-xl object-cover ring-1 ring-white/10" />
                    : <div className="w-11 h-11 rounded-xl bg-white/5 flex items-center justify-center">
                        <span className="font-mono text-sm text-[#888]">{(userName || 'U')[0].toUpperCase()}</span>
                      </div>
                  }
                  <div className="min-w-0">
                    <p className="font-mono font-bold text-sm text-white truncate">{userName || 'Trader'}</p>
                    <p className="font-mono text-[10px] text-[#888]">
                      {paused ? 'Bot is paused' : 'Bot is running'}
                    </p>
                  </div>
                </div>

                {/* Bot controls */}
                <p className="font-mono text-[10px] font-bold uppercase tracking-widest text-[#666] mb-2 px-1">
                  Bot
                </p>
                <div className="grid grid-cols-2 gap-2 mb-5">
                  {paused
                    ? (
                      <button
                        onClick={() => { onResume(); setMenuOpen(false) }}
                        className="flex flex-col items-center gap-2 p-4 rounded-2xl transition-all active:scale-95"
                        style={{ background: 'rgba(74,222,128,0.08)', color: '#4ADE80' }}
                      >
                        <Play size={22} weight="fill" />
                        <span className="font-mono text-xs font-bold">Resume</span>
                      </button>
                    )
                    : (
                      <button
                        onClick={() => { onStop(); setMenuOpen(false) }}
                        className="flex flex-col items-center gap-2 p-4 rounded-2xl neu-card transition-all active:scale-95"
                        style={{ color: '#888' }}
                      >
                        <Stop size={22} weight="fill" />
                        <span className="font-mono text-xs font-bold">Stop</span>
                      </button>
                    )
                  }
                  <button
                    onClick={() => { onOpenConfig(); setMenuOpen(false) }}
                    className="flex flex-col items-center gap-2 p-4 rounded-2xl neu-card transition-all active:scale-95"
                    style={{ color: '#888' }}
                  >
                    <SlidersHorizontal size={22} />
                    <span className="font-mono text-xs font-bold">Config</span>
                  </button>
                </div>

                {/* Connections */}
                <p className="font-mono text-[10px] font-bold uppercase tracking-widest text-[#666] mb-2 px-1">
                  Connections
                </p>
                <div className="grid grid-cols-2 gap-2 mb-5">
                  <button
                    onClick={() => { onOpenCredentials(); setMenuOpen(false) }}
                    className="flex flex-col items-center gap-2 p-4 rounded-2xl neu-card transition-all active:scale-95"
                    style={{ color: '#888' }}
                  >
                    <Key size={22} />
                    <span className="font-mono text-xs font-bold">Signet</span>
                  </button>
                  <button
                    onClick={() => { onOpenCredentials(); setMenuOpen(false) }}
                    className="flex flex-col items-center gap-2 p-4 rounded-2xl neu-card transition-all active:scale-95"
                    style={{ color: telegramConnected ? '#24A1DE' : '#888' }}
                  >
                    <TelegramLogo size={22} weight={telegramConnected ? 'fill' : 'regular'} />
                    <span className="font-mono text-xs font-bold">
                      {telegramConnected ? 'Telegram ✓' : 'Telegram'}
                    </span>
                  </button>
                </div>

                {/* Community */}
                <p className="font-mono text-[10px] font-bold uppercase tracking-widest text-[#666] mb-2 px-1">
                  Community
                </p>
                <a
                  href="https://t.me/vylthummingbird"
                  target="_blank"
                  rel="noopener noreferrer"
                  onClick={() => setMenuOpen(false)}
                  className="flex items-center gap-3 p-4 rounded-2xl neu-card mb-5 transition-all active:scale-95"
                  style={{ color: '#24A1DE' }}
                >
                  <TelegramLogo size={20} weight="fill" />
                  <div className="text-left min-w-0">
                    <p className="font-mono text-xs font-bold">Join Hummingbird Community</p>
                    <p className="font-mono text-[10px] text-[#666] truncate">Live trade feed · support · news</p>
                  </div>
                </a>

                {/* Sign out */}
                {onLogout && (
                  <button
                    onClick={() => { onLogout(); setMenuOpen(false) }}
                    className="w-full flex items-center justify-center gap-2 py-3 rounded-2xl neu-card transition-all active:scale-95"
                    style={{ color: '#EF4444' }}
                  >
                    <SignOut size={16} />
                    <span className="font-mono text-xs font-bold">Sign Out</span>
                  </button>
                )}
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* Spacer so page content isn't hidden behind the fixed bar */}
      <div className="lg:hidden h-16" aria-hidden="true" />
    </>
  )
}
