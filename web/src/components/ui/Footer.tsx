export default function Footer() {
  return (
    <footer className="border-t border-white/5 py-10">
      <div className="max-w-6xl mx-auto px-6 flex flex-col sm:flex-row items-center justify-between gap-4">
        <span className="flex items-center gap-2 font-mono text-sm text-[#555]">
          <img src="/logo.png" alt="" className="w-5 h-5 object-contain opacity-50" />
          Hummingbird — MIT License
        </span>
        <div className="flex items-center gap-6 font-mono text-xs text-[#555]">
          <a href="https://github.com/iamdecatalyst/hummingbird" target="_blank" rel="noopener noreferrer"
            className="hover:text-[#00A8FF] transition-colors">GitHub</a>
          <a href="https://signet.vylth.com" target="_blank" rel="noopener noreferrer"
            className="hover:text-[#00A8FF] transition-colors">Signet API</a>
          <a href="https://x.com/vylthofficial" target="_blank" rel="noopener noreferrer"
            className="hover:text-[#00A8FF] transition-colors">X / Twitter</a>
          <a href="https://t.me/iamdecatalyst" target="_blank" rel="noopener noreferrer"
            className="hover:text-[#00A8FF] transition-colors">Telegram</a>
          <span className="text-[#333]">Built by De Catalyst</span>
        </div>
      </div>
    </footer>
  )
}
