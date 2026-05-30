import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../AuthContext'
import { Bot, Globe, KeyRound } from 'lucide-react'
import i18n from '../i18n'

export default function Login() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { login, register } = useAuth()
  const [searchParams] = useSearchParams()

  const [isRegister, setIsRegister] = useState(false)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [nickname, setNickname] = useState('')
  const [inviteCode, setInviteCode] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  // Pre-fill invite code from URL
  useEffect(() => {
    const codeFromUrl = searchParams.get('code')
    if (codeFromUrl) {
      setInviteCode(codeFromUrl)
      setIsRegister(true)
    }
  }, [searchParams])

  const toggleLang = () => {
    const next = i18n.language === 'zh' ? 'en' : 'zh'
    i18n.changeLanguage(next)
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      if (isRegister) {
        if (username.length < 3) { setError(t('username_min')); setLoading(false); return }
        if (password.length < 6) { setError(t('password_min')); setLoading(false); return }
        await register(username, password, nickname || username, inviteCode)
      } else {
        await login(username, password)
      }
      navigate('/')
    } catch (err) {
      setError(isRegister ? t('register_error') : t('login_error'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <div className="login-header">
          <div className="login-logo">
            <Bot size={36} />
          </div>
          <h1>{t('app_name')}</h1>
          <p>{isRegister ? t('register_title') : t('login_title')}</p>
        </div>

        <form onSubmit={handleSubmit} className="login-form">
          {error && <div className="form-error">{error}</div>}

          <label>
            <span>{t('username')}</span>
            <input
              type="text"
              value={username}
              onChange={e => setUsername(e.target.value)}
              placeholder={t('username')}
              required
              autoFocus
            />
          </label>

          {isRegister && (
            <label>
              <span>{t('nickname')}</span>
              <input
                type="text"
                value={nickname}
                onChange={e => setNickname(e.target.value)}
                placeholder={t('nickname')}
              />
            </label>
          )}

          {isRegister && (
            <label>
              <span>{t('invite_code')}</span>
              <div className="invite-code-input-wrapper">
                <KeyRound size={16} className="invite-code-icon" />
                <input
                  type="text"
                  value={inviteCode}
                  onChange={e => setInviteCode(e.target.value)}
                  placeholder={t('invite_code')}
                />
              </div>
              <small className="invite-code-hint">{t('invite_code_hint')}</small>
            </label>
          )}

          <label>
            <span>{t('password')}</span>
            <input
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              placeholder={t('password')}
              required
            />
          </label>

          <button type="submit" className="btn btn-primary btn-full" disabled={loading}>
            {loading ? t('loading') : isRegister ? t('register') : t('login')}
          </button>
        </form>

        <div className="login-footer">
          {isRegister ? (
            <span>{t('has_account')} <button className="btn-link" onClick={() => { setIsRegister(false); setError('') }}>{t('go_login')}</button></span>
          ) : (
            <span>{t('no_account')} <button className="btn-link" onClick={() => { setIsRegister(true); setError('') }}>{t('go_register')}</button></span>
          )}
        </div>

        <button className="lang-toggle" onClick={toggleLang} title={t('language')}>
          <Globe size={16} /> {i18n.language === 'zh' ? 'English' : '中文'}
        </button>
      </div>
    </div>
  )
}
