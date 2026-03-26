import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'
import { api } from '../lib/api'

export default function CLIAuth() {
  const { token, loading } = useAuth()
  const navigate = useNavigate()
  const [cliToken, setCliToken] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (loading) return
    if (!token) {
      navigate('/login?next=/cli/auth', { replace: true })
      return
    }
    api.cliToken()
      .then(({ token }) => setCliToken(token))
      .catch(e => setError(e.message))
  }, [token, loading])

  function copy() {
    if (!cliToken) return
    navigator.clipboard.writeText(cliToken)
    setCopied(true)
    setTimeout(() => setCopied(false), 2500)
  }

  if (loading) return null

  return (
    <div style={{
      minHeight: '100vh',
      background: '#0d0d0d',
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      fontFamily: '"JetBrains Mono", "Fira Mono", monospace',
      padding: '2rem',
    }}>
      {/* Logo + title */}
      <div style={{ textAlign: 'center', marginBottom: '2.5rem' }}>
        <img src="/logo.png" alt="" style={{ width: 52, marginBottom: '1rem', filter: 'drop-shadow(0 0 12px rgba(0,168,255,0.4))' }} />
        <div style={{ color: '#00A8FF', fontWeight: 700, fontSize: '1.1rem', letterSpacing: 2 }}>HUMMINGBIRD</div>
        <div style={{ color: '#555', fontSize: '0.78rem', marginTop: 4 }}>CLI authentication</div>
      </div>

      {/* Terminal window */}
      <div style={{
        width: '100%',
        maxWidth: 640,
        background: '#111',
        border: '1px solid #222',
        borderRadius: 10,
        overflow: 'hidden',
        boxShadow: '0 0 40px rgba(0,168,255,0.08)',
      }}>
        {/* Title bar */}
        <div style={{
          background: '#1a1a1a',
          borderBottom: '1px solid #222',
          padding: '10px 16px',
          display: 'flex',
          alignItems: 'center',
          gap: 8,
        }}>
          <span style={{ width: 12, height: 12, borderRadius: '50%', background: '#EF4444', display: 'inline-block' }} />
          <span style={{ width: 12, height: 12, borderRadius: '50%', background: '#F59E0B', display: 'inline-block' }} />
          <span style={{ width: 12, height: 12, borderRadius: '50%', background: '#4ADE80', display: 'inline-block' }} />
          <span style={{ color: '#444', fontSize: '0.75rem', marginLeft: 8 }}>hummingbird — cli token</span>
        </div>

        {/* Body */}
        <div style={{ padding: '1.5rem 1.75rem' }}>
          <div style={{ color: '#555', fontSize: '0.78rem', marginBottom: '1.25rem', lineHeight: 1.7 }}>
            <span style={{ color: '#00A8FF' }}>$</span> hummingbird login<br />
            <span style={{ color: '#4ADE80' }}>→</span> Paste the token below when prompted.
          </div>

          {error && (
            <div style={{ color: '#EF4444', fontSize: '0.8rem', marginBottom: '1rem' }}>
              ✗ {error}
            </div>
          )}

          {!cliToken && !error && (
            <div style={{ color: '#555', fontSize: '0.8rem' }}>generating token…</div>
          )}

          {cliToken && (
            <>
              <div style={{ marginBottom: '0.75rem' }}>
                <div style={{ color: '#555', fontSize: '0.7rem', marginBottom: 6, letterSpacing: 1 }}>YOUR CLI TOKEN</div>
                <div style={{
                  background: '#0d0d0d',
                  border: '1px solid #2a2a2a',
                  borderRadius: 6,
                  padding: '14px 16px',
                  color: '#00A8FF',
                  fontSize: '0.72rem',
                  wordBreak: 'break-all',
                  lineHeight: 1.6,
                  letterSpacing: 0.5,
                }}>
                  {cliToken}
                </div>
              </div>

              <button
                onClick={copy}
                style={{
                  width: '100%',
                  padding: '10px',
                  background: copied ? '#052' : '#0d0d0d',
                  border: `1px solid ${copied ? '#4ADE80' : '#2a2a2a'}`,
                  borderRadius: 6,
                  color: copied ? '#4ADE80' : '#e0e0e0',
                  fontFamily: 'inherit',
                  fontSize: '0.8rem',
                  cursor: 'pointer',
                  transition: 'all 0.2s',
                  letterSpacing: 1,
                }}
              >
                {copied ? '✓  copied to clipboard' : 'copy token'}
              </button>

              <div style={{ color: '#333', fontSize: '0.72rem', marginTop: '1.25rem', lineHeight: 1.7 }}>
                <span style={{ color: '#555' }}>expires in</span> 7 days
                <br />
                <span style={{ color: '#555' }}>run</span>{' '}
                <span style={{ color: '#00A8FF' }}>hummingbird login</span>{' '}
                <span style={{ color: '#555' }}>again to generate a new one</span>
              </div>
            </>
          )}
        </div>
      </div>

      <div style={{ color: '#2a2a2a', fontSize: '0.7rem', marginTop: '2rem' }}>
        by VYLTH Strategies · @iamdecatalyst
      </div>
    </div>
  )
}
