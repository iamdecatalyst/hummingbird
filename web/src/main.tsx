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
