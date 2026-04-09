import { useState, useEffect } from 'react'
import { api } from '../api'
import LanguageToggle from '../components/LanguageToggle'
import { useI18n } from '../i18n'

interface LoginPageProps {
  onLogin: (token: string, user: any) => void
}

type TabMode = 'login' | 'register'

export default function LoginPage({ onLogin }: LoginPageProps) {
  const { tx } = useI18n()
  const [tab, setTab] = useState<TabMode>('login')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  // Login form state
  const [loginEmail, setLoginEmail] = useState('')
  const [loginPassword, setLoginPassword] = useState('')

  // GitHub OAuth
  const [githubClientId, setGithubClientId] = useState('')

  useEffect(() => {
    fetch('/api/config')
      .then(r => r.json())
      .then(d => {
        const data = d?.data || d
        if (data?.github_client_id) setGithubClientId(data.github_client_id)
      })
      .catch(() => {})
  }, [])

  // Register form state
  const [regDisplayName, setRegDisplayName] = useState('')
  const [regSlug, setRegSlug] = useState('')
  const [regEmail, setRegEmail] = useState('')
  const [regPassword, setRegPassword] = useState('')
  const [regConfirmPassword, setRegConfirmPassword] = useState('')

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!loginEmail.trim() || !loginPassword) return

    setLoading(true)
    setError('')

    try {
      const result = await api.login({
        email: loginEmail.trim(),
        password: loginPassword,
      })
      localStorage.setItem('token', result.access_token)
      localStorage.setItem('refresh_token', result.refresh_token)
      onLogin(result.access_token, result.user)
    } catch (err: any) {
      setError(err.message || tx('登录失败', 'Sign in failed'))
    } finally {
      setLoading(false)
    }
  }

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!regEmail.trim() || !regPassword || !regSlug.trim()) {
      setError(tx('请填写所有必填字段', 'Please fill in all required fields'))
      return
    }

    if (regPassword.length < 8) {
      setError(tx('密码至少需要 8 个字符', 'Password must be at least 8 characters'))
      return
    }

    if (regPassword !== regConfirmPassword) {
      setError(tx('两次输入的密码不一致', 'Passwords do not match'))
      return
    }

    setLoading(true)
    setError('')

    try {
      const result = await api.register({
        email: regEmail.trim(),
        password: regPassword,
        display_name: regDisplayName.trim(),
        slug: regSlug.trim(),
      })
      localStorage.setItem('token', result.access_token)
      localStorage.setItem('refresh_token', result.refresh_token)
      onLogin(result.access_token, result.user)
    } catch (err: any) {
      setError(err.message || tx('注册失败', 'Sign up failed'))
    } finally {
      setLoading(false)
    }
  }

  const handleGitHubLogin = () => {
    if (!githubClientId) {
      setError(tx('GitHub OAuth 未配置', 'GitHub OAuth is not configured'))
      return
    }
    const redirectUri = `${window.location.origin}/api/auth/github/callback`
    window.location.href = `https://github.com/login/oauth/authorize?client_id=${githubClientId}&redirect_uri=${encodeURIComponent(redirectUri)}&scope=read:user user:email`
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <div className="login-card-header">
          <LanguageToggle />
        </div>
        <h1 className="login-title">Agent Hub</h1>
        <p className="login-desc">
          {tx('个人 AI Agent 管理中心', 'Personal AI agent control center')}
        </p>

        <div className="auth-tabs">
          <button
            className={`auth-tab ${tab === 'login' ? 'auth-tab-active' : ''}`}
            onClick={() => { setTab('login'); setError('') }}
          >
            {tx('登录', 'Sign in')}
          </button>
          <button
            className={`auth-tab ${tab === 'register' ? 'auth-tab-active' : ''}`}
            onClick={() => { setTab('register'); setError('') }}
          >
            {tx('注册', 'Sign up')}
          </button>
        </div>

        {tab === 'login' ? (
          <form onSubmit={handleLogin} className="login-form">
            <div className="form-group">
              <label htmlFor="login-email">{tx('邮箱', 'Email')}</label>
              <input
                id="login-email"
                type="email"
                value={loginEmail}
                onChange={(e) => setLoginEmail(e.target.value)}
                placeholder="your@email.com"
                autoFocus
                disabled={loading}
              />
            </div>

            <div className="form-group">
              <label htmlFor="login-password">{tx('密码', 'Password')}</label>
              <input
                id="login-password"
                type="password"
                value={loginPassword}
                onChange={(e) => setLoginPassword(e.target.value)}
                placeholder={tx('输入密码', 'Enter password')}
                disabled={loading}
              />
            </div>

            {error && <div className="form-error">{error}</div>}

            <button type="submit" className="btn btn-primary btn-block" disabled={loading}>
              {loading ? tx('登录中...', 'Signing in...') : tx('登录', 'Sign in')}
            </button>

            <div className="auth-divider">
              <span>{tx('或', 'Or')}</span>
            </div>

            <button
              type="button"
              className="btn btn-outline btn-block btn-github"
              onClick={handleGitHubLogin}
              disabled={loading}
            >
              <svg className="github-icon" viewBox="0 0 16 16" width="18" height="18" fill="currentColor">
                <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
              </svg>
              {tx('GitHub 登录', 'Continue with GitHub')}
            </button>
          </form>
        ) : (
          <form onSubmit={handleRegister} className="login-form">
            <div className="form-group">
              <label htmlFor="reg-display-name">{tx('显示名称', 'Display name')}</label>
              <input
                id="reg-display-name"
                type="text"
                value={regDisplayName}
                onChange={(e) => setRegDisplayName(e.target.value)}
                placeholder={tx('你的名字', 'Your name')}
                autoFocus
                disabled={loading}
              />
            </div>

            <div className="form-group">
              <label htmlFor="reg-slug">{tx('用户标识 *', 'Username *')}</label>
              <input
                id="reg-slug"
                type="text"
                value={regSlug}
                onChange={(e) => setRegSlug(e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, ''))}
                placeholder="my-username"
                disabled={loading}
              />
            </div>

            <div className="form-group">
              <label htmlFor="reg-email">{tx('邮箱 *', 'Email *')}</label>
              <input
                id="reg-email"
                type="email"
                value={regEmail}
                onChange={(e) => setRegEmail(e.target.value)}
                placeholder="your@email.com"
                disabled={loading}
              />
            </div>

            <div className="form-group">
              <label htmlFor="reg-password">{tx('密码 *', 'Password *')}</label>
              <input
                id="reg-password"
                type="password"
                value={regPassword}
                onChange={(e) => setRegPassword(e.target.value)}
                placeholder={tx('至少 8 个字符', 'At least 8 characters')}
                disabled={loading}
              />
            </div>

            <div className="form-group">
              <label htmlFor="reg-confirm-password">{tx('确认密码 *', 'Confirm password *')}</label>
              <input
                id="reg-confirm-password"
                type="password"
                value={regConfirmPassword}
                onChange={(e) => setRegConfirmPassword(e.target.value)}
                placeholder={tx('再次输入密码', 'Enter password again')}
                disabled={loading}
              />
            </div>

            {error && <div className="form-error">{error}</div>}

            <button type="submit" className="btn btn-primary btn-block" disabled={loading}>
              {loading ? tx('注册中...', 'Signing up...') : tx('注册', 'Sign up')}
            </button>
          </form>
        )}
      </div>
    </div>
  )
}
