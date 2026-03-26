import { useEffect, useRef, useState } from 'react'
import { motion } from 'framer-motion'
import { Link } from 'react-router-dom'
import { GithubLogo, Star, ArrowRight } from '@phosphor-icons/react'

// Mock trade feed entries — look real, display fast
const FEED_ENTRIES = [
  { type: 'enter', mode: 'SNIPER',  mint: 'PUMP3R8A', score: 87, sol: '0.200', time: '09:14:02' },
  { type: 'exit',  mode: 'TP1',     mint: 'PUMP3R8A', pnl: '+0.184', pct: '+92%',  time: '09:17:21' },
  { type: 'enter', mode: 'SCALPER', mint: '7xKp2mNq', score: 72, sol: '0.050', time: '09:18:05' },
  { type: 'enter', mode: 'SNIPER',  mint: 'MNSHTf9x', score: 91, sol: '0.200', time: '09:19:33' },
  { type: 'exit',  mode: 'SL',      mint: '7xKp2mNq', pnl: '-0.010', pct: '-20%',  time: '09:20:14' },
  { type: 'exit',  mode: 'TP2',     mint: 'MNSHTf9x', pnl: '+0.820', pct: '+410%', time: '09:24:58' },
  { type: 'enter', mode: 'SNIPER',  mint: 'Boop7r3K', score: 83, sol: '0.100', time: '09:25:11' },
  { type: 'enter', mode: 'SCALPER', mint: 'VRTL9mzP', score: 68, sol: '0.050', time: '09:26:44' },
  { type: 'exit',  mode: 'TP1',     mint: 'Boop7r3K', pnl: '+0.094', pct: '+94%',  time: '09:29:02' },
  { type: 'exit',  mode: 'TP3',     mint: 'VRTL9mzP', pnl: '+0.372', pct: '+744%', time: '09:31:18' },
  { type: 'enter', mode: 'SNIPER',  mint: 'RAYLch2K', score: 79, sol: '0.100', time: '09:33:07' },
  { type: 'exit',  mode: 'TIMEOUT', mint: 'RAYLch2K', pnl: '-0.022', pct: '-22%',  time: '09:41:07' },
]

// Doubled for seamless loop
const FEED = [...FEED_ENTRIES, ...FEED_ENTRIES]

function FeedRow({ entry }: { entry: typeof FEED_ENTRIES[0] }) {
  if (entry.type === 'enter') {
    return (
      <div className="flex items-center gap-2 py-1.5 font-mono text-xs">
        <span className="text-[#00A8FF]">🐦</span>
        <span className="text-[#00A8FF] font-bold">ENTERED</span>
        <span className="text-white/40">[{entry.mode}]</span>
        <span className="text-white font-bold">{entry.mint}</span>
        <span className="text-white/40">score:</span>
        <span className="text-[#00A8FF]">{entry.score}</span>
        <span className="text-white/40">pos:</span>
        <span className="text-white">{entry.sol} SOL</span>
        <span className="ml-auto text-white/25">{entry.time}</span>
      </div>
    )
  }
  const isProfit = entry.pnl!.startsWith('+')
  const isLoss = entry.pnl!.startsWith('-')
  return (
    <div className="flex items-center gap-2 py-1.5 font-mono text-xs">
      <span>{isProfit ? '✅' : '❌'}</span>
      <span className={isProfit ? 'text-[#4ADE80] font-bold' : 'text-[#EF4444] font-bold'}>EXITED</span>
      <span className="text-white/40">[{entry.mode}]</span>
      <span className="text-white font-bold">{entry.mint}</span>
      <span className={`ml-auto font-bold ${isProfit ? 'text-[#4ADE80]' : 'text-[#EF4444]'}`}>
        {entry.pnl} SOL
      </span>
      <span className={`font-bold ${isProfit ? 'text-[#4ADE80]' : 'text-[#EF4444]'}`}>
        {entry.pct}
      </span>
    </div>
  )
}

// Animated counter
function Counter({ end, suffix = '', prefix = '' }: { end: number; suffix?: string; prefix?: string }) {
  const [val, setVal] = useState(0)
  const ref = useRef<HTMLSpanElement>(null)

  useEffect(() => {
    const obs = new IntersectionObserver(
      ([entry]) => {
        if (!entry.isIntersecting) return
        obs.disconnect()
        let start = 0
        const step = end / 60
        const timer = setInterval(() => {
          start += step
          if (start >= end) { setVal(end); clearInterval(timer) }
          else setVal(Math.floor(start))
        }, 16)
      },
      { threshold: 0.5 }
    )
    if (ref.current) obs.observe(ref.current)
    return () => obs.disconnect()
  }, [end])

  return (
    <span ref={ref} className="hb-glow-text font-mono font-bold">
      {prefix}{val.toLocaleString()}{suffix}
    </span>
  )
}

export default function Hero() {
  return (
    <section className="relative min-h-screen flex flex-col justify-center pt-24 pb-16 overflow-hidden">
      {/* Background grid */}
      <div
        className="absolute inset-0 opacity-[0.03]"
        style={{
          backgroundImage: `
            linear-gradient(rgba(0,168,255,0.5) 1px, transparent 1px),
            linear-gradient(90deg, rgba(0,168,255,0.5) 1px, transparent 1px)
          `,
          backgroundSize: '40px 40px',
        }}
      />

      {/* Blue radial glow */}
      <div className="absolute top-1/3 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[600px] rounded-full pointer-events-none"
        style={{ background: 'radial-gradient(circle, rgba(0,168,255,0.06) 0%, transparent 70%)' }}
      />

      <div className="relative max-w-6xl mx-auto px-6 w-full">
        <div className="grid lg:grid-cols-2 gap-12 items-center">

          {/* Left — copy */}
          <div>
            {/* Logo + status */}
            <motion.div
              initial={{ opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.5 }}
              className="flex items-center gap-4 mb-8"
            >
              <img
                src="/logo.png"
                alt="Hummingbird"
                className="w-14 h-14 object-contain animate-float"
                style={{ filter: 'drop-shadow(0 0 16px rgba(0,168,255,0.5))' }}
              />
              <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full neu-card-inset">
                <span className="status-dot-live" />
                <span className="font-mono text-xs text-[#a0a0a0] tracking-widest uppercase">
                  Live on Solana mainnet
                </span>
              </div>
            </motion.div>

            {/* Headline */}
            <motion.h1
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.6, delay: 0.1 }}
              className="font-mono font-bold leading-tight mb-4"
              style={{ fontSize: 'clamp(2rem, 5vw, 3.5rem)' }}
            >
              <span className="text-white">The fastest</span>
              <br />
              <span className="hb-gradient-text">trading agent</span>
              <br />
              <span className="text-white">on Solana.</span>
            </motion.h1>

            <motion.p
              initial={{ opacity: 0, y: 16 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.6, delay: 0.2 }}
              className="text-[#a0a0a0] text-lg mb-8 max-w-md leading-relaxed"
            >
              Hummingbird detects new token launches in{' '}
              <span className="text-white font-medium">&lt;100ms</span>, scores them across{' '}
              <span className="text-white font-medium">5 parallel signals</span>, and executes
              sniper + scalper trades — autonomously, 24/7.
            </motion.p>

            {/* Stack badges */}
            <motion.div
              initial={{ opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.5, delay: 0.3 }}
              className="flex flex-wrap gap-2 mb-10"
            >
              {['Rust', 'Python', 'Go', 'Solana', 'Open Source'].map(tag => (
                <span key={tag} className="font-mono text-xs px-3 py-1.5 rounded-full neu-card-inset text-[#a0a0a0]">
                  {tag}
                </span>
              ))}
            </motion.div>

            {/* CTAs */}
            <motion.div
              initial={{ opacity: 0, y: 16 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.5, delay: 0.4 }}
              className="flex flex-wrap gap-3"
            >
              <Link to="/dashboard" className="hb-btn">
                Launch Dashboard <ArrowRight size={16} weight="bold" />
              </Link>
              <a
                href="https://github.com/iamdecatalyst/hummingbird"
                target="_blank"
                rel="noopener noreferrer"
                className="neu-btn-ghost"
              >
                <Star size={15} weight="fill" className="text-[#00A8FF]" /> Star on GitHub
              </a>
            </motion.div>

            {/* Stats row */}
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ duration: 0.6, delay: 0.7 }}
              className="grid grid-cols-3 gap-4 mt-12 pt-10 border-t border-white/5"
            >
              <div>
                <div className="font-mono text-2xl font-bold mb-1">
                  <Counter end={100} prefix="<" suffix="ms" />
                </div>
                <div className="text-xs text-[#555] font-mono uppercase tracking-widest">Detection</div>
              </div>
              <div>
                <div className="font-mono text-2xl font-bold mb-1">
                  <Counter end={7} />
                  <span className="hb-glow-text font-mono font-bold"> chains</span>
                </div>
                <div className="text-xs text-[#555] font-mono uppercase tracking-widest">Platforms</div>
              </div>
              <div>
                <div className="font-mono text-2xl font-bold mb-1">
                  <Counter end={24} suffix="/7" />
                </div>
                <div className="text-xs text-[#555] font-mono uppercase tracking-widest">Autonomous</div>
              </div>
            </motion.div>
          </div>

          {/* Right — live terminal */}
          <motion.div
            initial={{ opacity: 0, x: 30 }}
            animate={{ opacity: 1, x: 0 }}
            transition={{ duration: 0.7, delay: 0.3 }}
            className="relative"
          >
            {/* Outer glow halo */}
            <div className="absolute -inset-4 rounded-3xl pointer-events-none"
              style={{ background: 'radial-gradient(ellipse, rgba(0,168,255,0.07) 0%, transparent 70%)' }}
            />

            {/* Terminal window */}
            <div className="relative rounded-2xl overflow-hidden scan-line"
              style={{
                background: '#0e0e0e',
                boxShadow: `
                  10px 10px 20px rgba(0,0,0,0.9),
                  -10px -10px 20px rgba(40,40,40,0.12),
                  0 0 0 1px rgba(0,168,255,0.1),
                  0 0 40px rgba(0,168,255,0.05)
                `,
              }}
            >
              {/* Title bar */}
              <div className="flex items-center gap-2 px-4 py-3 border-b border-white/5"
                style={{ background: '#0a0a0a' }}
              >
                <span className="w-3 h-3 rounded-full bg-[#ff5f57]" style={{ boxShadow: '0 0 6px rgba(255,95,87,0.5)' }} />
                <span className="w-3 h-3 rounded-full bg-[#febc2e]" style={{ boxShadow: '0 0 6px rgba(254,188,46,0.5)' }} />
                <span className="w-3 h-3 rounded-full bg-[#28c840]" style={{ boxShadow: '0 0 6px rgba(40,200,64,0.5)' }} />
                <span className="ml-3 font-mono text-xs text-[#555]">hummingbird — live trades</span>
                <span className="ml-auto flex items-center gap-1.5">
                  <span className="status-dot-live" />
                  <span className="font-mono text-xs text-[#00A8FF]">LIVE</span>
                </span>
              </div>

              {/* Scrolling feed */}
              <div className="h-[380px] overflow-hidden px-4 py-2" style={{ maskImage: 'linear-gradient(transparent, black 8%, black 92%, transparent)' }}>
                <div className="feed-scroll">
                  {FEED.map((entry, i) => (
                    <FeedRow key={i} entry={entry} />
                  ))}
                </div>
              </div>

              {/* Bottom prompt */}
              <div className="px-4 py-3 border-t border-white/5 font-mono text-xs text-[#555] flex items-center gap-2">
                <span className="text-[#00A8FF]">$</span>
                <span>watching pump.fun · moonshot · raydium · boop.fun</span>
                <span className="ml-auto animate-pulse">▋</span>
              </div>
            </div>
          </motion.div>

        </div>
      </div>
    </section>
  )
}
