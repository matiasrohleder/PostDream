import { useCallback, useEffect, useState } from 'react'
import { Navigate, Route, Routes, useNavigate } from 'react-router-dom'
import { api } from './lib/api.js'
import { AppContext, useTheme } from './lib/context.jsx'
import Layout from './components/Layout.jsx'
import Login from './pages/Login.jsx'
import Mailbox from './pages/Mailbox.jsx'
import Settings from './pages/Settings.jsx'
import Logo from './components/Logo.jsx'

export default function App() {
  const [config, setConfig] = useState(null)
  const [user, setUser] = useState(undefined) // undefined = loading, null = logged out
  const [inboxes, setInboxes] = useState([])
  const [theme, cycleTheme, setTheme] = useTheme()
  const navigate = useNavigate()

  const refreshInboxes = useCallback(async () => {
    try {
      setInboxes(await api('/inboxes'))
    } catch { /* handled by 401 listener */ }
  }, [])

  useEffect(() => {
    api('/config').then(setConfig).catch(() => setConfig({}))
    api('/auth/me').then(setUser).catch(() => setUser(null))
  }, [])

  useEffect(() => {
    if (user) refreshInboxes()
  }, [user, refreshInboxes])

  useEffect(() => {
    const onUnauthorized = () => {
      setUser(null)
      navigate('/login')
    }
    window.addEventListener('pd:unauthorized', onUnauthorized)
    return () => window.removeEventListener('pd:unauthorized', onUnauthorized)
  }, [navigate])

  const logout = async () => {
    await api('/auth/logout', { method: 'POST' }).catch(() => {})
    setUser(null)
    navigate('/login')
  }

  if (user === undefined || config === null) {
    return (
      <div className="splash">
        <Logo size={96} />
      </div>
    )
  }

  const ctx = { config, user, setUser, inboxes, refreshInboxes, logout, theme, cycleTheme, setTheme }
  return (
    <AppContext.Provider value={ctx}>
      <Routes>
        <Route path="/login" element={user ? <Navigate to="/" replace /> : <Login onLogin={setUser} config={config} />} />
        <Route element={user ? <Layout /> : <Navigate to="/login" replace />}>
          <Route path="/" element={<Mailbox />} />
          <Route path="/inbox/:inboxId" element={<Mailbox />} />
          <Route path="/settings/*" element={<Settings />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </AppContext.Provider>
  )
}
