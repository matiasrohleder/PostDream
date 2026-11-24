import { useState } from 'react'
import { api } from '../lib/api.js'
import { LanguageSelect, useT } from '../i18n/index.jsx'
import Logo from '../components/Logo.jsx'

export default function Login({ onLogin, config }) {
  const t = useT()
  const tokenMode = config?.auth_mode === 'token'
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async (e) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const body = tokenMode ? { token: password } : { email, password }
      onLogin(await api('/auth/login', { method: 'POST', body }))
    } catch (err) {
      const invalid = tokenMode ? t('Invalid access token') : t('Invalid email or password')
      setError(err.status === 401 ? invalid : err.status === 429 ? t('Too many failed attempts, try again later') : err.message)
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
        {!tokenMode && (
          <label>
            {t('Email')}
            <input type="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} required autoFocus />
          </label>
        )}
        <label>
          {tokenMode ? t('Access token') : t('Password')}
          <input
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoFocus={tokenMode}
          />
        </label>
        {error && <div className="form-error">{error}</div>}
        <button className="btn btn-primary btn-block" disabled={loading}>
          {loading ? t('Signing in…') : t('Sign in')}
        </button>
      </form>
    </div>
  )
}
