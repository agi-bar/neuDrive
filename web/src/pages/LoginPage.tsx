import { useEffect, useState } from 'react'
import { api, type AuthProvider } from '../api'
import LanguageToggle from '../components/LanguageToggle'
import { useI18n } from '../i18n'

export default function LoginPage() {
  const { tx } = useI18n()
  const [providers, setProviders] = useState<AuthProvider[]>([])
  const [error, setError] = useState('')
  const [loadingAction, setLoadingAction] = useState('')

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    setError(params.get('error') || '')

    api.getAuthProviders()
      .then((items) => {
        setProviders(items || [])
      })
      .catch((err: Error) => {
        setError(err.message || tx('加载登录方式失败', 'Failed to load sign-in options'))
      })
  }, [tx])

  const githubProvider = providers.find((provider) => provider.id === 'github')
  const pocketProvider = providers.find((provider) => provider.kind === 'oidc')
  const pocketEnabled = !!pocketProvider?.enabled
  const githubEnabled = !!githubProvider?.enabled
  const busy = loadingAction !== ''
  const providerHints = [
    !pocketEnabled ? tx('Pocket ID 登录和注册当前不可用。', 'Pocket ID login and signup are unavailable right now.') : '',
    !githubEnabled ? tx('GitHub 登录当前不可用。', 'GitHub login is unavailable right now.') : '',
  ].filter(Boolean)

  const handleProviderAction = async (
    provider: AuthProvider | undefined,
    action: 'login' | 'signup',
    loadingKey: string,
  ) => {
    if (!provider?.enabled) return

    try {
      setLoadingAction(loadingKey)
      setError('')
      const params = new URLSearchParams(window.location.search)
      const redirect = sanitizeLoginRedirect(params.get('redirect'))
      const resp = await api.startAuthProvider(provider.id, redirect, action)
      window.location.assign(resp.authorization_url)
    } catch (err: any) {
      setError(err.message || tx('启动登录失败', 'Failed to start sign-in'))
      setLoadingAction('')
    }
  }

  return (
    <div className="login-page">
      <div className="login-shell">
        <section className="login-hero">
          <div className="login-hero-copy">
            <h1 className="login-hero-title">neuDrive</h1>
            <p className="login-hero-slogan">
              {tx('一个 Hub，连接你所有的 AI Agent', 'One hub for all your AI agents')}
            </p>
            <p className="login-hero-subtitle">
              {tx('统一身份、记忆、技能与连接。', 'Identity, memory, skills, and connections in one place.')}
            </p>
          </div>
        </section>

        <section className="login-panel">
          <div className="login-panel-card">
            <div className="login-panel-header">
              <LanguageToggle />
            </div>

            {error && <div className="form-error login-panel-error">{error}</div>}

            <div className="login-actions">
              <button
                type="button"
                className="btn btn-primary btn-block"
                onClick={() => handleProviderAction(pocketProvider, 'login', 'pocket-login')}
                disabled={busy || !pocketEnabled}
              >
                {loadingAction === 'pocket-login' ? tx('跳转中...', 'Redirecting...') : tx('登录', 'Login')}
              </button>

              <button
                type="button"
                className="btn btn-outline btn-block"
                onClick={() => handleProviderAction(pocketProvider, 'signup', 'pocket-signup')}
                disabled={busy || !pocketEnabled}
              >
                {loadingAction === 'pocket-signup' ? tx('跳转中...', 'Redirecting...') : tx('注册', 'Sign up')}
              </button>

              <div className="auth-divider">
                <span>{tx('或', 'or')}</span>
              </div>

              <button
                type="button"
                className="btn btn-block btn-github"
                onClick={() => handleProviderAction(githubProvider, 'login', 'github-login')}
                disabled={busy || !githubEnabled}
              >
                {loadingAction === 'github-login'
                  ? tx('跳转中...', 'Redirecting...')
                  : tx('使用 GitHub 登录', 'Login with GitHub')}
              </button>
            </div>

            {providerHints.length > 0 && (
              <div className="login-provider-status">
                {providerHints.map((hint) => (
                  <p key={hint} className="login-provider-note">{hint}</p>
                ))}
              </div>
            )}
          </div>
        </section>
      </div>
    </div>
  )
}

function sanitizeLoginRedirect(raw: string | null): string {
  const redirect = (raw || '').trim()
  if (!redirect) {
    return '/'
  }

  try {
    const target = redirect.startsWith('/')
      ? new URL(redirect, window.location.origin)
      : new URL(redirect)
    const sameOrigin = target.origin === window.location.origin
    const path = target.pathname
    if (
      sameOrigin &&
      (
        path === '/login' ||
        (path.startsWith('/api/auth/providers/') && path.endsWith('/callback'))
      )
    ) {
      return '/'
    }
  } catch {
    return '/'
  }

  return redirect
}
