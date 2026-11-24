import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useOutletContext, useParams, useSearchParams } from 'react-router-dom'
import { api } from '../lib/api.js'
import { useApp, useToast } from '../lib/context.jsx'
import Filters, { emptyFilters } from '../components/Filters.jsx'
import MessageGrid from '../components/MessageGrid.jsx'
import MessageView from '../components/MessageView.jsx'
import Icon from '../components/Icon.jsx'
import Modal from '../components/Modal.jsx'
import { useT } from '../i18n/index.jsx'

function BulkTagModal({ ids, onClose, onDone }) {
  const t = useT()
  const [value, setValue] = useState('')
  const run = async (mode) => {
    const tags = value.split(',').map((x) => x.trim()).filter(Boolean)
    if (!tags.length) return
    await api('/messages/tags', { method: 'POST', body: { ids, [mode]: tags } })
    onDone()
  }
  return (
    <Modal title={t('Tag {n} email(s)', { n: ids.length })} onClose={onClose}>
      <div className="form">
        <label>{t('Tags (comma separated)')}<input autoFocus value={value} onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && run('add')} /></label>
        <div className="form-actions">
          <button className="btn" onClick={() => run('remove')} disabled={!value.trim()}>{t('Remove')}</button>
          <button className="btn btn-primary" onClick={() => run('add')} disabled={!value.trim()}>{t('Add')}</button>
        </div>
      </div>
    </Modal>
  )
}

export default function Mailbox() {
  const { inboxId } = useParams()
  const { inboxes, refreshInboxes } = useApp()
  const { liveEvent } = useOutletContext()
  const toast = useToast()
  const t = useT()

  // `filters` is what the user sees (updated on every keystroke); `applied` is the
  // debounced copy used for querying.
  const [filters, setFilters] = useState(emptyFilters)
  const [applied, setApplied] = useState(emptyFilters)
  useEffect(() => {
    if (filters === applied) return
    const id = setTimeout(() => setApplied(filters), 300)
    return () => clearTimeout(id)
  }, [filters]) // eslint-disable-line react-hooks/exhaustive-deps
  const [sort, setSort] = useState({ by: 'received_at', order: 'desc' })
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(50)
  const [data, setData] = useState({ items: [], total: 0 })
  const [loading, setLoading] = useState(false)
  const [selected, setSelected] = useState(new Set())
  const [openId, setOpenId] = useState(null)
  const [searchError, setSearchError] = useState('')
  const [tags, setTags] = useState([])
  const [tagging, setTagging] = useState(false)
  const [params, setParams] = useSearchParams()

  // Deep links: ?m=<message id> opens a message, ?q=<search> pre-fills the search.
  useEffect(() => {
    const m = params.get('m')
    const q = params.get('q')
    if (m) setOpenId(m)
    if (q !== null) {
      const f = { ...emptyFilters, q }
      setFilters(f)
      setApplied(f)
    }
    if (m || q !== null) setParams({}, { replace: true })
  }, [params]) // eslint-disable-line react-hooks/exhaustive-deps

  const loadTags = useCallback(() => {
    api('/tags', { query: { inbox: inboxId } }).then(setTags).catch(() => {})
  }, [inboxId])
  useEffect(() => { loadTags() }, [loadTags])

  const inbox = inboxes.find((i) => i.id === inboxId)
  const query = useMemo(() => {
    const q = { ...applied, inbox: inboxId, sort: sort.by, order: sort.order, page, page_size: pageSize }
    if (applied.since) q.since = new Date(applied.since).toISOString()
    if (applied.until) q.until = new Date(applied.until).toISOString()
    return q
  }, [applied, inboxId, sort, page, pageSize])

  const reqId = useRef(0)
  const load = useCallback(async () => {
    const my = ++reqId.current
    setLoading(true)
    try {
      const res = await api('/messages', { query })
      if (my === reqId.current) {
        setData(res)
        setSearchError('')
      }
    } catch (e) {
      if (e.status === 400 && my === reqId.current) setSearchError(e.message)
      else if (e.status !== 401) toast(e.message, 'error')
    } finally {
      if (my === reqId.current) setLoading(false)
    }
  }, [query, toast])

  useEffect(() => { load() }, [load])
  useEffect(() => { setPage(1); setSelected(new Set()) }, [applied, inboxId])
  const firstInbox = useRef(true)
  useEffect(() => {
    if (firstInbox.current) { firstInbox.current = false; return }
    setOpenId(null)
  }, [inboxId])

  // Live refresh when new mail arrives / something changes.
  useEffect(() => {
    if (!liveEvent) return
    if (liveEvent.type === 'message.new' && inboxId && liveEvent.inbox_id !== inboxId) return
    const t = setTimeout(load, 250)
    return () => clearTimeout(t)
  }, [liveEvent]) // eslint-disable-line react-hooks/exhaustive-deps

  // Children (MessageView effects) may hold on to an old afterChange: always use the latest loaders.
  const latest = useRef({})
  latest.current = { load, refreshInboxes, loadTags }
  const afterChange = useCallback(() => {
    latest.current.load()
    latest.current.refreshInboxes()
    latest.current.loadTags()
  }, [])
  const filterByTag = (tag) => {
    const term = /\s/.test(tag) ? `tag:"${tag}"` : `tag:${tag}`
    setFilters((f) => (f.q.includes(term) ? f : { ...f, q: (f.q ? f.q.trimEnd() + ' ' : '') + term }))
  }

  const ids = [...selected]
  const bulkDelete = async () => {
    if (!ids.length || !confirm(t('Delete {n} email(s)?', { n: ids.length }))) return
    await api('/messages/delete', { method: 'POST', body: { ids } })
    if (ids.includes(openId)) setOpenId(null)
    setSelected(new Set())
    toast(t('{n} email(s) deleted', { n: ids.length }), 'success')
    afterChange()
  }
  const bulkRead = async (read) => {
    await api('/messages/read', { method: 'POST', body: { ids, read } })
    setSelected(new Set())
    afterChange()
  }
  const markAllRead = async () => {
    await api('/messages/read', { method: 'POST', body: {}, query: { inbox: inboxId } })
    afterChange()
  }
  const deleteAll = async () => {
    const hasFilters = Object.values(applied).some(Boolean)
    const target = hasFilters ? t('the {n} emails matching the filters', { n: data.total }) : inbox ? t('ALL emails in "{name}"', { name: inbox.name }) : t('ALL emails in every inbox')
    if (!confirm(t('Delete {target}? This cannot be undone.', { target }))) return
    const { since, until, ...rest } = query
    const res = await api('/messages', { method: 'DELETE', query: { ...rest, since, until, page: undefined, page_size: undefined } })
    setOpenId(null)
    toast(t('{n} email(s) deleted', { n: res.deleted }), 'success')
    afterChange()
  }

  // Keyboard shortcuts: j/k to move, Esc to close, Delete to remove the open message.
  useEffect(() => {
    const onKey = (e) => {
      if (['INPUT', 'TEXTAREA', 'SELECT'].includes(e.target.tagName)) return
      const idx = data.items.findIndex((m) => m.id === openId)
      if (e.key === 'j' || e.key === 'ArrowDown') {
        const next = data.items[Math.min(idx + 1, data.items.length - 1)]
        if (next) setOpenId(next.id)
      } else if (e.key === 'k' || e.key === 'ArrowUp') {
        const prev = data.items[Math.max(idx - 1, 0)]
        if (prev) setOpenId(prev.id)
      } else if (e.key === 'Escape') {
        setOpenId(null)
      } else if (e.key === 'Delete' && openId) {
        api(`/messages/${openId}`, { method: 'DELETE' }).then(() => {
          const next = data.items[idx + 1] || data.items[idx - 1]
          setOpenId(next ? next.id : null)
          afterChange()
        })
      } else return
      e.preventDefault()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }) // re-bind every render to capture fresh state

  return (
    <div className={`mailbox ${openId ? 'with-detail' : ''}`}>
      <div className="mailbox-main">
        <div className="mailbox-header">
          <div>
            <h2>{inbox ? inbox.name : t('All inboxes')}</h2>
            <div className="muted small">
              {t('{n} email(s)', { n: data.total })}{inbox?.description ? ` · ${inbox.description}` : ''}
            </div>
          </div>
          <div className="toolbar">
            {selected.size > 0 ? (
              <>
                <span className="muted small">{t('{n} selected', { n: selected.size })}</span>
                <button className="btn" onClick={() => bulkRead(true)}><Icon name="mailOpen" /> {t('Read')}</button>
                <button className="btn" onClick={() => bulkRead(false)}><Icon name="mail" /> {t('Unread')}</button>
                <button className="btn" onClick={() => setTagging(true)}><Icon name="tag" /> {t('Tag')}</button>
                <button className="btn btn-danger" onClick={bulkDelete}><Icon name="trash" /> {t('Delete')}</button>
              </>
            ) : (
              <>
                <button className="btn" onClick={load} title={t('Refresh')}><Icon name="refresh" /></button>
                <button className="btn" onClick={markAllRead}><Icon name="check" /> {t('Mark all read')}</button>
                <button className="btn btn-danger-outline" onClick={deleteAll} disabled={!data.total}><Icon name="trash" /> {t('Empty')}</button>
              </>
            )}
          </div>
        </div>

        <Filters value={filters} onChange={setFilters} tags={tags} error={searchError} />

        <MessageGrid
          items={data.items}
          loading={loading}
          showInbox={!inboxId}
          sort={sort}
          onSort={setSort}
          selected={selected}
          onSelect={setSelected}
          openId={openId}
          onOpen={setOpenId}
          page={page}
          pageSize={pageSize}
          total={data.total}
          onPage={setPage}
          onPageSize={(n) => { setPageSize(n); setPage(1) }}
          onTagClick={filterByTag}
        />
      </div>

      {openId && (
        <MessageView
          id={openId}
          onClose={() => setOpenId(null)}
          onChanged={afterChange}
          onDeleted={() => { setOpenId(null); afterChange() }}
          tagSuggestions={tags.map((x) => x.tag)}
        />
      )}
      {tagging && (
        <BulkTagModal ids={ids} onClose={() => setTagging(false)}
          onDone={() => { setTagging(false); setSelected(new Set()); afterChange() }} />
      )}
    </div>
  )
}
