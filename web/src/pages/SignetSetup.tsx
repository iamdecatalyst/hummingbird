import { useState } from 'react'
import { motion } from 'framer-motion'
import { Key, Eye, EyeSlash, ArrowRight, Warning, CheckCircle } from '@phosphor-icons/react'
import { api } from '../lib/api'

interface Props {
  firstName: string
  onComplete: () => void
}

export default function SignetSetup({ firstName, onComplete }: Props) {
  const [apiKey,    setApiKey]    = useState('')
  const [apiSecret, setApiSecret] = useState('')
  const [showKey,   setShowKey]   = useState(false)
  const [showSec,   setShowSec]   = useState(false)
  const [loading,   setLoading]   = useState(false)
  const [error,     setError]     = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    setError('')
    try {
      await api.setupSignet(apiKey.trim(), apiSecret.trim())
      onComplete()
    } catch (err: any) {
      const msg = err?.message ?? ''
      if (msg.includes('invalid')) {
        setError('Invalid Signet credentials — check your API key and secret.')
      } else {
        setError(msg || 'Something went wrong.')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-[#0d0d0d] flex items-center justify-center px-6">
      <div className="w-full max-w-md">

        <motion.div
          initial={{ opacity: 0, y: -12 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5 }}
          className="text-center mb-8"
        >
          <img
            src="/logo.png"
            alt="Hummingbird"
            className="w-14 h-14 object-contain mx-auto mb-4"
            style={{ filter: 'drop-shadow(0 0 16px rgba(0,168,255,0.5))' }}
          />
          <h1 className="font-mono font-bold text-white text-xl mb-1">
            Welcome, {firstName}
          </h1>
          <p className="text-[#555] text-sm">
            One more step — connect your{' '}
            <a
              href="https://signet.vylth.com"
              target="_blank"
              rel="noopener noreferrer"
              className="text-[#00A8FF] hover:text-white transition-colors"
            >
              Signet wallet
            </a>
          </p>
        </motion.div>

        {/* What Signet is */}
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ duration: 0.5, delay: 0.1 }}
          className="neu-card-inset rounded-2xl p-4 mb-5 space-y-2"
        >
          {[
            'Non-custodial — your keys, your funds',
            'Free tier: 5 wallets, 1,000 requests/month',
            'Set up once, never enter again',
          ].map(line => (
            <div key={line} className="flex items-center gap-2 font-mono text-xs text-[#555]">
              <CheckCircle size={13} weight="fill" className="text-[#22c55e] shrink-0" />
              {line}
            </div>
          ))}
        </motion.div>

        <motion.form
          initial={{ opacity: 0, y: 12 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.15 }}
          onSubmit={handleSubmit}
          className="hb-card p-6 space-y-4"
        >
          {/* API Key */}
          <div>
            <label className="block font-mono text-xs text-[#555] uppercase tracking-widest mb-2">
              Signet API Key
            </label>
            <div className="relative">
              <Key size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-[#333]" weight="bold" />
              <input
                type={showKey ? 'text' : 'password'}
                value={apiKey}
                onChange={e => setApiKey(e.target.value)}
                placeholder="sgk_live_..."
                autoComplete="off"
                spellCheck={false}
                className="w-full bg-[#0a0a0a] border border-white/5 rounded-xl pl-8 pr-9 py-2.5
                           font-mono text-sm text-white placeholder-[#333]
                           focus:outline-none focus:border-[#00A8FF]/30 transition-colors"
              />
              <button type="button" onClick={() => setShowKey(v => !v)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-[#333] hover:text-[#666]">
                {showKey ? <EyeSlash size={14} /> : <Eye size={14} />}
              </button>
            </div>
          </div>

          {/* API Secret */}
          <div>
            <label className="block font-mono text-xs text-[#555] uppercase tracking-widest mb-2">
              Signet API Secret
            </label>
            <div className="relative">
              <Key size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-[#333]" weight="bold" />
              <input
                type={showSec ? 'text' : 'password'}
                value={apiSecret}
                onChange={e => setApiSecret(e.target.value)}
                placeholder="sgs_live_..."
                autoComplete="off"
                spellCheck={false}
                className="w-full bg-[#0a0a0a] border border-white/5 rounded-xl pl-8 pr-9 py-2.5
                           font-mono text-sm text-white placeholder-[#333]
                           focus:outline-none focus:border-[#00A8FF]/30 transition-colors"
              />
              <button type="button" onClick={() => setShowSec(v => !v)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-[#333] hover:text-[#666]">
                {showSec ? <EyeSlash size={14} /> : <Eye size={14} />}
              </button>
            </div>
          </div>

          {error && (
            <div className="flex items-start gap-2 text-[#EF4444] font-mono text-xs p-3 rounded-xl bg-[#EF4444]/5 border border-[#EF4444]/10">
              <Warning size={13} className="mt-0.5 shrink-0" weight="fill" />
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={loading || !apiKey || !apiSecret}
            className="hb-btn w-full justify-center disabled:opacity-40 disabled:cursor-not-allowed"
          >
            {loading
              ? <span className="animate-pulse">Connecting wallet...</span>
              : <>Start trading <ArrowRight size={15} weight="bold" /></>
            }
          </button>
        </motion.form>

        <p className="text-center text-[#222] text-xs font-mono mt-5">
          Don't have a Signet key?{' '}
          <a href="https://signet.vylth.com" target="_blank" rel="noopener noreferrer"
            className="text-[#333] hover:text-[#555] transition-colors">
            Get one free →
          </a>
        </p>
      </div>
    </div>
  )
}
