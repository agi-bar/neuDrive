import { useEffect } from 'react'
import { useLocation } from 'react-router-dom'
import {
  EXPIRY_OPTIONS,
  TOKEN_ENV_NAME,
  TRUST_LEVELS,
  useSetup,
} from '../SetupPage'
import { SetupSection } from './SetupShared'

export default function SetupTokensPage() {
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

  useEffect(() => {
    if (location.hash === '#token-creator') {
      document.getElementById('token-creator')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  }, [location.hash])

  return (
    <>
      <SetupSection
        icon={<>&#128273;</>}
        title="Token 管理"
        description={`共 ${tokens.length} 个，${activeTokens.length} 个有效。为 GPT Actions、脚本或其他自定义用途创建和管理独立 Token。`}
      >
        <div className="setup-note setup-note-first">
          推荐优先把 token 放进环境变量 <code>{TOKEN_ENV_NAME}</code> 或客户端自带的安全存储中。完整 token 只会在创建当下显示一次。
        </div>
      </SetupSection>

      <div className="setup-section" id="token-creator">
        <div className="setup-section-header">
          <span className="setup-section-icon">&#128273;</span>
          <div>
            <h3>创建新 Token</h3>
            <p className="setup-section-desc">为 GPT Actions、脚本或其他自定义用途创建独立的 Token。</p>
          </div>
        </div>

        <div className="card">
          <div className="form-group" style={{ marginBottom: 12 }}>
            <label>名称</label>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如: Claude Desktop"
            />
          </div>

          <div className="form-group" style={{ marginBottom: 12 }}>
            <label>预设权限</label>
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
                  <strong>Agent 完整权限</strong>
                  <span className="preset-radio-desc">读写 Memory、Skills、Inbox、Projects、Tree、Devices</span>
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
                  <strong>只读访问</strong>
                  <span className="preset-radio-desc">仅读取 Profile、Memory、Skills、Projects、Tree</span>
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
                  <span className="preset-radio-desc">仅开放 read:bundle / write:bundle，适合导入导出和迁移脚本</span>
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
                  <strong>自定义</strong>
                  <span className="preset-radio-desc">手动选择权限范围</span>
                </div>
              </label>
            </div>
          </div>

          {preset === 'custom' && scopeInfo && (
            <div className="form-group" style={{ marginBottom: 12 }}>
              <label>权限范围</label>
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
              <label>信任等级</label>
              <select
                className={`trust-select trust-l${trustLevel}`}
                value={trustLevel}
                onChange={(e) => setTrustLevel(Number(e.target.value))}
              >
                {TRUST_LEVELS.map((item) => (
                  <option key={item.value} value={item.value}>{item.label} - {item.desc}</option>
                ))}
              </select>
            </div>

            <div className="form-group" style={{ flex: 1 }}>
              <label>有效期</label>
              <select
                className="expiry-select"
                value={expiryDays}
                onChange={(e) => setExpiryDays(Number(e.target.value))}
              >
                {EXPIRY_OPTIONS.map((item) => (
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
            {manualCreating ? '生成中...' : '生成 Token'}
          </button>

          {newToken && (
            <div className="alert alert-success" style={{ marginTop: 16 }}>
              <strong>Token 已生成!</strong> 请立即保存，此 Token 仅显示一次。
              <div className="key-value" style={{ marginTop: 8 }}>
                <code>{newToken}</code>
                <button className="btn btn-sm" onClick={() => { copyToClipboard(newToken, 'new-token') }}>
                  {copied === 'new-token' ? '已复制' : '复制'}
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
            <h3>已有 Token</h3>
            <p className="setup-section-desc">
              共 {tokens.length} 个，{activeTokens.length} 个有效
            </p>
          </div>
        </div>

        {tokens.length === 0 ? (
          <div className="empty-state">
            <p>暂无 Token</p>
            <p className="empty-hint">你可以先查看上方连接模板；需要真实 secret 时，再在对应模式里创建或在这里手动创建一个新的 Token</p>
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
                        {renamingTokenId === token.id ? '保存中...' : '保存'}
                      </button>
                      <button
                        className="btn btn-sm btn-outline"
                        onClick={cancelRenameToken}
                        disabled={renamingTokenId === token.id}
                      >
                        取消
                      </button>
                    </>
                  ) : (
                    <>
                      <button
                        className="btn btn-sm btn-outline"
                        onClick={() => startRenameToken(token)}
                      >
                        改名
                      </button>
                      {!token.is_revoked && !token.is_expired && (
                        <button
                          className="btn btn-sm btn-danger"
                          onClick={() => { void handleRevoke(token.id) }}
                        >
                          吊销
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
