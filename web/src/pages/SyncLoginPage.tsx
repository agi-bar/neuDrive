import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type SyncTokenResponse } from '../api'
import LanguageToggle from '../components/LanguageToggle'
import { useI18n } from '../i18n'
import { formatDateTime } from './data/DataShared'

type CLIAccess = 'push' | 'pull' | 'both'

interface CLILoginRequest {
  callback: string
  state: string
  profile: string
  access: CLIAccess
  ttlMinutes: number
}

function accessLabel(access: CLIAccess, locale: 'zh-CN' | 'en') {
  switch (access) {
    case 'push':
      return locale === 'zh-CN' ? '仅上传到 Hub' : 'Push to Hub only'
    case 'pull':
      return locale === 'zh-CN' ? '仅从 Hub 拉取' : 'Pull from Hub only'
    default:
      return locale === 'zh-CN' ? '上传和拉取' : 'Push and pull'
  }
}

function parseCLILoginRequest(search: string, locale: 'zh-CN' | 'en'): { request: CLILoginRequest | null; error: string } {
  const params = new URLSearchParams(search)
  if (params.get('cli_login') !== '1') {
    return {
      request: null,
      error: locale === 'zh-CN'
        ? '这个页面需要由 `agenthub sync login` 自动打开。'
        : 'This page must be opened automatically by `agenthub sync login`.',
    }
  }

  const callback = params.get('cli_callback') || ''
  const state = params.get('cli_state') || ''
  const profile = params.get('cli_profile') || 'default'
  const access = (params.get('cli_access') || 'both') as CLIAccess
  const ttl = Number(params.get('cli_ttl_minutes') || 30)

  if (!callback || !state) {
    return {
      request: null,
      error: locale === 'zh-CN'
        ? '缺少必要的 CLI 登录参数，请回到终端重新执行 `agenthub sync login`。'
        : 'Missing required CLI login parameters. Please rerun `agenthub sync login` in the terminal.',
    }
  }

  if (!['push', 'pull', 'both'].includes(access)) {
    return {
      request: null,
      error: locale === 'zh-CN'
        ? '无效的访问范围参数，请回到终端重新执行 `agenthub sync login`。'
        : 'Invalid access scope parameter. Please rerun `agenthub sync login` in the terminal.',
    }
  }

  try {
    const callbackURL = new URL(callback)
    if (!['http:', 'https:'].includes(callbackURL.protocol) || !['127.0.0.1', 'localhost'].includes(callbackURL.hostname)) {
      return {
        request: null,
        error: locale === 'zh-CN'
          ? 'CLI 回调地址不是本机地址，已拒绝此次登录请求。'
          : 'The CLI callback URL is not local, so this login request was rejected.',
      }
    }
  } catch {
    return {
      request: null,
      error: locale === 'zh-CN'
        ? 'CLI 回调地址无效，请回到终端重新执行 `agenthub sync login`。'
        : 'The CLI callback URL is invalid. Please rerun `agenthub sync login` in the terminal.',
    }
  }

  return {
    request: {
      callback,
      state,
      profile,
      access,
      ttlMinutes: Number.isFinite(ttl) && ttl > 0 ? Math.max(5, Math.min(120, ttl)) : 30,
    },
    error: '',
  }
}

export default function SyncLoginPage() {
  const { locale, tx } = useI18n()
  const [syncToken, setSyncToken] = useState<SyncTokenResponse | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  const cliLogin = useMemo(() => parseCLILoginRequest(window.location.search, locale), [locale])

  const handleAuthorize = async () => {
    if (!cliLogin.request) return
    setBusy(true)
    setError('')
    setMessage('')

    try {
      const created = await api.createSyncToken({
        access: cliLogin.request.access,
        ttl_minutes: cliLogin.request.ttlMinutes,
      })
      setSyncToken(created)

      const callbackRes = await fetch(cliLogin.request.callback, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          state: cliLogin.request.state,
          profile: cliLogin.request.profile,
          token: created.token,
          expires_at: created.expires_at,
          api_base: created.api_base,
          scopes: created.scopes,
          usage: created.usage,
        }),
      })

      if (!callbackRes.ok) {
        throw new Error(tx(
          'Sync Token 已生成，但回填本地 CLI 失败。请确认终端还在等待登录，或改用手工 `agenthub sync login --token ...`。',
          'The Sync token was created, but sending it back to the local CLI failed. Make sure the terminal is still waiting, or use `agenthub sync login --token ...` manually.',
        ))
      }

      setMessage(tx(
        `已把 Sync Token 发回本地 CLI profile：${cliLogin.request.profile}。现在可以回到终端继续。`,
        `The Sync token was sent back to local CLI profile ${cliLogin.request.profile}. You can return to the terminal now.`,
      ))
    } catch (err: any) {
      setError(err.message || tx('授权 CLI 登录失败', 'CLI authorization failed'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="sync-login-page">
      <div className="sync-login-card">
        <div className="login-card-header">
          <LanguageToggle />
        </div>
        <div className="sync-login-eyebrow">Agent Hub CLI</div>
        <h1 className="login-title">{tx('授权本次 Sync 登录', 'Authorize this Sync sign-in')}</h1>
        <p className="login-desc">
          {tx(
            '这个页面只处理当前这一次 CLI 登录，不再混进完整的 Dashboard 管理界面。',
            'This page only handles the current CLI sign-in, without routing through the full dashboard UI.',
          )}
        </p>

        {cliLogin.error && (
          <div className="alert alert-error">
            {cliLogin.error}
          </div>
        )}

        {cliLogin.request && (
          <>
            <div className="sync-login-summary">
              <div className="sync-login-summary-row">
                <span>{tx('保存到 profile', 'Save to profile')}</span>
                <strong>{cliLogin.request.profile}</strong>
              </div>
              <div className="sync-login-summary-row">
                <span>{tx('授权范围', 'Access')}</span>
                <strong>{accessLabel(cliLogin.request.access, locale)}</strong>
              </div>
              <div className="sync-login-summary-row">
                <span>{tx('有效期', 'Expires in')}</span>
                <strong>{tx(`${cliLogin.request.ttlMinutes} 分钟`, `${cliLogin.request.ttlMinutes} minutes`)}</strong>
              </div>
            </div>

            <div className="sync-login-note">
              {tx(
                '点击下面的按钮后，Agent Hub 会生成一个短效 Sync Token，并自动发送回正在等待的本地 CLI。',
                'When you continue, Agent Hub will create a short-lived Sync token and automatically send it back to the waiting local CLI.',
              )}
            </div>

            <div className="sync-login-actions">
              <button className="btn btn-primary" disabled={busy} onClick={() => { void handleAuthorize() }}>
                {busy ? tx('授权中...', 'Authorizing...') : tx('继续并授权 CLI', 'Continue and authorize CLI')}
              </button>
              <Link to="/data/sync" className="btn">
                {tx('打开同步管理页', 'Open Sync page')}
              </Link>
            </div>
          </>
        )}

        {message && <div className="alert alert-success">{message}</div>}
        {error && <div className="alert alert-error">{error}</div>}

        {syncToken && error && (
          <div className="data-sync-token-box">
            <div className="data-record-title">{tx('手工回填备用 Token', 'Manual fallback token')}</div>
            <code className="data-sync-token">{syncToken.token}</code>
            <div className="data-record-meta">{tx('过期时间：', 'Expires at: ')}{formatDateTime(syncToken.expires_at, locale)}</div>
            <div className="data-record-secondary">
              {tx('如果终端还在等待，也可以手工执行：', 'If the terminal is still waiting, you can also run:')}
              <code> agenthub sync login --api-base {window.location.origin} --token &lt;PASTE_TOKEN&gt;</code>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
