import { useState, useEffect } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import LanguageToggle from '../components/LanguageToggle'
import { useI18n } from '../i18n'

interface ScopeInfo {
  scope: string
  label: string
}

interface AppInfo {
  app_name: string
  app_logo: string
  scopes: ScopeInfo[]
  client_id: string
  redirect_uri: string
}

export default function OAuthAuthorizePage() {
  const { tx } = useI18n()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()

  const [appInfo, setAppInfo] = useState<AppInfo | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [userName, setUserName] = useState('')

  const clientId = searchParams.get('client_id') || ''
  const redirectUri = searchParams.get('redirect_uri') || ''
  const scope = searchParams.get('scope') || ''
  const state = searchParams.get('state') || ''
  const responseType = searchParams.get('response_type') || ''

  useEffect(() => {
    const token = localStorage.getItem('token')

    if (!token) {
      // Not logged in — redirect to login with return URL
      navigate('/login?redirect=' + encodeURIComponent(window.location.href), { replace: true })
      return
    }

    // Verify token
    fetch('/api/auth/me', {
      headers: { Authorization: 'Bearer ' + token },
    })
      .then((res) => {
        if (res.status !== 200) {
          localStorage.removeItem('token')
          navigate('/login?redirect=' + encodeURIComponent(window.location.href), { replace: true })
          return null
        }
        return res.json()
      })
      .then((data) => {
        if (!data) return
        setUserName(data.display_name || data.name || data.slug || data.email || '')
      })
      .catch(() => {
        localStorage.removeItem('token')
        navigate('/login?redirect=' + encodeURIComponent(window.location.href), { replace: true })
      })

    // Fetch app info
    const params = new URLSearchParams({
      client_id: clientId,
      redirect_uri: redirectUri,
      scope,
      response_type: responseType,
    })
    fetch('/api/oauth/authorize-info?' + params.toString())
      .then((res) => res.json())
      .then((body) => {
        if (body.ok === false || body.code) {
          setError(body.message || tx('加载应用信息失败', 'Failed to load application info'))
        } else {
          setAppInfo(body.data || body)
        }
      })
      .catch(() => setError(tx('加载应用信息失败', 'Failed to load application info')))
      .finally(() => setLoading(false))
  }, [clientId, redirectUri, scope, state, responseType, navigate, tx])

  const handleAuthorize = () => {
    setSubmitting(true)
    const token = localStorage.getItem('token') || ''

    // Submit via form POST
    const form = document.createElement('form')
    form.method = 'POST'
    form.action = '/oauth/authorize'

    const fields: Record<string, string> = {
      client_id: clientId,
      redirect_uri: redirectUri,
      scope,
      state,
      action: 'approve',
      _token: token,
    }

    for (const [name, value] of Object.entries(fields)) {
      const input = document.createElement('input')
      input.type = 'hidden'
      input.name = name
      input.value = value
      form.appendChild(input)
    }

    document.body.appendChild(form)
    form.submit()
  }

  const handleDeny = () => {
    const url = new URL(redirectUri)
    url.searchParams.set('error', 'access_denied')
    if (state) url.searchParams.set('state', state)
    window.location.href = url.toString()
  }

  if (loading) {
    return (
      <div className="oauth-page">
        <div className="oauth-card">
          <div className="oauth-card-header">
            <LanguageToggle />
          </div>
          <div className="oauth-loading">{tx('加载中...', 'Loading...')}</div>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="oauth-page">
        <div className="oauth-card">
          <div className="oauth-card-header">
            <LanguageToggle />
          </div>
          <h1 className="oauth-title">Agent Hub</h1>
          <div className="oauth-error">{error}</div>
        </div>
      </div>
    )
  }

  return (
    <div className="oauth-page">
      <div className="oauth-card">
        <div className="oauth-card-header">
          <LanguageToggle />
        </div>
        <h1 className="oauth-title">Agent Hub</h1>
        <p className="oauth-subtitle">{tx('有应用正在请求访问你的账号', 'An application is requesting access to your account')}</p>

        {appInfo && (
          <>
            <div className="oauth-app-info">
              <div className="oauth-app-logo">
                {appInfo.app_logo ? (
                  <img src={appInfo.app_logo} alt={appInfo.app_name} />
                ) : (
                  <span>&#x1f916;</span>
                )}
              </div>
              <div>
                <div className="oauth-app-name">{appInfo.app_name}</div>
                <div className="oauth-app-sub">{tx('想要访问你的 Agent Hub 账号', 'wants to access your Agent Hub account')}</div>
              </div>
            </div>

            {appInfo.scopes && appInfo.scopes.length > 0 && (
              <div className="oauth-scopes">
                <h3>{tx('该应用将可以：', 'This application will be able to:')}</h3>
                {appInfo.scopes.map((s) => (
                  <div key={s.scope} className="oauth-scope-item">
                    <span className="oauth-scope-check">&#10003;</span>
                    <span>{s.label}</span>
                  </div>
                ))}
              </div>
            )}
          </>
        )}

        {userName && (
          <div className="oauth-user-status">
            &#10003; {tx('当前登录为', 'Logged in as')} <strong>{userName}</strong>
          </div>
        )}

        <div className="oauth-actions">
          <button className="btn btn-outline oauth-btn-deny" onClick={handleDeny} disabled={submitting}>
            {tx('拒绝', 'Deny')}
          </button>
          <button className="btn btn-primary oauth-btn-approve" onClick={handleAuthorize} disabled={submitting}>
            {submitting ? tx('授权中...', 'Authorizing...') : tx('授权', 'Authorize')}
          </button>
        </div>
      </div>
    </div>
  )
}
