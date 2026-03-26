import { motion } from 'framer-motion'
import { Lightning, Brain, Target } from '@phosphor-icons/react'

const STEPS = [
  {
    num: '01',
    lang: 'Rust',
    langColor: '#F74C00',
    langLogo: '/lang-rust.svg',
    icon: <Lightning size={28} weight="fill" color="#00A8FF" />,
    title: 'Detect',
    subtitle: 'Sub-100ms detection',
    desc: 'WebSocket listener subscribes to Solana program logs. When a new token launches on pump.fun, Moonshot, Raydium LaunchLab, or Boop, the event is captured in under 100ms and sent to the scorer.',
    detail: 'logsSubscribe · getTransaction · EVM eth_subscribe',
  },
  {
    num: '02',
    lang: 'Python',
    langColor: '#3776AB',
    langLogo: '/lang-python.svg',
    icon: <Brain size={28} weight="fill" color="#00A8FF" />,
    title: 'Score',
    subtitle: '5 parallel signals',
    desc: 'FastAPI scorer runs 5 async checks in parallel: dev wallet history, token supply distribution, bonding curve fill %, mint authority status, and social metadata — all in under 500ms.',
    detail: 'dev_wallet · supply · bonding · contract · social',
  },
  {
    num: '03',
    lang: 'Go',
    langColor: '#00ADD8',
    langLogo: '/lang-go.svg',
    icon: <Target size={28} weight="fill" color="#00A8FF" />,
    title: 'Execute',
    subtitle: 'Signet API · 3% slippage',
    desc: 'Orchestrator receives the score decision and executes buys via Signet. Positions are monitored every 2 seconds — staged take-profits at 2x, 5x, 10x, with stop-loss and rug detection.',
    detail: 'sniper · scalper · TP1/TP2/TP3 · stop-loss',
  },
]

export default function HowItWorks() {
  return (
    <section id="how-it-works" className="relative py-24 overflow-hidden">
      {/* Section divider glow */}
      <div className="absolute top-0 left-1/2 -translate-x-1/2 w-px h-24"
        style={{ background: 'linear-gradient(to bottom, transparent, rgba(0,168,255,0.3), transparent)' }}
      />

      <div className="max-w-6xl mx-auto px-6">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="text-center mb-16"
        >
          <span className="font-mono text-xs text-[#00A8FF] tracking-[3px] uppercase mb-4 block">
            Architecture
          </span>
          <h2 className="font-mono font-bold text-4xl text-white mb-4">
            How It Works
          </h2>
          <p className="text-[#a0a0a0] max-w-xl mx-auto">
            Three specialized services working in sequence, each written in the language best suited for its job.
          </p>
        </motion.div>

        {/* Steps */}
        <div className="relative">
          {/* Connecting line */}
          <div className="hidden lg:block absolute top-1/2 left-0 right-0 h-px -translate-y-1/2"
            style={{ background: 'linear-gradient(90deg, transparent, rgba(0,168,255,0.15), rgba(0,168,255,0.15), transparent)' }}
          />

          <div className="grid lg:grid-cols-3 gap-6">
            {STEPS.map((step, i) => (
              <motion.div
                key={step.num}
                initial={{ opacity: 0, y: 24 }}
                whileInView={{ opacity: 1, y: 0 }}
                viewport={{ once: true }}
                transition={{ duration: 0.5, delay: i * 0.15 }}
                className="hb-card p-6 relative group"
              >
                {/* Step number */}
                <div className="flex items-start justify-between mb-5">
                  <span className="font-mono text-5xl font-bold text-white/5 leading-none select-none">
                    {step.num}
                  </span>
                  <div
                    className="flex items-center gap-1.5 font-mono text-xs font-bold px-2.5 py-1 rounded-full"
                    style={{
                      background: `${step.langColor}15`,
                      color: step.langColor,
                      boxShadow: `0 0 12px ${step.langColor}20`,
                    }}
                  >
                    <img src={step.langLogo} alt={step.lang} className="w-3.5 h-3.5 object-contain" />
                    {step.lang}
                  </div>
                </div>

                {/* Icon + title */}
                <div className="mb-3">
                  <div className="mb-2">{step.icon}</div>
                  <h3 className="font-mono font-bold text-xl text-white mb-0.5">{step.title}</h3>
                  <p className="font-mono text-xs text-[#00A8FF]">{step.subtitle}</p>
                </div>

                <p className="text-[#a0a0a0] text-sm leading-relaxed mb-4">{step.desc}</p>

                {/* Code detail */}
                <div className="neu-card-inset px-3 py-2 font-mono text-xs text-[#555] group-hover:text-[#00A8FF]/60 transition-colors">
                  {step.detail}
                </div>

                {/* Arrow connector (not on last) */}
                {i < STEPS.length - 1 && (
                  <div className="hidden lg:block absolute -right-3 top-1/2 -translate-y-1/2 z-10">
                    <span className="font-mono text-[#00A8FF]/40 text-lg">→</span>
                  </div>
                )}
              </motion.div>
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}
