import { useEffect, useState } from 'react'
import { NavLink, Navigate, Route, Routes } from 'react-router-dom'
import { api } from '../lib/api.js'
import { notificationsEnabled, setNotifications, useApp, useToast } from '../lib/context.jsx'
import { LANGUAGES, useI18n, useT } from '../i18n/index.jsx'
import { copyText, formatDate } from '../lib/format.js'
import Icon from '../components/Icon.jsx'
import Modal from '../components/Modal.jsx'
import SetupSnippets from '../components/SetupSnippets.jsx'
import { ChaosModal, InboxSettingsModal, RulesModal, WebhooksModal } from '../components/InboxTools.jsx'
import ApiDocs from './ApiDocs.jsx'
import { THEME_LABELS } from '../components/Layout.jsx'

export default function Settings() {
  const { user, config } = useApp()
  const t = useT()
  const isAdmin = user.role === 'admin'
  return (
    <div className="settings">
      <nav className="settings-nav">
        <NavLink to="/settings/inboxes">{t('Inboxes')}</NavLink>
        {isAdmin && config.auth_enabled && <NavLink to="/settings/users">{t('Users')}</NavLink>}
        {config.auth_enabled && <NavLink to="/settings/account">{t('My account')}</NavLink>}
        <NavLink to="/settings/preferences">{t('Preferences')}</NavLink>
        <NavLink to="/settings/api">{t('API')}</NavLink>
        <NavLink to="/settings/server">{t('Server')}</NavLink>
      </nav>
      <div className="settings-body">
        <Routes>
          <Route path="inboxes" element={<InboxesPage />} />
          <Route path="users" element={isAdmin ? <UsersPage /> : <Navigate to="/settings/inboxes" />} />
          <Route path="account" element={<AccountPage />} />
          <Route path="preferences" element={<PreferencesPage />} />
          <Route path="api" element={<ApiDocs />} />
          <Route path="server" element={<ServerPage />} />
          <Route path="*" element={<Navigate to="/settings/inboxes" replace />} />
        </Routes>
      </div>
    </div>
  )
}

function Secret({ value }) {
  const [show, setShow] = useState(false)
  const toast = useToast()
  const t = useT()
  return (
    <span className="secret">
      <code>{show ? value : '•'.repeat(Math.min(value?.length || 8, 16))}</code>
      <button className="icon-btn" onClick={() => setShow((s) => !s)} title={t('Show')}><Icon name="eye" size={14} /></button>
      <button className="icon-btn" onClick={() => copyText(value).then(() => toast(t('Copied')))} title={t('Copy')}><Icon name="copy" size={14} /></button>
    </span>
  )
}

function Copyable({ value }) {
  const toast = useToast()
  const t = useT()
  return (
    <span className="secret">
      <code>{value}</code>
      <button className="icon-btn" onClick={() => copyText(String(value)).then(() => toast(t('Copied')))} title={t('Copy')}><Icon name="copy" size={14} /></button>
    </span>
  )
}

/* ---------------- Inboxes ---------------- */

function InboxesPage() {
  const { inboxes, refreshInboxes, user, config } = useApp()
  const t = useT()
  const isAdmin = user.role === 'admin'
  const [editing, setEditing] = useState(null) // null | {} (new) | inbox
  const [setupFor, setSetupFor] = useState(null)
  const [tool, setTool] = useState(null) // { kind: 'webhooks'|'rules'|'settings'|'chaos', inbox }
  const toast = useToast()
  const host = window.location.hostname

  const rotate = async (inbox) => {
    if (!confirm(t('Regenerate the SMTP password and API token of "{name}"? Configured apps will stop authenticating.', { name: inbox.name }))) return
    await api(`/inboxes/${inbox.id}/rotate`, { method: 'POST' })
    toast(t('Credentials regenerated'), 'success')
    refreshInboxes()
  }
  const remove = async (inbox) => {
    if (!confirm(t('Delete inbox "{name}" and ALL its emails?', { name: inbox.name }))) return
    try {
      await api(`/inboxes/${inbox.id}`, { method: 'DELETE' })
      toast(t('Inbox deleted'), 'success')
      refreshInboxes()
    } catch (e) { toast(e.message, 'error') }
  }

  return (
    <>
      <div className="page-head">
        <div>
          <h2>{t('Inboxes')}</h2>
          <p className="muted">
            {t('Each inbox is a tenant: it has its own SMTP credentials, API token and emails.')}{' '}
            {t("Emails authenticated with an inbox's username/password land in that inbox.")}{' '}
            {config.smtp_auth === 'optional' && t('Unauthenticated emails land in "{inbox}".', { inbox: config.default_inbox })}
            {config.smtp_auth === 'required' && t('Unauthenticated emails are rejected.')}
          </p>
        </div>
        {isAdmin && <button className="btn btn-primary" onClick={() => setEditing({})}><Icon name="plus" /> {t('New inbox')}</button>}
      </div>

      <div className="cards">
        {inboxes.map((i) => (
          <div key={i.id} className="card">
            <div className="card-head">
              <div>
                <h3>{i.name}</h3>
                <div className="muted small">
                  {i.slug} · {t('{n} emails', { n: i.message_count })} · {t('retention {h} h', { h: i.retention_hours ?? config.retention_hours })}
                </div>
                {i.description && <p className="small">{i.description}</p>}
              </div>
              {isAdmin && (
                <div className="row-actions">
                  <button className="icon-btn" onClick={() => setEditing(i)} title={t('Edit')}><Icon name="edit" /></button>
                  <button className="icon-btn" onClick={() => rotate(i)} title={t('Regenerate credentials')}><Icon name="key" /></button>
                  <button className="icon-btn danger" onClick={() => remove(i)} title={t('Delete')}><Icon name="trash" /></button>
                </div>
              )}
            </div>
            <table className="kv compact">
              <tbody>
                <tr><th>{t('Host')}</th><td><Copyable value={host} /></td></tr>
                <tr><th>{t('Port')}</th><td><Copyable value={config.smtp_port} /></td></tr>
                <tr><th>{t('Username')}</th><td><Copyable value={i.smtp_username} /></td></tr>
                <tr><th>{t('Password')}</th><td><Secret value={i.smtp_password} /></td></tr>
                <tr><th>{t('API token')}</th><td><Secret value={i.api_token} /></td></tr>
              </tbody>
            </table>
            {chaosOn(i) && <div className="chaos-banner"><Icon name="zap" size={14} /> {t('Chaos testing is active: some SMTP commands fail on purpose.')}</div>}
            <div className="card-actions">
              <button className="btn btn-small" onClick={() => setSetupFor(i)}><Icon name="code" size={14} /> {t('Show configuration examples')}</button>
              {isAdmin && (
                <>
                  <button className="btn btn-small" onClick={() => setTool({ kind: 'settings', inbox: i })}><Icon name="sliders" size={14} /> {t('Settings')}</button>
                  <button className="btn btn-small" onClick={() => setTool({ kind: 'webhooks', inbox: i })}><Icon name="webhook" size={14} /> {t('Webhooks')}</button>
                  <button className="btn btn-small" onClick={() => setTool({ kind: 'rules', inbox: i })}><Icon name="tag" size={14} /> {t('Rules')}</button>
                  <button className="btn btn-small" onClick={() => setTool({ kind: 'chaos', inbox: i })}><Icon name="zap" size={14} /> {t('Chaos')}</button>
                </>
              )}
            </div>
          </div>
        ))}
      </div>

      {editing && <InboxModal inbox={editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); refreshInboxes() }} />}
      {tool?.kind === 'webhooks' && <WebhooksModal inbox={tool.inbox} onClose={() => setTool(null)} />}
      {tool?.kind === 'rules' && <RulesModal inbox={tool.inbox} onClose={() => setTool(null)} />}
      {tool?.kind === 'settings' && <InboxSettingsModal inbox={tool.inbox} onClose={() => setTool(null)}
        onSaved={() => { setTool(null); refreshInboxes(); toast(t('Settings saved'), 'success') }} />}
      {tool?.kind === 'chaos' && <ChaosModal inbox={tool.inbox} onClose={() => setTool(null)} onSaved={() => { setTool(null); refreshInboxes() }} />}
      {setupFor && (
        <Modal title={t('Configure "{name}"', { name: setupFor.name })} onClose={() => setSetupFor(null)} wide>
          <SetupSnippets inbox={setupFor} host={host} port={config.smtp_port} pop3Port={config.pop3_port} />
        </Modal>
      )}
    </>
  )
}

const chaosOn = (i) => Object.values(i.chaos || {}).some((r) => r && r.probability > 0)

function InboxModal({ inbox, onClose, onSaved }) {
  const t = useT()
  const isNew = !inbox.id
  const [form, setForm] = useState({
    name: inbox.name || '', slug: inbox.slug || '', description: inbox.description || '',
    smtp_username: inbox.smtp_username || '', smtp_password: '', retention_hours: inbox.retention_hours ?? '',
  })
  const [error, setError] = useState('')
  const set = (k) => (e) => setForm((f) => ({ ...f, [k]: e.target.value }))

  const save = async (e) => {
    e.preventDefault()
    setError('')
    const body = { name: form.name, description: form.description, smtp_username: form.smtp_username || undefined }
    if (form.smtp_password) body.smtp_password = form.smtp_password
    if (form.retention_hours === '') { if (!isNew) body.clear_retention = true } else body.retention_hours = Number(form.retention_hours)
    try {
      if (isNew) await api('/inboxes', { method: 'POST', body: { ...body, slug: form.slug || undefined } })
      else await api(`/inboxes/${inbox.id}`, { method: 'PATCH', body })
      onSaved()
    } catch (err) { setError(err.message) }
  }

  return (
    <Modal title={isNew ? t('New inbox') : t('Edit "{name}"', { name: inbox.name })} onClose={onClose}>
      <form className="form" onSubmit={save}>
        <label>{t('Name')}<input value={form.name} onChange={set('name')} required autoFocus placeholder={t('e.g. Project X - staging')} /></label>
        {isNew && <label>{t('Slug (optional)')}<input value={form.slug} onChange={set('slug')} placeholder={t('generated from the name')} /></label>}
        <label>{t('Description')}<input value={form.description} onChange={set('description')} /></label>
        <label>{t('SMTP username')}<input value={form.smtp_username} onChange={set('smtp_username')} placeholder={t('defaults to the slug')} /></label>
        <label>{t('SMTP password')}<input value={form.smtp_password} onChange={set('smtp_password')} placeholder={isNew ? t('generated automatically') : t('leave empty to keep it')} /></label>
        <label>{t('Retention (hours)')}<input type="number" min="1" value={form.retention_hours} onChange={set('retention_hours')} placeholder={t('empty = global default')} /></label>
        {error && <div className="form-error">{error}</div>}
        <div className="form-actions">
          <button type="button" className="btn" onClick={onClose}>{t('Cancel')}</button>
          <button className="btn btn-primary">{t('Save')}</button>
        </div>
      </form>
    </Modal>
  )
}

/* ---------------- Users ---------------- */

function UsersPage() {
  const { inboxes, user: me } = useApp()
  const t = useT()
  const [users, setUsers] = useState([])
  const [editing, setEditing] = useState(null)
  const toast = useToast()
  const load = () => api('/users').then(setUsers)
  useEffect(() => { load() }, [])

  const remove = async (u) => {
    if (!confirm(t('Delete {email}?', { email: u.email }))) return
    try {
      await api(`/users/${u.id}`, { method: 'DELETE' })
      toast(t('User deleted'), 'success')
      load()
    } catch (e) { toast(e.message, 'error') }
  }
  const inboxName = (id) => inboxes.find((i) => i.id === id)?.name || '?'

  return (
    <>
      <div className="page-head">
        <div>
          <h2>{t('Users')}</h2>
          <p className="muted">{t('Admins see and manage everything. Members only see their assigned inboxes.')}</p>
        </div>
        <button className="btn btn-primary" onClick={() => setEditing({})}><Icon name="plus" /> {t('New user')}</button>
      </div>
      <div className="grid-wrap">
        <div className="grid-scroll">
          <table className="grid static">
            <thead><tr><th>{t('Email')}</th><th>{t('Name')}</th><th>{t('Role')}</th><th>{t('Inboxes')}</th><th>{t('Last login')}</th><th /></tr></thead>
            <tbody>
              {users.map((u) => (
                <tr key={u.id}>
                  <td>{u.email}{u.id === me.id && <span className="tag">{t('you')}</span>}</td>
                  <td>{u.name}</td>
                  <td><span className={`tag ${u.role === 'admin' ? 'tag-accent' : ''}`}>{u.role}</span></td>
                  <td className="small">{u.role === 'admin' ? t('All') : u.inbox_ids.map(inboxName).join(', ') || '—'}</td>
                  <td className="small">{u.last_login_at ? formatDate(u.last_login_at) : t('Never')}</td>
                  <td className="row-actions">
                    <button className="icon-btn" onClick={() => setEditing(u)} title={t('Edit')}><Icon name="edit" /></button>
                    {u.id !== me.id && <button className="icon-btn danger" onClick={() => remove(u)} title={t('Delete')}><Icon name="trash" /></button>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      {editing && <UserModal user={editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); load() }} />}
    </>
  )
}

function UserModal({ user, onClose, onSaved }) {
  const { inboxes } = useApp()
  const t = useT()
  const isNew = !user.id
  const [form, setForm] = useState({
    email: user.email || '', name: user.name || '', password: '', role: user.role || 'member', inbox_ids: user.inbox_ids || [],
  })
  const [error, setError] = useState('')
  const set = (k) => (e) => setForm((f) => ({ ...f, [k]: e.target.value }))
  const toggleInbox = (id) => setForm((f) => ({
    ...f, inbox_ids: f.inbox_ids.includes(id) ? f.inbox_ids.filter((x) => x !== id) : [...f.inbox_ids, id],
  }))

  const save = async (e) => {
    e.preventDefault()
    setError('')
    const body = { name: form.name, role: form.role, inbox_ids: form.inbox_ids }
    if (form.password) body.password = form.password
    try {
      if (isNew) await api('/users', { method: 'POST', body: { ...body, email: form.email } })
      else await api(`/users/${user.id}`, { method: 'PATCH', body })
      onSaved()
    } catch (err) { setError(err.message) }
  }

  return (
    <Modal title={isNew ? t('New user') : t('Edit {email}', { email: user.email })} onClose={onClose}>
      <form className="form" onSubmit={save}>
        {isNew && <label>{t('Email')}<input type="email" value={form.email} onChange={set('email')} required autoFocus /></label>}
        <label>{t('Name')}<input value={form.name} onChange={set('name')} /></label>
        <label>{t('Password')}<input type="password" value={form.password} onChange={set('password')} required={isNew} minLength={8}
          placeholder={isNew ? t('at least 8 characters') : t('leave empty to keep it')} autoComplete="new-password" /></label>
        <label>{t('Role')}
          <select value={form.role} onChange={set('role')}>
            <option value="member">member</option>
            <option value="admin">admin</option>
          </select>
        </label>
        {form.role === 'member' && (
          <fieldset className="checks">
            <legend>{t('Visible inboxes')}</legend>
            {inboxes.map((i) => (
              <label key={i.id} className="check">
                <input type="checkbox" checked={form.inbox_ids.includes(i.id)} onChange={() => toggleInbox(i.id)} /> {i.name}
              </label>
            ))}
          </fieldset>
        )}
        {error && <div className="form-error">{error}</div>}
        <div className="form-actions">
          <button type="button" className="btn" onClick={onClose}>{t('Cancel')}</button>
          <button className="btn btn-primary">{t('Save')}</button>
        </div>
      </form>
    </Modal>
  )
}

/* ---------------- Account ---------------- */

function AccountPage() {
  const { user } = useApp()
  const t = useT()
  const [form, setForm] = useState({ current_password: '', new_password: '', confirm: '' })
  const [msg, setMsg] = useState(null)
  const set = (k) => (e) => setForm((f) => ({ ...f, [k]: e.target.value }))

  const save = async (e) => {
    e.preventDefault()
    if (form.new_password !== form.confirm) return setMsg({ error: t('Passwords do not match') })
    try {
      await api('/auth/password', { method: 'PUT', body: { current_password: form.current_password, new_password: form.new_password } })
      setMsg({ ok: t('Password updated') })
      setForm({ current_password: '', new_password: '', confirm: '' })
    } catch (err) { setMsg({ error: err.message }) }
  }

  return (
    <>
      <div className="page-head"><div><h2>{t('My account')}</h2><p className="muted">{user.email} · {user.role}</p></div></div>
      <form className="form narrow" onSubmit={save}>
        <label>{t('Current password')}<input type="password" value={form.current_password} onChange={set('current_password')} required autoComplete="current-password" /></label>
        <label>{t('New password')}<input type="password" value={form.new_password} onChange={set('new_password')} required minLength={8} autoComplete="new-password" /></label>
        <label>{t('Repeat new password')}<input type="password" value={form.confirm} onChange={set('confirm')} required minLength={8} autoComplete="new-password" /></label>
        {msg?.error && <div className="form-error">{msg.error}</div>}
        {msg?.ok && <div className="form-ok">{msg.ok}</div>}
        <div className="form-actions"><button className="btn btn-primary">{t('Change password')}</button></div>
      </form>
    </>
  )
}

/* ---------------- Preferences ---------------- */

function PreferencesPage() {
  const { theme, setTheme } = useApp()
  const { lang, setLang, t } = useI18n()
  return (
    <>
      <div className="page-head"><div><h2>{t('Preferences')}</h2><p className="muted">{t('Saved in this browser.')}</p></div></div>
      <div className="form narrow">
        <label>{t('Language')}
          <select value={lang} onChange={(e) => setLang(e.target.value)}>
            {LANGUAGES.map((l) => <option key={l.code} value={l.code}>{l.label}</option>)}
          </select>
        </label>
        <div>
          <div className="field-label">{t('Theme')}</div>
          <div className="segmented">
            {Object.entries(THEME_LABELS).map(([k, label]) => (
              <button key={k} type="button" className={theme === k ? 'on' : ''} onClick={() => setTheme(k)}>
                <Icon name={k === 'dark' ? 'moon' : k === 'light' ? 'sun' : 'auto'} size={15} /> {t(label)}
              </button>
            ))}
          </div>
          <p className="muted small">{t('"System" follows your operating system\'s light/dark setting.')}</p>
        </div>
        <NotificationsPref />
      </div>
    </>
  )
}

function NotificationsPref() {
  const t = useT()
  const [on, setOn] = useState(notificationsEnabled())
  const supported = 'Notification' in window
  return (
    <div>
      <div className="field-label">{t('Desktop notifications')}</div>
      <label className="check">
        <input type="checkbox" checked={on} disabled={!supported} onChange={async (e) => setOn(await setNotifications(e.target.checked))} />
        {' '}{t('Notify me about new emails while this tab is in the background')}
      </label>
      {!supported && <p className="muted small">{t('Not supported by this browser.')}</p>}
      {supported && Notification.permission === 'denied' && <p className="muted small">{t('Notifications are blocked for this site in your browser settings.')}</p>}
    </div>
  )
}

function RelayLog() {
  const t = useT()
  const [list, setList] = useState(null)
  useEffect(() => { api('/relay/log').then(setList).catch(() => setList([])) }, [])
  if (!list) return null
  return (
    <>
      <h3>{t('Release audit log')}</h3>
      {list.length === 0 ? <p className="muted">{t('No emails released yet.')}</p> : (
        <div className="grid-wrap"><div className="grid-scroll">
          <table className="grid static">
            <thead><tr><th>{t('Date')}</th><th>{t('By')}</th><th>{t('Recipients')}</th><th>{t('Subject')}</th><th>{t('Result')}</th></tr></thead>
            <tbody>
              {list.map((e) => (
                <tr key={e.id}>
                  <td className="small">{formatDate(e.created_at)}</td>
                  <td className="small">{e.user_email}</td>
                  <td className="small">{e.recipients.join(', ')}</td>
                  <td>{e.subject}</td>
                  <td>{e.ok ? <span className="lbl-ok">OK</span> : <span className="lbl-no" title={e.error}>{t('Failed')}</span>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div></div>
      )}
    </>
  )
}

/* ---------------- Server ---------------- */

function ServerPage() {
  const { config, user } = useApp()
  const t = useT()
  const [stats, setStats] = useState(null)
  useEffect(() => { api('/stats').then(setStats) }, [])
  const authLabel = {
    off: t('Off (everything goes to the default inbox)'),
    optional: t('Optional (anonymous → default inbox)'),
    required: t('Required (anonymous rejected)'),
  }
  return (
    <>
      <div className="page-head"><div><h2>{t('Server')}</h2><p className="muted">{t('Configured through environment variables (see README).')}</p></div></div>
      {stats && (
        <div className="stats">
          <div className="stat"><b>{stats.total}</b><span>{t('emails')}</span></div>
          <div className="stat"><b>{stats.unread}</b><span>{t('unread')}</span></div>
          <div className="stat"><b>{stats.last_24h}</b><span>{t('last 24 h')}</span></div>
          <div className="stat"><b>{(stats.total_bytes / 1024 / 1024).toFixed(1)} MB</b><span>{t('stored')}</span></div>
        </div>
      )}
      <table className="kv">
        <tbody>
          <tr><th>{t('Version')}</th><td>{config.version}</td></tr>
          <tr><th>{t('SMTP port')}</th><td>{config.smtp_port}</td></tr>
          <tr><th>{t('SMTP authentication')}</th><td>{authLabel[config.smtp_auth] || config.smtp_auth}</td></tr>
          <tr><th>{t('UI login')}</th><td>{config.auth_enabled ? t('Required') : t('Disabled (AUTH_ENABLED=false)')}</td></tr>
          <tr><th>{t('Default retention')}</th><td>{t('{h} hours', { h: config.retention_hours })}</td></tr>
          <tr><th>{t('Max email size')}</th><td>{config.max_message_mb} MB</td></tr>
          <tr><th>{t('Default inbox')}</th><td><code>{config.default_inbox}</code></td></tr>
          <tr><th>{t('SMTP TLS')}</th><td>{config.smtp_require_tls ? t('Required (STARTTLS)') : config.smtp_tls ? t('Available (STARTTLS)') : t('Not configured')}</td></tr>
          <tr><th>POP3</th><td>{config.pop3_port ? t('Port {port} (inbox SMTP credentials)', { port: config.pop3_port }) : t('Disabled')}</td></tr>
          <tr><th>{t('Spam engine')}</th><td>{config.spam_engine || t('None')}</td></tr>
          <tr><th>{t('Release / relay')}</th><td>{config.relay_enabled ? t('Enabled') : t('Disabled')}</td></tr>
          <tr><th>{t('Prometheus metrics')}</th><td>{config.metrics_enabled ? t('Enabled') : t('Disabled')}</td></tr>
        </tbody>
      </table>
      {config.relay_enabled && user.role === 'admin' && <RelayLog />}
    </>
  )
}
