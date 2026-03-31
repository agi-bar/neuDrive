import { useState, useEffect } from 'react'
import { api } from '../api'

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
  2: 'L2 协作',
  3: 'L3 工作信任',
  4: 'L4 完全信任',
}

export default function InfoPage() {
  const [profiles, setProfiles] = useState<ProfileEntry[]>([])
  const [vaultScopes, setVaultScopes] = useState<VaultScope[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState('')
  const [editValues, setEditValues] = useState<Record<string, string>>({})
  const [successMsg, setSuccessMsg] = useState('')

  useEffect(() => {
    loadData()
  }, [])

  const loadData = async () => {
    try {
      const [profileData, vaultData] = await Promise.allSettled([
        api.getProfile(),
        api.getVaultScopes(),
      ])

      if (profileData.status === 'fulfilled') {
        setProfiles(profileData.value || [])
        // Initialize edit values
        const values: Record<string, string> = {}
        for (const cat of PROFILE_CATEGORIES) {
          const entries = (profileData.value || []).filter(
            (p: ProfileEntry) => p.category === cat.key
          )
          values[cat.key] = entries.map((e: ProfileEntry) => `${e.key}: ${e.value}`).join('\n')
        }
        setEditValues(values)
      }

      if (vaultData.status === 'fulfilled') {
        setVaultScopes(vaultData.value || [])
      }
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleSaveProfile = async (category: string) => {
    const text = editValues[category] || ''
    setSaving(category)
    setError('')

    try {
      // Parse text lines into key-value pairs
      const entries = text
        .split('\n')
        .map((line) => line.trim())
        .filter((line) => line.length > 0)
        .map((line) => {
          const colonIdx = line.indexOf(':')
          if (colonIdx > 0) {
            return {
              category,
              key: line.substring(0, colonIdx).trim(),
              value: line.substring(colonIdx + 1).trim(),
            }
          }
          return { category, key: line, value: '' }
        })

      await api.upsertProfile({ category, entries })
      setSuccessMsg(`${PROFILE_CATEGORIES.find((c) => c.key === category)?.label || category} 已保存`)
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
    <div className="page">
      <div className="page-header">
        <h2>信息配置</h2>
      </div>

      {error && <div className="alert alert-error">{error}</div>}
      {successMsg && <div className="alert alert-success">{successMsg}</div>}

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
                <button
                  className="btn btn-sm btn-primary"
                  onClick={() => handleSaveProfile(cat.key)}
                  disabled={saving === cat.key}
                >
                  {saving === cat.key ? '保存中...' : '保存'}
                </button>
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
              <span className="trust-badge trust-l2">L2 协作</span>
              <span>可访问工作相关信息</span>
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
