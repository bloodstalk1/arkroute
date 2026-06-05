import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import "@phosphor-icons/web/regular"
import "@phosphor-icons/web/light"
import "@phosphor-icons/web/bold"
import "@phosphor-icons/web/fill"
import './index.css'
import App from './App.jsx'

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
