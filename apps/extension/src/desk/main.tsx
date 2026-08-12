import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import '@/styles/app.css'
import { Desk } from '@/desk/Desk'

const host = document.getElementById('root')
if (host) createRoot(host).render(<StrictMode><Desk /></StrictMode>)
