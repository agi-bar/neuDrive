import { FormEvent, useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, type AuthProvider } from '../api'
import LanguageToggle from '../components/LanguageToggle'
import { useI18n } from '../i18n'

export default function LoginPage() {
  const { tx } = useI18n()
  const navigate = useNavigate()
  const [providers, setProviders] = useState<AuthProvider[]>([])
  const [error, setError] = useState('')
  const [loadingAction, setLoadingAction] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    setError(params.get('error') || '')
    api.getAuthProviders().then((items) => setProviders(items || [])).catch(() => setProviders([]))
  }, [])

  const githubProvider = providers.find((provider) => provider.id === 'github')
  const pocketProvider = providers.find((provider) => provider.kind === 'oidc')
  const githubEnabled = !!githubProvider?.enabled
  const pocketEnabled = !!pocketProvider?.enabled
  const busy = loadingAction !== ''

  const redirectTarget = () => {
    const params = new URLSearchParams(window.location.search)
    return sanitizeLoginRedirect(params.get('redirect'))
  }

  const handleProviderAction = async (provider: AuthProvider | undefined, loadingKey: string) => {
    if (!provider?.enabled) return
    setLoadingAction(loadingKey)
    setError('')
    try {
      const resp = await api.startAuthProvider(provider.id, redirectTarget(), 'login')
      window.location.assign(resp.authorization_url)
    } catch (err: any) {
      setError(err?.message || tx('启动登录失败', 'Failed to start sign-in'))
      setLoadingAction('')
    }
  }

  const handleEmailLogin = async (event: FormEvent) => {
    event.preventDefault()
    if (busy) return
    setLoadingAction('email')
    setError('')
    try {
      const response = await api.login({ email, password })
      localStorage.setItem('token', response.access_token)
      localStorage.setItem('refresh_token', response.refresh_token)
      window.location.assign(redirectTarget())
    } catch (err: any) {
      setError(err?.message || tx('登录失败', 'Login failed'))
      setLoadingAction('')
    }
  }

  return (
    <div className="login-page product-auth-page">
      <div className="login-shell compact-auth-shell">
        <section className="login-hero compact-login-hero">
          <div className="login-hero-copy">
            <Link to="/" className="public-brand">neuDrive</Link>
            <p className="login-hero-slogan">
              {tx('让所有 AI 工具共用同一份记忆。', 'One memory layer for all your AI agents.')}
            </p>
            <p className="login-hero-subtitle">
              {tx('快速回到你的 AI 数据、接入和记忆控制台。', 'Get back to your AI data, integrations, and memory console.')}
            </p>
          </div>
        </section>

        <section className="login-panel">
          <div className="login-panel-card">
            <div className="login-panel-header">
              <LanguageToggle />
            </div>
            <h1 className="login-title">{tx('登录 neuDrive', 'Log in to neuDrive')}</h1>
            <p className="login-desc">{tx('老用户快速进入产品。', 'Fast access for existing users.')}</p>
            {error && <div className="form-error login-panel-error">{error}</div>}

            <div className="login-actions">
              <button
                type="button"
                className="btn btn-primary btn-block"
                onClick={() => { void handleProviderAction(githubProvider, 'github') }}
                disabled={busy || !githubEnabled}
              >
                {loadingAction === 'github' ? tx('跳转中...', 'Redirecting...') : tx('使用 GitHub 登录', 'Continue with GitHub')}
              </button>
              <button
                type="button"
                className="btn btn-outline btn-block"
                onClick={() => { void handleProviderAction(pocketProvider, 'pocket') }}
                disabled={busy || !pocketEnabled}
              >
                {loadingAction === 'pocket' ? tx('跳转中...', 'Redirecting...') : tx('使用 Pocket ID 登录', 'Continue with Pocket ID')}
              </button>
            </div>

            <div className="auth-divider"><span>{tx('或使用邮箱', 'or use email')}</span></div>

            <form className="login-form" onSubmit={handleEmailLogin}>
              <input className="input" type="email" required placeholder="you@example.com" value={email} onChange={(event) => setEmail(event.target.value)} />
              <input className="input" type="password" required placeholder={tx('密码', 'Password')} value={password} onChange={(event) => setPassword(event.target.value)} />
              <button className="btn btn-outline btn-block" disabled={busy}>
                {loadingAction === 'email' ? tx('登录中...', 'Signing in...') : tx('登录', 'Log in')}
              </button>
            </form>

            <p className="login-note">
              {tx('还没有账户？', 'No account yet?')} <Link to="/signup">{tx('立即开始', 'Get started')}</Link>
            </p>
          </div>
        </section>
      </div>
    </div>
  )
}

function sanitizeLoginRedirect(raw: string | null): string {
  const redirect = (raw || '').trim()
  if (!redirect) return '/'
  try {
    const target = redirect.startsWith('/') ? new URL(redirect, window.location.origin) : new URL(redirect)
    if (target.origin !== window.location.origin) return '/'
    if (target.pathname === '/login' || target.pathname === '/signup') return '/'
    return `${target.pathname}${target.search}${target.hash}`
  } catch {
    return '/'
  }
}
