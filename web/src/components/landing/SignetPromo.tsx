import { motion } from 'framer-motion'
import { ArrowRight, Lock, Lightning, Globe, Sparkle } from '@phosphor-icons/react'

const STATS = [
  { icon: <Lightning size={16} weight="fill" />, value: '<1ms',  label: 'Signing latency' },
  { icon: <Globe    size={16} weight="fill" />, value: '12',     label: 'Chains supported' },
  { icon: <Lock     size={16} weight="fill" />, value: '100%',   label: 'Non-custodial' },
  { icon: <Sparkle  size={16} weight="fill" />, value: 'Free',   label: 'To get started' },
]

export default function SignetPromo() {
  return (
    <section className="relative py-24 overflow-hidden">
      {/* Green ambient glow */}
      <div
        className="absolute inset-0 flex items-center justify-center pointer-events-none"
        aria-hidden
      >
        <div
          className="w-[900px] h-[500px] rounded-full"
          style={{ background: 'radial-gradient(ellipse, rgba(34,197,94,0.05) 0%, transparent 70%)' }}
        />
      </div>

      <div className="max-w-6xl mx-auto px-6">
        <motion.div
          initial={{ opacity: 0, y: 24 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.7 }}
          className="relative rounded-3xl overflow-hidden"
          style={{
            background: 'linear-gradient(135deg, #060e0a 0%, #0a1510 50%, #060e0a 100%)',
            boxShadow: `
              10px 10px 30px rgba(0,0,0,0.9),
              -6px -6px 20px rgba(34,197,94,0.04),
              0 0 0 1px rgba(34,197,94,0.15),
              0 0 60px rgba(34,197,94,0.06)
            `,
          }}
        >
          {/* Corner accent lines — green */}
          <div className="absolute top-0 left-0 w-32 h-px" style={{ background: 'linear-gradient(90deg, rgba(34,197,94,0.6), transparent)' }} />
          <div className="absolute top-0 left-0 h-32 w-px" style={{ background: 'linear-gradient(180deg, rgba(34,197,94,0.6), transparent)' }} />
          <div className="absolute bottom-0 right-0 w-32 h-px" style={{ background: 'linear-gradient(270deg, rgba(34,197,94,0.6), transparent)' }} />
          <div className="absolute bottom-0 right-0 h-32 w-px" style={{ background: 'linear-gradient(0deg, rgba(34,197,94,0.6), transparent)' }} />

          {/* Scan-line texture */}
          <div
            className="absolute inset-0 pointer-events-none opacity-[0.015]"
            style={{
              backgroundImage: 'repeating-linear-gradient(0deg, rgba(34,197,94,1) 0px, rgba(34,197,94,1) 1px, transparent 1px, transparent 4px)',
            }}
          />

          <div className="relative p-8 lg:p-12">
            <div className="grid lg:grid-cols-2 gap-10 items-center">

              {/* Left — branding + copy */}
              <div>
                {/* Logo + name */}
                <motion.div
                  initial={{ opacity: 0, x: -16 }}
                  whileInView={{ opacity: 1, x: 0 }}
                  viewport={{ once: true }}
                  transition={{ duration: 0.5 }}
                  className="flex items-center gap-3 mb-6"
                >
                  <img
                    src="/signet-logo.png"
                    alt="Signet"
                    className="h-10 object-contain"
                    style={{ filter: 'drop-shadow(0 0 12px rgba(34,197,94,0.5))' }}
                  />
                  <div>
                    <div className="font-mono text-xs text-[#22c55e] tracking-[3px] uppercase">
                      Powered by
                    </div>
                    <div className="font-mono font-bold text-xl text-white leading-tight">
                      Vylth Signet
                    </div>
                  </div>
                </motion.div>

                <motion.h2
                  initial={{ opacity: 0, y: 12 }}
                  whileInView={{ opacity: 1, y: 0 }}
                  viewport={{ once: true }}
                  transition={{ duration: 0.5, delay: 0.1 }}
                  className="font-mono font-bold text-white mb-3"
                  style={{ fontSize: 'clamp(1.4rem, 3vw, 2rem)', lineHeight: 1.25 }}
                >
                  One API key.{' '}
                  <span style={{ color: '#22c55e' }}>Any chain.</span>
                  <br />
                  No seed phrases.
                </motion.h2>

                <motion.p
                  initial={{ opacity: 0, y: 8 }}
                  whileInView={{ opacity: 1, y: 0 }}
                  viewport={{ once: true }}
                  transition={{ duration: 0.5, delay: 0.18 }}
                  className="text-[#888] text-sm leading-relaxed mb-8 max-w-sm"
                >
                  Signet is the wallet infrastructure Hummingbird runs on. Non-custodial KMS — your
                  keys, stored encrypted, signed on-demand. Set up in minutes, free to start.
                </motion.p>

                <motion.a
                  initial={{ opacity: 0, y: 8 }}
                  whileInView={{ opacity: 1, y: 0 }}
                  viewport={{ once: true }}
                  transition={{ duration: 0.4, delay: 0.26 }}
                  href="https://signet.vylth.com"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-2 font-mono text-sm font-bold px-6 py-3 rounded-xl transition-all duration-200"
                  style={{
                    background: 'rgba(34,197,94,0.12)',
                    color: '#22c55e',
                    border: '1px solid rgba(34,197,94,0.3)',
                    boxShadow: '0 0 20px rgba(34,197,94,0.08)',
                  }}
                  onMouseEnter={e => {
                    const el = e.currentTarget
                    el.style.background = 'rgba(34,197,94,0.2)'
                    el.style.boxShadow = '0 0 30px rgba(34,197,94,0.2)'
                  }}
                  onMouseLeave={e => {
                    const el = e.currentTarget
                    el.style.background = 'rgba(34,197,94,0.12)'
                    el.style.boxShadow = '0 0 20px rgba(34,197,94,0.08)'
                  }}
                >
                  Get free API key <ArrowRight size={15} weight="bold" />
                </motion.a>
              </div>

              {/* Right — stats grid */}
              <div className="grid grid-cols-2 gap-3">
                {STATS.map((s, i) => (
                  <motion.div
                    key={s.label}
                    initial={{ opacity: 0, y: 16 }}
                    whileInView={{ opacity: 1, y: 0 }}
                    viewport={{ once: true }}
                    transition={{ duration: 0.4, delay: 0.1 + i * 0.07 }}
                    className="relative rounded-2xl p-5"
                    style={{
                      background: 'rgba(34,197,94,0.04)',
                      border: '1px solid rgba(34,197,94,0.1)',
                    }}
                  >
                    <div className="flex items-center gap-2 mb-3 text-[#22c55e]">
                      {s.icon}
                      <span className="font-mono text-xs text-[#4a7c5a] uppercase tracking-widest">
                        {s.label}
                      </span>
                    </div>
                    <div
                      className="font-mono font-bold text-3xl"
                      style={{
                        color: '#22c55e',
                        textShadow: '0 0 20px rgba(34,197,94,0.4)',
                      }}
                    >
                      {s.value}
                    </div>
                  </motion.div>
                ))}

                {/* Feature pills */}
                <motion.div
                  initial={{ opacity: 0 }}
                  whileInView={{ opacity: 1 }}
                  viewport={{ once: true }}
                  transition={{ duration: 0.5, delay: 0.45 }}
                  className="col-span-2 flex flex-wrap gap-2 mt-1"
                >
                  {['API-First', 'AI-Ready', 'Rust-Powered', 'Self-Custody'].map(pill => (
                    <span
                      key={pill}
                      className="font-mono text-xs px-3 py-1.5 rounded-full"
                      style={{
                        background: 'rgba(34,197,94,0.06)',
                        border: '1px solid rgba(34,197,94,0.15)',
                        color: '#4a7c5a',
                      }}
                    >
                      {pill}
                    </span>
                  ))}
                </motion.div>
              </div>

            </div>
          </div>
        </motion.div>
      </div>
    </section>
  )
}
