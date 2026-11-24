import { useEffect, useMemo, useState } from 'react'
import { api, attachmentUrl, rawUrl } from '../lib/api.js'
import { useApp, useToast } from '../lib/context.jsx'
import { copyText, formatBytes, formatDate } from '../lib/format.js'
import Icon from './Icon.jsx'
import Checks from './Checks.jsx'
import ReleaseModal from './ReleaseModal.jsx'
import { TagEditor } from './Tags.jsx'
import { useT } from '../i18n/index.jsx'

const widths = { desktop: '100%', tablet: '768px', phone: '375px' }

export default function MessageView({ id, onClose, onChanged, onDeleted, tagSuggestions = [] }) {
  const [msg, setMsg] = useState(null)
  const [tab, setTab] = useState('html')
  const [html, setHtml] = useState(null)
  const [raw, setRaw] = useState(null)
  const [device, setDevice] = useState('desktop')
  const [error, setError] = useState('')
  const [releasing, setReleasing] = useState(false)
  const { config } = useApp()
  const toast = useToast()
  const t = useT()

  useEffect(() => {
    let cancelled = false
    setMsg(null)
    setHtml(null)
    setRaw(null)
    setError('')
    api(`/messages/${id}`)
      .then((m) => {
        if (cancelled) return
        setMsg(m)
        setTab((t) => (t === 'html' && !m.has_html ? 'text' : t === 'text' && m.has_html ? 'html' : t))
        onChanged?.()
      })
      .catch((e) => !cancelled && setError(e.message))
    return () => { cancelled = true }
  }, [id]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!msg) return
    if (tab === 'html' && msg.has_html && html === null) api(`/messages/${id}/html`).then((r) => setHtml(r.html))
    if (tab === 'raw' && raw === null) api(`/messages/${id}/raw`).then(setRaw)
  }, [tab, msg, id, html, raw])

  // Links open in a new tab; no scripts run inside the sandboxed iframe.
  const srcDoc = useMemo(
    () => (html === null ? '' : `<base target="_blank"><meta name="referrer" content="no-referrer">${html}`),
    [html],
  )

  const del = async () => {
    await api(`/messages/${id}`, { method: 'DELETE' })
    toast(t('Email deleted'), 'success')
    onDeleted()
  }
  const saveTags = async (tags) => {
    try {
      const res = await api(`/messages/${id}/tags`, { method: 'PUT', body: { tags } })
      setMsg((m) => ({ ...m, tags: res.tags }))
      onChanged?.()
    } catch (e) { toast(e.message, 'error') }
  }
  const markUnread = async () => {
    await api('/messages/read', { method: 'POST', body: { ids: [id], read: false } })
    toast(t('Marked as unread'))
    onChanged()
  }

  const headerEntries = msg ? Object.entries(msg.headers || {}).sort(([a], [b]) => a.localeCompare(b)) : []
  const files = msg ? msg.attachments : []

  return (
    <section className="detail">
      <div className="detail-bar">
        <button className="icon-btn" onClick={onClose} title={t('Close (Esc)')}><Icon name="x" /></button>
        <div className="topbar-spacer" />
        {config.relay_enabled && msg && (
          <button className="btn" onClick={() => setReleasing(true)} title={t('Release to real recipients')}><Icon name="send" /></button>
        )}
        <a className="btn" href={rawUrl(id, true)} title={t('Download .eml')}><Icon name="download" /> .eml</a>
        <button className="btn" onClick={markUnread} title={t('Mark as unread')}><Icon name="mail" /></button>
        <button className="btn btn-danger-outline" onClick={del} title={t('Delete (Del)')}><Icon name="trash" /></button>
      </div>

      {error && <div className="form-error pad">{error}</div>}
      {!msg && !error && <div className="pad muted">{t('Loading…')}</div>}

      {msg && (
        <>
          <div className="detail-head">
            <h3>{msg.subject || t('(no subject)')}</h3>
            <dl className="meta">
              <dt>{t('From')}</dt><dd>{msg.from}</dd>
              <dt>{t('To')}</dt><dd>{msg.to}</dd>
              {msg.cc && (<><dt>CC</dt><dd>{msg.cc}</dd></>)}
              <dt>{t('Date')}</dt><dd>{formatDate(msg.received_at)} · {formatBytes(msg.size)} · <span className="tag">{msg.inbox_name}</span></dd>
              <dt>{t('Tags')}</dt><dd><TagEditor tags={msg.tags || []} onChange={saveTags} suggestions={tagSuggestions} /></dd>
            </dl>
          </div>

          <div className="tabs">
            {msg.has_html && <button className={tab === 'html' ? 'on' : ''} onClick={() => setTab('html')}>HTML</button>}
            <button className={tab === 'text' ? 'on' : ''} onClick={() => setTab('text')}>{t('Text')}</button>
            <button className={tab === 'headers' ? 'on' : ''} onClick={() => setTab('headers')}>{t('Headers')}</button>
            <button className={tab === 'raw' ? 'on' : ''} onClick={() => setTab('raw')}>Raw</button>
            <button className={tab === 'envelope' ? 'on' : ''} onClick={() => setTab('envelope')}>SMTP</button>
            {files.length > 0 && (
              <button className={tab === 'attachments' ? 'on' : ''} onClick={() => setTab('attachments')}>
                {t('Attachments')} ({files.length})
              </button>
            )}
            <button className={tab === 'checks' ? 'on' : ''} onClick={() => setTab('checks')}>
              <Icon name="shield" size={13} /> {t('Analysis')}
            </button>
            {tab === 'html' && (
              <div className="device-switch">
                {Object.keys(widths).map((d) => (
                  <button key={d} className={device === d ? 'on' : ''} onClick={() => setDevice(d)} title={t(d)}>
                    <Icon name={d} size={15} />
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className="detail-body">
            {tab === 'html' && (
              <div className="html-stage">
                {html === null ? <div className="pad muted">{t('Loading…')}</div> : (
                  <iframe title={t('HTML preview')} sandbox="allow-popups allow-popups-to-escape-sandbox"
                    srcDoc={srcDoc} style={{ width: widths[device] }} />
                )}
              </div>
            )}
            {tab === 'text' && <pre className="text-body">{msg.text_body || t('(no text part)')}</pre>}
            {tab === 'headers' && (
              <table className="kv">
                <tbody>
                  {headerEntries.map(([k, vs]) => vs.map((v, i) => (
                    <tr key={k + i}><th>{k}</th><td>{v}</td></tr>
                  )))}
                </tbody>
              </table>
            )}
            {tab === 'raw' && (
              <div>
                <button className="btn btn-small" onClick={() => copyText(raw || '').then(() => toast(t('Copied')))}>
                  <Icon name="copy" size={14} /> {t('Copy')}
                </button>
                <pre className="raw-body">{raw ?? t('Loading…')}</pre>
              </div>
            )}
            {tab === 'envelope' && (
              <table className="kv">
                <tbody>
                  <tr><th>MAIL FROM</th><td>{msg.envelope_from || '—'}</td></tr>
                  <tr><th>RCPT TO</th><td>{msg.envelope_to.join(', ') || '—'}</td></tr>
                  <tr><th>Message-ID</th><td>{msg.message_id || '—'}</td></tr>
                  <tr><th>{t('Client IP')}</th><td>{msg.client_ip || '—'}</td></tr>
                  <tr><th>{t('SMTP user')}</th><td>{msg.smtp_user || t('(anonymous)')}</td></tr>
                  <tr><th>ID</th><td><code>{msg.id}</code></td></tr>
                </tbody>
              </table>
            )}
            {tab === 'checks' && <Checks msg={msg} />}
            {tab === 'attachments' && (
              <ul className="attachments">
                {files.map((a) => (
                  <li key={a.id}>
                    <Icon name="clip" />
                    <div>
                      <div>{a.filename}</div>
                      <div className="muted small">{a.content_type} · {formatBytes(a.size)}{a.inline ? ` · ${t('inline')}` : ''}</div>
                    </div>
                    <a className="btn btn-small" href={attachmentUrl(id, a.id)}><Icon name="download" size={14} /> {t('Download')}</a>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </>
      )}
      {releasing && msg && <ReleaseModal msg={msg} onClose={() => setReleasing(false)} />}
    </section>
  )
}
