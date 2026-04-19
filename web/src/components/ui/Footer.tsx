export default function Footer() {
  return (
    <footer className="border-t border-white/5 py-10">
      <div className="max-w-6xl mx-auto px-6 flex flex-col sm:flex-row items-center justify-between gap-4">
        <span className="flex items-center gap-2 font-mono text-sm text-[#555]">
          <img src="/logo.png" alt="" className="w-5 h-5 object-contain opacity-50" />
          Hummingbird — MIT License
        </span>
        <div className="flex items-center gap-5">
          <a href="https://github.com/iamdecatalyst/hummingbird" target="_blank" rel="noopener noreferrer"
            title="GitHub" className="text-[#555] hover:text-white transition-colors">
            <img src="/github.svg" alt="GitHub" className="w-5 h-5 opacity-50 hover:opacity-100 transition-opacity" />
          </a>
          <a href="https://signet.vylth.com" target="_blank" rel="noopener noreferrer"
            title="Signet API" className="text-[#555] hover:text-[#22c55e] transition-colors">
            <img src="/signet-logo.png" alt="Signet" className="h-5 object-contain opacity-50 hover:opacity-100 transition-opacity" />
          </a>
          <a href="https://t.me/vylthummingbird" target="_blank" rel="noopener noreferrer"
            title="Telegram Community" className="text-[#555] hover:text-[#24A1DE] transition-colors">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12 0C5.373 0 0 5.373 0 12s5.373 12 12 12 12-5.373 12-12S18.627 0 12 0zm5.894 8.221-1.97 9.28c-.145.658-.537.818-1.084.508l-3-2.21-1.447 1.394c-.16.16-.295.295-.605.295l.213-3.053 5.56-5.023c.242-.213-.054-.333-.373-.12L7.19 13.137l-2.96-.924c-.643-.204-.657-.643.136-.953l11.57-4.461c.537-.194 1.006.131.958.422z"/>
            </svg>
          </a>
          <a href="https://x.com/vylthofficial" target="_blank" rel="noopener noreferrer"
            title="X / Twitter" className="text-[#555] hover:text-white transition-colors">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
              <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"/>
            </svg>
          </a>
        </div>
      </div>
    </footer>
  )
}
