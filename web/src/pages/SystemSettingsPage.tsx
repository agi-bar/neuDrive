import { useMemo, useState, useEffect } from 'react'
import { api, type LocalConfigFile } from '../api'
import { useI18n } from '../i18n'

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
    api_base: 'https://neudrive.ai',
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

export default function SystemSettingsPage() {
  const { tx } = useI18n()
  const [configViewMode, setConfigViewMode] = useState<ConfigViewMode>('settings')
  const [localConfig, setLocalConfig] = useState<LocalConfigFile | null>(null)
  const [localConfigBusy, setLocalConfigBusy] = useState(false)
  const [localConfigSaving, setLocalConfigSaving] = useState(false)
  const [localConfigError, setLocalConfigError] = useState('')
  const [localConfigMessage, setLocalConfigMessage] = useState('')
  const [localConfigRaw, setLocalConfigRaw] = useState('')
  const [settingsDraft, setSettingsDraft] = useState<LocalSettingsDraft>(() => draftFromRaw(''))
  const [visibleSecrets, setVisibleSecrets] = useState<Record<string, boolean>>({})

  const syncLocalConfig = (config: LocalConfigFile) => {
    setLocalConfig(config)
    setLocalConfigRaw(config.raw || '')
    setSettingsDraft(draftFromRaw(config.raw || ''))
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

  useEffect(() => {
    void loadLocalConfig()
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
          <p className="materials-subtitle">{tx('这里只保留本地 config.json 与 daemon 相关设置。Git Mirror 已拆到单独页面，方便 local 和 hosted 共用同一套操作流。', 'This page only manages the local config.json and daemon settings. Git Mirror now lives on its own page so local and hosted can share the same workflow.')}</p>
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
                      <input id="config-current-target" value={settingsDraft.currentTarget} onChange={(e) => updateSettingsDraft({ currentTarget: e.target.value })} />
                    </div>
                    <div className="form-group">
                      <label htmlFor="config-listen-addr">{tx('本地监听地址', 'Local listen address')}</label>
                      <div className="data-sync-field-note">{tx('本地 daemon 的 HTTP 监听地址。', 'The HTTP listen address for the local daemon.')}</div>
                      <input id="config-listen-addr" value={settingsDraft.listenAddr} onChange={(e) => updateSettingsDraft({ listenAddr: e.target.value })} placeholder="127.0.0.1:42690" />
                    </div>
                    <div className="form-group">
                      <label htmlFor="config-storage">{tx('本地存储后端', 'Local storage backend')}</label>
                      <div className="data-sync-field-note">{tx('通常保持 sqlite；只有你明确准备好了数据库时再切到 postgres。', 'Usually keep sqlite unless you have intentionally prepared a database for postgres.')}</div>
                      <input id="config-storage" value={settingsDraft.storage} onChange={(e) => updateSettingsDraft({ storage: e.target.value })} placeholder="sqlite" />
                    </div>
                    <div className="form-group">
                      <label htmlFor="config-sqlite-path">SQLite path</label>
                      <div className="data-sync-field-note">{tx('当 local.storage=sqlite 时使用的数据库文件位置。', 'The SQLite database file used when local.storage=sqlite.')}</div>
                      <input id="config-sqlite-path" value={settingsDraft.sqlitePath} onChange={(e) => updateSettingsDraft({ sqlitePath: e.target.value })} placeholder="~/.local/share/neudrive/neudrive.db" />
                    </div>
                    <div className="form-group">
                      <label htmlFor="config-database-url">Database URL</label>
                      <div className="data-sync-field-note">{tx('当 local.storage=postgres 时使用的数据库连接串。', 'The database connection string used when local.storage=postgres.')}</div>
                      {renderSensitiveInput('config-database-url', settingsDraft.databaseURL, (value) => updateSettingsDraft({ databaseURL: value }), 'database-url', 'postgres://localhost:5432/neudrive?sslmode=disable')}
                    </div>
                    <div className="form-group">
                      <label htmlFor="config-git-mirror-path">{tx('Git Mirror 默认目录', 'Default Git Mirror path')}</label>
                      <div className="data-sync-field-note">{tx('`neudrive git init` 默认会把 mirror 写到这里。', 'This is the default path used by `neudrive git init`.')}</div>
                      <input id="config-git-mirror-path" value={settingsDraft.gitMirrorPath} onChange={(e) => updateSettingsDraft({ gitMirrorPath: e.target.value })} placeholder="./neudrive-export/git-mirror" />
                    </div>
                    <div className="form-group">
                      <label htmlFor="config-public-base-url">Public base URL</label>
                      <div className="data-sync-field-note">{tx('Dashboard、OAuth 回调和本地 CLI 页面跳转时使用的服务地址。', 'The service URL used by the dashboard, OAuth callbacks, and local CLI browser redirects.')}</div>
                      <input id="config-public-base-url" value={settingsDraft.publicBaseURL} onChange={(e) => updateSettingsDraft({ publicBaseURL: e.target.value })} placeholder="http://127.0.0.1:42690" />
                    </div>
                  </div>
                </section>

                <section className="data-sync-settings-section">
                  <h4 className="data-sync-section-title">{tx('敏感配置', 'Sensitive settings')}</h4>
                  <div className="data-sync-settings-grid">
                    <div className="form-group">
                      <label htmlFor="config-jwt-secret">JWT secret</label>
                      <div className="data-sync-field-note">{tx('本地 auth token 签名密钥。通常只在自管部署或调试时才需要改。', 'The signing secret for local auth tokens. You usually only change this for self-managed setups or debugging.')}</div>
                      {renderSensitiveInput('config-jwt-secret', settingsDraft.jwtSecret, (value) => updateSettingsDraft({ jwtSecret: value }), 'jwt-secret')}
                    </div>
                    <div className="form-group">
                      <label htmlFor="config-vault-master-key">Vault master key</label>
                      <div className="data-sync-field-note">{tx('本地 vault 的主密钥；请谨慎修改。', 'The master key for the local vault. Change with care.')}</div>
                      {renderSensitiveInput('config-vault-master-key', settingsDraft.vaultMasterKey, (value) => updateSettingsDraft({ vaultMasterKey: value }), 'vault-master-key')}
                    </div>
                  </div>
                </section>

                <section className="data-sync-settings-section">
                  <h4 className="data-sync-section-title">{tx('高级 JSON 字段', 'Advanced JSON fields')}</h4>
                  <div className="data-sync-settings-grid data-sync-settings-grid-wide">
                    <div className="form-group data-sync-settings-span-wide">
                      <label htmlFor="config-profiles-json">profiles</label>
                      <div className="data-sync-field-note">{tx('hosted profile 集合。适合维护多个官方云端或自托管环境。', 'The set of hosted profiles. Useful when you maintain multiple official-cloud or self-hosted environments.')}</div>
                      <textarea id="config-profiles-json" className="data-sync-config-editor data-sync-json-editor" value={settingsDraft.profilesJson} onChange={(e) => updateSettingsDraft({ profilesJson: e.target.value })} />
                      <div className="data-sync-example">
                        <div className="data-sync-example-title">Example</div>
                        <pre>{REMOTE_PROFILES_EXAMPLE}</pre>
                      </div>
                    </div>
                    <div className="form-group data-sync-settings-span-wide">
                      <label htmlFor="config-connections-json">local.connections</label>
                      <div className="data-sync-field-note">{tx('本地 adapter / MCP 连接配置。', 'The local adapter / MCP connection map.')}</div>
                      <textarea id="config-connections-json" className="data-sync-config-editor data-sync-json-editor" value={settingsDraft.connectionsJson} onChange={(e) => updateSettingsDraft({ connectionsJson: e.target.value })} />
                      <div className="data-sync-example">
                        <div className="data-sync-example-title">Example</div>
                        <pre>{LOCAL_CONNECTIONS_EXAMPLE}</pre>
                      </div>
                    </div>
                  </div>
                </section>
              </div>
            ) : (
              <textarea className="data-sync-config-editor" value={localConfigRaw} onChange={(e) => { setLocalConfigRaw(e.target.value); setLocalConfigMessage(''); setLocalConfigError('') }} spellCheck={false} />
            )}

            <div className="data-sync-actions">
              <button type="button" className={`btn ${configViewMode === 'settings' ? 'data-sync-mode-button-active' : ''}`} onClick={handleSwitchToSettings}>
                {tx('设置视图', 'Settings view')}
              </button>
              <button type="button" className={`btn ${configViewMode === 'raw' ? 'data-sync-mode-button-active' : ''}`} onClick={handleSwitchToRaw}>
                config.json
              </button>
              <button type="button" className="btn btn-primary" disabled={localConfigSaving} onClick={handleLocalConfigSave}>
                {localConfigSaving ? tx('保存中...', 'Saving...') : tx('保存配置', 'Save settings')}
              </button>
            </div>
          </>
        )}
        {localConfigError && <div className="alert alert-warn" style={{ marginTop: 16 }}>{localConfigError}</div>}
        {localConfigMessage && <div className="alert alert-ok" style={{ marginTop: 16 }}>{localConfigMessage}</div>}
      </div>
    </div>
  )
}
