import { useState, useEffect } from 'react'
import { api } from '../api'

interface Collaboration {
  id: string
  owner_user_id: string
  guest_user_id: string
  shared_paths: string[]
  permissions: string
  expires_at?: string
  created_at: string
}

export default function CollaborationsPage() {
  const [owned, setOwned] = useState<Collaboration[]>([])
  const [shared, setShared] = useState<Collaboration[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState({
    guest_slug: '',
    shared_paths: '',
    permissions: 'read',
    expires_in_days: '',
  })

  useEffect(() => {
    loadCollaborations()
  }, [])

  const loadCollaborations = async () => {
    try {
      const data = await api.getCollaborations()
      setOwned(data.owned || [])
      setShared(data.shared || [])
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!form.guest_slug.trim() || !form.shared_paths.trim()) return

    setCreating(true)
    setError('')

    try {
      const paths = form.shared_paths.split(',').map((p) => p.trim()).filter(Boolean)
      const payload: any = {
        guest_slug: form.guest_slug.trim(),
        shared_paths: paths,
        permissions: form.permissions,
      }
      if (form.expires_in_days) {
        payload.expires_in_days = parseInt(form.expires_in_days, 10)
      }
      await api.createCollaboration(payload)
      setForm({ guest_slug: '', shared_paths: '', permissions: 'read', expires_in_days: '' })
      setShowForm(false)
      await loadCollaborations()
    } catch (err: any) {
      setError(err.message)
    } finally {
      setCreating(false)
    }
  }

  const handleRevoke = async (id: string) => {
    if (!window.confirm('确认撤销此协作？')) return
    try {
      await api.revokeCollaboration(id)
      setOwned((prev) => prev.filter((c) => c.id !== id))
    } catch (err: any) {
      setError(err.message)
    }
  }

  const formatTime = (ts?: string) => {
    if (!ts) return '-'
    try {
      return new Date(ts).toLocaleString('zh-CN')
    } catch {
      return ts
    }
  }

  if (loading) {
    return <div className="page-loading">加载中...</div>
  }

  return (
    <div className="page">
      <div className="page-header">
        <h2>协作管理</h2>
        <button className="btn btn-primary" onClick={() => setShowForm(true)}>
          新建协作
        </button>
      </div>

      {error && <div className="alert alert-error">{error}</div>}

      {showForm && (
        <div className="card form-card">
          <h3 className="card-title">新建协作</h3>
          <form onSubmit={handleCreate}>
            <div className="form-row">
              <div className="form-group">
                <label htmlFor="collab-guest">协作用户 Slug</label>
                <input
                  id="collab-guest"
                  type="text"
                  value={form.guest_slug}
                  onChange={(e) => setForm({ ...form, guest_slug: e.target.value })}
                  placeholder="对方的用户标识"
                  disabled={creating}
                />
              </div>
              <div className="form-group">
                <label htmlFor="collab-paths">共享路径</label>
                <input
                  id="collab-paths"
                  type="text"
                  value={form.shared_paths}
                  onChange={(e) => setForm({ ...form, shared_paths: e.target.value })}
                  placeholder="/skills, /projects/demo"
                  disabled={creating}
                />
                <small>多个路径用逗号分隔</small>
              </div>
            </div>
            <div className="form-row">
              <div className="form-group">
                <label htmlFor="collab-perm">权限</label>
                <select
                  id="collab-perm"
                  value={form.permissions}
                  onChange={(e) => setForm({ ...form, permissions: e.target.value })}
                  disabled={creating}
                >
                  <option value="read">只读</option>
                  <option value="readwrite">读写</option>
                </select>
              </div>
              <div className="form-group">
                <label htmlFor="collab-expiry">有效期（天）</label>
                <input
                  id="collab-expiry"
                  type="number"
                  value={form.expires_in_days}
                  onChange={(e) => setForm({ ...form, expires_in_days: e.target.value })}
                  placeholder="留空表示永久"
                  disabled={creating}
                  min="1"
                />
              </div>
            </div>
            <div className="form-actions">
              <button type="submit" className="btn btn-primary" disabled={creating}>
                {creating ? '创建中...' : '创建'}
              </button>
              <button type="button" className="btn" onClick={() => setShowForm(false)} disabled={creating}>
                取消
              </button>
            </div>
          </form>
        </div>
      )}

      <h3 style={{ marginTop: 24 }}>我创建的协作</h3>
      {owned.length === 0 ? (
        <div className="empty-state">
          <p>还没有创建协作</p>
          <p className="empty-hint">创建协作来共享文件给其他用户</p>
        </div>
      ) : (
        <div className="table-container">
          <table className="table">
            <thead>
              <tr>
                <th>协作对象</th>
                <th>共享路径</th>
                <th>权限</th>
                <th>过期时间</th>
                <th>创建时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {owned.map((c) => (
                <tr key={c.id}>
                  <td>{c.guest_user_id}</td>
                  <td>
                    {c.shared_paths.map((p, i) => (
                      <span key={i} className="badge" style={{ marginRight: 4 }}>
                        {p}
                      </span>
                    ))}
                  </td>
                  <td>
                    <span className={`badge ${c.permissions === 'readwrite' ? 'badge-platform' : ''}`}>
                      {c.permissions === 'read' ? '只读' : '读写'}
                    </span>
                  </td>
                  <td className="cell-time">{formatTime(c.expires_at)}</td>
                  <td className="cell-time">{formatTime(c.created_at)}</td>
                  <td>
                    <button className="btn btn-sm btn-danger" onClick={() => handleRevoke(c.id)}>
                      撤销
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <h3 style={{ marginTop: 24 }}>共享给我的协作</h3>
      {shared.length === 0 ? (
        <div className="empty-state">
          <p>还没有其他用户共享给你</p>
        </div>
      ) : (
        <div className="table-container">
          <table className="table">
            <thead>
              <tr>
                <th>所有者</th>
                <th>共享路径</th>
                <th>权限</th>
                <th>过期时间</th>
                <th>创建时间</th>
              </tr>
            </thead>
            <tbody>
              {shared.map((c) => (
                <tr key={c.id}>
                  <td>{c.owner_user_id}</td>
                  <td>
                    {c.shared_paths.map((p, i) => (
                      <span key={i} className="badge" style={{ marginRight: 4 }}>
                        {p}
                      </span>
                    ))}
                  </td>
                  <td>
                    <span className={`badge ${c.permissions === 'readwrite' ? 'badge-platform' : ''}`}>
                      {c.permissions === 'read' ? '只读' : '读写'}
                    </span>
                  </td>
                  <td className="cell-time">{formatTime(c.expires_at)}</td>
                  <td className="cell-time">{formatTime(c.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
