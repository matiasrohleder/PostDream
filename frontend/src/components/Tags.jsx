import { useState } from 'react'
import Icon from './Icon.jsx'
import { useT } from '../i18n/index.jsx'

// Stable color per tag name (8 palette slots defined as CSS variables).
export function tagColor(tag) {
  let h = 0
  for (const ch of tag.toLowerCase()) h = (h * 31 + ch.codePointAt(0)) >>> 0
  return `tag-c${h % 8}`
}

export function TagChip({ tag, onRemove, onClick }) {
  const t = useT()
  return (
    <span className={`tag-chip ${tagColor(tag)} ${onClick ? 'clickable' : ''}`} onClick={onClick} title={onClick ? t('Filter by this tag') : tag}>
      {tag}
      {onRemove && (
        <button type="button" className="tag-x" onClick={(e) => { e.stopPropagation(); onRemove(tag) }} aria-label={t('Remove tag')}>
          <Icon name="x" size={11} />
        </button>
      )}
    </span>
  )
}

/** Editable tag list: chips with remove buttons plus an inline input to add one. */
export function TagEditor({ tags, onChange, suggestions = [] }) {
  const [adding, setAdding] = useState(false)
  const [value, setValue] = useState('')
  const t = useT()
  const add = () => {
    const v = value.trim()
    if (v && !tags.some((x) => x.toLowerCase() === v.toLowerCase())) onChange([...tags, v])
    setValue('')
    setAdding(false)
  }
  return (
    <div className="tag-editor">
      {tags.map((tag) => <TagChip key={tag} tag={tag} onRemove={(x) => onChange(tags.filter((y) => y !== x))} />)}
      {adding ? (
        <>
          <input className="tag-input" autoFocus value={value} list="tag-suggestions" maxLength={100}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') { e.preventDefault(); add() }
              if (e.key === 'Escape') { e.stopPropagation(); setAdding(false); setValue('') }
            }}
            onBlur={add} placeholder={t('tag name')} />
          <datalist id="tag-suggestions">
            {suggestions.filter((s) => !tags.includes(s)).map((s) => <option key={s} value={s} />)}
          </datalist>
        </>
      ) : (
        <button type="button" className="tag-add" onClick={() => setAdding(true)}><Icon name="plus" size={12} /> {t('Tag')}</button>
      )}
    </div>
  )
}
