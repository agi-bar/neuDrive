import { useEffect, useState } from 'react'
import { api, type RoleRecord } from '../../api'
import { formatDateTime } from './DataShared'

export default function DataRolesPage() {
  const [roles, setRoles] = useState<RoleRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    const load = async () => {
      try {
        const data = await api.getRoles()
        setRoles([...data].sort((a, b) => a.name.localeCompare(b.name)))
      } catch (err: any) {
        setError(err.message || '加载 Roles 失败')
      } finally {
        setLoading(false)
      }
    }

    load()
  }, [])

  if (loading) {
    return <div className="page-loading">加载中...</div>
  }

  return (
    <div className="page">
      <div className="page-header page-header-stack">
        <div>
          <h2>Roles</h2>
          <p className="page-subtitle">这里显示当前 Hub 中定义的角色、作用范围和生命周期配置。</p>
        </div>
      </div>

      {error && <div className="alert alert-warn">{error}</div>}

      {roles.length === 0 ? (
        <div className="empty-state">
          <p>还没有 Roles</p>
          <p className="empty-hint">创建角色之后，这里会显示它们的访问路径和生命周期。</p>
        </div>
      ) : (
        <div className="data-record-list">
          {roles.map((role) => (
            <div key={role.id || role.name} className="card data-record-item">
              <div className="data-record-head">
                <div className="data-record-title">{role.name}</div>
                <div className="data-inline-list">
                  {role.role_type && <span className="dashboard-inline-chip">{role.role_type}</span>}
                  {role.lifecycle && <span className="dashboard-inline-chip">{role.lifecycle}</span>}
                </div>
              </div>
              <div className="data-record-secondary">
                允许路径：{role.allowed_paths && role.allowed_paths.length > 0 ? role.allowed_paths.join(', ') : '未设置'}
              </div>
              <div className="data-record-secondary">
                Vault Scopes：{role.allowed_vault_scopes && role.allowed_vault_scopes.length > 0 ? role.allowed_vault_scopes.join(', ') : '未设置'}
              </div>
              <div className="data-record-meta">{formatDateTime(role.created_at)}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
