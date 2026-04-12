import { useState, useEffect } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { motion, AnimatePresence } from 'framer-motion'
import { GithubLogo, XLogo, List, X } from '@phosphor-icons/react'

export default function Nav() {
  const [scrolled, setScrolled] = useState(false)
  const [open, setOpen] = useState(false)
  const { pathname } = useLocation()

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 40)
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [])

  return (
    <motion.header
      initial={{ y: -20, opacity: 0 }}
      animate={{ y: 0, opacity: 1 }}
      transition={{ duration: 0.5, ease: 'easeOut' }}
      className={`fixed top-0 left-0 right-0 z-50 transition-all duration-300 ${
        scrolled
          ? 'bg-[#0d0d0d]/90 backdrop-blur-md shadow-neu-raised'
          : 'bg-transparent'
      }`}
    >
      <div className="max-w-6xl mx-auto px-6 h-16 flex items-center justify-between">
        {/* Logo */}
        <Link to="/" className="flex items-center gap-3 group">
          <img
            src="/logo.png"
            alt="Hummingbird"
            className="w-8 h-8 object-contain transition-transform duration-300 group-hover:scale-110"
            style={{ filter: 'drop-shadow(0 0 8px rgba(0,168,255,0.4))' }}
          />
          <span className="font-mono font-bold tracking-widest text-white group-hover:text-[#00A8FF] transition-colors duration-200">
            HUMMINGBIRD
          </span>
        </Link>

        {/* Desktop nav */}
        <nav className="hidden md:flex items-center gap-1">
          <NavLink href="/#how-it-works">How It Works</NavLink>
          <NavLink href="/#features">Features</NavLink>
          <NavLink href="https://github.com/iamdecatalyst/hummingbird" external>
            <GithubLogo size={15} className="inline -mt-0.5 mr-1" />GitHub
          </NavLink>
          <NavLink href="https://x.com/vylthofficial" external>
            <XLogo size={15} className="inline -mt-0.5 mr-1" />X
          </NavLink>
          {pathname !== '/dashboard' && (
            <Link
              to="/dashboard"
              className="ml-4 hb-btn text-sm py-2.5 px-5"
            >
              Launch Dashboard →
            </Link>
          )}
        </nav>

        {/* Mobile hamburger */}
        <button
          className="md:hidden w-10 h-10 flex items-center justify-center neu-tile rounded-xl"
          onClick={() => setOpen(o => !o)}
          aria-label="Toggle menu"
        >
          {open ? <X size={18} /> : <List size={18} />}
        </button>
      </div>

      {/* Mobile menu */}
      <AnimatePresence>
        {open && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: 'auto' }}
            exit={{ opacity: 0, height: 0 }}
            className="md:hidden bg-[#0d0d0d]/95 backdrop-blur-md border-t border-white/5"
          >
            <div className="px-6 py-4 flex flex-col gap-3">
              <MobileLink href="/#how-it-works" onClick={() => setOpen(false)}>How It Works</MobileLink>
              <MobileLink href="/#features" onClick={() => setOpen(false)}>Features</MobileLink>
              <MobileLink href="https://github.com/iamdecatalyst/hummingbird" onClick={() => setOpen(false)}>
                GitHub ↗
              </MobileLink>
              <MobileLink href="https://x.com/vylthofficial" onClick={() => setOpen(false)}>
                X / Twitter ↗
              </MobileLink>
              <Link to="/dashboard" className="hb-btn justify-center mt-2" onClick={() => setOpen(false)}>
                Launch Dashboard →
              </Link>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </motion.header>
  )
}

function NavLink({ href, children, external }: { href: string; children: React.ReactNode; external?: boolean }) {
  return (
    <a
      href={href}
      target={external ? '_blank' : undefined}
      rel={external ? 'noopener noreferrer' : undefined}
      className="px-4 py-2 font-mono text-sm text-[#a0a0a0] hover:text-white transition-colors duration-200"
    >
      {children}
    </a>
  )
}

function MobileLink({ href, children, onClick }: { href: string; children: React.ReactNode; onClick?: () => void }) {
  return (
    <a
      href={href}
      onClick={onClick}
      className="font-mono text-sm text-[#a0a0a0] hover:text-white py-2 transition-colors"
    >
      {children}
    </a>
  )
}
