import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'

export const AppContext = createContext(null)
export const useApp = () => useContext(AppContext)

/* ---------- toasts ---------- */
const ToastContext = createContext(() => {})
export const useToast = () => useContext(ToastContext)

export function ToastProvider({ children }) {
  const [toasts, setToasts] = useState([])
  const push = useCallback((message, kind = 'info') => {
    const id = Math.random().toString(36).slice(2)
    setToasts((t) => [...t, { id, message, kind }])
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 4000)
  }, [])
  return (
    <ToastContext.Provider value={push}>
      {children}
      <div className="toasts">
        {toasts.map((t) => (
          <div key={t.id} className={`toast toast-${t.kind}`}>{t.message}</div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}

/* ---------- live events (SSE) ---------- */
export function useLiveEvents(onEvent, enabled = true) {
  const handler = useRef(onEvent)
  handler.current = onEvent
  const [connected, setConnected] = useState(false)
  useEffect(() => {
    if (!enabled) return
    const es = new EventSource('/api/events')
    es.onopen = () => setConnected(true)
    es.onerror = () => setConnected(false)
    es.onmessage = (e) => {
      try {
        handler.current(JSON.parse(e.data))
      } catch { /* ignore */ }
    }
    return () => es.close()
  }, [enabled])
  return connected
}

/* ---------- theme ---------- */
// 'auto' (default) follows the operating system's light/dark preference.
export function useTheme() {
  const [theme, setTheme] = useState(() => {
    try {
      return localStorage.getItem('pd-theme') || 'auto'
    } catch {
      return 'auto'
    }
  })
  useEffect(() => {
    const root = document.documentElement
    if (theme === 'auto') root.removeAttribute('data-theme')
    else root.setAttribute('data-theme', theme)
    try { localStorage.setItem('pd-theme', theme) } catch { /* ignore */ }
  }, [theme])
  const cycle = () => setTheme((t) => (t === 'auto' ? 'dark' : t === 'dark' ? 'light' : 'auto'))
  return [theme, cycle, setTheme]
}

/* ---------- desktop notifications ---------- */
const NOTIFY_KEY = 'pd-notify'
export function notificationsEnabled() {
  try {
    return localStorage.getItem(NOTIFY_KEY) === '1' && 'Notification' in window && Notification.permission === 'granted'
  } catch {
    return false
  }
}
export async function setNotifications(on) {
  if (on && 'Notification' in window && Notification.permission !== 'granted') {
    if ((await Notification.requestPermission()) !== 'granted') return false
  }
  try { localStorage.setItem(NOTIFY_KEY, on ? '1' : '0') } catch { /* ignore */ }
  return on
}
