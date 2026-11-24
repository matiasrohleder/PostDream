import { getLocale } from '../i18n/index.jsx'

export function formatBytes(n) {
  if (n == null) return ''
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

export function relativeTime(iso) {
  const rtf = new Intl.RelativeTimeFormat(getLocale(), { numeric: 'auto' })
  const diff = (new Date(iso).getTime() - Date.now()) / 1000
  const abs = Math.abs(diff)
  if (abs < 60) return rtf.format(Math.round(diff), 'second')
  if (abs < 3600) return rtf.format(Math.round(diff / 60), 'minute')
  if (abs < 86400) return rtf.format(Math.round(diff / 3600), 'hour')
  return rtf.format(Math.round(diff / 86400), 'day')
}

export function formatDate(iso) {
  return new Date(iso).toLocaleString(getLocale(), {
    year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit',
  })
}

/** "Name <a@b.c>" -> "Name" (falls back to the address). */
export function displayName(addr) {
  if (!addr) return ''
  const first = addr.split(',')[0].trim()
  const m = first.match(/^"?([^"<]*)"?\s*<([^>]+)>/)
  if (m) return m[1].trim() || m[2]
  return first
}

export async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    const ta = document.createElement('textarea')
    ta.value = text
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    ta.remove()
    return true
  }
}
