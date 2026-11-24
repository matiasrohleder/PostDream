import { useEffect, useMemo, useRef, useState } from 'react'
import Icon from './Icon.jsx'
import { useT } from '../i18n/index.jsx'

export const emptyFilters = { q: '', from: '', to: '', subject: '', since: '', until: '', has_attachments: '', unread: '' }

const presets = [
  { label: 'Last hour', hours: 1 },
  { label: 'Today', hours: 24 },
  { label: '3 days', hours: 72 },
]

// Search fields with a short description (English source strings, translated at render).
const FIELDS = [
  ['from:', 'Sender (name or address)'],
  ['to:', 'Recipient in To or the envelope'],
  ['cc:', 'Recipient in Cc'],
  ['bcc:', 'Envelope recipient not in To/Cc'],
  ['subject:', 'Subject contains'],
  ['tag:', 'Has this tag'],
  ['is:', 'read, unread, tagged or spam'],
  ['has:', 'attachment, inline or html'],
  ['larger:', 'Bigger than a size, e.g. 1M'],
  ['smaller:', 'Smaller than a size, e.g. 500K'],
  ['after:', 'Received after a date (2026/11/01) or age (2h, 3d)'],
  ['before:', 'Received before a date or age'],
  ['addressed:', 'Any sender or recipient field'],
  ['reply-to:', 'Reply-To contains'],
  ['message-id:', 'Message-ID'],
  ['header:', 'Header exists, or Name=value'],
  ['inbox:', 'Inbox slug or name'],
  ['smtp-user:', 'Authenticated SMTP user'],
  ['ip:', 'Client IP (prefix, e.g. 10.0.)'],
  ['spam:', 'Spam score, e.g. >5'],
]
const VALUES = {
  'is:': ['read', 'unread', 'tagged', 'spam'],
  'has:': ['attachment', 'inline', 'html'],
  'after:': ['1h', '24h', '7d'],
  'before:': ['1h', '24h', '7d'],
  'larger:': ['100K', '1M', '5M'],
  'smaller:': ['10K', '100K'],
  'spam:': ['>5', '>10', '<1'],
}
const HISTORY_KEY = 'pd-search-history'

function loadHistory() {
  try {
    return JSON.parse(localStorage.getItem(HISTORY_KEY) || '[]')
  } catch {
    return []
  }
}
export function rememberSearch(q) {
  if (!q?.trim()) return
  try {
    const h = [q.trim(), ...loadHistory().filter((x) => x !== q.trim())].slice(0, 12)
    localStorage.setItem(HISTORY_KEY, JSON.stringify(h))
  } catch { /* storage unavailable */ }
}

function toLocalInput(d) {
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** The token being typed: text after the last space that is not inside quotes. */
function currentToken(text) {
  let inQuote = false
  let start = 0
  for (let i = 0; i < text.length; i++) {
    if (text[i] === '"') inQuote = !inQuote
    else if (text[i] === ' ' && !inQuote) start = i + 1
  }
  return { start, token: text.slice(start) }
}

function SearchBox({ value, onChange, tags, error }) {
  const t = useT()
  const [focus, setFocus] = useState(false)
  const [active, setActive] = useState(0)
  const [help, setHelp] = useState(false)
  const input = useRef(null)

  const suggestions = useMemo(() => {
    if (!focus) return []
    if (!value.trim()) return loadHistory().map((h) => ({ value: h, label: h, kind: 'history', replaceAll: true }))
    const { token } = currentToken(value)
    const neg = /^[-!]/.test(token) ? token[0] : ''
    const tok = token.slice(neg.length).toLowerCase()
    if (!tok) return []
    const colon = tok.indexOf(':')
    if (colon === -1) {
      return FIELDS.filter(([f]) => f.startsWith(tok) && f !== tok)
        .map(([f, d]) => ({ value: neg + f, label: f, desc: t(d), kind: 'field', open: true }))
    }
    const field = tok.slice(0, colon + 1)
    const partial = tok.slice(colon + 1)
    const values = field === 'tag:' ? tags.map((x) => x.tag) : VALUES[field] || []
    return values.filter((v) => v.toLowerCase().startsWith(partial) && v.toLowerCase() !== partial)
      .slice(0, 10)
      .map((v) => ({ value: neg + field + (/\s/.test(v) ? `"${v}"` : v), label: v, kind: 'value' }))
  }, [focus, value, tags, t])

  useEffect(() => setActive(0), [suggestions.length])

  const apply = (s) => {
    if (s.replaceAll) {
      onChange(s.value)
    } else {
      const { start } = currentToken(value)
      onChange(value.slice(0, start) + s.value + (s.open ? '' : ' '))
    }
    input.current?.focus()
  }

  const onKey = (e) => {
    if (!suggestions.length) {
      if (e.key === 'Enter') rememberSearch(value)
      return
    }
    if (e.key === 'ArrowDown') { e.preventDefault(); setActive((a) => (a + 1) % suggestions.length) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setActive((a) => (a - 1 + suggestions.length) % suggestions.length) }
    else if (e.key === 'Tab' || (e.key === 'Enter' && suggestions[active])) { e.preventDefault(); apply(suggestions[active]) }
    else if (e.key === 'Escape') { setFocus(false) }
  }

  return (
    <div className="search-wrap">
      <div className={`search-box ${error ? 'has-error' : ''}`}>
        <Icon name="search" />
        <input ref={input} value={value} spellCheck={false} autoComplete="off"
          placeholder={t('Search… e.g. from:shop subject:"order" -is:read')}
          onChange={(e) => onChange(e.target.value)} onKeyDown={onKey}
          onFocus={() => setFocus(true)} onBlur={() => { setTimeout(() => setFocus(false), 150); rememberSearch(value) }} />
        {value && <button className="icon-btn" onClick={() => onChange('')} title={t('Clear')}><Icon name="x" size={14} /></button>}
        <button className={`icon-btn ${help ? 'on' : ''}`} onClick={() => setHelp((h) => !h)} title={t('Search syntax')}><Icon name="help" size={16} /></button>
      </div>
      {error && <div className="search-error">{error}</div>}
      {suggestions.length > 0 && (
        <ul className="suggest" role="listbox">
          {suggestions.map((s, i) => (
            <li key={s.kind + s.value} className={i === active ? 'on' : ''} onMouseDown={(e) => { e.preventDefault(); apply(s) }}
              onMouseEnter={() => setActive(i)}>
              <Icon name={s.kind === 'history' ? 'clock' : s.kind === 'field' ? 'filter' : 'chevronRight'} size={13} />
              <code>{s.label}</code>
              {s.desc && <span className="muted small">{s.desc}</span>}
            </li>
          ))}
        </ul>
      )}
      {help && (
        <div className="search-help">
          <div className="search-help-head">
            <b>{t('Search syntax')}</b>
            <button className="icon-btn" onClick={() => setHelp(false)}><Icon name="x" size={14} /></button>
          </div>
          <p className="muted small">{t('Words match subject, addresses and body. Combine filters with spaces; prefix with - or ! to negate; use quotes for phrases.')}</p>
          <div className="help-grid">
            {FIELDS.map(([f, d]) => (
              <button key={f} className="help-row" onClick={() => { onChange((value ? value.trimEnd() + ' ' : '') + f); input.current?.focus() }}>
                <code>{f}</code><span>{t(d)}</span>
              </button>
            ))}
          </div>
          <p className="muted small">{t('Examples')}: <code>from:shop "order #" -is:read</code> · <code>tag:signup after:2h</code> · <code>has:attachment larger:1M</code></p>
        </div>
      )}
    </div>
  )
}

/** Controlled filter bar. The parent debounces before querying (see Mailbox). */
export default function Filters({ value: draft, onChange, tags = [], error }) {
  const [open, setOpen] = useState(false)
  const t = useT()
  const setDraft = (fn) => onChange(typeof fn === 'function' ? fn(draft) : fn)
  const set = (k) => (e) => setDraft((d) => ({ ...d, [k]: e.target.value }))
  const active = Object.entries(draft).filter(([k, v]) => v && k !== 'q').length

  return (
    <div className="filters">
      <div className="filters-row">
        <SearchBox value={draft.q} onChange={(q) => setDraft((d) => ({ ...d, q }))} tags={tags} error={error} />
        <select value={draft.unread} onChange={set('unread')} title={t('Status')}>
          <option value="">{t('All')}</option>
          <option value="true">{t('Unread')}</option>
          <option value="false">{t('Read')}</option>
        </select>
        <button className={`btn ${open || active ? 'btn-active' : ''}`} onClick={() => setOpen((o) => !o)}>
          <Icon name="filter" /> {t('Filters')}{active ? ` (${active})` : ''}
        </button>
        {(active > 0 || draft.q) && (
          <button className="btn btn-ghost" onClick={() => setDraft(emptyFilters)}>{t('Clear')}</button>
        )}
      </div>
      {open && (
        <div className="filters-advanced">
          <label>{t('From')}<input placeholder={t('sender@…')} value={draft.from} onChange={set('from')} /></label>
          <label>{t('To / CC')}<input placeholder={t('recipient@…')} value={draft.to} onChange={set('to')} /></label>
          <label>{t('Subject')}<input placeholder={t('contains…')} value={draft.subject} onChange={set('subject')} /></label>
          <label>{t('Since')}<input type="datetime-local" value={draft.since} onChange={set('since')} /></label>
          <label>{t('Until')}<input type="datetime-local" value={draft.until} onChange={set('until')} /></label>
          <label>{t('Attachments')}
            <select value={draft.has_attachments} onChange={set('has_attachments')}>
              <option value="">{t('Any')}</option>
              <option value="true">{t('With attachments')}</option>
              <option value="false">{t('Without attachments')}</option>
            </select>
          </label>
          <div className="presets">
            {presets.map((p) => (
              <button key={p.label} className="chip" onClick={() =>
                setDraft((d) => ({ ...d, since: toLocalInput(new Date(Date.now() - p.hours * 3600e3)), until: '' }))}>
                {t(p.label)}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
