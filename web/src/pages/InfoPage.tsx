import { useState, useEffect } from 'react'
import { api, MemoryConflict } from '../api'
import { useI18n } from '../i18n'
import { sourceLabel } from './data/DataShared'

interface ProfileEntry {
  id?: string
  category: string
  key: string
  value: string
  source?: string
}

interface VaultScope {
  id?: string
  scope: string
  description: string
  trust_level: number
}

interface InfoPageProps {
  title?: string
}

export default function InfoPage({ title }: InfoPageProps) {
  const { locale, tx } = useI18n()
  const [profiles, setProfiles] = useState<ProfileEntry[]>([])
  const [vaultScopes, setVaultScopes] = useState<VaultScope[]>([])
  const [conflicts, setConflicts] = useState<MemoryConflict[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState('')
  const [resolving, setResolving] = useState('')
  const [editValues, setEditValues] = useState<Record<string, string>>({})
  const [successMsg, setSuccessMsg] = useState('')

  const profileCategories = [
    { key: 'preferences', label: tx('个人偏好', 'Preferences'), placeholder: tx('例如：喜欢简洁的代码风格，偏好 TypeScript...', 'For example: prefers concise code style and TypeScript...') },
    { key: 'relationships', label: tx('人际关系', 'Relationships'), placeholder: tx('例如：Alice 是同事，负责后端开发...', 'For example: Alice is a teammate who owns backend development...') },
    { key: 'principles', label: tx('行为准则', 'Principles'), placeholder: tx('例如：重要决定需要确认，不要自动发送消息...', 'For example: confirm major decisions and do not send messages automatically...') },
  ]

  const trustLabels: Record<number, string> = {
    1: tx('L1 访客', 'L1 Visitor'),
    2: tx('L2 共享', 'L2 Shared'),
    3: tx('L3 工作信任', 'L3 Work Trust'),
    4: tx('L4 完全信任', 'L4 Full Trust'),
  }

  const pageTitle = title || tx('我的资料', 'My Profile')

  useEffect(() => {
    loadData()
  }, [])

  const loadData = async () => {
    try {
      const [profileData, profileSnapshotData, vaultData, conflictData] = await Promise.allSettled([
        api.getProfile(),
        api.getTreeSnapshot('/memory/profile'),
        api.getVaultScopes(),
        api.getConflicts(),
      ])

      if (profileData.status === 'fulfilled') {
        const raw = profileData.value || {}
        // API returns {user_id, display_name, preferences: {key: value}}
        // Transform preferences map into ProfileEntry[] for display
        const prefs = raw.preferences || {}
        const sourceLookup = new Map<string, string>()
        if (profileSnapshotData.status === 'fulfilled') {
          profileSnapshotData.value.entries.forEach((entry) => {
            const name = entry.path.split('/').pop()?.replace(/\.md$/i, '') || ''
            if (name) sourceLookup.set(name, entry.source || '')
          })
        }
        const entries: ProfileEntry[] = Object.entries(prefs).map(([key, value]) => ({
          category: key,
          key: key,
          value: String(value),
          source: sourceLookup.get(key) || '',
        }))
        setProfiles(entries)
        // Initialize edit values from preferences
        const values: Record<string, string> = {}
        for (const cat of profileCategories) {
          values[cat.key] = prefs[cat.key] || ''
        }
        setEditValues(values)
      }

      if (vaultData.status === 'fulfilled') {
        setVaultScopes(vaultData.value || [])
      }

      if (conflictData.status === 'fulfilled') {
        setConflicts(conflictData.value || [])
      }
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleResolveConflict = async (conflictId: string, resolution: string) => {
    setResolving(conflictId)
    setError('')
    try {
      await api.resolveConflict(conflictId, resolution)
      setConflicts((prev) => prev.filter((c) => c.id !== conflictId))
      setSuccessMsg(tx('冲突已解决', 'Conflict resolved'))
      setTimeout(() => setSuccessMsg(''), 2000)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setResolving('')
    }
  }

  const handleSaveProfile = async (category: string) => {
    const text = editValues[category] || ''
    setSaving(category)
    setError('')

    try {
      // Send as {preferences: {category: content}} — backend expects this format
      await api.upsertProfile({
        preferences: { [category]: text },
      })
      await loadData()
      setSuccessMsg(tx(`${profileCategories.find((c) => c.key === category)?.label || category} 已保存`, `${profileCategories.find((c) => c.key === category)?.label || category} saved`))
      setTimeout(() => setSuccessMsg(''), 2000)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setSaving('')
    }
  }

  const handleSaveAll = async () => {
    setSaving('all')
    setError('')

    try {
      const prefs: Record<string, string> = {}
      for (const cat of profileCategories) {
        prefs[cat.key] = editValues[cat.key] || ''
      }
      await api.upsertProfile({ preferences: prefs })
      await loadData()
      setSuccessMsg(tx('所有配置已保存', 'All settings saved'))
      setTimeout(() => setSuccessMsg(''), 2000)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setSaving('')
    }
  }

  if (loading) {
    return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>
  }

  return (
    <div className="page materials-page">
      <div className="page-header">
        <div>
          <h2>{pageTitle}</h2>
          <p className="page-subtitle">{tx('这里统一管理个人偏好、行为准则、冲突记录和 Vault 访问范围。', 'Manage profile preferences, principles, conflict records, and Vault access scopes here.')}</p>
        </div>
      </div>

      {error && <div className="alert alert-error">{error}</div>}
      {successMsg && <div className="alert alert-success">{successMsg}</div>}

      {conflicts.length > 0 && (
        <section className="section">
          <div className="alert alert-error" style={{ marginBottom: '1rem' }}>
            <strong>{tx(`检测到 ${conflicts.length} 个记忆冲突`, `${conflicts.length} memory conflicts detected`)}</strong> -
            {tx('不同 Agent 平台记录了矛盾的偏好，请选择保留哪个版本。', 'Different agent platforms recorded conflicting preferences. Choose which version to keep.')}
          </div>
          {conflicts.map((c) => (
            <div key={c.id} className="card" style={{ marginBottom: '1rem' }}>
              <div className="card-header">
                <h4 className="card-title">
                  {tx('冲突', 'Conflict')}: {c.category}
                </h4>
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem', padding: '1rem' }}>
                <div>
                  <strong>{tx('来源 A', 'Source A')}: {c.source_a}</strong>
                  <pre style={{ whiteSpace: 'pre-wrap', background: '#f5f5f5', padding: '0.5rem', borderRadius: '4px', marginTop: '0.5rem' }}>
                    {c.content_a}
                  </pre>
                  <button
                    className="btn btn-sm btn-primary"
                    style={{ marginTop: '0.5rem' }}
                    disabled={resolving === c.id}
                    onClick={() => handleResolveConflict(c.id, 'keep_a')}
                  >
                    {tx('保留 A', 'Keep A')}
                  </button>
                </div>
                <div>
                  <strong>{tx('来源 B', 'Source B')}: {c.source_b}</strong>
                  <pre style={{ whiteSpace: 'pre-wrap', background: '#f5f5f5', padding: '0.5rem', borderRadius: '4px', marginTop: '0.5rem' }}>
                    {c.content_b}
                  </pre>
                  <button
                    className="btn btn-sm btn-primary"
                    style={{ marginTop: '0.5rem' }}
                    disabled={resolving === c.id}
                    onClick={() => handleResolveConflict(c.id, 'keep_b')}
                  >
                    {tx('保留 B', 'Keep B')}
                  </button>
                </div>
              </div>
              <div style={{ padding: '0 1rem 1rem', display: 'flex', gap: '0.5rem' }}>
                <button
                  className="btn btn-sm"
                  disabled={resolving === c.id}
                  onClick={() => handleResolveConflict(c.id, 'keep_both')}
                >
                  {tx('两者都保留', 'Keep both')}
                </button>
                <button
                  className="btn btn-sm"
                  disabled={resolving === c.id}
                  onClick={() => handleResolveConflict(c.id, 'dismiss')}
                >
                  {tx('忽略', 'Dismiss')}
                </button>
              </div>
            </div>
          ))}
        </section>
      )}

      <section className="section">
        <h3 className="section-title">{tx('个人偏好', 'Preferences')}</h3>
        <p className="section-desc">
          {tx('Agent 会记住这些信息，在对话和任务中参考使用。格式为 "键: 值"，每行一条。', 'Agents will remember this information and use it during conversations and tasks. Format each line as "key: value".')}
        </p>

        <div className="profile-cards">
          {profileCategories.map((cat) => (
            <div key={cat.key} className="card">
              <div className="card-header">
                <h4 className="card-title">{cat.label}</h4>
                <span className="dashboard-inline-chip">
                  {sourceLabel(profiles.find((entry) => entry.key === cat.key)?.source, locale)}
                </span>
              </div>
              <textarea
                className="profile-textarea"
                value={editValues[cat.key] || ''}
                onChange={(e) =>
                  setEditValues({ ...editValues, [cat.key]: e.target.value })
                }
                placeholder={cat.placeholder}
                rows={5}
              />
            </div>
          ))}
        </div>

        <div style={{ marginTop: 16 }}>
          <button
            className="btn btn-primary"
            onClick={handleSaveAll}
            disabled={saving === 'all'}
          >
            {saving === 'all' ? tx('保存中...', 'Saving...') : tx('保存所有配置', 'Save all settings')}
          </button>
        </div>
      </section>

      <section className="section">
        <h3 className="section-title">{tx('安全存储', 'Secure storage')}</h3>
        <p className="section-desc">
          {tx('Vault 中的敏感信息按信任等级控制访问。只有达到对应信任等级的连接才能读取。', 'Sensitive Vault data is gated by trust level. Only connections with sufficient trust can read it.')}
        </p>

        {vaultScopes.length === 0 ? (
          <div className="empty-state">
            <p>{tx('暂无安全存储配置', 'No secure storage scopes yet')}</p>
            <p className="empty-hint">{tx('安全存储用于管理 API 密钥、密码等敏感信息的访问权限', 'Secure storage manages access to sensitive data such as API keys and passwords.')}</p>
          </div>
        ) : (
          <div className="vault-list">
            {vaultScopes.map((scope, i) => (
              <div key={scope.id || i} className="card vault-card">
                <div className="vault-header">
                  <span className="vault-scope">{scope.scope}</span>
                  <span className={`trust-badge trust-l${scope.trust_level}`}>
                    {trustLabels[scope.trust_level] || `L${scope.trust_level}`}
                  </span>
                </div>
                <p className="vault-desc">{scope.description || tx('无描述', 'No description')}</p>
              </div>
            ))}
          </div>
        )}

        <div className="trust-legend">
          <h4>{tx('信任等级说明', 'Trust levels')}</h4>
          <div className="legend-grid">
            <div className="legend-item">
              <span className="trust-badge trust-l1">{tx('L1 访客', 'L1 Visitor')}</span>
              <span>{tx('只能访问公开信息', 'Can only access public information')}</span>
            </div>
            <div className="legend-item">
              <span className="trust-badge trust-l2">{tx('L2 共享', 'L2 Shared')}</span>
              <span>{tx('可访问有限共享信息', 'Can access limited shared information')}</span>
            </div>
            <div className="legend-item">
              <span className="trust-badge trust-l3">{tx('L3 工作信任', 'L3 Work Trust')}</span>
              <span>{tx('可访问敏感工作信息', 'Can access sensitive work information')}</span>
            </div>
            <div className="legend-item">
              <span className="trust-badge trust-l4">{tx('L4 完全信任', 'L4 Full Trust')}</span>
              <span>{tx('完全访问所有信息', 'Can access everything')}</span>
            </div>
          </div>
        </div>
      </section>
    </div>
  )
}
