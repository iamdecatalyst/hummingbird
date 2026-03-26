import { useState } from 'react'
import { motion } from 'framer-motion'
import { Key, Eye, EyeSlash, ArrowRight, Warning } from '@phosphor-icons/react'

interface Props {
  onSignin: (apiKey: string, apiSecret: string) => Promise<void>
}

export default function Login({ onSignin }: Props) {
  const [apiKey,    setApiKey]    = useState('')
  const [apiSecret, setApiSecret] = useState('')
  const [showKey,   setShowKey]   = useState(false)
  const [showSec,   setShowSec]   = useState(false)
  const [loading,   setLoading]   = useState(false)
  const [error,     setError]     = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!apiKey.trim() || !apiSecret.trim()) return
    setLoading(true)
    setError('')
    try {
      await onSignin(apiKey.trim(), apiSecret.trim())
    } catch (err: any) {
      const msg = err?.message ?? 'Connection failed'
      if (msg.includes('401') || msg.includes('invalid')) {
        setError('Invalid Signet credentials. Check your API key and secret.')
      } else {
        setError(msg)
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-[#0d0d0d] flex items-center justify-center px-6">
      <div className="w-full max-w-md">

        {/* Logo + title */}
        <motion.div
          initial={{ opacity: 0, y: -16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5 }}
          className="text-center mb-10"
        >
          <img
            src="/logo.png"
            alt="Hummingbird"
            className="w-16 h-16 object-contain mx-auto mb-5"
            style={{ filter: 'drop-shadow(0 0 20px rgba(0,168,255,0.5))' }}
          />
          <h1 className="font-mono font-bold text-white text-2xl mb-2">
            Connect your wallet
          </h1>
          <p className="text-[#555] text-sm">
            Enter your{' '}
            <a
              href="https://signet.vylth.com"
              target="_blank"
              rel="noopener noreferrer"
              className="text-[#00A8FF] hover:text-white transition-colors"
            >
              Signet API key
            </a>
            {' '}to start trading
          </p>
        </motion.div>

        {/* Form card */}
        <motion.form
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.1 }}
          onSubmit={handleSubmit}
          className="hb-card p-7 space-y-4"
        >
          {/* API Key */}
          <div>
            <label className="block font-mono text-xs text-[#555] uppercase tracking-widest mb-2">
              API Key
            </label>
            <div className="relative">
              <Key
                size={15}
                className="absolute left-3 top-1/2 -translate-y-1/2 text-[#333]"
                weight="bold"
              />
              <input
                type={showKey ? 'text' : 'password'}
                value={apiKey}
                onChange={e => setApiKey(e.target.value)}
                placeholder="sgk_live_..."
                autoComplete="off"
                spellCheck={false}
                className="w-full bg-[#0a0a0a] border border-white/5 rounded-xl pl-9 pr-10 py-3
                           font-mono text-sm text-white placeholder-[#333]
                           focus:outline-none focus:border-[#00A8FF]/40 transition-colors"
              />
              <button
                type="button"
                onClick={() => setShowKey(v => !v)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-[#333] hover:text-[#666] transition-colors"
              >
                {showKey ? <EyeSlash size={15} /> : <Eye size={15} />}
              </button>
            </div>
          </div>

          {/* API Secret */}
          <div>
            <label className="block font-mono text-xs text-[#555] uppercase tracking-widest mb-2">
              API Secret
            </label>
            <div className="relative">
              <Key
                size={15}
                className="absolute left-3 top-1/2 -translate-y-1/2 text-[#333]"
                weight="bold"
              />
              <input
                type={showSec ? 'text' : 'password'}
                value={apiSecret}
                onChange={e => setApiSecret(e.target.value)}
                placeholder="sgs_live_..."
                autoComplete="off"
                spellCheck={false}
                className="w-full bg-[#0a0a0a] border border-white/5 rounded-xl pl-9 pr-10 py-3
                           font-mono text-sm text-white placeholder-[#333]
                           focus:outline-none focus:border-[#00A8FF]/40 transition-colors"
              />
              <button
                type="button"
                onClick={() => setShowSec(v => !v)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-[#333] hover:text-[#666] transition-colors"
              >
                {showSec ? <EyeSlash size={15} /> : <Eye size={15} />}
              </button>
            </div>
          </div>

          {/* Error */}
          {error && (
            <div className="flex items-start gap-2 text-[#EF4444] font-mono text-xs p-3 rounded-xl bg-[#EF4444]/5 border border-[#EF4444]/10">
              <Warning size={14} className="mt-0.5 shrink-0" weight="fill" />
              {error}
            </div>
          )}

          {/* Submit */}
          <button
            type="submit"
            disabled={loading || !apiKey || !apiSecret}
            className="hb-btn w-full justify-center mt-2 disabled:opacity-40 disabled:cursor-not-allowed"
          >
            {loading ? (
              <span className="animate-pulse">Verifying...</span>
            ) : (
              <>Start trading <ArrowRight size={16} weight="bold" /></>
            )}
          </button>
        </motion.form>

        {/* Footer note */}
        <motion.p
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ duration: 0.5, delay: 0.3 }}
          className="text-center text-[#333] text-xs font-mono mt-6"
        >
          Your credentials are encrypted at rest and never shared.{' '}
          <a
            href="https://github.com/iamdecatalyst/hummingbird"
            target="_blank"
            rel="noopener noreferrer"
            className="text-[#444] hover:text-[#666] transition-colors"
          >
            Verify in source →
          </a>
        </motion.p>

      </div>
    </div>
  )
}
