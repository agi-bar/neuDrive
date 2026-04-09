import { useEffect } from 'react'
import { useLocation } from 'react-router-dom'
import { useI18n } from '../../i18n'
import {
  EXPIRY_OPTIONS,
  TOKEN_ENV_NAME,
  TRUST_LEVELS,
  useSetup,
} from '../SetupPage'
import { SetupSection } from './SetupShared'

export default function SetupTokensPage() {
  const { tx } = useI18n()
  const location = useLocation()
  const {
    tokens,
    activeTokens,
    scopeInfo,
    copied,
    customScopes,
    editingTokenId,
    editingTokenName,
    expiryDays,
    handleCreateToken,
    handleRenameToken,
    handleRevoke,
    manualCreating,
    name,
    newToken,
    preset,
    renamingTokenId,
    setEditingTokenName,
    setExpiryDays,
    setName,
    setPreset,
    setTrustLevel,
    startRenameToken,
    toggleScope,
    trustLevel,
    trustLabel,
    presetLabel,
    formatExpiry,
    cancelRenameToken,
    copyToClipboard,
  } = useSetup()

  const trustLevels = [
    { value: 1, label: tx('L1 访客', 'L1 Visitor'), desc: tx('只能读取公开信息', 'Can only read public information') },
    { value: 2, label: tx('L2 共享', 'L2 Shared'), desc: tx('可读取有限共享资源', 'Can read limited shared resources') },
    { value: 3, label: tx('L3 工作', 'L3 Work'), desc: tx('可读写常规资源', 'Can read and write regular resources') },
    { value: 4, label: tx('L4 完全信任', 'L4 Full Trust'), desc: tx('完整访问权限', 'Full access') },
  ]

  const expiryOptions = [
    { value: 7, label: tx('7天', '7 days') },
    { value: 30, label: tx('30天', '30 days') },
    { value: 90, label: tx('90天', '90 days') },
    { value: 365, label: tx('365天', '365 days') },
    { value: 0, label: tx('永不过期', 'Never expires') },
  ]

  useEffect(() => {
    if (location.hash === '#token-creator') {
      document.getElementById('token-creator')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  }, [location.hash])

  return (
    <>
      <SetupSection
        icon={<>&#128273;</>}
        title={tx('Token 管理', 'Token Manager')}
        description={tx(`共 ${tokens.length} 个，${activeTokens.length} 个有效。为 GPT Actions、脚本或其他自定义用途创建和管理独立 Token。`, `${tokens.length} total, ${activeTokens.length} active. Create and manage standalone tokens for GPT Actions, scripts, or other custom use cases.`)}
      >
        <div className="setup-note setup-note-first">
          {tx('推荐优先把 token 放进环境变量 ', 'Prefer storing tokens in the environment variable ')}<code>{TOKEN_ENV_NAME}</code>{tx(' 或客户端自带的安全存储中。完整 token 只会在创建当下显示一次。', ' or the client\'s secure storage. The full token value is shown only once when created.')}
        </div>
      </SetupSection>

      <div className="setup-section" id="token-creator">
          <div className="setup-section-header">
            <span className="setup-section-icon">&#128273;</span>
            <div>
            <h3>{tx('创建新 Token', 'Create token')}</h3>
            <p className="setup-section-desc">{tx('为 GPT Actions、脚本或其他自定义用途创建独立的 Token。', 'Create a standalone token for GPT Actions, scripts, or any other custom use case.')}</p>
            </div>
          </div>

        <div className="card">
          <div className="form-group" style={{ marginBottom: 12 }}>
            <label>{tx('名称', 'Name')}</label>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={tx('例如: Claude Desktop', 'For example: Claude Desktop')}
            />
          </div>

          <div className="form-group" style={{ marginBottom: 12 }}>
            <label>{tx('预设权限', 'Preset access')}</label>
            <div className="preset-radio-group">
              <label className={`preset-radio ${preset === 'agent' ? 'preset-radio-active' : ''}`}>
                <input
                  type="radio"
                  name="preset"
                  checked={preset === 'agent'}
                  onChange={() => { setPreset('agent'); setTrustLevel(4); setExpiryDays(90) }}
                />
                <span className="preset-radio-dot" />
                <div>
                  <strong>{tx('Agent 完整权限', 'Agent full access')}</strong>
                  <span className="preset-radio-desc">{tx('读写 Memory、Skills、Projects、Tree，并读取认证相关 Vault', 'Read and write Memory, Skills, Projects, Tree, and auth-related Vault scopes')}</span>
                </div>
              </label>
              <label className={`preset-radio ${preset === 'readonly' ? 'preset-radio-active' : ''}`}>
                <input
                  type="radio"
                  name="preset"
                  checked={preset === 'readonly'}
                  onChange={() => { setPreset('readonly'); setTrustLevel(3); setExpiryDays(30) }}
                />
                <span className="preset-radio-dot" />
                <div>
                  <strong>{tx('只读访问', 'Read-only access')}</strong>
                  <span className="preset-radio-desc">{tx('仅读取 Profile、Memory、Skills、Projects、Tree', 'Only read Profile, Memory, Skills, Projects, and Tree')}</span>
                </div>
              </label>
              <label className={`preset-radio ${preset === 'sync' ? 'preset-radio-active' : ''}`}>
                <input
                  type="radio"
                  name="preset"
                  checked={preset === 'sync'}
                  onChange={() => { setPreset('sync'); setTrustLevel(3); setExpiryDays(30) }}
                />
                <span className="preset-radio-dot" />
                <div>
                  <strong>Bundle Sync</strong>
                  <span className="preset-radio-desc">{tx('仅开放 read:bundle / write:bundle，适合导入导出和迁移脚本', 'Only enable read:bundle / write:bundle for import/export and migration scripts')}</span>
                </div>
              </label>
              <label className={`preset-radio ${preset === 'custom' ? 'preset-radio-active' : ''}`}>
                <input
                  type="radio"
                  name="preset"
                  checked={preset === 'custom'}
                  onChange={() => setPreset('custom')}
                />
                <span className="preset-radio-dot" />
                <div>
                  <strong>{tx('自定义', 'Custom')}</strong>
                  <span className="preset-radio-desc">{tx('手动选择权限范围', 'Choose scopes manually')}</span>
                </div>
              </label>
            </div>
          </div>

          {preset === 'custom' && scopeInfo && (
            <div className="form-group" style={{ marginBottom: 12 }}>
              <label>{tx('权限范围', 'Scopes')}</label>
              <div className="scope-grid">
                {Object.entries(scopeInfo.categories).map(([category, scopes]) => (
                  <div key={category} className="scope-grid-category">
                    <div className="scope-grid-category-name">{category}</div>
                    {scopes.map((scope) => (
                      <label key={scope} className="scope-grid-item">
                        <input
                          type="checkbox"
                          checked={customScopes.includes(scope)}
                          onChange={() => toggleScope(scope)}
                        />
                        <span>{scope}</span>
                      </label>
                    ))}
                  </div>
                ))}
              </div>
            </div>
          )}

          <div style={{ display: 'flex', gap: 12, marginBottom: 12 }}>
            <div className="form-group" style={{ flex: 1 }}>
              <label>{tx('信任等级', 'Trust level')}</label>
              <select
                className={`trust-select trust-l${trustLevel}`}
                value={trustLevel}
                onChange={(e) => setTrustLevel(Number(e.target.value))}
              >
                {trustLevels.map((item) => (
                  <option key={item.value} value={item.value}>{item.label} - {item.desc}</option>
                ))}
              </select>
            </div>

            <div className="form-group" style={{ flex: 1 }}>
              <label>{tx('有效期', 'Expiry')}</label>
              <select
                className="expiry-select"
                value={expiryDays}
                onChange={(e) => setExpiryDays(Number(e.target.value))}
              >
                {expiryOptions.map((item) => (
                  <option key={item.value} value={item.value}>{item.label}</option>
                ))}
              </select>
            </div>
          </div>

          <button
            className="btn btn-primary"
            onClick={() => { void handleCreateToken() }}
            disabled={manualCreating || (preset === 'custom' && customScopes.length === 0) || !name.trim()}
          >
            {manualCreating ? tx('生成中...', 'Creating...') : tx('生成 Token', 'Create token')}
          </button>

          {newToken && (
            <div className="alert alert-success" style={{ marginTop: 16 }}>
              <strong>{tx('Token 已生成!', 'Token created!')}</strong> {tx('请立即保存，此 Token 仅显示一次。', 'Save it now. This token is shown only once.')}
              <div className="key-value" style={{ marginTop: 8 }}>
                <code>{newToken}</code>
                <button className="btn btn-sm" onClick={() => { copyToClipboard(newToken, 'new-token') }}>
                  {copied === 'new-token' ? tx('已复制', 'Copied') : tx('复制', 'Copy')}
                </button>
              </div>
            </div>
          )}
        </div>
      </div>

      <div className="setup-section">
        <div className="setup-section-header">
          <span className="setup-section-icon">&#128218;</span>
          <div>
            <h3>{tx('已有 Token', 'Existing tokens')}</h3>
            <p className="setup-section-desc">
              {tx(`共 ${tokens.length} 个，${activeTokens.length} 个有效`, `${tokens.length} total, ${activeTokens.length} active`)}
            </p>
          </div>
        </div>

        {tokens.length === 0 ? (
          <div className="empty-state">
            <p>{tx('暂无 Token', 'No tokens yet')}</p>
            <p className="empty-hint">{tx('你可以先查看上方连接模板；需要真实 secret 时，再在对应模式里创建或在这里手动创建一个新的 Token', 'You can review the setup templates above first. When you need a real secret, create one in the relevant mode or manually create a new token here.')}</p>
          </div>
        ) : (
          <div className="token-list">
            {tokens.map((token) => (
              <div
                key={token.id}
                className={`token-list-item ${token.is_revoked || token.is_expired ? 'token-list-item-inactive' : ''}`}
              >
                <div className="token-list-main">
                  {editingTokenId === token.id ? (
                    <div className="token-inline-edit">
                      <input
                        className="token-inline-input"
                        value={editingTokenName}
                        onChange={(e) => setEditingTokenName(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') {
                            e.preventDefault()
                            void handleRenameToken(token)
                          }
                          if (e.key === 'Escape') {
                            e.preventDefault()
                            cancelRenameToken()
                          }
                        }}
                        autoFocus
                      />
                      <code className="token-list-prefix">{token.token_prefix}...</code>
                    </div>
                  ) : (
                    <>
                      <div className="token-list-name">{token.name}</div>
                      <code className="token-list-prefix">{token.token_prefix}...</code>
                    </>
                  )}
                </div>
                <div className="token-list-meta">
                  <span className={`trust-badge trust-l${token.max_trust_level}`}>
                    {trustLabel(token.max_trust_level)}
                  </span>
                  <span className="token-list-sep">&middot;</span>
                  <span>{presetLabel(token)}</span>
                  <span className="token-list-sep">&middot;</span>
                  <span>{formatExpiry(token)}</span>
                </div>
                <div className="token-list-actions">
                  {editingTokenId === token.id ? (
                    <>
                      <button
                        className="btn btn-sm btn-primary"
                        onClick={() => { void handleRenameToken(token) }}
                        disabled={renamingTokenId === token.id || !editingTokenName.trim()}
                      >
                        {renamingTokenId === token.id ? tx('保存中...', 'Saving...') : tx('保存', 'Save')}
                      </button>
                      <button
                        className="btn btn-sm btn-outline"
                        onClick={cancelRenameToken}
                        disabled={renamingTokenId === token.id}
                      >
                        {tx('取消', 'Cancel')}
                      </button>
                    </>
                  ) : (
                    <>
                      <button
                        className="btn btn-sm btn-outline"
                        onClick={() => startRenameToken(token)}
                      >
                        {tx('改名', 'Rename')}
                      </button>
                      {!token.is_revoked && !token.is_expired && (
                        <button
                          className="btn btn-sm btn-danger"
                          onClick={() => { void handleRevoke(token.id) }}
                        >
                          {tx('吊销', 'Revoke')}
                        </button>
                      )}
                    </>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  )
}
