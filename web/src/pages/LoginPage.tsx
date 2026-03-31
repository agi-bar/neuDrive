import { useState } from 'react'
import { api } from '../api'

interface LoginPageProps {
  onLogin: (token: string, user: any) => void
}

type TabMode = 'login' | 'register'

export default function LoginPage({ onLogin }: LoginPageProps) {
  const [tab, setTab] = useState<TabMode>('login')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  // Login form state
  const [loginEmail, setLoginEmail] = useState('')
  const [loginPassword, setLoginPassword] = useState('')

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
      setError(err.message || '登录失败')
    } finally {
      setLoading(false)
    }
  }

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!regEmail.trim() || !regPassword || !regSlug.trim()) {
      setError('请填写所有必填字段')
      return
    }

    if (regPassword.length < 8) {
      setError('密码至少需要 8 个字符')
      return
    }

    if (regPassword !== regConfirmPassword) {
      setError('两次输入的密码不一致')
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
      setError(err.message || '注册失败')
    } finally {
      setLoading(false)
    }
  }

  const handleGitHubLogin = () => {
    const clientId = (window as any).__GITHUB_CLIENT_ID__
    if (!clientId) {
      setError('GitHub OAuth 未配置')
      return
    }
    const redirectUri = `${window.location.origin}/api/auth/github/callback`
    window.location.href = `https://github.com/login/oauth/authorize?client_id=${clientId}&redirect_uri=${encodeURIComponent(redirectUri)}&scope=read:user user:email`
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <h1 className="login-title">Agent Hub</h1>
        <p className="login-desc">
          个人 AI Agent 管理中心
        </p>

        <div className="auth-tabs">
          <button
            className={`auth-tab ${tab === 'login' ? 'auth-tab-active' : ''}`}
            onClick={() => { setTab('login'); setError('') }}
          >
            登录
          </button>
          <button
            className={`auth-tab ${tab === 'register' ? 'auth-tab-active' : ''}`}
            onClick={() => { setTab('register'); setError('') }}
          >
            注册
          </button>
        </div>

        {tab === 'login' ? (
          <form onSubmit={handleLogin} className="login-form">
            <div className="form-group">
              <label htmlFor="login-email">邮箱</label>
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
              <label htmlFor="login-password">密码</label>
              <input
                id="login-password"
                type="password"
                value={loginPassword}
                onChange={(e) => setLoginPassword(e.target.value)}
                placeholder="输入密码"
                disabled={loading}
              />
            </div>

            {error && <div className="form-error">{error}</div>}

            <button type="submit" className="btn btn-primary btn-block" disabled={loading}>
              {loading ? '登录中...' : '登录'}
            </button>

            <div className="auth-divider">
              <span>或</span>
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
              GitHub 登录
            </button>
          </form>
        ) : (
          <form onSubmit={handleRegister} className="login-form">
            <div className="form-group">
              <label htmlFor="reg-display-name">显示名称</label>
              <input
                id="reg-display-name"
                type="text"
                value={regDisplayName}
                onChange={(e) => setRegDisplayName(e.target.value)}
                placeholder="你的名字"
                autoFocus
                disabled={loading}
              />
            </div>

            <div className="form-group">
              <label htmlFor="reg-slug">用户标识 *</label>
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
              <label htmlFor="reg-email">邮箱 *</label>
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
              <label htmlFor="reg-password">密码 *</label>
              <input
                id="reg-password"
                type="password"
                value={regPassword}
                onChange={(e) => setRegPassword(e.target.value)}
                placeholder="至少 8 个字符"
                disabled={loading}
              />
            </div>

            <div className="form-group">
              <label htmlFor="reg-confirm-password">确认密码 *</label>
              <input
                id="reg-confirm-password"
                type="password"
                value={regConfirmPassword}
                onChange={(e) => setRegConfirmPassword(e.target.value)}
                placeholder="再次输入密码"
                disabled={loading}
              />
            </div>

            {error && <div className="form-error">{error}</div>}

            <button type="submit" className="btn btn-primary btn-block" disabled={loading}>
              {loading ? '注册中...' : '注册'}
            </button>
          </form>
        )}
      </div>
    </div>
  )
}
