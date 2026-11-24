import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../lib/api.js'
import { useApp, useToast } from '../lib/context.jsx'
import { copyText, formatDate } from '../lib/format.js'
import Icon from './Icon.jsx'
import Modal from './Modal.jsx'
import { TagChip } from './Tags.jsx'
import { useT } from '../i18n/index.jsx'

/* ---------------- Webhooks ---------------- */

function Deliveries({ hook }) {
  const t = useT()
  const [list, setList] = useState(null)
  useEffect(() => { api(`/webhooks/${hook.id}/deliveries`).then(setList) }, [hook.id, hook.last_delivery_at])
  if (!list) return <p className="muted small">{t('Loading…')}</p>
  if (!list.length) return <p className="muted small">{t('No deliveries yet.')}</p>
  return (
    <table className="kv compact deliveries">
      <tbody>
        {list.map((d) => (
          <tr key={d.id}>
            <th><span className={`status-pill ${d.status_code >= 200 && d.status_code < 300 ? 'ok' : 'bad'}`}>{d.status_code || 'ERR'}</span></th>
            <td className="small">
              {formatDate(d.created_at)} · <code>{d.event}</code> · {t('attempt {n}', { n: d.attempt })} · {d.duration_ms} ms
              {d.error && <div className="lbl-no">{d.error}</div>}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

export function WebhooksModal({ inbox, onClose }) {
  const t = useT()
  const toast = useToast()
  const [hooks, setHooks] = useState([])
  const [url, setUrl] = useState('')
  const [error, setError] = useState('')
  const [open, setOpen] = useState(null)
  const [reveal, setReveal] = useState({})
  const load = () => api(`/inboxes/${inbox.id}/webhooks`).then(setHooks)
  useEffect(() => { load() }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const create = async (e) => {
    e.preventDefault()
    setError('')
    try {
      const h = await api(`/inboxes/${inbox.id}/webhooks`, { method: 'POST', body: { url } })
      setUrl('')
      setOpen(h.id)
      setReveal((r) => ({ ...r, [h.id]: true }))
      load()
    } catch (err) { setError(err.message) }
  }
  const act = async (h, fn) => {
    try { await fn(); load() } catch (err) { toast(err.message, 'error') }
  }
  const test = (h) => act(h, async () => {
    const r = await api(`/webhooks/${h.id}/test`, { method: 'POST' })
    if (r.error) toast(t('Ping failed: {error}', { error: r.error }), 'error')
    else toast(t('Ping delivered: HTTP {code} in {ms} ms', { code: r.status_code, ms: r.duration_ms }), 'success')
  })

  return (
    <Modal title={t('Webhooks of "{name}"', { name: inbox.name })} onClose={onClose} wide>
      <p className="muted small">
        {t('A signed POST is sent for every email received in this inbox. Failed deliveries are retried with backoff (10s, 30s, 2m, 10m, 30m).')}{' '}
        {t('Verify the X-PostDream-Signature header: t=<unix>,v1=HMAC-SHA256(secret, "<t>.<body>").')}
      </p>
      <form className="inline-form" onSubmit={create}>
        <input type="url" required placeholder="https://ci.example.com/hooks/mail" value={url} onChange={(e) => setUrl(e.target.value)} />
        <button className="btn btn-primary"><Icon name="plus" /> {t('Add webhook')}</button>
      </form>
      {error && <div className="form-error">{error}</div>}
      <p className="muted small">{t('Internal addresses (localhost, 10.x, 192.168.x…) are blocked unless listed in the inbox settings under "Allowed internal hosts".')}</p>
      <div className="hook-list">
        {hooks.map((h) => (
          <div key={h.id} className={`hook ${h.active ? '' : 'inactive'}`}>
            <div className="hook-row">
              <span className={`status-pill ${h.last_status == null ? '' : h.last_status >= 200 && h.last_status < 300 ? 'ok' : 'bad'}`}>
                {h.last_status ?? '—'}
              </span>
              <code className="hook-url">{h.url}</code>
              <div className="row-actions">
                <button className="icon-btn" title={t('Send test ping')} onClick={() => test(h)}><Icon name="play" /></button>
                <button className="icon-btn" title={h.active ? t('Disable') : t('Enable')}
                  onClick={() => act(h, () => api(`/webhooks/${h.id}`, { method: 'PATCH', body: { active: !h.active } }))}>
                  <Icon name={h.active ? 'check' : 'x'} />
                </button>
                <button className="icon-btn" title={t('Deliveries')} onClick={() => setOpen(open === h.id ? null : h.id)}><Icon name="clock" /></button>
                <button className="icon-btn danger" title={t('Delete')} onClick={() => confirm(t('Delete this webhook?')) &&
                  act(h, () => api(`/webhooks/${h.id}`, { method: 'DELETE' }))}><Icon name="trash" /></button>
              </div>
            </div>
            <div className="small hook-secret">
              {t('Signing secret')}: <code>{reveal[h.id] ? h.secret : '•'.repeat(16)}</code>
              <button className="icon-btn" onClick={() => setReveal((r) => ({ ...r, [h.id]: !r[h.id] }))}><Icon name="eye" size={13} /></button>
              <button className="icon-btn" onClick={() => copyText(h.secret).then(() => toast(t('Copied')))}><Icon name="copy" size={13} /></button>
              <button className="icon-btn" title={t('Regenerate secret')} onClick={() => confirm(t('Regenerate the signing secret? Receivers must be updated.')) &&
                act(h, () => api(`/webhooks/${h.id}/rotate`, { method: 'POST' }))}><Icon name="key" size={13} /></button>
            </div>
            {open === h.id && <Deliveries hook={h} />}
          </div>
        ))}
        {!hooks.length && <p className="muted">{t('No webhooks yet.')}</p>}
      </div>
    </Modal>
  )
}

/* ---------------- Tag rules ---------------- */

export function RulesModal({ inbox, onClose }) {
  const t = useT()
  const toast = useToast()
  const navigate = useNavigate()
  const [rules, setRules] = useState([])
  const [form, setForm] = useState({ query: '', tag: '', global: false })
  const [error, setError] = useState('')
  const load = () => api('/rules', { query: { inbox: inbox.id } }).then(setRules)
  useEffect(() => { load() }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const create = async (e) => {
    e.preventDefault()
    setError('')
    try {
      await api('/rules', { method: 'POST', body: { query: form.query, tag: form.tag, inbox_id: form.global ? null : inbox.id } })
      setForm({ query: '', tag: '', global: false })
      load()
    } catch (err) { setError(err.message) }
  }
  const remove = async (r) => {
    try { await api(`/rules/${r.id}`, { method: 'DELETE' }); load() } catch (err) { toast(err.message, 'error') }
  }
  const preview = (q) => { onClose(); navigate(`/inbox/${inbox.id}?q=${encodeURIComponent(q)}`) }

  return (
    <Modal title={t('Auto-tag rules for "{name}"', { name: inbox.name })} onClose={onClose} wide>
      <p className="muted small">
        {t('New emails matching a rule get its tag. Rules use the search syntax, e.g. subject:"reset password" or from:billing has:attachment.')}{' '}
        {t('Tags are also added automatically from plus-addresses (qa+signup@… → signup) and from an X-Tags header.')}
      </p>
      <form className="rule-form" onSubmit={create}>
        <input required placeholder={t('Search query, e.g. subject:welcome')} value={form.query} onChange={(e) => setForm({ ...form, query: e.target.value })} />
        <input required placeholder={t('Tag')} maxLength={100} value={form.tag} onChange={(e) => setForm({ ...form, tag: e.target.value })} />
        <label className="check small"><input type="checkbox" checked={form.global} onChange={(e) => setForm({ ...form, global: e.target.checked })} /> {t('All inboxes')}</label>
        <button className="btn btn-primary"><Icon name="plus" /> {t('Add rule')}</button>
      </form>
      {error && <div className="form-error">{error}</div>}
      <table className="kv">
        <tbody>
          {rules.map((r) => (
            <tr key={r.id}>
              <th><TagChip tag={r.tag} /></th>
              <td>
                <code>{r.query}</code>{!r.inbox_id && <span className="tag">{t('all inboxes')}</span>}
              </td>
              <td className="row-actions">
                <button className="icon-btn" title={t('Show matching emails')} onClick={() => preview(r.query)}><Icon name="search" /></button>
                <button className="icon-btn danger" title={t('Delete')} onClick={() => remove(r)}><Icon name="trash" /></button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {!rules.length && <p className="muted">{t('No rules yet.')}</p>}
    </Modal>
  )
}

/* ---------------- Advanced settings ---------------- */

const lines = (arr) => (arr || []).join('\n')
const toList = (s) => s.split(/[\n,]+/).map((x) => x.trim()).filter(Boolean)

export function InboxSettingsModal({ inbox, onClose, onSaved }) {
  const t = useT()
  const { config } = useApp()
  const [form, setForm] = useState({
    spam_check: inbox.spam_check, allowed_hosts: lines(inbox.allowed_hosts), link_domains: lines(inbox.link_domains),
    allowed_recipients: inbox.allowed_recipients || '', relay_recipients: inbox.relay_recipients || '',
  })
  const [error, setError] = useState('')
  const set = (k) => (e) => setForm((f) => ({ ...f, [k]: e.target.type === 'checkbox' ? e.target.checked : e.target.value }))
  const save = async (e) => {
    e.preventDefault()
    setError('')
    try {
      await api(`/inboxes/${inbox.id}/settings`, {
        method: 'PATCH',
        body: { ...form, allowed_hosts: toList(form.allowed_hosts), link_domains: toList(form.link_domains) },
      })
      onSaved()
    } catch (err) { setError(err.message) }
  }
  return (
    <Modal title={t('Settings of "{name}"', { name: inbox.name })} onClose={onClose} wide>
      <form className="form" onSubmit={save}>
        <label className="check">
          <input type="checkbox" checked={form.spam_check} onChange={set('spam_check')} disabled={!config.spam_engine} />
          {t('Spam check: score every received email')}
          {!config.spam_engine && <span className="muted small"> ({t('no spam engine configured')})</span>}
        </label>
        <label>{t('Allowed recipients (regex)')}
          <input value={form.allowed_recipients} onChange={set('allowed_recipients')} placeholder="@(acme\.test|example\.org)$" />
          <span className="muted small">{t('SMTP rejects RCPT TO addresses that do not match. Empty = accept all.')}</span>
        </label>
        <div className="two-cols">
          <label>{t('Expected link domains')}
            <textarea rows={4} value={form.link_domains} onChange={set('link_domains')} placeholder={'staging.acme.test\ncdn.acme.test'} />
            <span className="muted small">{t('Link check warns about links to any other domain (e.g. production URLs in QA mail).')}</span>
          </label>
          <label>{t('Allowed internal hosts')}
            <textarea rows={4} value={form.allowed_hosts} onChange={set('allowed_hosts')} placeholder={'ci.internal\n*.corp.local\n10.1.0.0/16'} />
            <span className="muted small">{t('Private hosts/CIDRs that webhooks and the link check may reach for this inbox.')}</span>
          </label>
        </div>
        <label>{t('Allowed release recipients (regex)')}
          <input value={form.relay_recipients} onChange={set('relay_recipients')} placeholder="@acme\.test$" />
          <span className="muted small">
            {config.relay_enabled
              ? t('Emails can only be released to addresses matching this pattern. Empty = releasing disabled for this inbox.')
              : t('Releasing is disabled on this server (RELAY_SMTP_URL is not set).')}
          </span>
        </label>
        {error && <div className="form-error">{error}</div>}
        <div className="form-actions">
          <button type="button" className="btn" onClick={onClose}>{t('Cancel')}</button>
          <button className="btn btn-primary">{t('Save')}</button>
        </div>
      </form>
    </Modal>
  )
}

/* ---------------- Chaos ---------------- */

const STAGES = [
  ['auth', 'AUTH', 'Login with the inbox credentials'],
  ['mail', 'MAIL FROM', 'Start of every transaction'],
  ['rcpt', 'RCPT TO', 'Every recipient'],
  ['data', 'DATA', 'Message body (after upload)'],
]

export function ChaosModal({ inbox, onClose, onSaved }) {
  const t = useT()
  const toast = useToast()
  const init = Object.fromEntries(STAGES.map(([k]) => {
    const r = inbox.chaos?.[k]
    return [k, { on: !!r, code: r?.code ?? 451, probability: r?.probability ?? 100, message: r?.message ?? '' }]
  }))
  const [form, setForm] = useState(init)
  const [error, setError] = useState('')
  const set = (stage, k) => (e) => setForm((f) => ({
    ...f, [stage]: { ...f[stage], [k]: e.target.type === 'checkbox' ? e.target.checked : e.target.value },
  }))
  const save = async (e) => {
    e.preventDefault()
    setError('')
    const body = {}
    for (const [k] of STAGES) {
      const r = form[k]
      if (r.on) body[k] = { code: Number(r.code), probability: Number(r.probability), message: r.message || undefined }
    }
    try {
      await api(`/inboxes/${inbox.id}/chaos`, { method: 'PUT', body })
      toast(Object.keys(body).length ? t('Chaos rules enabled') : t('Chaos disabled'), 'success')
      onSaved()
    } catch (err) { setError(err.message) }
  }
  const example = `curl -X PUT ${window.location.origin}/api/inboxes/${inbox.id}/chaos \\
  -H "Authorization: Bearer $POSTDREAM_TOKEN" -H "Content-Type: application/json" \\
  -d '{"rcpt": {"code": 451, "probability": 50}}'
# ...run your resilience test, then disable:
curl -X PUT ${window.location.origin}/api/inboxes/${inbox.id}/chaos -H "Authorization: Bearer $POSTDREAM_TOKEN" -d '{}'`
  return (
    <Modal title={t('Chaos testing for "{name}"', { name: inbox.name })} onClose={onClose} wide>
      <form className="form" onSubmit={save}>
        <p className="muted small">{t('Make SMTP fail on purpose to test how your app handles errors. Only this inbox is affected.')}</p>
        <table className="kv chaos-table">
          <thead><tr><th /><th>{t('Stage')}</th><th>{t('Reply code')}</th><th>{t('Probability %')}</th><th>{t('Message (optional)')}</th></tr></thead>
          <tbody>
            {STAGES.map(([k, label, desc]) => (
              <tr key={k}>
                <td><input type="checkbox" checked={form[k].on} onChange={set(k, 'on')} /></td>
                <td><b>{label}</b><div className="muted small">{t(desc)}</div></td>
                <td><input type="number" min="400" max="599" value={form[k].code} onChange={set(k, 'code')} disabled={!form[k].on} /></td>
                <td><input type="number" min="0" max="100" value={form[k].probability} onChange={set(k, 'probability')} disabled={!form[k].on} /></td>
                <td><input value={form[k].message} maxLength={200} onChange={set(k, 'message')} disabled={!form[k].on} placeholder="Mailbox busy" /></td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="muted small">{t('4xx codes are temporary (clients should retry), 5xx are permanent.')} {t('Tests can toggle this with the inbox API token:')}</p>
        <div className="snippet-box"><pre>{example}</pre></div>
        {error && <div className="form-error">{error}</div>}
        <div className="form-actions">
          <button type="button" className="btn" onClick={onClose}>{t('Cancel')}</button>
          <button className="btn btn-primary">{t('Save')}</button>
        </div>
      </form>
    </Modal>
  )
}
