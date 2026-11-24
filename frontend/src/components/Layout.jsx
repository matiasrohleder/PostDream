import { useEffect, useRef, useState } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { notificationsEnabled, useApp, useLiveEvents, useToast } from '../lib/context.jsx'
import { LanguageSelect, useT } from '../i18n/index.jsx'
import { displayName } from '../lib/format.js'
import Logo from './Logo.jsx'
import Icon from './Icon.jsx'

export const THEME_LABELS = { auto: 'System', light: 'Light', dark: 'Dark' }

export default function Layout() {
  const { user, inboxes, refreshInboxes, logout, config, theme, cycleTheme } = useApp()
  const t = useT()
  const [liveEvent, setLiveEvent] = useState(null)
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const toast = useToast()
  const navigate = useNavigate()
  const refreshTimer = useRef(null)

  const connected = useLiveEvents((e) => {
    setLiveEvent({ ...e, at: Date.now() })
    if (e.type === 'message.new' && e.data) {
      const from = displayName(e.data.from) || t('New email')
      const subject = e.data.subject || t('(no subject)')
      toast(`📬 ${from}: ${subject}`, 'success')
      if (document.hidden && notificationsEnabled()) {
        try {
          const n = new Notification(from, { body: subject, tag: e.data.id, icon: '/favicon.svg' })
          n.onclick = () => {
            window.focus()
            navigate(`/inbox/${e.data.inbox_id}?m=${e.data.id}`)
            n.close()
          }
        } catch { /* notifications unavailable */ }
      }
    }
    clearTimeout(refreshTimer.current)
    refreshTimer.current = setTimeout(refreshInboxes, 300)
  })

  const totalUnread = inboxes.reduce((n, i) => n + i.unread_count, 0)
  const total = inboxes.reduce((n, i) => n + i.message_count, 0)

  useEffect(() => {
    document.title = totalUnread > 0 ? `(${totalUnread}) PostDream` : 'PostDream'
  }, [totalUnread])

  return (
    <div className={`app ${sidebarOpen ? 'sidebar-open' : ''}`}>
      <header className="topbar">
        <button className="icon-btn mobile-only" onClick={() => setSidebarOpen((o) => !o)} aria-label={t('Menu')}>
          <Icon name="menu" />
        </button>
        <a className="brand" href="/" onClick={(e) => { e.preventDefault(); navigate('/') }}>
          <Logo size={34} withText />
        </a>
        <div className="topbar-spacer" />
        <span className={`live-dot ${connected ? 'on' : ''}`} title={connected ? t('Live updates connected') : t('Reconnecting…')}>
          {connected ? t('Live') : t('Offline')}
        </span>
        <LanguageSelect className="hide-mobile" />
        <button className="icon-btn" onClick={cycleTheme} title={`${t('Theme')}: ${t(THEME_LABELS[theme])}`}>
          <Icon name={theme === 'dark' ? 'moon' : theme === 'light' ? 'sun' : 'auto'} />
        </button>
        <NavLink to="/settings" className="icon-btn" title={t('Settings')}>
          <Icon name="settings" />
        </NavLink>
        {config.auth_enabled && (
          <div className="user-chip">
            <span className="avatar">{(user.name || user.email || '?')[0].toUpperCase()}</span>
            <span className="user-email">{user.email}</span>
            <button className="icon-btn" onClick={logout} title={t('Sign out')}>
              <Icon name="logout" />
            </button>
          </div>
        )}
      </header>

      <aside className="sidebar" onClick={() => setSidebarOpen(false)}>
        <div className="sidebar-title">{t('Inboxes')}</div>
        <NavLink to="/" end className="inbox-link">
          <Icon name="all" />
          <span className="inbox-name">{t('All')}</span>
          {totalUnread > 0 && <span className="badge">{totalUnread}</span>}
          <span className="count">{total}</span>
        </NavLink>
        {inboxes.map((i) => (
          <NavLink key={i.id} to={`/inbox/${i.id}`} className="inbox-link">
            <Icon name="inbox" />
            <span className="inbox-name" title={i.description || i.name}>{i.name}</span>
            {i.unread_count > 0 && <span className="badge">{i.unread_count}</span>}
            <span className="count">{i.message_count}</span>
          </NavLink>
        ))}
        {inboxes.length === 0 && <p className="muted small pad">{t('You have no inboxes assigned.')}</p>}
        <div className="sidebar-footer">
          <div className="small muted">{t('SMTP on port')} <b>{config.smtp_port}</b></div>
          <div className="small muted">{t('Retention')}: {config.retention_hours} h</div>
          <div className="small muted">v{config.version}</div>
        </div>
      </aside>

      <main className="content">
        <Outlet context={{ liveEvent }} />
      </main>
    </div>
  )
}
