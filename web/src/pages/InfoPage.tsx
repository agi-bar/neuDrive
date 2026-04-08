import { useState, useEffect } from 'react'
import { api, MemoryConflict } from '../api'

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

const PROFILE_CATEGORIES = [
  { key: 'preferences', label: '个人偏好', placeholder: '例如：喜欢简洁的代码风格，偏好 TypeScript...' },
  { key: 'relationships', label: '人际关系', placeholder: '例如：Alice 是同事，负责后端开发...' },
  { key: 'principles', label: '行为准则', placeholder: '例如：重要决定需要确认，不要自动发送消息...' },
]

const TRUST_LABELS: Record<number, string> = {
  1: 'L1 访客',
  2: 'L2 共享',
  3: 'L3 工作信任',
  4: 'L4 完全信任',
}

interface InfoPageProps {
  title?: string
}

export default function InfoPage({ title = '我的资料' }: InfoPageProps) {
  const [profiles, setProfiles] = useState<ProfileEntry[]>([])
  const [vaultScopes, setVaultScopes] = useState<VaultScope[]>([])
  const [conflicts, setConflicts] = useState<MemoryConflict[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState('')
  const [resolving, setResolving] = useState('')
  const [editValues, setEditValues] = useState<Record<string, string>>({})
  const [successMsg, setSuccessMsg] = useState('')

  useEffect(() => {
    loadData()
  }, [])

  const loadData = async () => {
    try {
      const [profileData, vaultData, conflictData] = await Promise.allSettled([
        api.getProfile(),
        api.getVaultScopes(),
        api.getConflicts(),
      ])

      if (profileData.status === 'fulfilled') {
        const raw = profileData.value || {}
        // API returns {user_id, display_name, preferences: {key: value}}
        // Transform preferences map into ProfileEntry[] for display
        const prefs = raw.preferences || {}
        const entries: ProfileEntry[] = Object.entries(prefs).map(([key, value]) => ({
          category: key,
          key: key,
          value: String(value),
        }))
        setProfiles(entries)
        // Initialize edit values from preferences
        const values: Record<string, string> = {}
        for (const cat of PROFILE_CATEGORIES) {
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
      setSuccessMsg('冲突已解决')
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
      setSuccessMsg(`${PROFILE_CATEGORIES.find((c) => c.key === category)?.label || category} 已保存`)
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
      for (const cat of PROFILE_CATEGORIES) {
        prefs[cat.key] = editValues[cat.key] || ''
      }
      await api.upsertProfile({ preferences: prefs })
      setSuccessMsg('所有配置已保存')
      setTimeout(() => setSuccessMsg(''), 2000)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setSaving('')
    }
  }

  if (loading) {
    return <div className="page-loading">加载中...</div>
  }

  return (
    <div className="page materials-page">
      <div className="page-header">
        <div>
          <h2>{title}</h2>
          <p className="page-subtitle">这里统一管理个人偏好、行为准则、冲突记录和 Vault 访问范围。</p>
        </div>
      </div>

      {error && <div className="alert alert-error">{error}</div>}
      {successMsg && <div className="alert alert-success">{successMsg}</div>}

      {conflicts.length > 0 && (
        <section className="section">
          <div className="alert alert-error" style={{ marginBottom: '1rem' }}>
            <strong>检测到 {conflicts.length} 个记忆冲突</strong> —
            不同 Agent 平台记录了矛盾的偏好，请选择保留哪个版本。
          </div>
          {conflicts.map((c) => (
            <div key={c.id} className="card" style={{ marginBottom: '1rem' }}>
              <div className="card-header">
                <h4 className="card-title">
                  冲突: {c.category}
                </h4>
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem', padding: '1rem' }}>
                <div>
                  <strong>来源 A: {c.source_a}</strong>
                  <pre style={{ whiteSpace: 'pre-wrap', background: '#f5f5f5', padding: '0.5rem', borderRadius: '4px', marginTop: '0.5rem' }}>
                    {c.content_a}
                  </pre>
                  <button
                    className="btn btn-sm btn-primary"
                    style={{ marginTop: '0.5rem' }}
                    disabled={resolving === c.id}
                    onClick={() => handleResolveConflict(c.id, 'keep_a')}
                  >
                    保留 A
                  </button>
                </div>
                <div>
                  <strong>来源 B: {c.source_b}</strong>
                  <pre style={{ whiteSpace: 'pre-wrap', background: '#f5f5f5', padding: '0.5rem', borderRadius: '4px', marginTop: '0.5rem' }}>
                    {c.content_b}
                  </pre>
                  <button
                    className="btn btn-sm btn-primary"
                    style={{ marginTop: '0.5rem' }}
                    disabled={resolving === c.id}
                    onClick={() => handleResolveConflict(c.id, 'keep_b')}
                  >
                    保留 B
                  </button>
                </div>
              </div>
              <div style={{ padding: '0 1rem 1rem', display: 'flex', gap: '0.5rem' }}>
                <button
                  className="btn btn-sm"
                  disabled={resolving === c.id}
                  onClick={() => handleResolveConflict(c.id, 'keep_both')}
                >
                  两者都保留
                </button>
                <button
                  className="btn btn-sm"
                  disabled={resolving === c.id}
                  onClick={() => handleResolveConflict(c.id, 'dismiss')}
                >
                  忽略
                </button>
              </div>
            </div>
          ))}
        </section>
      )}

      <section className="section">
        <h3 className="section-title">个人偏好</h3>
        <p className="section-desc">
          Agent 会记住这些信息，在对话和任务中参考使用。格式为 "键: 值"，每行一条。
        </p>

        <div className="profile-cards">
          {PROFILE_CATEGORIES.map((cat) => (
            <div key={cat.key} className="card">
              <div className="card-header">
                <h4 className="card-title">{cat.label}</h4>
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
            {saving === 'all' ? '保存中...' : '保存所有配置'}
          </button>
        </div>
      </section>

      <section className="section">
        <h3 className="section-title">安全存储</h3>
        <p className="section-desc">
          Vault 中的敏感信息按信任等级控制访问。只有达到对应信任等级的连接才能读取。
        </p>

        {vaultScopes.length === 0 ? (
          <div className="empty-state">
            <p>暂无安全存储配置</p>
            <p className="empty-hint">安全存储用于管理 API 密钥、密码等敏感信息的访问权限</p>
          </div>
        ) : (
          <div className="vault-list">
            {vaultScopes.map((scope, i) => (
              <div key={scope.id || i} className="card vault-card">
                <div className="vault-header">
                  <span className="vault-scope">{scope.scope}</span>
                  <span className={`trust-badge trust-l${scope.trust_level}`}>
                    {TRUST_LABELS[scope.trust_level] || `L${scope.trust_level}`}
                  </span>
                </div>
                <p className="vault-desc">{scope.description || '无描述'}</p>
              </div>
            ))}
          </div>
        )}

        <div className="trust-legend">
          <h4>信任等级说明</h4>
          <div className="legend-grid">
            <div className="legend-item">
              <span className="trust-badge trust-l1">L1 访客</span>
              <span>只能访问公开信息</span>
            </div>
            <div className="legend-item">
              <span className="trust-badge trust-l2">L2 共享</span>
              <span>可访问有限共享信息</span>
            </div>
            <div className="legend-item">
              <span className="trust-badge trust-l3">L3 工作信任</span>
              <span>可访问敏感工作信息</span>
            </div>
            <div className="legend-item">
              <span className="trust-badge trust-l4">L4 完全信任</span>
              <span>完全访问所有信息</span>
            </div>
          </div>
        </div>
      </section>
    </div>
  )
}
