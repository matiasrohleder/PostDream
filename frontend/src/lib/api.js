export class ApiError extends Error {
  constructor(status, message) {
    super(message)
    this.status = status
  }
}

function buildQuery(query) {
  if (!query) return ''
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(query)) {
    if (v !== undefined && v !== null && v !== '' && v !== false) p.set(k, v)
  }
  const s = p.toString()
  return s ? `?${s}` : ''
}

export async function api(path, { method = 'GET', body, query } = {}) {
  const res = await fetch(`/api${path}${buildQuery(query)}`, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : {},
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  })
  const ct = res.headers.get('content-type') || ''
  const data = ct.includes('application/json') ? await res.json() : await res.text()
  if (!res.ok) {
    if (res.status === 401 && !path.startsWith('/auth/login')) {
      window.dispatchEvent(new Event('pd:unauthorized'))
    }
    throw new ApiError(res.status, (data && data.error) || res.statusText)
  }
  return data
}

export const rawUrl = (id, download) => `/api/messages/${id}/raw${download ? '?download=1' : ''}`
export const attachmentUrl = (id, aid) => `/api/messages/${id}/attachments/${aid}`
