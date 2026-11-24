import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import es from './es.js'
import de from './de.js'

// Keys are the English source strings; English therefore needs no dictionary.
export const LANGUAGES = [
  { code: 'en', label: 'English' },
  { code: 'es', label: 'Español' },
  { code: 'de', label: 'Deutsch' },
]
const dictionaries = { en: {}, es, de }
const STORAGE_KEY = 'pd-lang'

function detectLanguage() {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved && dictionaries[saved]) return saved
  } catch { /* storage unavailable */ }
  for (const l of navigator.languages || [navigator.language || 'en']) {
    const code = l.slice(0, 2).toLowerCase()
    if (dictionaries[code]) return code
  }
  return 'en'
}

let currentLang = detectLanguage()

/** Translate `key`, replacing `{name}` placeholders with `vars.name`. */
export function translate(key, vars) {
  let s = dictionaries[currentLang]?.[key] ?? key
  if (vars) s = s.replace(/\{(\w+)\}/g, (m, k) => (vars[k] !== undefined ? String(vars[k]) : m))
  return s
}

export const getLocale = () => currentLang

const I18nContext = createContext({ lang: 'en', setLang: () => {}, t: translate })

export function I18nProvider({ children }) {
  const [lang, setLangState] = useState(currentLang)
  const setLang = useCallback((l) => {
    currentLang = l
    setLangState(l)
  }, [])
  useEffect(() => {
    document.documentElement.lang = lang
    try { localStorage.setItem(STORAGE_KEY, lang) } catch { /* ignore */ }
  }, [lang])
  // A new `t` identity per language makes consumers re-render on change.
  const value = useMemo(() => ({ lang, setLang, t: (k, v) => translate(k, v) }), [lang, setLang])
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}

export const useI18n = () => useContext(I18nContext)
export const useT = () => useContext(I18nContext).t

export function LanguageSelect({ className = '' }) {
  const { lang, setLang, t } = useI18n()
  return (
    <select className={`lang-select ${className}`} value={lang} onChange={(e) => setLang(e.target.value)} title={t('Language')} aria-label={t('Language')}>
      {LANGUAGES.map((l) => <option key={l.code} value={l.code}>{l.label}</option>)}
    </select>
  )
}
