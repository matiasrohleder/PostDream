import { displayName, formatBytes, formatDate, relativeTime } from '../lib/format.js'
import Icon from './Icon.jsx'
import { TagChip } from './Tags.jsx'
import { useT } from '../i18n/index.jsx'

const columns = [
  { key: 'received_at', label: 'Received', sortable: true, className: 'col-date' },
  { key: 'from', label: 'From', sortable: true, className: 'col-from' },
  { key: 'to', label: 'To', sortable: true, className: 'col-to' },
  { key: 'subject', label: 'Subject', sortable: true, className: 'col-subject' },
  { key: 'inbox', label: 'Inbox', sortable: true, className: 'col-inbox', inboxOnly: true },
  { key: 'attachments', label: <Icon name="clip" size={14} />, className: 'col-att' },
  { key: 'size', label: 'Size', sortable: true, className: 'col-size' },
]

export default function MessageGrid({
  items, loading, showInbox, sort, onSort, selected, onSelect, openId, onOpen,
  page, pageSize, total, onPage, onPageSize, onTagClick,
}) {
  const t = useT()
  const cols = columns.filter((c) => !c.inboxOnly || showInbox)
  const allChecked = items.length > 0 && items.every((m) => selected.has(m.id))
  const toggleAll = () => onSelect(allChecked ? new Set() : new Set(items.map((m) => m.id)))
  const toggle = (id) => {
    const next = new Set(selected)
    next.has(id) ? next.delete(id) : next.add(id)
    onSelect(next)
  }
  const clickSort = (key) =>
    onSort((s) => ({ by: key, order: s.by === key && s.order === 'desc' ? 'asc' : 'desc' }))
  const pages = Math.max(1, Math.ceil(total / pageSize))

  return (
    <div className="grid-wrap">
      <div className="grid-scroll">
        <table className={`grid ${loading ? 'is-loading' : ''}`}>
          <thead>
            <tr>
              <th className="col-check">
                <input type="checkbox" checked={allChecked} onChange={toggleAll} aria-label={t('Select all')} />
              </th>
              {cols.map((c) => (
                <th key={c.key} className={`${c.className} ${c.sortable ? 'sortable' : ''}`}
                  onClick={c.sortable ? () => clickSort(c.key) : undefined}>
                  {typeof c.label === 'string' ? t(c.label) : c.label}
                  {sort.by === c.key && <span className="sort-arrow">{sort.order === 'desc' ? '▼' : '▲'}</span>}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {items.map((m) => (
              <tr key={m.id} className={`${m.is_read ? '' : 'unread'} ${openId === m.id ? 'active' : ''}`}
                onClick={() => onOpen(m.id)}>
                <td className="col-check" onClick={(e) => e.stopPropagation()}>
                  <input type="checkbox" checked={selected.has(m.id)} onChange={() => toggle(m.id)} />
                </td>
                <td className="col-date" title={formatDate(m.received_at)}>
                  {!m.is_read && <span className="unread-dot" />}
                  {relativeTime(m.received_at)}
                </td>
                <td className="col-from" title={m.from}>{displayName(m.from)}</td>
                <td className="col-to" title={m.to}>{displayName(m.to)}</td>
                <td className="col-subject">
                  {m.tags?.map((tag) => (
                    <TagChip key={tag} tag={tag} onClick={onTagClick ? (e) => { e.stopPropagation(); onTagClick(tag) } : undefined} />
                  ))}
                  <span className="subject">{m.subject || t('(no subject)')}</span>
                  <span className="snippet">{m.snippet}</span>
                </td>
                {showInbox && <td className="col-inbox"><span className="tag">{m.inbox_name}</span></td>}
                <td className="col-att">{m.attachment_count > 0 && <span title={t('{n} attachment(s)', { n: m.attachment_count })}><Icon name="clip" size={14} /></span>}</td>
                <td className="col-size">{formatBytes(m.size)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {!loading && items.length === 0 && (
          <div className="empty">
            <img src="/logo.svg" alt="" />
            <p>{t('No emails caught here yet.')}</p>
            <p className="muted small">{t('Send an email over SMTP and it will show up in real time.')}</p>
          </div>
        )}
      </div>
      <div className="pager">
        <span className="muted small">
          {total === 0 ? '0' : `${(page - 1) * pageSize + 1}–${Math.min(page * pageSize, total)}`} {t('of')} {total}
        </span>
        <select value={pageSize} onChange={(e) => onPageSize(Number(e.target.value))}>
          {[25, 50, 100, 200].map((n) => <option key={n} value={n}>{n} / {t('page')}</option>)}
        </select>
        <button className="icon-btn" disabled={page <= 1} onClick={() => onPage(page - 1)}><Icon name="chevronLeft" /></button>
        <span className="small">{page} / {pages}</span>
        <button className="icon-btn" disabled={page >= pages} onClick={() => onPage(page + 1)}><Icon name="chevronRight" /></button>
      </div>
    </div>
  )
}
