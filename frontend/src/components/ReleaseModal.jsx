import { useState } from 'react'
import { api } from '../lib/api.js'
import { useApp, useToast } from '../lib/context.jsx'
import Modal from './Modal.jsx'
import { useT } from '../i18n/index.jsx'

export default function ReleaseModal({ msg, onClose }) {
  const t = useT()
  const toast = useToast()
  const { inboxes } = useApp()
  const inbox = inboxes.find((i) => i.id === msg.inbox_id)
  const [to, setTo] = useState(msg.envelope_to.join(', '))
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (e) => {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      const recipients = to.split(/[,\s]+/).map((x) => x.trim()).filter(Boolean)
      await api(`/messages/${msg.id}/release`, { method: 'POST', body: { to: recipients } })
      toast(t('Email released to {n} recipient(s)', { n: recipients.length }), 'success')
      onClose()
    } catch (err) { setError(err.message) } finally { setBusy(false) }
  }
  return (
    <Modal title={t('Release email')} onClose={onClose}>
      <form className="form" onSubmit={submit}>
        <p className="muted small">
          {t('Sends this email to real recipients through the configured relay server. Every release is recorded in the audit log.')}
        </p>
        {inbox?.relay_recipients
          ? <p className="small">{t('Allowed recipients for this inbox')}: <code>{inbox.relay_recipients}</code></p>
          : <div className="form-error">{t('Releasing is disabled for this inbox: an admin must set "Allowed release recipients" in its settings.')}</div>}
        <label>{t('Recipients')}<input value={to} onChange={(e) => setTo(e.target.value)} placeholder="qa@example.com, dev@example.com" autoFocus /></label>
        {error && <div className="form-error">{error}</div>}
        <div className="form-actions">
          <button type="button" className="btn" onClick={onClose}>{t('Cancel')}</button>
          <button className="btn btn-primary" disabled={busy || !to.trim()}>{busy ? t('Sending…') : t('Release')}</button>
        </div>
      </form>
    </Modal>
  )
}
