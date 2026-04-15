import { useEffect, useMemo, useState } from 'react'
import {
  api,
  type CreateGitMirrorRepoRequest,
  type GitMirrorGitHubTestResult,
  type GitMirrorRepo,
  type GitMirrorSettings,
  type UpdateGitMirrorRequest,
} from '../api'
import { useI18n } from '../i18n'
import { formatDateTime } from './data/DataShared'

const DEFAULT_DRAFT: UpdateGitMirrorRequest = {
  auto_commit_enabled: false,
  auto_push_enabled: false,
  auth_mode: 'github_token',
  remote_name: 'origin',
  remote_url: '',
  remote_branch: 'main',
}

const DEFAULT_CREATE_REPO: CreateGitMirrorRepoRequest = {
  owner_login: '',
  repo_name: '',
  description: '',
  private: true,
  remote_name: 'origin',
  remote_branch: 'main',
}

function repoPermissionRank(permission?: string) {
  switch (permission) {
    case 'admin':
      return 3
    case 'write':
      return 2
    case 'read':
      return 1
    default:
      return 0
  }
}

function canWriteRepo(permission?: string) {
  return permission === 'admin' || permission === 'write'
}

export default function GitMirrorPage() {
  const { locale, tx } = useI18n()
  const [mirror, setMirror] = useState<GitMirrorSettings | null>(null)
  const [busy, setBusy] = useState(false)
  const [saving, setSaving] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [testing, setTesting] = useState(false)
  const [reposBusy, setReposBusy] = useState(false)
  const [repos, setRepos] = useState<GitMirrorRepo[]>([])
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [tokenInput, setTokenInput] = useState('')
  const [tokenTest, setTokenTest] = useState<GitMirrorGitHubTestResult | null>(null)
  const [draft, setDraft] = useState<UpdateGitMirrorRequest>(DEFAULT_DRAFT)
  const [showCreateRepo, setShowCreateRepo] = useState(false)
  const [createRepoBusy, setCreateRepoBusy] = useState(false)
  const [createRepoDraft, setCreateRepoDraft] = useState<CreateGitMirrorRepoRequest>(DEFAULT_CREATE_REPO)
  const [repoQuery, setRepoQuery] = useState('')
  const [repoOwnerFilter, setRepoOwnerFilter] = useState('')
  const [repoWritableOnly, setRepoWritableOnly] = useState(true)
  const [pendingRepoURL, setPendingRepoURL] = useState('')

  const syncDraft = (settings: GitMirrorSettings) => {
    setDraft({
      auto_commit_enabled: settings.auto_commit_enabled,
      auto_push_enabled: settings.auto_push_enabled,
      auth_mode: settings.auth_mode,
      remote_name: settings.remote_name || 'origin',
      remote_url: settings.remote_url || '',
      remote_branch: settings.remote_branch || 'main',
    })
    setCreateRepoDraft((prev) => ({
      ...prev,
      owner_login: settings.github_app_user_login || prev.owner_login,
      remote_name: settings.remote_name || prev.remote_name || 'origin',
      remote_branch: settings.remote_branch || prev.remote_branch || 'main',
    }))
    setTokenInput('')
    setTokenTest(null)
  }

  const loadMirror = async () => {
    setBusy(true)
    setError('')
    try {
      const settings = await api.getGitMirror()
      setMirror(settings)
      syncDraft(settings)
    } catch (err: any) {
      setError(err.message || tx('加载 Git Mirror 配置失败', 'Failed to load Git Mirror settings'))
    } finally {
      setBusy(false)
    }
  }

  const loadRepos = async () => {
    if (!mirror?.github_app_user_connected) return
    setReposBusy(true)
    try {
      const nextRepos = await api.listGitMirrorGitHubAppRepos()
      setRepos(nextRepos)
    } catch (err: any) {
      setError(err.message || tx('加载 GitHub 仓库列表失败', 'Failed to load GitHub repositories'))
    } finally {
      setReposBusy(false)
    }
  }

  useEffect(() => {
    void loadMirror()
  }, [])

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const status = params.get('github_app_status')
    const callbackError = params.get('github_app_error')
    if (!status && !callbackError) return
    if (callbackError) {
      setError(callbackError)
    } else if (status === 'connected') {
      setMessage(tx('GitHub App 已连接。现在可以选择或创建仓库了。', 'GitHub App connected. You can now select or create a repository.'))
    }
    void loadMirror()
    params.delete('github_app_status')
    params.delete('github_app_error')
    const nextSearch = params.toString()
    window.history.replaceState({}, '', `${window.location.pathname}${nextSearch ? `?${nextSearch}` : ''}`)
  }, [tx])

  useEffect(() => {
    if (mirror?.auth_mode === 'github_app_user' && mirror.github_app_user_connected) {
      void loadRepos()
    }
  }, [mirror?.auth_mode, mirror?.github_app_user_connected])

  const updateDraft = (patch: Partial<UpdateGitMirrorRequest>) => {
    setDraft((prev) => {
      const next = { ...prev, ...patch }
      if (!next.auto_commit_enabled) {
        next.auto_push_enabled = false
      }
      if (next.auto_push_enabled) {
        next.auto_commit_enabled = true
      }
      return next
    })
    setError('')
    setMessage('')
    setTokenTest(null)
  }

  const executionMode = mirror?.execution_mode || 'hosted'
  const isLocalExecution = executionMode === 'local'

  const authOptions = useMemo(() => {
    const options: Array<UpdateGitMirrorRequest['auth_mode']> = ['github_token', 'github_app_user']
    if (isLocalExecution) {
      options.unshift('local_credentials')
    }
    return options
  }, [isLocalExecution])

  const tokenVerificationCurrent = useMemo(() => {
    if (!mirror) return false
    if (draft.auth_mode !== 'github_token') return true
    if (tokenInput.trim()) {
      return !!tokenTest?.ok
    }
    return !!mirror.github_token_configured &&
      !!mirror.github_token_verified_at &&
      (mirror.remote_url || '') === (draft.remote_url || '')
  }, [draft.auth_mode, draft.remote_url, mirror, tokenInput, tokenTest])

  const selectedRepo = useMemo(
    () => repos.find((repo) => repo.clone_url === draft.remote_url),
    [draft.remote_url, repos],
  )
  const repoOwners = useMemo(
    () => Array.from(new Set(repos.map((repo) => repo.owner_login).filter(Boolean))).sort((left, right) => left.localeCompare(right)),
    [repos],
  )
  const filteredRepos = useMemo(() => {
    const query = repoQuery.trim().toLowerCase()
    return repos
      .filter((repo) => {
        if (repoOwnerFilter && repo.owner_login !== repoOwnerFilter) {
          return false
        }
        if (repoWritableOnly && !canWriteRepo(repo.viewer_permission)) {
          return false
        }
        if (!query) {
          return true
        }
        const haystack = `${repo.full_name} ${repo.owner_login} ${repo.repo_name}`.toLowerCase()
        return haystack.includes(query)
      })
      .sort((left, right) => {
        const permissionDelta = repoPermissionRank(right.viewer_permission) - repoPermissionRank(left.viewer_permission)
        if (permissionDelta !== 0) {
          return permissionDelta
        }
        return left.full_name.localeCompare(right.full_name)
      })
  }, [repoOwnerFilter, repoQuery, repoWritableOnly, repos])
  const syncStateHint = useMemo(() => {
    if (!mirror) return ''
    if (mirror.sync_state === 'queued') {
      if (mirror.sync_next_attempt_at) {
        return tx('后台已排队，下一次重试时间：', 'Queued in the background. Next retry: ') + formatDateTime(mirror.sync_next_attempt_at, locale)
      }
      if (mirror.sync_requested_at) {
        return tx('后台已排队，等待 worker 处理。排队时间：', 'Queued in the background. Requested at: ') + formatDateTime(mirror.sync_requested_at, locale)
      }
      return tx('后台已排队，等待 worker 处理。', 'Queued in the background and waiting for the worker.')
    }
    if (mirror.sync_state === 'running') {
      if (mirror.sync_started_at) {
        return tx('后台同步进行中，开始于：', 'Background sync is running since ') + formatDateTime(mirror.sync_started_at, locale)
      }
      return tx('后台同步进行中。', 'Background sync is running.')
    }
    if (mirror.sync_state === 'error') {
      return mirror.last_error || mirror.last_push_error || tx('最近一次后台同步失败。', 'The latest background sync failed.')
    }
    return mirror.message || ''
  }, [locale, mirror, tx])
  const syncStateHintClass = mirror?.sync_state === 'error' ? 'alert alert-warn' : 'alert alert-ok'

  const handleStartGitHubAppBrowser = async () => {
    setError('')
    setMessage('')
    updateDraft({ auth_mode: 'github_app_user' })
    try {
      const result = await api.startGitMirrorGitHubAppBrowser(window.location.pathname)
      window.location.assign(result.authorization_url)
    } catch (err: any) {
      setError(err.message || tx('启动 GitHub App 授权失败', 'Failed to start GitHub App authorization'))
    }
  }

  const handleDisconnectGitHubApp = async () => {
    setSaving(true)
    setError('')
    setMessage('')
    try {
      await api.disconnectGitMirrorGitHubAppUser()
      await loadMirror()
      setRepos([])
      setMessage(tx('GitHub App 已断开连接', 'GitHub App disconnected'))
    } catch (err: any) {
      setError(err.message || tx('断开 GitHub App 失败', 'Failed to disconnect GitHub App'))
    } finally {
      setSaving(false)
    }
  }

  const handleTestToken = async () => {
    setTesting(true)
    setError('')
    setMessage('')
    try {
      const result = await api.testGitMirrorGitHubTokenGeneric({
        remote_url: draft.remote_url || '',
        github_token: tokenInput.trim(),
      })
      setTokenTest(result)
      if (result.normalized_remote_url) {
        setDraft((prev) => ({ ...prev, remote_url: result.normalized_remote_url || prev.remote_url }))
      }
      if (result.ok) {
        setMessage(result.message || tx('GitHub token 可用', 'GitHub token is valid'))
      } else {
        setError(result.message || tx('GitHub token 校验失败', 'GitHub token validation failed'))
      }
    } catch (err: any) {
      setError(err.message || tx('GitHub token 测试失败', 'Failed to test GitHub token'))
    } finally {
      setTesting(false)
    }
  }

  const persistDraft = async (nextDraft: UpdateGitMirrorRequest, successMessage: string) => {
    if (nextDraft.auth_mode === 'github_token' && nextDraft.auto_push_enabled && !tokenVerificationCurrent) {
      setError(tx('启用 GitHub token 自动推送前，请先测试并确认 token 可用。', 'Test and verify the GitHub token before enabling auto push.'))
      return null
    }
    setSaving(true)
    setError('')
    setMessage('')
    try {
      const saved = await api.updateGitMirror({
        ...nextDraft,
        github_token: tokenInput.trim() || undefined,
      })
      setMirror(saved)
      syncDraft(saved)
      setMessage(successMessage)
      return saved
    } catch (err: any) {
      setError(err.message || tx('保存 Git Mirror 配置失败', 'Failed to save Git Mirror settings'))
      return null
    } finally {
      setSaving(false)
    }
  }

  const handleSave = async () => {
    await persistDraft(draft, tx('Git Mirror 配置已保存', 'Git Mirror settings saved'))
  }

  const handleSync = async () => {
    setSyncing(true)
    setError('')
    setMessage('')
    try {
      const result = await api.syncGitMirror()
      await loadMirror()
      setMessage(result.message || tx('已触发 Git Mirror 同步', 'Git Mirror sync triggered'))
    } catch (err: any) {
      setError(err.message || tx('触发 Git Mirror 同步失败', 'Failed to trigger Git Mirror sync'))
    } finally {
      setSyncing(false)
    }
  }

  const handleSelectRepo = async (repo: GitMirrorRepo) => {
    const nextDraft: UpdateGitMirrorRequest = {
      ...draft,
      auth_mode: 'github_app_user',
      remote_name: draft.remote_name || 'origin',
      remote_url: repo.clone_url,
      remote_branch: repo.default_branch || 'main',
    }
    setPendingRepoURL(repo.clone_url)
    setDraft(nextDraft)
    try {
      await persistDraft(nextDraft, tx('仓库已选中并保存。现在可以直接开启 auto push。', 'Repository selected and saved. You can turn on auto push now.'))
    } finally {
      setPendingRepoURL('')
    }
  }

  const handleCreateRepo = async () => {
    if (!createRepoDraft.owner_login.trim() || !createRepoDraft.repo_name.trim()) {
      setError(tx('请填写 owner 和 repo 名称。', 'Fill in both the owner and repository name.'))
      return
    }
    setCreateRepoBusy(true)
    setError('')
    setMessage('')
    try {
      const repo = await api.createGitMirrorGitHubAppRepo(createRepoDraft)
      setShowCreateRepo(false)
      await loadRepos()
      const nextDraft: UpdateGitMirrorRequest = {
        ...draft,
        auth_mode: 'github_app_user',
        remote_name: createRepoDraft.remote_name || draft.remote_name || 'origin',
        remote_url: repo.clone_url,
        remote_branch: createRepoDraft.remote_branch || repo.default_branch || 'main',
      }
      setDraft(nextDraft)
      await persistDraft(nextDraft, tx('仓库已创建并保存到 Git Mirror。', 'Repository created and saved to Git Mirror.'))
    } catch (err: any) {
      setError(err.message || tx('创建 GitHub 仓库失败', 'Failed to create the GitHub repository'))
    } finally {
      setCreateRepoBusy(false)
    }
  }

  if (busy && !mirror) {
    return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>
  }

  return (
    <div className="page materials-page">
      <section className="materials-hero">
        <div className="materials-hero-copy">
          <div className="materials-kicker">neuDrive Mirror</div>
          <h2 className="materials-title">{tx('Git Mirror', 'Git Mirror')}</h2>
          <p className="materials-subtitle">{tx('把 neuDrive Hub 持续同步到 Git 仓库。local 模式继续支持本地 worktree；hosted 模式则会在后台排队执行。', 'Continuously sync your neuDrive Hub into a Git repository. Local mode keeps using a local worktree, while hosted mode runs queued syncs in the background.')}</p>
        </div>
        <div className="materials-actions">
          <button className="btn" type="button" disabled={reposBusy || !mirror?.github_app_user_connected} onClick={() => void loadRepos()}>
            {reposBusy ? tx('刷新中...', 'Refreshing...') : tx('刷新仓库列表', 'Refresh repos')}
          </button>
          <button className="btn btn-primary" type="button" disabled={syncing} onClick={handleSync}>
            {syncing ? tx('同步中...', 'Syncing...') : tx('立即同步', 'Sync now')}
          </button>
        </div>
      </section>

      {error && <div className="alert alert-warn" style={{ marginBottom: 16 }}>{error}</div>}
      {message && <div className="alert alert-ok" style={{ marginBottom: 16 }}>{message}</div>}

      <div className="data-sync-status-grid">
        <div className="data-sync-status-card">
          <div className="data-record-title">{tx('执行模式', 'Execution mode')}</div>
          <div className="data-record-secondary">{executionMode}</div>
        </div>
        <div className="data-sync-status-card">
          <div className="data-record-title">{tx('同步状态', 'Sync state')}</div>
          <div className="data-record-secondary">{mirror?.sync_state || 'idle'}</div>
        </div>
        <div className="data-sync-status-card">
          <div className="data-record-title">{tx('认证方式', 'Auth mode')}</div>
          <div className="data-record-secondary">{mirror?.auth_mode || draft.auth_mode}</div>
        </div>
        <div className="data-sync-status-card">
          <div className="data-record-title">{tx('最近同步', 'Last sync')}</div>
          <div className="data-record-secondary">{mirror?.last_synced_at ? formatDateTime(mirror.last_synced_at, locale) : tx('还没有', 'Not yet')}</div>
        </div>
        <div className="data-sync-status-card">
          <div className="data-record-title">{tx('下次重试', 'Next retry')}</div>
          <div className="data-record-secondary">{mirror?.sync_next_attempt_at ? formatDateTime(mirror.sync_next_attempt_at, locale) : tx('无', 'None')}</div>
        </div>
        <div className="data-sync-status-card">
          <div className="data-record-title">{tx('尝试次数', 'Attempt count')}</div>
          <div className="data-record-secondary">{mirror?.sync_attempt_count || 0}</div>
        </div>
      </div>

      {syncStateHint && (
        <div className={syncStateHintClass} style={{ marginTop: 16, marginBottom: 0 }}>
          {syncStateHint}
        </div>
      )}

      <div className="materials-panel data-sync-card" style={{ marginTop: 16 }}>
        <div className="card-header">
          <h3 className="card-title">{tx('Mirror 配置', 'Mirror settings')}</h3>
        </div>

        {isLocalExecution && !mirror?.enabled && (
          <div className="alert alert-warn" style={{ marginTop: 12 }}>
            {tx('先运行 `neudrive git init` 初始化本地 mirror，再回来配置自动 commit / push。', 'Run `neudrive git init` first to initialize the local mirror, then come back to configure auto commit / push.')}
          </div>
        )}

        {isLocalExecution && mirror?.path && (
          <div className="data-record-secondary" style={{ marginTop: 12 }}>
            {tx('本地目录：', 'Local path: ')}<code>{mirror.path}</code>
          </div>
        )}

        <div className="data-sync-settings-shell">
          <section className="data-sync-settings-section">
            <h4 className="data-sync-section-title">{tx('同步策略', 'Sync strategy')}</h4>
            <div className="data-sync-toggle-grid">
              <label className="data-sync-toggle-card">
                <div className="data-sync-toggle-copy">
                  <div className="data-sync-toggle-title">{tx('自动 commit', 'Auto commit')}</div>
                  <div className="data-sync-field-note">{tx('每次 mirror sync 时自动提交一次变更。', 'Create a Git commit automatically on each mirror sync.')}</div>
                </div>
                <input type="checkbox" checked={draft.auto_commit_enabled} onChange={(e) => updateDraft({ auto_commit_enabled: e.target.checked })} />
              </label>
              <label className="data-sync-toggle-card">
                <div className="data-sync-toggle-copy">
                  <div className="data-sync-toggle-title">{tx('自动 push', 'Auto push')}</div>
                  <div className="data-sync-field-note">{tx('同步成功后继续推送到远端仓库。', 'Push to the remote repository after each successful sync.')}</div>
                </div>
                <input type="checkbox" checked={draft.auto_push_enabled} onChange={(e) => updateDraft({ auto_push_enabled: e.target.checked })} />
              </label>
            </div>
          </section>

          <section className="data-sync-settings-section">
            <h4 className="data-sync-section-title">{tx('远端与认证', 'Remote and auth')}</h4>
            <div className="data-sync-settings-grid">
              <div className="form-group">
                <label htmlFor="git-mirror-auth-mode">{tx('认证方式', 'Auth mode')}</label>
                <select
                  id="git-mirror-auth-mode"
                  value={draft.auth_mode}
                  onChange={(e) => {
                    const nextAuthMode = e.target.value as UpdateGitMirrorRequest['auth_mode']
                    updateDraft({ auth_mode: nextAuthMode })
                    if (nextAuthMode === 'github_app_user' && mirror?.github_app_user_connected) {
                      void loadRepos()
                    }
                  }}
                >
                  {authOptions.map((option) => (
                    <option key={option} value={option}>{option}</option>
                  ))}
                </select>
              </div>
              <div className="form-group">
                <label htmlFor="git-mirror-remote-name">Remote name</label>
                <input id="git-mirror-remote-name" value={draft.remote_name || ''} onChange={(e) => updateDraft({ remote_name: e.target.value })} />
              </div>
              <div className="form-group">
                <label htmlFor="git-mirror-remote-branch">{tx('目标分支', 'Target branch')}</label>
                <input id="git-mirror-remote-branch" value={draft.remote_branch || ''} onChange={(e) => updateDraft({ remote_branch: e.target.value })} />
              </div>
              <div className="form-group data-sync-settings-span-wide">
                <label htmlFor="git-mirror-remote-url">{tx('仓库 URL', 'Repository URL')}</label>
                <input id="git-mirror-remote-url" value={draft.remote_url || ''} onChange={(e) => updateDraft({ remote_url: e.target.value })} placeholder="https://github.com/owner/repo.git" />
              </div>
            </div>

            {draft.auth_mode === 'github_token' && (
              <div className="data-sync-token-box">
                <div className="data-record-title">{tx('GitHub Token', 'GitHub token')}</div>
                <div className="data-sync-field-note">{tx('保留现有 PAT 模式。你可以粘贴新 token，也可以继续使用已保存的 token。', 'Keep using the existing PAT mode. Paste a new token or continue with the saved one.')}</div>
                <input
                  className="data-sync-secret-input"
                  type="password"
                  value={tokenInput}
                  onChange={(e) => { setTokenInput(e.target.value); setTokenTest(null); setError(''); setMessage('') }}
                  placeholder={mirror?.github_token_configured ? tx('留空以继续使用已保存的 token', 'Leave blank to keep the saved token') : 'ghp_xxx'}
                />
                <div className="data-sync-actions data-sync-actions-compact">
                  <button className="btn" type="button" disabled={testing} onClick={handleTestToken}>
                    {testing ? tx('测试中...', 'Testing...') : tx('测试 token', 'Test token')}
                  </button>
                  {mirror?.github_token_configured && (
                    <span className="data-record-secondary">
                      {tx('已保存 token', 'Saved token')} · {mirror.github_token_login || tx('未验证', 'Unverified')}
                    </span>
                  )}
                </div>
              </div>
            )}

            {draft.auth_mode === 'github_app_user' && (
              <div className="data-sync-token-box">
                <div className="data-record-title">GitHub App user</div>
                <div className="data-sync-field-note">{tx('由官方 GitHub App 代替用户手动粘贴 PAT。连接后可以直接选仓库或创建仓库。', 'Use the official GitHub App instead of pasting a PAT. Once connected, you can pick or create a repository directly.')}</div>
                <div className="data-sync-actions data-sync-actions-compact">
                  {!mirror?.github_app_user_connected ? (
                    <button className="btn btn-primary" type="button" onClick={handleStartGitHubAppBrowser}>
                      {tx('连接 GitHub', 'Connect GitHub')}
                    </button>
                  ) : (
                    <>
                      <button className="btn btn-primary" type="button" onClick={() => void loadRepos()}>
                        {tx('加载仓库', 'Load repositories')}
                      </button>
                      <button className="btn" type="button" disabled={saving} onClick={handleDisconnectGitHubApp}>
                        {tx('断开连接', 'Disconnect')}
                      </button>
                    </>
                  )}
                </div>
                {mirror?.github_app_user_connected && (
                  <div className="data-record-secondary" style={{ marginTop: 12 }}>
                    GitHub {mirror.github_app_user_login || tx('已连接', 'Connected')}
                    {mirror.github_app_user_authorized_at ? ` · ${tx('授权时间', 'Authorized')} ${formatDateTime(mirror.github_app_user_authorized_at, locale)}` : ''}
                    {mirror.github_app_user_refresh_expires_at ? ` · ${tx('refresh 到期', 'Refresh expires')} ${formatDateTime(mirror.github_app_user_refresh_expires_at, locale)}` : ''}
                    {mirror.github_repo_permission ? ` · ${tx('当前仓库权限', 'Current repo permission')} ${mirror.github_repo_permission}` : ''}
                  </div>
                )}

                {mirror?.github_app_user_connected && (
                  <>
                    <div className="data-sync-settings-grid" style={{ marginTop: 12 }}>
                      <div className="form-group">
                        <label htmlFor="git-mirror-repo-query">{tx('搜索仓库', 'Search repositories')}</label>
                        <input
                          id="git-mirror-repo-query"
                          value={repoQuery}
                          onChange={(e) => setRepoQuery(e.target.value)}
                          placeholder={tx('按 owner / repo 搜索', 'Search by owner / repo')}
                        />
                      </div>
                      <div className="form-group">
                        <label htmlFor="git-mirror-owner-filter">{tx('Owner', 'Owner')}</label>
                        <select id="git-mirror-owner-filter" value={repoOwnerFilter} onChange={(e) => setRepoOwnerFilter(e.target.value)}>
                          <option value="">{tx('全部 owner', 'All owners')}</option>
                          {repoOwners.map((owner) => (
                            <option key={owner} value={owner}>{owner}</option>
                          ))}
                        </select>
                      </div>
                      <label className="data-sync-toggle-card">
                        <div className="data-sync-toggle-copy">
                          <div className="data-sync-toggle-title">{tx('只看可写仓库', 'Writable only')}</div>
                          <div className="data-sync-field-note">{tx('优先只展示拥有 write / admin 权限的 repo。', 'Only show repositories where you already have write or admin access.')}</div>
                        </div>
                        <input type="checkbox" checked={repoWritableOnly} onChange={(e) => setRepoWritableOnly(e.target.checked)} />
                      </label>
                    </div>

                    <div style={{ display: 'grid', gap: 10, marginTop: 12 }}>
                      {filteredRepos.length === 0 ? (
                        <div className="data-record-secondary">
                          {reposBusy
                            ? tx('仓库列表刷新中...', 'Refreshing repositories...')
                            : tx('没有匹配的仓库。你可以放宽筛选，或直接创建一个新仓库。', 'No repositories match the current filters. Relax the filters or create a new repository.')}
                        </div>
                      ) : (
                        filteredRepos.slice(0, 24).map((repo) => (
                          <div
                            key={repo.clone_url}
                            className="data-sync-status-card"
                            style={{ display: 'flex', justifyContent: 'space-between', gap: 16, alignItems: 'center' }}
                          >
                            <div>
                              <div className="data-record-title">{repo.full_name}</div>
                              <div className="data-record-secondary">
                                {repo.owner_type || 'user'} · {repo.default_branch || 'main'} · {repo.viewer_permission || 'none'}
                              </div>
                            </div>
                            <button
                              className="btn"
                              type="button"
                              disabled={saving && pendingRepoURL === repo.clone_url}
                              onClick={() => void handleSelectRepo(repo)}
                            >
                              {selectedRepo?.clone_url === repo.clone_url
                                ? tx('重新保存', 'Save again')
                                : saving && pendingRepoURL === repo.clone_url
                                  ? tx('保存中...', 'Saving...')
                                  : tx('使用此仓库', 'Use this repo')}
                            </button>
                          </div>
                        ))
                      )}
                    </div>

                    <div className="data-sync-actions data-sync-actions-compact" style={{ marginTop: 12 }}>
                      <button className="btn" type="button" onClick={() => setShowCreateRepo((current) => !current)}>
                        {showCreateRepo ? tx('取消创建', 'Cancel create') : tx('创建仓库', 'Create repository')}
                      </button>
                    </div>

                    {showCreateRepo && (
                      <div className="data-sync-settings-grid" style={{ marginTop: 12 }}>
                        <div className="form-group">
                          <label htmlFor="create-repo-owner">Owner</label>
                          <input
                            id="create-repo-owner"
                            list="git-mirror-owner-options"
                            value={createRepoDraft.owner_login}
                            onChange={(e) => setCreateRepoDraft((prev) => ({ ...prev, owner_login: e.target.value }))}
                            placeholder={mirror.github_app_user_login || 'owner'}
                          />
                          <datalist id="git-mirror-owner-options">
                            {repoOwners.map((owner) => (
                              <option key={owner} value={owner} />
                            ))}
                          </datalist>
                        </div>
                        <div className="form-group">
                          <label htmlFor="create-repo-name">{tx('仓库名', 'Repository name')}</label>
                          <input id="create-repo-name" value={createRepoDraft.repo_name} onChange={(e) => setCreateRepoDraft((prev) => ({ ...prev, repo_name: e.target.value }))} placeholder="neudrive-mirror" />
                        </div>
                        <div className="form-group">
                          <label htmlFor="create-repo-description">{tx('描述', 'Description')}</label>
                          <input id="create-repo-description" value={createRepoDraft.description || ''} onChange={(e) => setCreateRepoDraft((prev) => ({ ...prev, description: e.target.value }))} />
                        </div>
                        <label className="data-sync-toggle-card">
                          <div className="data-sync-toggle-copy">
                            <div className="data-sync-toggle-title">{tx('私有仓库', 'Private repository')}</div>
                            <div className="data-sync-field-note">{tx('关闭时会创建 public repo。', 'Turn this off to create a public repository.')}</div>
                          </div>
                          <input type="checkbox" checked={createRepoDraft.private} onChange={(e) => setCreateRepoDraft((prev) => ({ ...prev, private: e.target.checked }))} />
                        </label>
                        <div className="data-sync-actions data-sync-actions-compact">
                          <button className="btn btn-primary" type="button" disabled={createRepoBusy} onClick={handleCreateRepo}>
                            {createRepoBusy ? tx('创建中...', 'Creating...') : tx('创建并保存', 'Create and save')}
                          </button>
                        </div>
                      </div>
                    )}
                  </>
                )}
              </div>
            )}
          </section>

          <section className="data-sync-settings-section">
            <h4 className="data-sync-section-title">{tx('最近状态', 'Recent status')}</h4>
            <div className="data-sync-settings-grid">
              <div className="data-sync-status-card">
                <div className="data-record-title">{tx('排队时间', 'Queued at')}</div>
                <div className="data-record-secondary">
                  {mirror?.sync_requested_at ? formatDateTime(mirror.sync_requested_at, locale) : tx('还没有', 'Not yet')}
                </div>
              </div>
              <div className="data-sync-status-card">
                <div className="data-record-title">{tx('开始执行', 'Started at')}</div>
                <div className="data-record-secondary">
                  {mirror?.sync_started_at ? formatDateTime(mirror.sync_started_at, locale) : tx('还没有', 'Not yet')}
                </div>
              </div>
              <div className="data-sync-status-card">
                <div className="data-record-title">{tx('最后 commit', 'Last commit')}</div>
                <div className="data-record-secondary">
                  {mirror?.last_commit_hash || tx('还没有', 'Not yet')}
                  {mirror?.last_commit_at ? ` · ${formatDateTime(mirror.last_commit_at, locale)}` : ''}
                </div>
              </div>
              <div className="data-sync-status-card">
                <div className="data-record-title">{tx('最后 push', 'Last push')}</div>
                <div className="data-record-secondary">
                  {mirror?.last_push_at ? formatDateTime(mirror.last_push_at, locale) : tx('还没有', 'Not yet')}
                </div>
              </div>
              <div className="data-sync-status-card">
                <div className="data-record-title">{tx('最近错误', 'Last error')}</div>
                <div className="data-record-secondary">{mirror?.last_error || mirror?.last_push_error || tx('无', 'None')}</div>
              </div>
            </div>
          </section>
        </div>

        <div className="data-sync-actions">
          <button className="btn btn-primary" type="button" disabled={saving} onClick={handleSave}>
            {saving ? tx('保存中...', 'Saving...') : tx('保存配置', 'Save settings')}
          </button>
        </div>
      </div>
    </div>
  )
}
