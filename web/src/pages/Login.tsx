import { motion } from 'framer-motion'
import { useNexus } from '@vylth/nexus-react'

export default function Login() {
  const { login, isLoading } = useNexus()

  return (
    <div className="min-h-screen bg-[#0d0d0d] flex items-center justify-center px-6">
      <div className="w-full max-w-sm text-center">

        <motion.div
          initial={{ opacity: 0, y: -16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5 }}
          className="mb-10"
        >
          <img
            src="/logo.png"
            alt="Hummingbird"
            className="w-16 h-16 object-contain mx-auto mb-5"
            style={{ filter: 'drop-shadow(0 0 20px rgba(0,168,255,0.5))' }}
          />
          <h1 className="font-mono font-bold text-white text-2xl mb-2">Hummingbird</h1>
          <p className="text-[#555] text-sm">Autonomous Solana trading agent</p>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.1 }}
          className="hb-card p-7 space-y-4"
        >
          <button
            onClick={() => login()}
            disabled={isLoading}
            className="w-full flex items-center justify-center gap-3 font-mono font-bold text-sm
                       px-6 py-3.5 rounded-xl transition-all duration-200 disabled:opacity-50"
            style={{
              background: 'linear-gradient(135deg, #141414, #1a1a1a)',
              border: '1px solid rgba(255,255,255,0.08)',
              boxShadow: '0 0 0 0 transparent',
              color: '#fff',
            }}
            onMouseEnter={e => {
              (e.currentTarget as HTMLButtonElement).style.borderColor = 'rgba(0,168,255,0.3)'
              ;(e.currentTarget as HTMLButtonElement).style.boxShadow = '0 0 20px rgba(0,168,255,0.08)'
            }}
            onMouseLeave={e => {
              (e.currentTarget as HTMLButtonElement).style.borderColor = 'rgba(255,255,255,0.08)'
              ;(e.currentTarget as HTMLButtonElement).style.boxShadow = '0 0 0 0 transparent'
            }}
          >
            {/* Nexus V logo */}
            <span className="font-mono font-black text-[#00A8FF] text-base leading-none">V</span>
            {isLoading ? 'Redirecting...' : 'Continue with Nexus'}
          </button>

          <p className="text-[#2a2a2a] text-xs font-mono">
            Sign in with your VYLTH account
          </p>
        </motion.div>

        <motion.p
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ duration: 0.5, delay: 0.3 }}
          className="text-[#222] text-xs font-mono mt-6"
        >
          Don't have a VYLTH account?{' '}
          <a
            href="https://accounts.vylth.com/register"
            target="_blank"
            rel="noopener noreferrer"
            className="text-[#333] hover:text-[#555] transition-colors"
          >
            Create one free →
          </a>
        </motion.p>

      </div>
    </div>
  )
}
