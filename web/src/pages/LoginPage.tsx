import { useEffect, useState } from 'react'
import { api, type AuthProvider } from '../api'
import LanguageToggle from '../components/LanguageToggle'
import { useI18n } from '../i18n'

interface LoginPageProps {
  onLogin: (token: string, user: any) => void
}

export default function LoginPage(_: LoginPageProps) {
  const { tx } = useI18n()
  const [providers, setProviders] = useState<AuthProvider[]>([])
  const [error, setError] = useState('')
  const [loadingProvider, setLoadingProvider] = useState('')

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    setError(params.get('error') || '')

    api.getAuthProviders()
      .then((items) => {
        setProviders((items || []).filter((provider) => provider.enabled))
      })
      .catch((err: Error) => {
        setError(err.message || tx('加载登录方式失败', 'Failed to load sign-in options'))
      })
  }, [tx])

  const handleProviderLogin = async (provider: AuthProvider) => {
    try {
      setLoadingProvider(provider.id)
      setError('')
      const params = new URLSearchParams(window.location.search)
      const redirect = params.get('redirect') || '/'
      const resp = await api.startAuthProvider(provider.id, redirect)
      window.location.href = resp.authorization_url
    } catch (err: any) {
      setError(err.message || tx('启动登录失败', 'Failed to start sign-in'))
      setLoadingProvider('')
    }
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <div className="login-card-header">
          <LanguageToggle />
        </div>
        <h1 className="login-title">neuDrive</h1>
        <p className="login-desc">
          {tx('使用统一 OIDC / OAuth 登录', 'Continue with your configured identity provider')}
        </p>

        <div className="login-form">
          {error && <div className="form-error">{error}</div>}

          {providers.length === 0 ? (
            <div className="oauth-error">
              {tx('当前没有可用的登录方式，请先配置认证 Provider。', 'No sign-in providers are configured yet.')}
            </div>
          ) : (
            providers.map((provider) => (
              <button
                key={provider.id}
                type="button"
                className={`btn btn-block ${provider.id === 'github' ? 'btn-outline btn-github' : 'btn-primary'}`}
                onClick={() => handleProviderLogin(provider)}
                disabled={loadingProvider !== '' && loadingProvider !== provider.id}
              >
                {loadingProvider === provider.id
                  ? tx('跳转中...', 'Redirecting...')
                  : tx(`继续使用 ${provider.display_name}`, `Continue with ${provider.display_name}`)}
              </button>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
