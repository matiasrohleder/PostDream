import { useState } from 'react'
import { api } from '../lib/api.js'
import { LanguageSelect, useT } from '../i18n/index.jsx'
import Logo from '../components/Logo.jsx'

export default function Login({ onLogin }) {
  const t = useT()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async (e) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      onLogin(await api('/auth/login', { method: 'POST', body: { email, password } }))
    } catch (err) {
      setError(err.status === 401 ? t('Invalid email or password') : err.status === 429 ? t('Too many failed attempts, try again later') : err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-page">
      <div className="login-lang"><LanguageSelect /></div>
      <form className="login-card" onSubmit={submit}>
        <Logo size={120} />
        <h1>
          Post<b>Dream</b>
        </h1>
        <p className="muted">{t('Catch every email your test environments send.')}</p>
        <label>
          {t('Email')}
          <input type="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} required autoFocus />
        </label>
        <label>
          {t('Password')}
          <input type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        </label>
        {error && <div className="form-error">{error}</div>}
        <button className="btn btn-primary btn-block" disabled={loading}>
          {loading ? t('Signing in…') : t('Sign in')}
        </button>
      </form>
    </div>
  )
}
