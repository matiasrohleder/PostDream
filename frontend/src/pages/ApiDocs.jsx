import { useEffect, useMemo, useState } from 'react'
import { api } from '../lib/api.js'
import { useApp, useToast } from '../lib/context.jsx'
import { copyText } from '../lib/format.js'
import Icon from '../components/Icon.jsx'
import { useT } from '../i18n/index.jsx'

const METHODS = ['get', 'post', 'put', 'patch', 'delete']

function schemaName(s) {
  if (!s) return ''
  if (s.$ref) return s.$ref.split('/').pop()
  if (s.type === 'array') return `${schemaName(s.items)}[]`
  if (Array.isArray(s.type)) return s.type.join(' | ')
  return s.type || 'object'
}

function Operation({ method, path, op, base }) {
  const t = useT()
  const toast = useToast()
  const [open, setOpen] = useState(false)
  const body = op.requestBody?.content?.['application/json']?.schema
  const ok = Object.entries(op.responses || {}).find(([code]) => code.startsWith('2'))
  const okSchema = ok?.[1]?.content?.['application/json']?.schema
  const auth = (op.security || []).map((s) => Object.keys(s)[0])
  const curl = `curl ${method !== 'get' ? `-X ${method.toUpperCase()} ` : ''}${base}${path.replace(/\{(\w+)\}/g, '<$1>')}` +
    (auth.includes('bearerAuth') ? ' \\\n  -H "Authorization: Bearer $POSTDREAM_TOKEN"' : '') +
    (body ? ` \\\n  -H "Content-Type: application/json" -d '{…}'` : '')
  return (
    <div className="op">
      <button className="op-head" onClick={() => setOpen((o) => !o)}>
        <span className={`method m-${method}`}>{method.toUpperCase()}</span>
        <code className="op-path">{path}</code>
        <span className="op-summary">{op.summary}</span>
        {auth.length === 0 && <span className="tag">{t('public')}</span>}
        {auth.length === 1 && auth[0] === 'cookieAuth' && <span className="tag">{t('UI session')}</span>}
      </button>
      {open && (
        <div className="op-body">
          {op.description && <p className="small">{op.description}</p>}
          {op.parameters?.length > 0 && (
            <table className="kv compact">
              <tbody>
                {op.parameters.map((p) => (
                  <tr key={p.in + p.name}>
                    <th><code>{p.name}</code> <span className="muted small">{p.in}{p.required ? ' *' : ''}</span></th>
                    <td className="small">{p.description || schemaName(p.schema)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          {body && <p className="small">{t('Body')}: <code>{schemaName(body)}</code></p>}
          {okSchema && <p className="small">{t('Response')} {ok[0]}: <code>{schemaName(okSchema)}</code></p>}
          <div className="snippet-box">
            <pre>{curl}</pre>
            <button className="btn btn-small copy-btn" onClick={() => copyText(curl).then(() => toast(t('Copied')))}><Icon name="copy" size={13} /></button>
          </div>
        </div>
      )}
    </div>
  )
}

export default function ApiDocs() {
  const t = useT()
  const { inboxes } = useApp()
  const [spec, setSpec] = useState(null)
  const [filter, setFilter] = useState('')
  useEffect(() => { api('/openapi.json').then(setSpec) }, [])
  const base = window.location.origin
  const groups = useMemo(() => {
    if (!spec) return []
    const byTag = new Map((spec.tags || []).map((x) => [x.name, []]))
    const f = filter.toLowerCase()
    for (const [path, ops] of Object.entries(spec.paths)) {
      for (const m of METHODS) {
        const op = ops[m]
        if (!op) continue
        if (f && !`${m} ${path} ${op.summary}`.toLowerCase().includes(f)) continue
        const tag = op.tags?.[0] || 'Other'
        if (!byTag.has(tag)) byTag.set(tag, [])
        byTag.get(tag).push({ method: m, path, op })
      }
    }
    return [...byTag.entries()].filter(([, ops]) => ops.length)
  }, [spec, filter])

  const token = inboxes[0]?.api_token || '<api_token>'
  const e2e = `# Wait up to 30s for the password-reset email of a test user and print its text body
curl -s "${base}/api/messages/latest?wait=30s&q=to:new-user@example.com%20subject:reset" \\
  -H "Authorization: Bearer ${token}" | jq -r .text_body`

  return (
    <>
      <div className="page-head">
        <div>
          <h2>{t('API')}</h2>
          <p className="muted">
            {t('Authenticate with your UI session or with an inbox API token (Authorization: Bearer …), which only sees that inbox.')}{' '}
            <a href="/api/openapi.json" target="_blank" rel="noreferrer">openapi.json</a> (OpenAPI {spec?.openapi || '3.1'})
          </p>
        </div>
      </div>
      <div className="snippet-box"><pre>{e2e}</pre></div>
      <div className="search-box narrow-search">
        <Icon name="search" />
        <input placeholder={t('Filter endpoints…')} value={filter} onChange={(e) => setFilter(e.target.value)} />
      </div>
      {!spec && <p className="muted">{t('Loading…')}</p>}
      {groups.map(([tag, ops]) => (
        <section key={tag} className="op-group">
          <h3>{t(tag)}</h3>
          {ops.map((o) => <Operation key={o.method + o.path} {...o} base={base} />)}
        </section>
      ))}
    </>
  )
}
