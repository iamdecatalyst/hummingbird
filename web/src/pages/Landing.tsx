import Nav from '../components/ui/Nav'
import Hero from '../components/landing/Hero'
import HowItWorks from '../components/landing/HowItWorks'
import Features from '../components/landing/Features'
import SignetPromo from '../components/landing/SignetPromo'
import CTA from '../components/landing/CTA'
import Footer from '../components/ui/Footer'

export default function Landing() {
  return (
    <div className="min-h-screen bg-[#0d0d0d]">
      <Nav />
      <Hero />

      {/* Divider */}
      <div className="max-w-6xl mx-auto px-6">
        <div className="h-px bg-gradient-to-r from-transparent via-white/8 to-transparent" />
      </div>

      <HowItWorks />

      <div className="max-w-6xl mx-auto px-6">
        <div className="h-px bg-gradient-to-r from-transparent via-white/8 to-transparent" />
      </div>

      <Features />

      <div className="max-w-6xl mx-auto px-6">
        <div className="h-px bg-gradient-to-r from-transparent via-[#22c55e]/10 to-transparent" />
      </div>

      <SignetPromo />

      <div className="max-w-6xl mx-auto px-6">
        <div className="h-px bg-gradient-to-r from-transparent via-white/8 to-transparent" />
      </div>

      <CTA />
      <Footer />
    </div>
  )
}
