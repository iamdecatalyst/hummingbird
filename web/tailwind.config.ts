import type { Config } from 'tailwindcss'

export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        hb: {
          blue:    '#00A8FF',
          'blue-dim': '#0078CC',
          'blue-dark': '#004A8F',
          navy:    '#0A1628',
          'navy-light': '#0D1F3C',
        },
        neu: {
          base:     '#0d0d0d',
          card:     '#141414',
          input:    '#0a0a0a',
          elevated: '#1a1a1a',
        },
      },
      fontFamily: {
        mono: ['"JetBrains Mono"', '"Space Mono"', 'monospace'],
        sans: ['Inter', 'system-ui', 'sans-serif'],
        serif: ['"Crimson Pro"', 'Georgia', 'serif'],
      },
      boxShadow: {
        'neu-raised': '6px 6px 12px rgba(0,0,0,0.7), -6px -6px 12px rgba(40,40,40,0.15)',
        'neu-raised-lg': '10px 10px 20px rgba(0,0,0,0.85), -10px -10px 20px rgba(50,50,50,0.2)',
        'neu-pressed': 'inset 4px 4px 8px rgba(0,0,0,0.7), inset -4px -4px 8px rgba(40,40,40,0.08)',
        'hb-glow': '0 0 30px rgba(0,168,255,0.25), 0 0 60px rgba(0,168,255,0.1)',
        'hb-glow-sm': '0 0 16px rgba(0,168,255,0.3)',
      },
      animation: {
        'pulse-slow': 'pulse 3s cubic-bezier(0.4,0,0.6,1) infinite',
        'scroll-up': 'scrollUp 12s linear infinite',
        'float': 'float 4s ease-in-out infinite',
        'glow-pulse': 'glowPulse 2s ease-in-out infinite',
      },
      keyframes: {
        scrollUp: {
          '0%': { transform: 'translateY(0)' },
          '100%': { transform: 'translateY(-50%)' },
        },
        float: {
          '0%, 100%': { transform: 'translateY(0px)' },
          '50%': { transform: 'translateY(-10px)' },
        },
        glowPulse: {
          '0%, 100%': { boxShadow: '0 0 20px rgba(0,168,255,0.2)' },
          '50%': { boxShadow: '0 0 40px rgba(0,168,255,0.5), 0 0 80px rgba(0,168,255,0.2)' },
        },
      },
    },
  },
  plugins: [],
} satisfies Config
