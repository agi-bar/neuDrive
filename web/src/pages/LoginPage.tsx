import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, type AuthProvider } from '../api'
import { useI18n } from '../i18n'
import { PublicShell } from './PublicPages'

export default function LoginPage() {
  const { tx } = useI18n()
  const navigate = useNavigate()
  const [providers, setProviders] = useState<AuthProvider[]>([])
  const [error, setError] = useState('')
  const [identifier, setIdentifier] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [loadingAction, setLoadingAction] = useState('')

  useEffect(() => {
    document.title = tx('登录 — neuDrive', 'Log in — neuDrive')
  }, [tx])

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
  const hasExternalProviders = githubEnabled || pocketEnabled
  const providerMessages = useMemo(() => {
    const messages: string[] = []
    if (!pocketEnabled) messages.push(tx('Pocket ID 登录当前不可用。', 'Pocket ID login is unavailable right now.'))
    if (!githubEnabled) messages.push(tx('GitHub 登录当前不可用。', 'GitHub login is unavailable right now.'))
    return messages
  }, [githubEnabled, pocketEnabled, tx])

  const redirectTarget = () => {
    const params = new URLSearchParams(window.location.search)
    return sanitizeLoginRedirect(params.get('redirect'))
  }

  const persistAuth = (accessToken: string, refreshToken?: string) => {
    localStorage.setItem('token', accessToken)
    if (refreshToken) localStorage.setItem('refresh_token', refreshToken)
    else localStorage.removeItem('refresh_token')
  }

  const handlePasswordLogin = async () => {
    if (!identifier.trim() || !password) {
      setError(tx('请填写用户名或邮箱，以及密码。', 'Enter your username or email, and password.'))
      return
    }
    setSubmitting(true)
    setError('')
    try {
      const auth = await api.login({ identifier: identifier.trim(), email: identifier.trim(), password })
      persistAuth(auth.access_token, auth.refresh_token)
      navigate(redirectTarget(), { replace: true })
    } catch (err: any) {
      setError(err?.message || tx('登录失败', 'Sign-in failed'))
    } finally {
      setSubmitting(false)
    }
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

  return (
    <PublicShell>
      <main className="auth-split">
        <section className="auth-copy">
          <p className="public-kicker">{tx('欢迎回来', 'Welcome back')}</p>
          <h1>{tx('回到你的 AI 工作资料层。', 'Return to your AI workspace layer.')}</h1>
          <p>{tx('登录后继续管理 neuDrive 里的数据、接入方式和开发者访问。', 'Sign in to manage your neuDrive data, integrations, and developer access.')}</p>
        </section>
        <section className="auth-card">
          <h1 className="login-title">{tx('登录 neuDrive', 'Log in to neuDrive')}</h1>
          <p className="login-desc">{tx('使用用户名或邮箱进入产品。', 'Use your username or email to access the product.')}</p>
          {error && <div className="alert alert-warn">{error}</div>}

          <div className="login-form-fields">
            <label className="form-field">
              <span>{tx('用户名或邮箱', 'Username or email')}</span>
              <input
                type="text"
                value={identifier}
                onChange={(event) => setIdentifier(event.target.value)}
                placeholder={tx('例如：alice 或 alice@example.com', 'For example: alice or alice@example.com')}
                autoComplete="username"
              />
            </label>
            <label className="form-field">
              <span>{tx('密码', 'Password')}</span>
              <input
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder={tx('输入密码', 'Enter your password')}
                autoComplete="current-password"
              />
            </label>
            <button type="button" className="btn btn-primary btn-block" disabled={submitting} onClick={() => { void handlePasswordLogin() }}>
              {submitting ? tx('登录中...', 'Signing in...') : tx('登录', 'Log in')}
            </button>
          </div>

          {providerMessages.length > 0 && (
            <div className="login-provider-note">
              {providerMessages.map((message) => (
                <p key={message}>{message}</p>
              ))}
            </div>
          )}

          {hasExternalProviders && (
            <div className="login-actions">
              {pocketEnabled && (
                <button
                  type="button"
                  className="btn btn-outline btn-block"
                  onClick={() => { void handleProviderAction(pocketProvider, 'pocket') }}
                  disabled={busy}
                >
                  {loadingAction === 'pocket' ? tx('跳转中...', 'Redirecting...') : tx('使用 Pocket ID 登录', 'Continue with Pocket ID')}
                </button>
              )}
              {githubEnabled && (
                <button
                  type="button"
                  className="btn btn-outline btn-block"
                  onClick={() => { void handleProviderAction(githubProvider, 'github') }}
                  disabled={busy}
                >
                  {loadingAction === 'github' ? tx('跳转中...', 'Redirecting...') : tx('使用 GitHub 登录', 'Continue with GitHub')}
                </button>
              )}
            </div>
          )}

          <p className="login-note">
            {tx('还没有账户？', 'No account yet?')} <Link to="/signup">{tx('创建账号', 'Create account')}</Link>
          </p>
        </section>
      </main>
    </PublicShell>
  )
}

function sanitizeLoginRedirect(raw: string | null): string {
  const redirect = (raw || '').trim()
  if (!redirect) return '/'
  try {
    const target = redirect.startsWith('/') ? new URL(redirect, window.location.origin) : new URL(redirect)
    if (target.origin !== window.location.origin) return '/'
    if (target.pathname === '/login' || target.pathname === '/signup') return '/'
    if (isStaticAssetPath(target.pathname)) return '/'
    return `${target.pathname}${target.search}${target.hash}`
  } catch {
    return '/'
  }
}

function isStaticAssetPath(pathname: string) {
  if (pathname.startsWith('/assets/')) return true
  if (pathname === '/favicon.ico' || pathname.startsWith('/favicon-') || pathname === '/apple-touch-icon.png') return true
  if (pathname === '/logo-mark.png' || pathname === '/logo-social.png') return true
  return pathname === '/robots.txt' || pathname === '/sitemap.xml'
}
