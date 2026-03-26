import { motion } from 'framer-motion'
import { Link } from 'react-router-dom'

export default function CTA() {
  return (
    <section className="relative py-24 overflow-hidden">
      {/* Large glow behind the card */}
      <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
        <div className="w-[700px] h-[400px] rounded-full"
          style={{ background: 'radial-gradient(ellipse, rgba(0,168,255,0.06) 0%, transparent 70%)' }}
        />
      </div>

      <div className="max-w-4xl mx-auto px-6">
        <motion.div
          initial={{ opacity: 0, y: 24 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.7 }}
          className="relative rounded-3xl p-12 text-center overflow-hidden"
          style={{
            background: '#0f0f0f',
            boxShadow: `
              10px 10px 20px rgba(0,0,0,0.85),
              -10px -10px 20px rgba(40,40,40,0.12),
              0 0 0 1px rgba(0,168,255,0.12),
              0 0 60px rgba(0,168,255,0.05)
            `,
          }}
        >
          {/* Corner accent lines */}
          <div className="absolute top-0 left-0 w-24 h-px" style={{ background: 'linear-gradient(90deg, rgba(0,168,255,0.5), transparent)' }} />
          <div className="absolute top-0 left-0 h-24 w-px" style={{ background: 'linear-gradient(180deg, rgba(0,168,255,0.5), transparent)' }} />
          <div className="absolute bottom-0 right-0 w-24 h-px" style={{ background: 'linear-gradient(270deg, rgba(0,168,255,0.5), transparent)' }} />
          <div className="absolute bottom-0 right-0 h-24 w-px" style={{ background: 'linear-gradient(0deg, rgba(0,168,255,0.5), transparent)' }} />

          <span className="font-mono text-xs text-[#00A8FF] tracking-[3px] uppercase mb-6 block">
            Open Source · Free to Run
          </span>

          <h2 className="font-mono font-bold text-white mb-4"
            style={{ fontSize: 'clamp(1.8rem, 4vw, 3rem)', lineHeight: 1.2 }}
          >
            Start trading in{' '}
            <span className="hb-gradient-text">minutes.</span>
          </h2>

          <p className="text-[#a0a0a0] text-lg mb-10 max-w-xl mx-auto leading-relaxed">
            Clone the repo, set your Signet API key, and Hummingbird is live.
            No infrastructure. No private key risk. No babysitting.
          </p>

          <div className="flex flex-col sm:flex-row gap-4 justify-center mb-12">
            <Link to="/dashboard" className="hb-btn text-base px-8 py-4">
              Launch Dashboard →
            </Link>
            <a
              href="https://github.com/iamdecatalyst/hummingbird"
              target="_blank"
              rel="noopener noreferrer"
              className="neu-btn-ghost text-base px-8 py-4"
            >
              ★ View on GitHub
            </a>
          </div>

          {/* Signet callout */}
          <div className="inline-flex items-center gap-3 neu-card-inset px-5 py-3 rounded-2xl">
            <span className="font-mono text-xs text-[#555]">POWERED BY</span>
            <span className="font-mono text-sm font-bold text-white">Signet</span>
            <span className="font-mono text-xs text-[#555]">—</span>
            <a
              href="https://signet.vylth.com"
              target="_blank"
              rel="noopener noreferrer"
              className="font-mono text-xs text-[#00A8FF] hover:text-white transition-colors"
            >
              Get free API key →
            </a>
          </div>
        </motion.div>
      </div>
    </section>
  )
}
