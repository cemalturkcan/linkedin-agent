import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import '@/styles/app.css'
import { Panel } from '@/panel/Panel'

const host = document.getElementById('root')
if (host) createRoot(host).render(<StrictMode><Panel /></StrictMode>)
