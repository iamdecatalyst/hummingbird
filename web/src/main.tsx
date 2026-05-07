import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { NexusProvider } from '@vylth/nexus-react'
import App from './App'
import './styles/index.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <NexusProvider
        clientId={import.meta.env.VITE_NEXUS_CLIENT_ID ?? ''}
        redirectUri={`${window.location.origin}/auth/callback`}
      >
        <App />
      </NexusProvider>
    </BrowserRouter>
  </React.StrictMode>,
)

// ---------------------------------------------------------------------------
// Vylth annotator widget — DIR-0081
// CLINICAL PHI SAFETY: This block is STRICTLY excluded from production builds.
// It must never run in any environment that has access to real patient data.
// Only enable with synthetic data in dev/staging.
// ---------------------------------------------------------------------------
if (import.meta.env.MODE !== 'production') {
  const token = import.meta.env.VITE_ANNOT_TOKEN
  if (token) {
    const script = document.createElement('script')
    script.src = 'https://annot.vylth.com/w.js'
    script.setAttribute('data-project', 'hummingbird')
    script.setAttribute('data-token', token)
    script.setAttribute('data-webhook', 'https://annot.vylth.com/v1/feedback')
    script.setAttribute(
      'data-redact',
      '.patient-name,.patient-id,.mrn,.dob,.ssn,.clinical-note,.diagnosis,.medication,.provider-name',
    )
    document.body.appendChild(script)
  }
}
