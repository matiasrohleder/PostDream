import { useEffect, useState } from 'react'
import { api } from '../lib/api.js'
import { useApp } from '../lib/context.jsx'
import { formatBytes, formatDate } from '../lib/format.js'
import Icon from './Icon.jsx'
import { useT } from '../i18n/index.jsx'

function scoreClass(score) {
  if (score >= 90) return 'good'
  if (score >= 70) return 'fair'
  return 'poor'
}

function Section({ title, icon, right, children }) {
  return (
    <section className="check-section">
      <div className="check-head">
        <h4><Icon name={icon} size={16} /> {title}</h4>
        {right}
      </div>
      {children}
    </section>
  )
}

function useLoad(path, enabled = true) {
  const [state, setState] = useState({ data: null, error: '', loading: false })
  const load = (p = path) => {
    setState((s) => ({ ...s, loading: true, error: '' }))
    return api(p)
      .then((data) => setState({ data, error: '', loading: false }))
      .catch((e) => setState({ data: null, error: e.message, loading: false }))
  }
  useEffect(() => {
    if (enabled) load()
  }, [path, enabled]) // eslint-disable-line react-hooks/exhaustive-deps
  return [state, load]
}

function HTMLCheck({ id }) {
  const t = useT()
  const [{ data, error, loading }] = useLoad(`/messages/${id}/html-check`)
  const [showAll, setShowAll] = useState(false)
  if (loading || (!data && !error)) return <p className="muted">{t('Loading…')}</p>
  if (error) return <div className="form-error">{error}</div>
  const limited = data.features.filter((f) => f.supported < 100)
  const list = showAll ? data.features : limited
  return (
    <>
      <div className="score-row">
        <div className={`score-ring ${scoreClass(data.score)}`}><b>{Math.round(data.score)}%</b><span>{t('support')}</span></div>
        <div className="client-bars">
          {data.clients.map((c) => (
            <div key={c.family} className="client-bar" title={Object.values(c.platforms).join(', ')}>
              <span className="client-name">{c.name}</span>
              <span className="bar"><span className={scoreClass(c.score)} style={{ width: `${c.score}%` }} /></span>
              <span className="small">{Math.round(c.score)}%</span>
            </div>
          ))}
        </div>
      </div>
      {data.warnings.length > 0 && (
        <ul className="issue-list">
          {data.warnings.map((w) => (
            <li key={w.id} className={`lvl-${w.level}`}>
              <b>{t(w.message)}</b>{w.detail && <span className="muted small"> — {t(w.detail)}</span>}
            </li>
          ))}
        </ul>
      )}
      <div className="feature-head">
        <span className="small muted">
          {t('{n} feature(s) detected, {m} with limited support', { n: data.features.length, m: limited.length })} · {formatBytes(data.html_bytes)}
        </span>
        {data.features.length > limited.length && (
          <button className="btn btn-small btn-ghost" onClick={() => setShowAll((s) => !s)}>{showAll ? t('Only problems') : t('Show all')}</button>
        )}
      </div>
      <table className="kv feature-table">
        <tbody>
          {list.map((f) => (
            <tr key={f.slug}>
              <th>
                <a href={f.url} target="_blank" rel="noreferrer noopener">{f.title}</a>
                <div className="support-bar" title={`${f.supported}% / ${f.partial}%`}>
                  <span className="y" style={{ width: `${f.supported}%` }} /><span className="a" style={{ width: `${f.partial}%` }} />
                </div>
              </th>
              <td className="small">
                {f.unsupported.length > 0 && <div><span className="lbl-no">{t('Not supported')}:</span> {f.unsupported.join(', ')}</div>}
                {f.partial_in.length > 0 && <div><span className="lbl-partial">{t('Partial')}:</span> {f.partial_in.join(', ')}</div>}
                {f.unsupported.length === 0 && f.partial_in.length === 0 && <span className="lbl-ok">{t('Supported everywhere tested')}</span>}
                {f.notes?.map((n) => <div key={n} className="muted">{n}</div>)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className="muted small">{t('Data: caniemail.com ({date}).', { date: data.data_updated.slice(0, 10) })}</p>
    </>
  )
}

const LINK_WARN = {
  localhost: 'points to localhost',
  insecure: 'plain HTTP',
  'unexpected-domain': 'unexpected domain',
  blocked: 'internal address blocked',
}

function LinkCheck({ id }) {
  const t = useT()
  const [run, setRun] = useState(false)
  const [{ data, error, loading }, load] = useLoad(`/messages/${id}/link-check`, run)
  useEffect(() => setRun(false), [id])
  return (
    <Section title={t('Links & images')} icon="webhook" right={
      <button className="btn btn-small" disabled={loading} onClick={() => (run ? load() : setRun(true))}>
        <Icon name="play" size={13} /> {loading ? t('Checking…') : data ? t('Check again') : t('Check links')}
      </button>
    }>
      {!run && <p className="muted small">{t('Requests every link and image of this email (internal addresses are blocked unless allowed in the inbox settings).')}</p>}
      {error && <div className="form-error">{error}</div>}
      {data && (
        <>
          <p className="small">
            <b>{data.total}</b> {t('checked')} · <span className={data.broken ? 'lbl-no' : 'lbl-ok'}>{t('{n} broken', { n: data.broken })}</span>
            {' · '}<span className={data.warnings ? 'lbl-partial' : ''}>{t('{n} with warnings', { n: data.warnings })}</span>
            {data.truncated && ` · ${t('only the first 200 were checked')}`}
          </p>
          <table className="kv link-table">
            <tbody>
              {data.links.map((l) => (
                <tr key={l.url}>
                  <th><span className={`status-pill ${l.ok ? 'ok' : 'bad'}`}>{l.status || '—'}</span></th>
                  <td>
                    <div className="link-url">{l.kind === 'image' && <Icon name="eye" size={12} />} {l.url}</div>
                    {l.error && <div className="small lbl-no">{l.error}</div>}
                    {l.warnings.map((w) => <span key={w} className="warn-chip">{t(LINK_WARN[w] || w)}</span>)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </Section>
  )
}

const STATUS_ICON = { pass: 'check', warn: 'help', fail: 'x', info: 'help' }

function Deliverability({ id }) {
  const t = useT()
  const [{ data, error, loading }] = useLoad(`/messages/${id}/deliverability`)
  return (
    <Section title={t('Deliverability')} icon="shield" right={data && (
      <span className="small">
        <span className="lbl-ok">{data.pass} ✓</span> · <span className="lbl-partial">{data.warn} !</span> · <span className="lbl-no">{data.fail} ✕</span>
      </span>
    )}>
      {loading && <p className="muted">{t('Loading…')}</p>}
      {error && <div className="form-error">{error}</div>}
      {data && (
        <ul className="check-list">
          {data.checks.map((c) => (
            <li key={c.id} className={`st-${c.status}`}>
              <span className="st-icon"><Icon name={STATUS_ICON[c.status]} size={13} /></span>
              <div>
                <div>{t(c.title)}</div>
                {c.detail && <div className="muted small break">{t(c.detail)}</div>}
              </div>
            </li>
          ))}
        </ul>
      )}
    </Section>
  )
}

function Spam({ id, report }) {
  const t = useT()
  const { config } = useApp()
  const [data, setData] = useState(report || null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  useEffect(() => { setData(report || null); setError('') }, [id, report])
  const run = async () => {
    setLoading(true)
    setError('')
    try {
      setData(await api(`/messages/${id}/spam`, { query: { refresh: 1 } }))
    } catch (e) { setError(e.message) } finally { setLoading(false) }
  }
  if (!config.spam_engine) {
    return (
      <Section title={t('Spam score')} icon="zap">
        <p className="muted small">{t('No spam engine configured. Set RSPAMD_URL or SPAMASSASSIN_ADDR to score messages.')}</p>
      </Section>
    )
  }
  return (
    <Section title={t('Spam score')} icon="zap" right={
      <button className="btn btn-small" onClick={run} disabled={loading}><Icon name="refresh" size={13} /> {loading ? t('Checking…') : data ? t('Check again') : t('Analyse')}</button>
    }>
      {error && <div className="form-error">{error}</div>}
      {!data && !error && <p className="muted small">{t('Not analysed yet. Enable "Spam check" in the inbox settings to score every message automatically.')}</p>}
      {data && (
        <>
          <div className="spam-summary">
            <span className={`status-pill ${data.is_spam ? 'bad' : 'ok'}`}>{data.is_spam ? t('Spam') : t('Not spam')}</span>
            <b>{data.score.toFixed(1)}</b> / {data.threshold} <span className="muted small">· {data.engine}{data.action ? ` · ${data.action}` : ''} · {formatDate(data.checked_at)}</span>
          </div>
          <table className="kv">
            <tbody>
              {data.rules.filter((r) => r.score !== 0).concat(data.rules.filter((r) => r.score === 0)).slice(0, 40).map((r) => (
                <tr key={r.name}>
                  <th className={r.score > 0 ? 'lbl-no' : r.score < 0 ? 'lbl-ok' : 'muted'}>{r.score > 0 ? '+' : ''}{r.score.toFixed(1)}</th>
                  <td><code>{r.name}</code> <span className="muted small">{r.description}</span></td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </Section>
  )
}

export default function Checks({ msg }) {
  const t = useT()
  return (
    <div className="checks-panel">
      {msg.has_html && (
        <Section title={t('Email client compatibility')} icon="desktop">
          <HTMLCheck id={msg.id} />
        </Section>
      )}
      <Deliverability id={msg.id} />
      <Spam id={msg.id} report={msg.spam_report} />
      <LinkCheck id={msg.id} />
    </div>
  )
}
