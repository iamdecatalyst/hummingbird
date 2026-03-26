import { motion } from 'framer-motion'

const FEATURES = [
  {
    icon: '🌐',
    title: 'Multi-Platform',
    desc: 'Monitors pump.fun, Moonshot, Raydium LaunchLab, Boop.fun on Solana — plus Base, BNB, and more via EVM eth_subscribe.',
  },
  {
    icon: '🎯',
    title: 'Sniper + Scalper',
    desc: 'Two modes work together. Sniper catches new launches &lt;100ms. Scalper finds second-wave momentum on 8-25 minute old tokens.',
  },
  {
    icon: '📈',
    title: 'Staged Take-Profits',
    desc: 'Sells 40% at 2x, 40% more at 5x, and the rest at 10x. Locks gains progressively so moonshots are never fully sold too early.',
  },
  {
    icon: '🛡️',
    title: 'Rug Detection',
    desc: 'Exits immediately when dev wallet sells >5% supply, liquidity drops, or price crashes >15% in 10 seconds.',
  },
  {
    icon: '📊',
    title: 'Risk Controls',
    desc: 'Max 5 concurrent positions, configurable daily loss limit (default 30%), per-trade stop-loss at -25%.',
  },
  {
    icon: '🤖',
    title: 'Telegram Control',
    desc: 'Full bot with inline keyboards. Real-time P&L, open positions, toggle sniper/scalper on the fly. ASCII dashboard that looks insane.',
  },
  {
    icon: '🔑',
    title: 'Powered by Signet',
    desc: 'Uses Signet\'s custodial wallet API for all trade execution — no private keys in config files. Set up in minutes.',
  },
  {
    icon: '⚡',
    title: 'Open Source',
    desc: 'Fully open on GitHub. Run it yourself, fork it, extend it. The scoring engine is the only part you\'ll want to keep private.',
  },
]

export default function Features() {
  return (
    <section id="features" className="relative py-24">
      <div className="max-w-6xl mx-auto px-6">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="text-center mb-16"
        >
          <span className="font-mono text-xs text-[#00A8FF] tracking-[3px] uppercase mb-4 block">
            Capabilities
          </span>
          <h2 className="font-mono font-bold text-4xl text-white mb-4">
            Everything You Need
          </h2>
          <p className="text-[#a0a0a0] max-w-xl mx-auto">
            Production-grade components assembled into a complete system you can run today.
          </p>
        </motion.div>

        <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-4">
          {FEATURES.map((f, i) => (
            <motion.div
              key={f.title}
              initial={{ opacity: 0, y: 20 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true }}
              transition={{ duration: 0.4, delay: i * 0.07 }}
              className="neu-tile p-5 group cursor-default"
            >
              <div className="text-2xl mb-3">{f.icon}</div>
              <h3 className="font-mono font-bold text-sm text-white mb-2 group-hover:text-[#00A8FF] transition-colors">
                {f.title}
              </h3>
              <p
                className="text-[#666] text-xs leading-relaxed"
                dangerouslySetInnerHTML={{ __html: f.desc }}
              />
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  )
}
