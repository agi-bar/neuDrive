import { useEffect, useMemo, useState } from 'react'
import {
  api,
  type GitMirrorGitHubTestResult,
  type GitMirrorSettings,
  type PublicConfig,
  type UpdateGitMirrorRequest,
} from '../api'
import { useI18n } from '../i18n'
import { formatDateTime, localizeGitHubAccessMessage } from './data/DataShared'

type AuthMode = UpdateGitMirrorRequest['auth_mode']
type AuthHelp = {
  title: string
  intro: string
  steps?: string[]
  footer?: string
}

const DEFAULT_AUTH_MODE: AuthMode = 'local_credentials'
const DEFAULT_REMOTE_NAME = 'origin'
const DEFAULT_REMOTE_BRANCH = 'main'
const GITHUB_APP_PERMISSION_UPDATE_REQUIRED_CODE = 'github_app_permission_update_required'

function githubRepoLabel(remoteURL?: string) {
  const value = (remoteURL || '').trim()
  if (!value) return ''
  const sshMatch = value.match(/^git@github\.com:([^/]+\/[^.]+)(?:\.git)?$/)
  if (sshMatch) return sshMatch[1]
  const httpsMatch = value.match(/^https:\/\/github\.com\/([^/]+\/[^.]+)(?:\.git)?$/)
  if (httpsMatch) return httpsMatch[1]
  return value
}

function githubRepoHref(remoteURL?: string) {
  const label = githubRepoLabel(remoteURL)
  if (!label || label === remoteURL) return remoteURL || ''
  return `https://github.com/${label.replace(/\.git$/, '')}`
}

function latestUpdateTime(mirror: GitMirrorSettings | null) {
  return mirror?.last_push_at || mirror?.last_synced_at || ''
}

function isGitHubHTTPSURL(value: string) {
  try {
    const parsed = new URL(value.trim())
    return (parsed.protocol === 'https:' || parsed.protocol === 'http:') && parsed.hostname.toLowerCase() === 'github.com'
  } catch {
    return false
  }
}

function authModeForExecution(mode: AuthMode | undefined, executionMode: string, tokenConfigured?: boolean): AuthMode {
  if (executionMode === 'hosted' && mode === 'local_credentials') {
    return tokenConfigured ? 'github_token' : 'github_app_user'
  }
  return mode || (executionMode === 'hosted' ? 'github_app_user' : DEFAULT_AUTH_MODE)
}

function delay(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms))
}

function isGitHubAppPermissionUpdateRequiredError(err: any) {
  const message = String(err?.message || '')
  return err?.code === GITHUB_APP_PERMISSION_UPDATE_REQUIRED_CODE ||
    /Repository Administration/i.test(message) ||
    /Resource not accessible by integration/i.test(message)
}

function PencilIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" d="m4 20 4.6-1.1L19.5 8a2.1 2.1 0 0 0 0-3L19 4.5a2.1 2.1 0 0 0-3 0L5.1 15.4 4 20Z" />
      <path fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" d="m14.5 6.5 3 3" />
    </svg>
  )
}

function InfoIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" strokeWidth="2" />
      <path fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" d="M9.8 9a2.4 2.4 0 0 1 4.6 1c0 1.8-2.4 2-2.4 3.7" />
      <path fill="currentColor" d="M12 17.8a1.1 1.1 0 1 0 0-2.2 1.1 1.1 0 0 0 0 2.2Z" />
    </svg>
  )
}

export default function GitMirrorPage() {
  const { locale, tx } = useI18n()
  const [mirror, setMirror] = useState<GitMirrorSettings | null>(null)
  const [publicConfig, setPublicConfig] = useState<PublicConfig | null>(null)
  const [busy, setBusy] = useState(true)
  const [working, setWorking] = useState(false)
  const [testing, setTesting] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [githubAppError, setGithubAppError] = useState('')
  const [githubAppReauthRequired, setGithubAppReauthRequired] = useState(false)
  const [authMode, setAuthMode] = useState<AuthMode>(DEFAULT_AUTH_MODE)
  const [authHelpOpen, setAuthHelpOpen] = useState(false)
  const [remoteURL, setRemoteURL] = useState('')
  const [urlEditing, setUrlEditing] = useState(false)
  const [tokenInput, setTokenInput] = useState('')
  const [tokenEditing, setTokenEditing] = useState(false)
  const [tokenTest, setTokenTest] = useState<GitMirrorGitHubTestResult | null>(null)
  const [syncRetryUntil, setSyncRetryUntil] = useState(0)
  const [nowTick, setNowTick] = useState(Date.now())

  const loadPage = async () => {
    setBusy(true)
    setError('')
    try {
      const [settings, config] = await Promise.all([
        api.getGitMirror(),
        api.getPublicConfig().catch(() => ({} as PublicConfig)),
      ])
      const nextExecutionMode = settings.execution_mode || config.git_mirror_execution_mode || 'hosted'
      setMirror(settings)
      setPublicConfig(config)
      if (settings.github_app_user_connected) {
        setGithubAppReauthRequired(false)
      }
      setAuthMode(authModeForExecution(settings.auth_mode, nextExecutionMode, settings.github_token_configured))
      setRemoteURL(settings.remote_url || '')
      setUrlEditing(!(settings.remote_url || '').trim() && settings.auth_mode !== 'github_app_user')
      setTokenInput('')
      setTokenEditing(!settings.github_token_configured)
      setTokenTest(null)
    } catch (err: any) {
      setError(err.message || tx('加载 GitHub Backup 失败', 'Failed to load GitHub Backup'))
    } finally {
      setBusy(false)
    }
  }

  useEffect(() => {
    void loadPage()
  }, [])

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const status = params.get('github_app_status')
    const callbackError = params.get('github_app_error')
    if (!status && !callbackError) return
    if (callbackError) {
      setGithubAppError(callbackError)
    } else if (status === 'connected') {
      setGithubAppReauthRequired(false)
      setMessage(tx('GitHub 已连接。现在可以创建备份仓库。', 'GitHub connected. You can create the backup repository now.'))
    }
    void loadPage()
    params.delete('github_app_status')
    params.delete('github_app_error')
    const nextSearch = params.toString()
    window.history.replaceState({}, '', `${window.location.pathname}${nextSearch ? `?${nextSearch}` : ''}`)
  }, [tx])

  useEffect(() => {
    if (!syncRetryUntil) return
    const timer = window.setInterval(() => setNowTick(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [syncRetryUntil])

  const executionMode = mirror?.execution_mode || publicConfig?.git_mirror_execution_mode || 'hosted'
  const isLocalExecution = executionMode === 'local'
  const githubAppConfigured = !!publicConfig?.github_app_enabled
  const githubAppUnavailableMessage = tx(
    '当前服务没有配置 GitHub App 授权。请设置 GITHUB_APP_CLIENT_ID、GITHUB_APP_CLIENT_SECRET、GITHUB_APP_SLUG、PUBLIC_BASE_URL 和 JWT_SECRET 后重启服务。',
    'GitHub App authorization is not configured for this service. Set GITHUB_APP_CLIENT_ID, GITHUB_APP_CLIENT_SECRET, GITHUB_APP_SLUG, PUBLIC_BASE_URL, and JWT_SECRET, then restart the service.',
  )
  const remoteRepoURL = mirror?.remote_url || ''
  const lastUpdate = latestUpdateTime(mirror)
  const authHelp: AuthHelp = authMode === 'local_credentials'
    ? {
        title: tx('本机 Git 凭证', 'Local Git credentials'),
        intro: tx(
          '仅本机模式适用。neuDrive 会复用这台机器已有的 Git SSH key 或 credential helper，适合你已经在本机配置好 GitHub 推送权限的情况。',
          'Available in local mode only. neuDrive reuses this machine’s Git SSH key or credential helper, which is best when GitHub push access is already configured locally.',
        ),
      }
    : authMode === 'github_token'
      ? {
          title: tx('GitHub Token', 'GitHub token'),
          intro: tx(
            '适合你想自己创建备份仓库，并把权限精确控制在某一个仓库里的情况。',
            'Use this when you want to create the backup repository yourself and keep the token scoped to that single repository.',
          ),
          steps: [
            tx('先在 GitHub 新建一个私有仓库，例如 neudrive-backup。', 'Create a private GitHub repository first, for example neudrive-backup.'),
            tx('进入 GitHub Settings → Developer settings → Personal access tokens → Fine-grained tokens，创建新 token。', 'Go to GitHub Settings → Developer settings → Personal access tokens → Fine-grained tokens, then generate a new token.'),
            tx('Repository access 只选择这个备份仓库。', 'For Repository access, select only this backup repository.'),
            tx('Repository permissions 里给 Contents 读写权限；Metadata 保持默认只读。', 'For Repository permissions, set Contents to Read and write; keep Metadata at the default read-only access.'),
            tx('回到这里填写仓库 URL 和 token，先测试 token，通过后保存。', 'Come back here, enter the repository URL and token, test the token, then save after it passes.'),
          ],
          footer: tx(
            'Token 只显示一次，neuDrive 会加密保存，不会在页面回显原值。',
            'GitHub only shows the token once. neuDrive stores it encrypted and will not show the raw value again.',
          ),
        }
      : {
          title: tx('GitHub App 授权', 'GitHub App authorization'),
          intro: tx(
            '推荐的一键方式。连接 GitHub 后，neuDrive 会自动创建或复用你的私有 neudrive-backup 仓库，不需要你手动保存 token。',
            'Recommended for the easiest setup. After you connect GitHub, neuDrive creates or reuses your private neudrive-backup repository without asking you to manage a token.',
          ),
          steps: [
            tx('点击连接 GitHub，按 GitHub 页面完成 App 安装和用户授权。', 'Click Connect GitHub, then finish the App installation and user authorization on GitHub.'),
            tx('回到 neuDrive 后点击创建私有备份仓库。', 'Back in neuDrive, click Create private backup repo.'),
            tx('之后点击立即同步，后台会把 neuDrive 数据推送到这个仓库。', 'Then click Sync now; the worker pushes your neuDrive data to that repository.'),
          ],
          footer: tx(
            '如果 GitHub 提示批准新权限，批准后再回到这里重试。',
            'If GitHub asks you to approve updated permissions, approve them and then retry here.',
          ),
        }
  const retrySeconds = useMemo(() => {
    if (!syncRetryUntil) return 0
    return Math.max(0, Math.ceil((syncRetryUntil - nowTick) / 1000))
  }, [nowTick, syncRetryUntil])
  const syncCooldownSeconds = Math.max(0, Number(publicConfig?.git_mirror_manual_sync_cooldown_seconds || 0))
  const selectedAuthCanSync = authMode === 'github_app_user'
    ? !!mirror?.github_app_user_connected
    : authMode === 'github_token'
      ? !!mirror?.github_token_configured
      : true
  const syncDisabled = working || retrySeconds > 0 || !remoteRepoURL
  const destinationConfigured = !!remoteRepoURL
  const syncAvailable = destinationConfigured && selectedAuthCanSync
  const tokenModeNeedsTokenBeforeSave = authMode === 'github_token' && !mirror?.github_token_configured && !tokenInput.trim()
  const remotePlaceholder = authMode === 'local_credentials'
    ? 'git@github.com:owner/neudrive-backup.git'
    : 'https://github.com/owner/neudrive-backup.git'

  const setInlineMessage = (nextMessage: string) => {
    setMessage(nextMessage)
    setError('')
    setGithubAppError('')
  }

  const setInlineError = (nextError: string) => {
    setError(nextError)
    setMessage('')
  }

  const hostedSyncSettledMessage = (settings: GitMirrorSettings) => {
    if (settings.sync_state === 'error' || settings.remote_conflict || settings.last_error || settings.last_push_error) {
      return settings.message || tx('最近一次后台同步需要处理。', 'The latest background sync needs attention.')
    }
    const requestedAt = Date.parse(settings.sync_requested_at || '')
    const lastSyncedAt = Date.parse(settings.last_synced_at || '')
    const lastCommitAt = Date.parse(settings.last_commit_at || '')
    const requestCompleted = Number.isFinite(requestedAt) && Number.isFinite(lastSyncedAt) && lastSyncedAt >= requestedAt
    const noNewCommit = requestCompleted && (!Number.isFinite(lastCommitAt) || lastCommitAt < requestedAt)
    if (noNewCommit) {
      return tx(
        '已检查，没有新的变更需要提交；备份仓库已是最新状态。',
        'Checked: no new changes to commit; the backup repository is up to date.',
      )
    }
    return settings.message || tx('后台同步已完成。', 'Background sync completed.')
  }

  const pollHostedSyncUntilSettled = async () => {
    for (let attempt = 0; attempt < 12; attempt += 1) {
      await delay(1500)
      const next = await api.getGitMirror()
      setMirror(next)
      const state = next.sync_state || 'idle'
      if (state !== 'queued' && state !== 'running') {
        setInlineMessage(hostedSyncSettledMessage(next))
        return
      }
    }
  }

  const handleConnectGitHubApp = async () => {
    setError('')
    setMessage('')
    setGithubAppError('')
    if (!githubAppConfigured) {
      setGithubAppError(githubAppUnavailableMessage)
      return
    }
    setWorking(true)
    try {
      const result = await api.startGitMirrorGitHubAppBrowser(window.location.pathname)
      window.location.assign(result.authorization_url)
    } catch (err: any) {
      setGithubAppError(err.message || tx('启动 GitHub 授权失败', 'Failed to start GitHub authorization'))
    } finally {
      setWorking(false)
    }
  }

  const handleCreateDefaultBackupRepo = async () => {
    setError('')
    setMessage('')
    setGithubAppError('')
    if (!githubAppConfigured) {
      setGithubAppError(githubAppUnavailableMessage)
      return
    }
    setWorking(true)
    try {
      const result = await api.createDefaultGitMirrorGitHubAppBackupRepo()
      setMirror(result.settings)
      setAuthMode(result.settings.auth_mode || 'github_app_user')
      setRemoteURL(result.settings.remote_url || '')
      setUrlEditing(false)
      setGithubAppReauthRequired(false)
      setInlineMessage(tx('备份仓库已准备好。', 'Backup repository is ready.'))
    } catch (err: any) {
      if (isGitHubAppPermissionUpdateRequiredError(err)) {
        await loadPage()
        setAuthMode('github_app_user')
        setGithubAppReauthRequired(true)
        setGithubAppError(tx(
          'GitHub App 权限已更新，我们已自动断开旧的 GitHub Backup 授权。请先在 GitHub 批准新的 Repository Administration 读写权限，然后回到这里重新连接。',
          'GitHub App permissions were updated, so we disconnected the old GitHub Backup authorization. Approve the new Repository Administration read/write permission on GitHub, then reconnect here.',
        ))
        return
      }
      setInlineError(err.message || tx('创建备份仓库失败', 'Failed to create the backup repository'))
    } finally {
      setWorking(false)
    }
  }

  const handleDisconnectGitHubApp = async () => {
    setWorking(true)
    setError('')
    setMessage('')
    setGithubAppError('')
    try {
      await api.disconnectGitMirrorGitHubAppUser()
      await loadPage()
      setGithubAppReauthRequired(false)
      setInlineMessage(tx('GitHub 已断开连接', 'GitHub disconnected'))
    } catch (err: any) {
      setInlineError(err.message || tx('断开 GitHub 失败', 'Failed to disconnect GitHub'))
    } finally {
      setWorking(false)
    }
  }

  const handleSaveLocalDestination = async () => {
    if (!remoteURL.trim()) {
      setInlineError(tx('请填写 GitHub 仓库 URL。', 'Enter a GitHub repository URL.'))
      return
    }
    if (authMode === 'local_credentials' && isGitHubHTTPSURL(remoteURL)) {
      setInlineError(tx(
        '本机 Git 凭证模式请填写 git@github.com:owner/repo.git 形式的仓库 URL。',
        'Local Git credentials require a repository URL in the form git@github.com:owner/repo.git.',
      ))
      setUrlEditing(true)
      return
    }
    setWorking(true)
    setError('')
    setMessage('')
    try {
      const saved = await api.updateGitMirror({
        auto_commit_enabled: true,
        auto_push_enabled: true,
        auth_mode: authMode,
        remote_name: DEFAULT_REMOTE_NAME,
        remote_url: remoteURL.trim(),
        remote_branch: DEFAULT_REMOTE_BRANCH,
        github_token: authMode === 'github_token' ? tokenInput.trim() || undefined : undefined,
      })
      setMirror(saved)
      setRemoteURL(saved.remote_url || '')
      setUrlEditing(false)
      setTokenInput('')
      if (authMode === 'github_token') {
        setTokenEditing(!saved.github_token_configured)
      }
      setTokenTest(null)
      setInlineMessage(tx('备份目标已保存。', 'Backup destination saved.'))
    } catch (err: any) {
      setInlineError(err.message || tx('保存备份目标失败', 'Failed to save the backup destination'))
    } finally {
      setWorking(false)
    }
  }

  const handleTestToken = async () => {
    setTesting(true)
    setError('')
    setMessage('')
    try {
      const result = await api.testGitMirrorGitHubTokenGeneric({
        remote_url: remoteURL.trim(),
        github_token: tokenInput.trim(),
      })
      setTokenTest(result)
      if (result.normalized_remote_url) {
        setRemoteURL(result.normalized_remote_url)
      }
    } catch (err: any) {
      setInlineError(err.message || tx('GitHub token 测试失败', 'Failed to test GitHub token'))
    } finally {
      setTesting(false)
    }
  }

  const handleSaveToken = async () => {
    if (!remoteURL.trim()) {
      setInlineError(tx('请先填写 GitHub 仓库 URL。', 'Enter the GitHub repository URL first.'))
      return
    }
    if (!tokenInput.trim()) {
      setInlineError(tx('请填写 GitHub token。', 'Enter a GitHub token.'))
      return
    }
    setWorking(true)
    setError('')
    setMessage('')
    try {
      const saved = await api.updateGitMirror({
        auto_commit_enabled: true,
        auto_push_enabled: true,
        auth_mode: 'github_token',
        remote_name: DEFAULT_REMOTE_NAME,
        remote_url: remoteURL.trim(),
        remote_branch: DEFAULT_REMOTE_BRANCH,
        github_token: tokenInput.trim(),
      })
      setMirror(saved)
      setRemoteURL(saved.remote_url || remoteURL.trim())
      setTokenInput('')
      setTokenEditing(!saved.github_token_configured)
      setTokenTest(null)
      setInlineMessage(tx('GitHub token 已保存。', 'GitHub token saved.'))
    } catch (err: any) {
      setInlineError(err.message || tx('保存 GitHub token 失败', 'Failed to save GitHub token'))
    } finally {
      setWorking(false)
    }
  }

  const handleClearToken = async () => {
    setWorking(true)
    setError('')
    setMessage('')
    try {
      const saved = await api.updateGitMirror({
        auto_commit_enabled: true,
        auto_push_enabled: false,
        auth_mode: 'github_token',
        remote_name: DEFAULT_REMOTE_NAME,
        remote_url: remoteURL.trim(),
        remote_branch: DEFAULT_REMOTE_BRANCH,
        clear_github_token: true,
      })
      setMirror(saved)
      setTokenInput('')
      setTokenEditing(true)
      setTokenTest(null)
      setInlineMessage(tx('已清除保存的 GitHub token。', 'Saved GitHub token was cleared.'))
    } catch (err: any) {
      setInlineError(err.message || tx('清除 GitHub token 失败', 'Failed to clear GitHub token'))
    } finally {
      setWorking(false)
    }
  }

  const handleSync = async (forceRemoteOverwrite = false) => {
    if (retrySeconds > 0) {
      setInlineError(tx(`请 ${retrySeconds} 秒后再同步。`, `Try syncing again in ${retrySeconds} seconds.`))
      return
    }
    setWorking(true)
    setError('')
    setMessage('')
    try {
      const result = await api.syncGitMirror({ force_remote_overwrite: forceRemoteOverwrite || undefined })
      await loadPage()
      setSyncRetryUntil(syncCooldownSeconds > 0 ? Date.now() + syncCooldownSeconds * 1000 : 0)
      const queuedHostedSync = result.execution_mode === 'hosted' && (result.sync_state === 'queued' || result.sync_state === 'running')
      setInlineMessage(queuedHostedSync
        ? tx('同步请求已提交，后台正在处理。完成后页面会自动更新状态。', 'Sync request submitted. The background worker is processing it, and this page will update when it finishes.')
        : result.message || tx('已触发同步。', 'Sync started.'))
      if (queuedHostedSync) {
        void pollHostedSyncUntilSettled().catch(() => undefined)
      }
    } catch (err: any) {
      if (err?.code === 'rate_limit_exceeded' && err.retry_after_sec) {
        setSyncRetryUntil(Date.now() + Number(err.retry_after_sec) * 1000)
      }
      setInlineError(err.message || tx('触发同步失败', 'Failed to start sync'))
    } finally {
      setWorking(false)
    }
  }

  const renderStatus = () => (
    <div className="git-mirror-status-line">
      <span>
        {tx('最近更新时间：', 'Last update: ')}
        {lastUpdate ? formatDateTime(lastUpdate, locale) : tx('还没有同步', 'Not synced yet')}
      </span>
    </div>
  )

  const renderFeedback = () => {
    if (error) {
      return <div className="alert alert-warn git-mirror-inline-feedback">{error}</div>
    }
    if (message) {
      return <div className="alert alert-ok git-mirror-inline-feedback">{message}</div>
    }
    return null
  }

  const renderSyncActions = () => syncAvailable && (
    <div className="data-sync-actions data-sync-actions-compact" style={{ marginTop: 16 }}>
      <button className="btn btn-primary" type="button" disabled={syncDisabled} onClick={() => void handleSync(false)}>
        {working ? tx('处理中...', 'Working...') : retrySeconds > 0 ? tx(`请稍候 ${retrySeconds}s`, `Wait ${retrySeconds}s`) : tx('立即同步', 'Sync now')}
      </button>
    </div>
  )

  const renderRemoteConflict = () => mirror?.remote_conflict && (
    <div className="alert alert-warn" style={{ marginTop: 16 }}>
      {tx('远端仓库有 neuDrive 之外的新提交。普通同步已停止，确认后可以用 neuDrive 覆盖远端。', 'The remote repository has commits outside neuDrive. Normal sync is blocked; confirm overwrite to replace the remote with neuDrive.')}
      <div className="data-sync-actions data-sync-actions-compact" style={{ marginTop: 12 }}>
        <button className="btn btn-primary" type="button" disabled={working || retrySeconds > 0} onClick={() => void handleSync(true)}>
          {tx('用 neuDrive 覆盖远端', 'Overwrite remote with neuDrive')}
        </button>
      </div>
    </div>
  )

  const renderGitHubAppFlow = () => (
    <div className="data-sync-token-box" style={{ marginTop: 16 }}>
      <div className="data-record-title">{tx('GitHub App 授权', 'GitHub App authorization')}</div>
      <div className="data-sync-field-note">
        {tx('授权后 neuDrive 会创建或复用你的私有 neudrive-backup 仓库。', 'After authorization, neuDrive creates or reuses your private neudrive-backup repository.')}
      </div>
      {!githubAppConfigured && (
        <div className="alert alert-warn" style={{ marginTop: 12 }}>
          {githubAppUnavailableMessage}
        </div>
      )}
      {githubAppError && !githubAppReauthRequired && (
        <div className="alert alert-warn" style={{ marginTop: 12 }}>
          {githubAppError}
        </div>
      )}
      {githubAppReauthRequired && !mirror?.github_app_user_connected && (
        <div className="alert alert-warn" style={{ marginTop: 12 }}>
          <div className="data-record-title">{tx('需要重新批准 GitHub App 权限', 'GitHub App permissions need approval')}</div>
          {githubAppError && (
            <div className="data-record-secondary" style={{ marginTop: 6 }}>
              {githubAppError}
            </div>
          )}
          <div className="data-record-secondary" style={{ marginTop: 8 }}>
            {tx('我们已断开旧授权。点击下面按钮后，GitHub 会要求你批准新权限并重新授权。', 'The old authorization was disconnected. Click the button below to approve the new permissions on GitHub and reconnect.')}
          </div>
          <div className="data-sync-actions data-sync-actions-compact">
            <button
              className="btn btn-primary"
              type="button"
              disabled={working}
              onClick={handleConnectGitHubApp}
            >
              {working ? tx('连接中...', 'Connecting...') : tx('批准新权限并连接 GitHub', 'Approve permissions and connect GitHub')}
            </button>
          </div>
        </div>
      )}
      {!mirror?.github_app_user_connected ? (
        !githubAppReauthRequired && <div className="data-sync-actions data-sync-actions-compact">
          <button className="btn btn-primary" type="button" disabled={working} onClick={handleConnectGitHubApp}>
            {working ? tx('连接中...', 'Connecting...') : tx('连接 GitHub', 'Connect GitHub')}
          </button>
        </div>
      ) : (
        <>
          <div className="data-record-secondary" style={{ marginTop: 12 }}>
            GitHub {mirror.github_app_user_login || tx('已连接', 'Connected')}
          </div>
          {!remoteRepoURL && (
            <div className="data-sync-actions data-sync-actions-compact">
              <button className="btn btn-primary" type="button" disabled={working} onClick={handleCreateDefaultBackupRepo}>
                {working ? tx('创建中...', 'Creating...') : tx('创建私有备份仓库', 'Create private backup repo')}
              </button>
            </div>
          )}
        </>
      )}
    </div>
  )

  const renderLocalAuthControls = () => (
    <>
      <div className="data-sync-settings-grid git-mirror-auth-grid" style={{ marginTop: 16 }}>
        <div className="form-group git-mirror-auth-mode-field">
          <label htmlFor="git-mirror-auth-mode">{tx('认证方式', 'Auth mode')}</label>
          <div className="git-mirror-auth-control-row">
            <select
              id="git-mirror-auth-mode"
              className="git-mirror-auth-select"
              value={authMode}
              onChange={(event) => {
                const next = event.target.value as AuthMode
                setAuthMode(next)
                setError('')
                setMessage('')
                setGithubAppError('')
                if (next !== 'github_app_user') {
                  setGithubAppReauthRequired(false)
                }
                setTokenTest(null)
                setAuthHelpOpen(false)
                setUrlEditing(next !== 'github_app_user' && !remoteURL.trim())
                setTokenEditing(next === 'github_token' && !mirror?.github_token_configured)
              }}
            >
              {isLocalExecution && (
                <option value="local_credentials">{tx('本机 Git 凭证', 'Local Git credentials')}</option>
              )}
              <option value="github_token">{tx('GitHub Token', 'GitHub token')}</option>
              <option value="github_app_user">{tx('GitHub App 授权', 'GitHub App authorization')}</option>
            </select>
            <div className="git-mirror-auth-help">
              <button
                className="git-mirror-help-button"
                type="button"
                aria-label={tx('查看认证方式说明', 'Show auth mode help')}
                aria-expanded={authHelpOpen}
                onClick={() => setAuthHelpOpen((open) => !open)}
              >
                <InfoIcon />
              </button>
              {authHelpOpen && (
                <div className="git-mirror-help-popover" role="dialog">
                  <div className="data-record-title">{authHelp.title}</div>
                  <div className="data-record-secondary">{authHelp.intro}</div>
                  {authHelp.steps && (
                    <ol className="git-mirror-help-steps">
                      {authHelp.steps.map((step) => (
                        <li key={step}>{step}</li>
                      ))}
                    </ol>
                  )}
                  {authHelp.footer && (
                    <div className="data-record-secondary git-mirror-help-footer">{authHelp.footer}</div>
                  )}
                </div>
              )}
            </div>
          </div>
        </div>
        {authMode !== 'github_app_user' && (
          <div className="form-group data-sync-settings-span-wide git-mirror-repo-field">
            <label htmlFor="git-mirror-remote-url">{tx('仓库 URL', 'Repository URL')}</label>
            {urlEditing || !remoteURL.trim() ? (
              <div className="git-mirror-repo-edit-row">
                <input
                  id="git-mirror-remote-url"
                  value={remoteURL}
                  autoFocus={urlEditing}
                  onChange={(event) => {
                    setRemoteURL(event.target.value)
                    setTokenTest(null)
                    setError('')
                    setMessage('')
                  }}
                  placeholder={remotePlaceholder}
                />
                <button className="btn btn-primary" type="button" disabled={working || !remoteURL.trim() || tokenModeNeedsTokenBeforeSave} onClick={handleSaveLocalDestination}>
                  {working ? tx('保存中...', 'Saving...') : tx('保存备份目标', 'Save backup destination')}
                </button>
              </div>
            ) : (
              <div className="git-mirror-repo-read-row">
                <a href={githubRepoHref(remoteURL)} target="_blank" rel="noreferrer">
                  {remoteURL}
                </a>
                <button
                  className="git-mirror-icon-button"
                  type="button"
                  aria-label={tx('编辑仓库 URL', 'Edit repository URL')}
                  title={tx('编辑仓库 URL', 'Edit repository URL')}
                  onClick={() => setUrlEditing(true)}
                >
                  <PencilIcon />
                </button>
              </div>
            )}
          </div>
        )}
      </div>

      {renderFeedback()}

      {authMode === 'github_token' && (
        <div className="data-sync-token-box git-mirror-token-box" style={{ marginTop: 12 }}>
          <div className="data-record-title">{tx('GitHub Token', 'GitHub token')}</div>
          <div className="data-sync-field-note">
            {mirror?.github_token_configured
              ? tx('Token 已配置；输入新 token 后可替换。建议使用只授权这个备份仓库的 fine-grained token。', 'A token is configured; enter a new token to replace it. A fine-grained token scoped only to this backup repository is recommended.')
              : tx('建议使用 fine-grained token，只给这个备份仓库 Contents 读写权限。具体步骤见认证方式旁边的 ?。', 'Use a fine-grained token scoped only to this backup repository with Contents read/write access. See the ? next to Auth mode for the steps.')}
          </div>
          {tokenEditing || !mirror?.github_token_configured ? (
            <>
              <div className="git-mirror-token-edit-row">
                <input
                  className="data-sync-secret-input"
                  type="password"
                  value={tokenInput}
                  onChange={(event) => {
                    setTokenInput(event.target.value)
                    setTokenTest(null)
                    setError('')
                    setMessage('')
                  }}
                  placeholder="ghp_xxx"
                />
                <button className="btn btn-primary" type="button" disabled={working || !remoteURL.trim() || !tokenInput.trim()} onClick={handleSaveToken}>
                  {working ? tx('保存中...', 'Saving...') : tx('保存 token', 'Save token')}
                </button>
              </div>
              <div className="data-sync-actions data-sync-actions-compact">
                <button className="btn" type="button" disabled={testing || !remoteURL.trim() || !tokenInput.trim()} onClick={handleTestToken}>
                  {testing ? tx('测试中...', 'Testing...') : tx('测试 token', 'Test token')}
                </button>
                {mirror?.github_token_configured && (
                  <button
                    className="btn-text"
                    type="button"
                    disabled={working}
                    onClick={() => {
                      setTokenInput('')
                      setTokenTest(null)
                      setTokenEditing(false)
                    }}
                  >
                    {tx('取消', 'Cancel')}
                  </button>
                )}
              </div>
            </>
          ) : (
            <>
              <div className="git-mirror-repo-read-row git-mirror-token-read-row">
                <span>
                  {tx('Token 已配置', 'Token configured')}
                  {mirror?.github_token_login ? ` · ${mirror.github_token_login}` : ''}
                </span>
                <button
                  className="git-mirror-icon-button"
                  type="button"
                  aria-label={tx('编辑 GitHub token', 'Edit GitHub token')}
                  title={tx('编辑 GitHub token', 'Edit GitHub token')}
                  onClick={() => setTokenEditing(true)}
                >
                  <PencilIcon />
                </button>
              </div>
              <div className="data-sync-actions data-sync-actions-compact">
                <button className="btn" type="button" disabled={working} onClick={handleClearToken}>
                  {tx('清除已保存 token', 'Clear saved token')}
                </button>
              </div>
            </>
          )}
          {tokenTest && (
            <div className={tokenTest.ok ? 'alert alert-ok' : 'alert alert-warn'} style={{ marginTop: 12 }}>
              {localizeGitHubAccessMessage(tokenTest.message, locale)}
            </div>
          )}
        </div>
      )}

      {authMode === 'github_app_user' && renderGitHubAppFlow()}
    </>
  )

  if (busy) {
    return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>
  }

  return (
    <div className="page materials-page">
      <div className="materials-panel data-sync-card">
        <div className="card-header">
          <h3 className="card-title">{tx('Backup destination', 'Backup destination')}</h3>
        </div>
        <p className="data-record-secondary">
          {tx('选择一个 GitHub 仓库作为 neuDrive 的备份位置，授权后可以一键同步并保留可恢复的版本记录。', 'Choose a GitHub repository as your neuDrive backup destination, then sync on demand with recoverable version history.')}
        </p>

        {renderStatus()}
        {renderRemoteConflict()}

        {renderLocalAuthControls()}
        {renderSyncActions()}

        {authMode === 'github_app_user' && mirror?.github_app_user_connected && (
          <div className="data-sync-actions data-sync-actions-compact" style={{ marginTop: 16 }}>
            <button className="btn-text" type="button" disabled={working} onClick={handleDisconnectGitHubApp}>
              {tx('断开 GitHub', 'Disconnect GitHub')}
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
