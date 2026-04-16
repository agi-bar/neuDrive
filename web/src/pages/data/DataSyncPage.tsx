import { useEffect, useMemo, useState } from 'react'
import { useI18n } from '../../i18n'
import {
  api,
  type GitMirrorGitHubTestResult,
  type GitMirrorSettings,
  type LocalConfigFile,
  type UpdateGitMirrorRequest,
} from '../../api'
import { formatDateTime } from './DataShared'

type ConfigViewMode = 'settings' | 'raw'

type LocalSettingsDraft = {
  currentTarget: string
  profilesJson: string
  listenAddr: string
  storage: string
  sqlitePath: string
  databaseURL: string
  gitMirrorPath: string
  publicBaseURL: string
  jwtSecret: string
  vaultMasterKey: string
  connectionsJson: string
}

const REMOTE_PROFILES_EXAMPLE = `${JSON.stringify({
  official: {
    api_base: 'https://agenthub.agi.bar',
    token: 'eyJhbGciOi...',
    refresh_token: 'ndr_refresh_xxxxxxxx',
    expires_at: '2026-12-31T23:59:59Z',
    scopes: ['admin', 'offline_access'],
    auth_mode: 'oauth_session',
  },
}, null, 2)}\n`

const LOCAL_CONNECTIONS_EXAMPLE = `${JSON.stringify({
  'codex-local': {
    transport: 'stdio',
    entrypoint_type: 'binary',
    entrypoint_path: '/Users/you/.local/bin/neudrive-mcp',
    managed_paths: ['/skills', '/memory', '/projects'],
    chat_usage: ['codex'],
  },
}, null, 2)}\n`

function EyeIcon({ visible }: { visible: boolean }) {
  if (visible) {
    return (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <path d="M3 3l18 18" />
        <path d="M10.6 10.6a3 3 0 0 0 4.24 4.24" />
        <path d="M9.88 5.09A10.94 10.94 0 0 1 12 5c5 0 9.27 3.11 11 7-0.6 1.37-1.54 2.62-2.73 3.65" />
        <path d="M6.61 6.61C4.62 7.9 3.1 9.79 2 12c1.73 3.89 6 7 10 7a10.8 10.8 0 0 0 4.39-.93" />
      </svg>
    )
  }
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M2 12s3.64-7 10-7 10 7 10 7-3.64 7-10 7-10-7-10-7Z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  )
}

function isRecord(value: unknown): value is Record<string, any> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function defaultLocalConfigObject() {
  return {
    version: 3,
    current_target: 'local',
    local: {},
  } as Record<string, any>
}

function stringifyLocalConfig(value: Record<string, any>) {
  return `${JSON.stringify(value, null, 2)}\n`
}

function parseLocalConfigObject(raw: string): { value: Record<string, any>; error: string } {
  if (!raw.trim()) {
    return { value: defaultLocalConfigObject(), error: '' }
  }
  try {
    const parsed = JSON.parse(raw)
    if (!isRecord(parsed)) {
      return { value: defaultLocalConfigObject(), error: 'config.json must contain a top-level JSON object.' }
    }
    return { value: parsed, error: '' }
  } catch (err: any) {
    return { value: defaultLocalConfigObject(), error: err?.message || 'config.json is not valid JSON.' }
  }
}

function readString(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function readPrettyObject(value: unknown) {
  if (!isRecord(value)) {
    return '{}'
  }
  return JSON.stringify(value, null, 2)
}

function normalizeTargetInput(value: string) {
  const trimmed = value.trim()
  if (!trimmed || trimmed === 'local') {
    return { target: trimmed || 'local', profile: '' }
  }
  if (trimmed.startsWith('profile:')) {
    return {
      target: trimmed,
      profile: trimmed.slice('profile:'.length).trim(),
    }
  }
  return {
    target: `profile:${trimmed}`,
    profile: trimmed,
  }
}

function draftFromRaw(raw: string): LocalSettingsDraft {
  const parsed = parseLocalConfigObject(raw)
  const root = parsed.value
  const local = isRecord(root.local) ? root.local : {}
  const currentTarget = normalizeTargetInput(readString(root.current_target) || readString(root.current_profile)).target
  return {
    currentTarget,
    profilesJson: readPrettyObject(root.profiles),
    listenAddr: readString(local.listen_addr),
    storage: readString(local.storage),
    sqlitePath: readString(local.sqlite_path),
    databaseURL: readString(local.database_url),
    gitMirrorPath: readString(local.git_mirror_path),
    publicBaseURL: readString(local.public_base_url),
    jwtSecret: readString(local.jwt_secret),
    vaultMasterKey: readString(local.vault_master_key),
    connectionsJson: readPrettyObject(local.connections),
  }
}

function parseObjectField(raw: string, fieldName: string): { value: Record<string, any>; error: string } {
  const trimmed = raw.trim()
  if (!trimmed) {
    return { value: {}, error: '' }
  }
  try {
    const parsed = JSON.parse(trimmed)
    if (!isRecord(parsed)) {
      return { value: {}, error: `${fieldName} must be a JSON object.` }
    }
    return { value: parsed, error: '' }
  } catch (err: any) {
    return { value: {}, error: err?.message || `${fieldName} is not valid JSON.` }
  }
}

function setOptionalString(target: Record<string, any>, key: string, value: string) {
  const trimmed = value.trim()
  if (trimmed) {
    target[key] = trimmed
    return
  }
  delete target[key]
}

function buildConfigFromDraft(baseRaw: string, draft: LocalSettingsDraft): { value: Record<string, any>; error: string } {
  const parsedBase = parseLocalConfigObject(baseRaw)
  if (parsedBase.error) {
    return { value: defaultLocalConfigObject(), error: parsedBase.error }
  }

  const profiles = parseObjectField(draft.profilesJson, 'profiles')
  if (profiles.error) {
    return { value: parsedBase.value, error: profiles.error }
  }

  const connections = parseObjectField(draft.connectionsJson, 'local.connections')
  if (connections.error) {
    return { value: parsedBase.value, error: connections.error }
  }

  const next = JSON.parse(JSON.stringify(parsedBase.value || defaultLocalConfigObject())) as Record<string, any>
  const local = isRecord(next.local) ? next.local : {}
  const currentTarget = normalizeTargetInput(draft.currentTarget)
  setOptionalString(next, 'current_target', currentTarget.target)
  setOptionalString(next, 'current_profile', currentTarget.profile)
  next.profiles = profiles.value

  setOptionalString(local, 'listen_addr', draft.listenAddr)
  setOptionalString(local, 'storage', draft.storage)
  setOptionalString(local, 'sqlite_path', draft.sqlitePath)
  setOptionalString(local, 'database_url', draft.databaseURL)
  setOptionalString(local, 'git_mirror_path', draft.gitMirrorPath)
  setOptionalString(local, 'public_base_url', draft.publicBaseURL)
  setOptionalString(local, 'jwt_secret', draft.jwtSecret)
  setOptionalString(local, 'vault_master_key', draft.vaultMasterKey)
  local.connections = connections.value

  next.local = local
  return { value: next, error: '' }
}

export default function DataSyncPage() {
  const { locale, tx } = useI18n()
  const [configViewMode, setConfigViewMode] = useState<ConfigViewMode>('settings')
  const [localConfig, setLocalConfig] = useState<LocalConfigFile | null>(null)
  const [localConfigBusy, setLocalConfigBusy] = useState(false)
  const [localConfigSaving, setLocalConfigSaving] = useState(false)
  const [localConfigError, setLocalConfigError] = useState('')
  const [localConfigMessage, setLocalConfigMessage] = useState('')
  const [localConfigRaw, setLocalConfigRaw] = useState('')
  const [settingsDraft, setSettingsDraft] = useState<LocalSettingsDraft>(() => draftFromRaw(''))
  const [visibleSecrets, setVisibleSecrets] = useState<Record<string, boolean>>({})
  const [gitMirror, setGitMirror] = useState<GitMirrorSettings | null>(null)
  const [gitMirrorBusy, setGitMirrorBusy] = useState(false)
  const [gitMirrorSaving, setGitMirrorSaving] = useState(false)
  const [gitMirrorTesting, setGitMirrorTesting] = useState(false)
  const [gitMirrorError, setGitMirrorError] = useState('')
  const [gitMirrorMessage, setGitMirrorMessage] = useState('')
  const [gitMirrorTokenInput, setGitMirrorTokenInput] = useState('')
  const [gitMirrorTokenTest, setGitMirrorTokenTest] = useState<GitMirrorGitHubTestResult | null>(null)
  const [gitMirrorDraft, setGitMirrorDraft] = useState<UpdateGitMirrorRequest>({
    auto_commit_enabled: false,
    auto_push_enabled: false,
    auth_mode: 'local_credentials',
    remote_name: 'origin',
    remote_url: '',
    remote_branch: 'main',
  })
  const gitMirrorEnabled = !!gitMirror?.enabled

  const syncLocalConfig = (config: LocalConfigFile) => {
    setLocalConfig(config)
    setLocalConfigRaw(config.raw || '')
    setSettingsDraft(draftFromRaw(config.raw || ''))
  }

  const syncGitMirrorDraft = (settings: GitMirrorSettings) => {
    setGitMirrorDraft({
      auto_commit_enabled: settings.auto_commit_enabled,
      auto_push_enabled: settings.auto_push_enabled,
      auth_mode: settings.auth_mode,
      remote_name: settings.remote_name || 'origin',
      remote_url: settings.remote_url || '',
      remote_branch: settings.remote_branch || 'main',
    })
    setGitMirrorTokenInput('')
    setGitMirrorTokenTest(null)
  }

  const loadLocalConfig = async () => {
    setLocalConfigBusy(true)
    setLocalConfigError('')
    try {
      const config = await api.getLocalConfig()
      syncLocalConfig(config)
    } catch (err: any) {
      setLocalConfigError(err.message || tx('加载 config.json 失败', 'Failed to load config.json'))
    } finally {
      setLocalConfigBusy(false)
    }
  }

  const loadGitMirror = async () => {
    setGitMirrorBusy(true)
    setGitMirrorError('')
    try {
      const settings = await api.getLocalGitMirror()
      setGitMirror(settings)
      syncGitMirrorDraft(settings)
    } catch (err: any) {
      setGitMirrorError(err.message || tx('加载 Git Mirror 配置失败', 'Failed to load Git Mirror settings'))
    } finally {
      setGitMirrorBusy(false)
    }
  }

  useEffect(() => {
    void loadLocalConfig()
    void loadGitMirror()
  }, [])

  const localConfigRawValidationError = useMemo(() => {
    if (!localConfigRaw.trim()) {
      return tx('config.json 不能为空。', 'config.json cannot be empty.')
    }
    const parsed = parseLocalConfigObject(localConfigRaw)
    if (parsed.error) {
      return parsed.error
    }
    return ''
  }, [localConfigRaw, tx])

  const settingsValidationError = useMemo(() => {
    const result = buildConfigFromDraft(localConfigRaw, settingsDraft)
    return result.error
  }, [localConfigRaw, settingsDraft])

  const activeConfigValidationError = configViewMode === 'raw'
    ? localConfigRawValidationError
    : settingsValidationError

  const updateSettingsDraft = (patch: Partial<LocalSettingsDraft>) => {
    setSettingsDraft((prev) => ({ ...prev, ...patch }))
    setLocalConfigMessage('')
    setLocalConfigError('')
  }

  const toggleSecretVisibility = (key: string) => {
    setVisibleSecrets((prev) => ({ ...prev, [key]: !prev[key] }))
  }

  const handleSwitchToRaw = () => {
    const next = buildConfigFromDraft(localConfigRaw, settingsDraft)
    if (next.error) {
      setLocalConfigError(next.error)
      setLocalConfigMessage('')
      return
    }
    setLocalConfigRaw(stringifyLocalConfig(next.value))
    setConfigViewMode('raw')
    setLocalConfigError('')
    setLocalConfigMessage('')
  }

  const handleSwitchToSettings = () => {
    const parsed = parseLocalConfigObject(localConfigRaw)
    if (parsed.error) {
      setLocalConfigError(tx('当前 config.json 不是有效 JSON，修复后才能切回设置视图。', 'Fix the current config.json before switching back to the settings view.'))
      setLocalConfigMessage('')
      return
    }
    setSettingsDraft(draftFromRaw(localConfigRaw))
    setConfigViewMode('settings')
    setLocalConfigError('')
    setLocalConfigMessage('')
  }

  const handleLocalConfigSave = async () => {
    const rawToSave = configViewMode === 'raw'
      ? localConfigRaw
      : stringifyLocalConfig(buildConfigFromDraft(localConfigRaw, settingsDraft).value)

    if (activeConfigValidationError) {
      setLocalConfigError(activeConfigValidationError)
      setLocalConfigMessage('')
      return
    }

    setLocalConfigSaving(true)
    setLocalConfigError('')
    setLocalConfigMessage('')
    try {
      const saved = await api.updateLocalConfig({ raw: rawToSave })
      syncLocalConfig(saved)
      setLocalConfigMessage(tx('系统设置已保存', 'System settings saved'))
    } catch (err: any) {
      setLocalConfigError(err.message || tx('保存系统设置失败', 'Failed to save system settings'))
    } finally {
      setLocalConfigSaving(false)
    }
  }

  const updateGitMirrorDraft = (patch: Partial<UpdateGitMirrorRequest>) => {
    setGitMirrorDraft((prev) => {
      const next = { ...prev, ...patch }
      if (!next.auto_commit_enabled) {
        next.auto_push_enabled = false
      }
      if (next.auto_push_enabled) {
        next.auto_commit_enabled = true
      }
      return next
    })
    setGitMirrorMessage('')
    setGitMirrorError('')
    setGitMirrorTokenTest(null)
  }

  const gitMirrorVerificationCurrent = useMemo(() => {
    if (!gitMirror) return false
    if (gitMirrorDraft.auth_mode !== 'github_token') return true
    if (gitMirrorTokenInput.trim()) {
      return !!gitMirrorTokenTest?.ok
    }
    return !!gitMirror.github_token_configured &&
      !!gitMirror.github_token_verified_at &&
      (gitMirror.remote_url || '') === (gitMirrorDraft.remote_url || '')
  }, [gitMirror, gitMirrorDraft.auth_mode, gitMirrorDraft.remote_url, gitMirrorTokenInput, gitMirrorTokenTest])

  const handleGitMirrorTest = async () => {
    setGitMirrorTesting(true)
    setGitMirrorError('')
    setGitMirrorMessage('')
    try {
      const result = await api.testGitMirrorGitHubToken({
        remote_url: gitMirrorDraft.remote_url || '',
        github_token: gitMirrorTokenInput.trim(),
      })
      setGitMirrorTokenTest(result)
      if (result.normalized_remote_url) {
        setGitMirrorDraft((prev) => ({ ...prev, remote_url: result.normalized_remote_url || prev.remote_url }))
      }
      if (!result.ok) {
        setGitMirrorError(result.message || tx('GitHub token 校验失败', 'GitHub token validation failed'))
      } else {
        setGitMirrorMessage(result.message || tx('GitHub token 可用', 'GitHub token is valid'))
      }
    } catch (err: any) {
      setGitMirrorError(err.message || tx('GitHub token 测试失败', 'Failed to test GitHub token'))
    } finally {
      setGitMirrorTesting(false)
    }
  }

  const handleGitMirrorSave = async () => {
    if (!gitMirrorEnabled) {
      setGitMirrorError(tx('请先运行 `neudrive git init` 初始化本地 mirror，然后再保存这些设置。', 'Run `neudrive git init` first to initialize the local mirror, then save these settings.'))
      setGitMirrorMessage('')
      return
    }
    if (gitMirrorDraft.auth_mode === 'github_token' && gitMirrorDraft.auto_push_enabled && !gitMirrorVerificationCurrent) {
      setGitMirrorError(tx('启用 GitHub token 自动推送前，请先测试并确认 token 可用。', 'Test and verify the GitHub token before enabling auto push.'))
      return
    }
    setGitMirrorSaving(true)
    setGitMirrorError('')
    setGitMirrorMessage('')
    try {
      const saved = await api.updateLocalGitMirror({
        ...gitMirrorDraft,
        github_token: gitMirrorTokenInput.trim() || undefined,
      })
      setGitMirror(saved)
      syncGitMirrorDraft(saved)
      setGitMirrorMessage(tx('Git Mirror 配置已保存', 'Git Mirror settings saved'))
    } catch (err: any) {
      setGitMirrorError(err.message || tx('保存 Git Mirror 配置失败', 'Failed to save Git Mirror settings'))
    } finally {
      setGitMirrorSaving(false)
    }
  }

  const handleGitMirrorClearToken = async () => {
    setGitMirrorSaving(true)
    setGitMirrorError('')
    setGitMirrorMessage('')
    try {
      const saved = await api.updateLocalGitMirror({
        ...gitMirrorDraft,
        auto_push_enabled: false,
        clear_github_token: true,
      })
      setGitMirror(saved)
      syncGitMirrorDraft(saved)
      setGitMirrorMessage(tx('已清除保存的 GitHub token', 'Saved GitHub token was cleared'))
    } catch (err: any) {
      setGitMirrorError(err.message || tx('清除 GitHub token 失败', 'Failed to clear the GitHub token'))
    } finally {
      setGitMirrorSaving(false)
    }
  }

  const renderSensitiveInput = (
    id: string,
    value: string,
    onChange: (value: string) => void,
    visibilityKey: string,
    placeholder?: string,
  ) => {
    const visible = !!visibleSecrets[visibilityKey]
    const hasValue = value.trim().length > 0
    return (
      <div className="data-sync-secret-row">
        <input
          id={id}
          className="data-sync-secret-input"
          type="text"
          value={visible ? value : (hasValue ? '******' : '')}
          readOnly={!visible}
          onChange={(e) => {
            if (!visible) return
            onChange(e.target.value)
          }}
          placeholder={visible ? placeholder : (placeholder || '******')}
        />
        <button
          type="button"
          className="btn data-sync-visibility-btn"
          onClick={() => toggleSecretVisibility(visibilityKey)}
          aria-label={visible ? tx('隐藏敏感值', 'Hide sensitive value') : tx('显示敏感值', 'Show sensitive value')}
          title={visible ? tx('隐藏', 'Hide') : tx('显示', 'Show')}
        >
          <EyeIcon visible={visible} />
        </button>
      </div>
    )
  }

  return (
    <div className="page materials-page">
      <section className="materials-hero">
        <div className="materials-hero-copy">
          <div className="materials-kicker">neuDrive Local</div>
          <h2 className="materials-title">{tx('系统设置', 'System Settings')}</h2>
          <p className="materials-subtitle">{tx('这里集中管理本地 config.json 与 Git Mirror。默认先展示更友好的设置表单；切到 config.json 时也能直接查看和编辑原始 JSON。', 'Manage local config.json and Git Mirror here. The default view is a friendlier settings form, and you can still switch to config.json to edit the raw JSON directly.')}</p>
        </div>
      </section>

      <div className="materials-panel data-sync-card">
        <div className="card-header">
          <h3 className="card-title">{tx('本地配置', 'Local Configuration')}</h3>
        </div>
        <p className="data-record-secondary">
          {tx('“设置” 视图会把常用配置拆成更容易理解的表单，并解释每个字段的作用；“config.json” 视图则显示当前完整原始内容。', 'The "Settings" view breaks common configuration into more approachable fields with explanations, while "config.json" shows the full raw file content.')}
        </p>
        {localConfig?.path && (
          <div className="data-record-secondary">
            {tx('文件位置：', 'File path: ')}<code>{localConfig.path}</code>
          </div>
        )}
        <div className="alert alert-warn" style={{ marginTop: 12 }}>
          {tx('注意：这里可能包含 token、密钥和本地路径。部分配置变更需要重启本地 daemon 后才会生效。', 'This file can contain tokens, secrets, and local paths. Some changes require restarting the local daemon before they take effect.')}
        </div>
        {localConfigBusy && <div className="page-loading">{tx('加载中...', 'Loading...')}</div>}
        {!localConfigBusy && (
          <>
            {configViewMode === 'settings' ? (
              <div className="data-sync-settings-shell">
                <section className="data-sync-settings-section">
                  <h4 className="data-sync-section-title">{tx('基础配置', 'Basics')}</h4>
                  <div className="data-sync-settings-grid">
                    <div className="form-group">
                      <label htmlFor="config-current-target">{tx('当前 target', 'Current target')}</label>
                      <div className="data-sync-field-note">{tx('CLI 默认使用的 target。填 `local` 或 `profile:official`；也可以直接填 profile 名称。', 'The default CLI target. Use `local` or `profile:official`; a bare profile name also works.')}</div>
                      <input
                        id="config-current-target"
                        value={settingsDraft.currentTarget}
                        onChange={(e) => updateSettingsDraft({ currentTarget: e.target.value })}
                        placeholder={tx('例如 local 或 profile:official', 'For example local or profile:official')}
                      />
                    </div>
                    <div className="form-group">
                      <label htmlFor="config-public-base-url">{tx('本地公开地址', 'Public base URL')}</label>
                      <div className="data-sync-field-note">{tx('Dashboard、OAuth 回调和本地 CLI 页面跳转时使用的服务地址。', 'The service URL used by the dashboard, OAuth callbacks, and local CLI browser redirects.')}</div>
                      <input
                        id="config-public-base-url"
                        value={settingsDraft.publicBaseURL}
                        onChange={(e) => updateSettingsDraft({ publicBaseURL: e.target.value })}
                        placeholder="http://127.0.0.1:42690"
                      />
                    </div>
                  </div>
                </section>

                <section className="data-sync-settings-section">
                  <h4 className="data-sync-section-title">{tx('本地服务', 'Local Runtime')}</h4>
                  <div className="data-sync-settings-grid">
                    <div className="form-group">
                      <label htmlFor="config-listen-addr">{tx('监听地址', 'Listen address')}</label>
                      <div className="data-sync-field-note">{tx('本地 daemon 监听的 host:port。通常保持自动分配的地址即可。', 'The host:port used by the local daemon. The auto-assigned address is usually the safest choice.')}</div>
                      <input
                        id="config-listen-addr"
                        value={settingsDraft.listenAddr}
                        onChange={(e) => updateSettingsDraft({ listenAddr: e.target.value })}
                        placeholder="127.0.0.1:42690"
                      />
                    </div>
                    <div className="form-group">
                      <label htmlFor="config-storage">{tx('存储后端', 'Storage backend')}</label>
                      <div className="data-sync-field-note">{tx('本地模式通常使用 SQLite；如果你手动接 Postgres，也可以在这里切换。', 'Local mode usually uses SQLite. Switch this only if you intentionally want to use Postgres.')}</div>
                      <select
                        id="config-storage"
                        value={settingsDraft.storage}
                        onChange={(e) => updateSettingsDraft({ storage: e.target.value })}
                      >
                        <option value="">{tx('自动', 'Auto')}</option>
                        <option value="sqlite">SQLite</option>
                        <option value="postgres">Postgres</option>
                      </select>
                    </div>
                    <div className="form-group">
                      <label htmlFor="config-sqlite-path">{tx('SQLite 文件路径', 'SQLite file path')}</label>
                      <div className="data-sync-field-note">{tx('当存储后端是 SQLite 时，本地数据库会写到这里。', 'When the storage backend is SQLite, the local database file is stored here.')}</div>
                      <input
                        id="config-sqlite-path"
                        value={settingsDraft.sqlitePath}
                        onChange={(e) => updateSettingsDraft({ sqlitePath: e.target.value })}
                        placeholder="~/.local/share/neudrive/local.db"
                      />
                    </div>
                    <div className="form-group">
                      <label htmlFor="config-database-url">{tx('Postgres 连接串', 'Postgres URL')}</label>
                      <div className="data-sync-field-note">{tx('只有在你把存储后端切到 Postgres 时才会使用。', 'Only used when the storage backend is switched to Postgres.')}</div>
                      <input
                        id="config-database-url"
                        value={settingsDraft.databaseURL}
                        onChange={(e) => updateSettingsDraft({ databaseURL: e.target.value })}
                        placeholder="postgres://user:pass@host:5432/neudrive?sslmode=disable"
                      />
                    </div>
                    <div className="form-group">
                      <label htmlFor="config-git-mirror-path">{tx('Git Mirror 目录', 'Git Mirror path')}</label>
                      <div className="data-sync-field-note">{tx('`neudrive git init` 默认会把 Hub 导出到这里。', 'This is the default export directory used by `neudrive git init`.')}</div>
                      <input
                        id="config-git-mirror-path"
                        value={settingsDraft.gitMirrorPath}
                        onChange={(e) => updateSettingsDraft({ gitMirrorPath: e.target.value })}
                        placeholder="./neudrive-export/git-mirror"
                      />
                    </div>
                  </div>
                </section>

                <section className="data-sync-settings-section">
                  <h4 className="data-sync-section-title">{tx('安全与令牌', 'Security and Tokens')}</h4>
                  <div className="data-sync-settings-grid">
                    <div className="form-group">
                      <label htmlFor="config-jwt-secret">JWT Secret</label>
                      <div className="data-sync-field-note">{tx('本地登录 token 的签名密钥。改动后通常需要重启 daemon。', 'Signing key for local login tokens. Restarting the daemon is usually required after changing it.')}</div>
                      {renderSensitiveInput(
                        'config-jwt-secret',
                        settingsDraft.jwtSecret,
                        (next) => updateSettingsDraft({ jwtSecret: next }),
                        'jwtSecret',
                      )}
                    </div>
                    <div className="form-group">
                      <label htmlFor="config-vault-master-key">{tx('Vault 主密钥', 'Vault master key')}</label>
                      <div className="data-sync-field-note">{tx('用来加密本地 secret。除非你明确知道自己在做什么，否则不要随意更换。', 'Used to encrypt local secrets. Do not change it casually unless you know exactly what you are doing.')}</div>
                      {renderSensitiveInput(
                        'config-vault-master-key',
                        settingsDraft.vaultMasterKey,
                        (next) => updateSettingsDraft({ vaultMasterKey: next }),
                        'vaultMasterKey',
                      )}
                    </div>
                  </div>
                </section>

                <section className="data-sync-settings-section">
                  <h4 className="data-sync-section-title">{tx('高级 JSON 配置', 'Advanced JSON Settings')}</h4>
                  <div className="data-sync-settings-grid data-sync-settings-grid-wide">
                    <div className="form-group">
                      <label htmlFor="config-profiles-json">{tx('远端 profiles', 'Remote profiles')}</label>
                      <div className="data-sync-field-note">{tx('这里保存 hosted profiles 的 API 地址、token、refresh token、auth mode 和过期时间。保持 JSON object 结构。', 'Stores hosted profile API bases, tokens, refresh tokens, auth modes, and expiry times. Keep this as a JSON object.')}</div>
                      <textarea
                        id="config-profiles-json"
                        className="data-sync-json-editor"
                        spellCheck={false}
                        value={settingsDraft.profilesJson}
                        onChange={(e) => updateSettingsDraft({ profilesJson: e.target.value })}
                      />
                      <div className="data-sync-example">
                        <div className="data-sync-example-title">{tx('Example', 'Example')}</div>
                        <pre>{REMOTE_PROFILES_EXAMPLE}</pre>
                      </div>
                    </div>
                    <div className="form-group">
                      <label htmlFor="config-connections-json">{tx('本地连接定义', 'Local connections')}</label>
                      <div className="data-sync-field-note">{tx('这里保存本地连接器、受管路径和平台回填信息。保持 JSON object 结构。', 'Stores local connector definitions, managed paths, and platform callback metadata. Keep this as a JSON object.')}</div>
                      <textarea
                        id="config-connections-json"
                        className="data-sync-json-editor"
                        spellCheck={false}
                        value={settingsDraft.connectionsJson}
                        onChange={(e) => updateSettingsDraft({ connectionsJson: e.target.value })}
                      />
                      <div className="data-sync-example">
                        <div className="data-sync-example-title">{tx('Example', 'Example')}</div>
                        <pre>{LOCAL_CONNECTIONS_EXAMPLE}</pre>
                      </div>
                    </div>
                  </div>
                </section>
              </div>
            ) : (
              <>
                <p className="data-record-secondary" style={{ marginTop: 12 }}>
                  {tx('当前显示完整的 config.json 原始内容。这里适合处理表单视图暂时没有覆盖到的高级配置。', 'You are viewing the full raw config.json content. This is useful for advanced settings not covered by the form view yet.')}
                </p>
                <textarea
                  className="data-sync-config-editor"
                  aria-label="Local config.json editor"
                  spellCheck={false}
                  value={localConfigRaw}
                  onChange={(e) => {
                    setLocalConfigRaw(e.target.value)
                    setLocalConfigMessage('')
                    setLocalConfigError('')
                  }}
                />
              </>
            )}

            {activeConfigValidationError && <div className="alert alert-warn" style={{ marginTop: 12 }}>{activeConfigValidationError}</div>}
            {localConfigMessage && <div className="alert alert-success" style={{ marginTop: 12 }}>{localConfigMessage}</div>}
            {localConfigError && <div className="alert alert-error">{localConfigError}</div>}

            <div className="data-sync-actions">
              <button className="btn" onClick={() => { void loadLocalConfig() }} disabled={localConfigBusy || localConfigSaving}>
                {tx('重新加载', 'Reload')}
              </button>
              <button className="btn btn-primary" onClick={() => { void handleLocalConfigSave() }} disabled={localConfigBusy || localConfigSaving || !!activeConfigValidationError}>
                {localConfigSaving ? tx('保存中...', 'Saving...') : tx('保存', 'Save')}
              </button>
              <button
                className={`btn ${configViewMode === 'settings' ? 'data-sync-mode-button-active' : ''}`}
                onClick={handleSwitchToSettings}
                disabled={localConfigBusy || localConfigSaving}
              >
                {tx('设置', 'Settings')}
              </button>
              <button
                className={`btn ${configViewMode === 'raw' ? 'data-sync-mode-button-active' : ''}`}
                onClick={handleSwitchToRaw}
                disabled={localConfigBusy || localConfigSaving}
              >
                config.json
              </button>
            </div>
          </>
        )}
      </div>

      <div className="materials-panel data-sync-card">
        <div className="card-header">
          <h3 className="card-title">{tx('Git Mirror', 'Git Mirror')}</h3>
        </div>
        <p className="data-record-secondary">
          {tx('每次 Hub 写入后，本地 mirror 会先刷新文件，再按这里的设置自动 commit，并可选自动 push 到 GitHub。', 'After each Hub write, the local mirror refreshes first, then auto-commits and can optionally auto-push to GitHub from this configuration.')}
        </p>
        {gitMirrorBusy && <div className="page-loading">{tx('加载中...', 'Loading...')}</div>}
        {!gitMirrorBusy && gitMirror && !gitMirrorEnabled && (
          <div className="alert alert-warn">
            {tx('当前还没有初始化 Git Mirror。你现在可以先查看下面的配置项；运行 `neudrive git init` 后，再回来保存并启用自动 commit / push。', 'No Git Mirror is initialized yet. You can review the configuration below now; run `neudrive git init` before coming back to save and enable auto commit / push.')}
          </div>
        )}
        {!gitMirrorBusy && gitMirror && (
          <>
            {gitMirrorEnabled && (
              <div className="data-sync-status-grid">
                <div className="data-sync-status-card">
                  <div className="data-record-title">{tx('镜像目录', 'Mirror path')}</div>
                  <code>{gitMirror.path}</code>
                </div>
                <div className="data-sync-status-card">
                  <div className="data-record-title">{tx('最近同步', 'Last sync')}</div>
                  <div className="data-record-secondary">{formatDateTime(gitMirror.last_synced_at, locale)}</div>
                  {gitMirror.message && <div className="data-record-secondary">{gitMirror.message}</div>}
                </div>
                <div className="data-sync-status-card">
                  <div className="data-record-title">{tx('最近提交', 'Last commit')}</div>
                  <div className="data-record-secondary">{gitMirror.last_commit_hash ? `${gitMirror.last_commit_hash.slice(0, 8)} · ${formatDateTime(gitMirror.last_commit_at, locale)}` : tx('暂无', 'None yet')}</div>
                </div>
                <div className="data-sync-status-card">
                  <div className="data-record-title">{tx('最近推送', 'Last push')}</div>
                  <div className="data-record-secondary">{gitMirror.last_push_at ? formatDateTime(gitMirror.last_push_at, locale) : tx('暂无', 'None yet')}</div>
                  {gitMirror.last_push_error && <div className="data-record-secondary">{gitMirror.last_push_error}</div>}
                </div>
              </div>
            )}

            <div className="data-sync-settings-shell">
              <section className="data-sync-settings-section">
                <h4 className="data-sync-section-title">{tx('同步行为', 'Sync behavior')}</h4>
                <div className="data-sync-toggle-grid">
                  <label className="data-sync-toggle-card">
                    <div className="data-sync-toggle-copy">
                      <div className="data-sync-toggle-title">{tx('自动 commit', 'Auto commit')}</div>
                      <div className="data-sync-field-note">
                        {tx('每次 Hub 写入后，自动在本地 mirror 仓库创建一条提交。', 'Automatically creates a commit in the local mirror repository after each Hub write.')}
                      </div>
                    </div>
                    <input
                      type="checkbox"
                      checked={gitMirrorDraft.auto_commit_enabled}
                      onChange={(e) => updateGitMirrorDraft({ auto_commit_enabled: e.target.checked })}
                    />
                  </label>
                  <label className="data-sync-toggle-card">
                    <div className="data-sync-toggle-copy">
                      <div className="data-sync-toggle-title">{tx('自动 push', 'Auto push')}</div>
                      <div className="data-sync-field-note">
                        {tx('把最新提交自动推送到远端；启用时会同时确保自动 commit 处于开启状态。', 'Automatically pushes the latest commit to the remote. Enabling this also ensures auto commit stays on.')}
                      </div>
                    </div>
                    <input
                      type="checkbox"
                      checked={gitMirrorDraft.auto_push_enabled}
                      onChange={(e) => updateGitMirrorDraft({ auto_push_enabled: e.target.checked, auto_commit_enabled: e.target.checked ? true : gitMirrorDraft.auto_commit_enabled })}
                    />
                  </label>
                </div>
              </section>

              <section className="data-sync-settings-section">
                <h4 className="data-sync-section-title">{tx('远端配置', 'Remote settings')}</h4>
                <div className="data-sync-settings-grid">
                  <div className="form-group">
                    <label htmlFor="git-mirror-auth-mode">{tx('认证方式', 'Auth mode')}</label>
                    <div className="data-sync-field-note">{tx('选择是复用本机 Git 凭证，还是为这个 mirror 单独保存 GitHub token。', 'Choose whether to reuse your machine Git credentials or save a dedicated GitHub token for this mirror.')}</div>
                    <select
                      id="git-mirror-auth-mode"
                      aria-label="Git mirror auth mode"
                      value={gitMirrorDraft.auth_mode}
                      onChange={(e) => updateGitMirrorDraft({ auth_mode: e.target.value as 'local_credentials' | 'github_token' })}
                    >
                      <option value="local_credentials">{tx('本机 Git 凭证', 'Local Git credentials')}</option>
                      <option value="github_token">{tx('GitHub Token', 'GitHub token')}</option>
                    </select>
                  </div>
                  <div className="form-group">
                    <label htmlFor="git-mirror-remote-url">{tx('仓库 URL', 'Repository URL')}</label>
                    <div className="data-sync-field-note">{tx('填写 GitHub 仓库地址，例如 HTTPS clone URL。', 'Enter the GitHub repository address, for example the HTTPS clone URL.')}</div>
                    <input
                      id="git-mirror-remote-url"
                      aria-label="Git mirror repo URL"
                      value={gitMirrorDraft.remote_url || ''}
                      onChange={(e) => updateGitMirrorDraft({ remote_url: e.target.value })}
                      placeholder={tx('GitHub repo URL，例如 https://github.com/org/repo.git', 'GitHub repo URL, for example https://github.com/org/repo.git')}
                    />
                  </div>
                  <div className="form-group">
                    <label htmlFor="git-mirror-remote-branch">{tx('目标分支', 'Target branch')}</label>
                    <div className="data-sync-field-note">{tx('自动 push 时默认推送到这个分支。', 'Auto push targets this branch by default.')}</div>
                    <input
                      id="git-mirror-remote-branch"
                      aria-label="Git mirror branch"
                      value={gitMirrorDraft.remote_branch || ''}
                      onChange={(e) => updateGitMirrorDraft({ remote_branch: e.target.value })}
                      placeholder="main"
                    />
                  </div>
                  <div className="form-group">
                    <label htmlFor="git-mirror-remote-name">Remote name</label>
                    <div className="data-sync-field-note">{tx('当前固定使用 origin。', 'This is currently fixed to origin.')}</div>
                    <input
                      id="git-mirror-remote-name"
                      value={gitMirrorDraft.remote_name || 'origin'}
                      disabled
                      readOnly
                    />
                  </div>
                </div>
              </section>

              {gitMirrorDraft.auth_mode === 'local_credentials' ? (
                <section className="data-sync-settings-section">
                  <h4 className="data-sync-section-title">{tx('认证说明', 'Credential notes')}</h4>
                  <div className="alert alert-warn">
                    {tx('本模式会复用你机器上现有的 Git 凭证（SSH key、credential helper 或已登录 Git 环境）。', 'This mode reuses the Git credentials already available on your machine, such as SSH keys or a credential helper.')}
                  </div>
                </section>
              ) : (
                <section className="data-sync-settings-section">
                  <h4 className="data-sync-section-title">{tx('GitHub Token', 'GitHub token')}</h4>
                  <div className="data-sync-settings-grid">
                    <div className="form-group data-sync-settings-span-wide">
                      <label htmlFor="git-mirror-token">{tx('GitHub Token', 'GitHub token')}</label>
                      <div className="data-sync-field-note">
                        {gitMirror.github_token_configured
                          ? tx('当前已经保存过一个 token；这里不会回显原值。填写新 token 会替换旧值。', 'A token is already saved; its raw value is never shown again here. Entering a new token will replace it.')
                          : tx('当前还没有保存 GitHub token。', 'No GitHub token is saved yet.')}
                      </div>
                      <div className="data-sync-secret-row">
                        <input
                          id="git-mirror-token"
                          aria-label="GitHub token"
                          className="data-sync-secret-input"
                          type={visibleSecrets.gitMirrorToken ? 'text' : 'password'}
                          value={gitMirrorTokenInput}
                          onChange={(e) => {
                            setGitMirrorTokenInput(e.target.value)
                            setGitMirrorTokenTest(null)
                            setGitMirrorError('')
                            setGitMirrorMessage('')
                          }}
                          placeholder={tx('******', '******')}
                        />
                        <button
                          type="button"
                          className="btn data-sync-visibility-btn"
                          onClick={() => toggleSecretVisibility('gitMirrorToken')}
                          aria-label={visibleSecrets.gitMirrorToken ? tx('隐藏 GitHub token', 'Hide GitHub token') : tx('显示 GitHub token', 'Show GitHub token')}
                          title={visibleSecrets.gitMirrorToken ? tx('隐藏', 'Hide') : tx('显示', 'Show')}
                        >
                          <EyeIcon visible={!!visibleSecrets.gitMirrorToken} />
                        </button>
                      </div>
                    </div>
                  </div>
                  <div className="data-sync-actions data-sync-actions-compact">
                    <button className="btn" onClick={() => { void handleGitMirrorTest() }} disabled={gitMirrorTesting || !gitMirrorDraft.remote_url || !gitMirrorTokenInput.trim()}>
                      {gitMirrorTesting ? tx('测试中...', 'Testing...') : tx('测试 Token', 'Test token')}
                    </button>
                    {gitMirror.github_token_configured && (
                      <button className="btn" onClick={() => { void handleGitMirrorClearToken() }} disabled={gitMirrorSaving}>
                        {tx('清除已保存 Token', 'Clear saved token')}
                      </button>
                    )}
                  </div>
                  <div className="data-inline-list" style={{ marginTop: 12 }}>
                    <span className="token-list-prefix">
                      {tx('已保存 Token', 'Saved token')} {gitMirror.github_token_configured ? tx('是', 'Yes') : tx('否', 'No')}
                    </span>
                    {gitMirror.github_token_login && (
                      <span className="token-list-prefix">
                        GitHub {gitMirror.github_token_login}
                      </span>
                    )}
                    {gitMirror.github_repo_permission && (
                      <span className="token-list-prefix">
                        {tx('仓库权限', 'Repo permission')} {gitMirror.github_repo_permission}
                      </span>
                    )}
                  </div>
                  {gitMirror.github_token_verified_at && (
                    <div className="data-record-secondary" style={{ marginTop: 8 }}>
                      {tx('最近验证时间：', 'Last verified: ')}{formatDateTime(gitMirror.github_token_verified_at, locale)}
                    </div>
                  )}
                  {gitMirrorTokenTest && (
                    <div className={gitMirrorTokenTest.ok ? 'alert alert-success' : 'alert alert-warn'} style={{ marginTop: 12 }}>
                      {gitMirrorTokenTest.message}
                    </div>
                  )}
                </section>
              )}
            </div>

            {gitMirrorMessage && <div className="alert alert-success" style={{ marginTop: 12 }}>{gitMirrorMessage}</div>}
            {gitMirrorError && <div className="alert alert-error">{gitMirrorError}</div>}
            {gitMirrorEnabled && gitMirror.last_error && <div className="alert alert-warn">{gitMirror.last_error}</div>}

            <div className="data-sync-actions">
              <button className="btn btn-primary" onClick={() => { void handleGitMirrorSave() }} disabled={gitMirrorSaving || !gitMirrorEnabled}>
                {gitMirrorSaving ? tx('保存中...', 'Saving...') : tx('保存 Git Mirror 配置', 'Save Git Mirror settings')}
              </button>
              {!gitMirrorEnabled && (
                <span className="data-record-secondary">
                  {tx('先运行 `neudrive git init`，再保存这些配置。', 'Run `neudrive git init` before saving these settings.')}
                </span>
              )}
              {gitMirrorDraft.auth_mode === 'github_token' && gitMirrorDraft.auto_push_enabled && !gitMirrorVerificationCurrent && (
                <span className="data-record-secondary">
                  {tx('启用 GitHub token 自动推送前，需要先测试 token。', 'Test the token before enabling GitHub-token auto push.')}
                </span>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  )
}
