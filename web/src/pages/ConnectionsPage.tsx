import { useState, useEffect } from 'react'
import { api } from '../api'

const TRUST_LEVELS = [
  { value: 1, label: 'L1 访客', className: 'trust-l1' },
  { value: 2, label: 'L2 协作', className: 'trust-l2' },
  { value: 3, label: 'L3 工作信任', className: 'trust-l3' },
  { value: 4, label: 'L4 完全信任', className: 'trust-l4' },
]

interface Connection {
  id: string
  name: string
  platform: string
  trust_level: number
  last_used?: string
  api_key?: string
  created_at?: string
}

export default function ConnectionsPage() {
  const [connections, setConnections] = useState<Connection[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [newConn, setNewConn] = useState({ name: '', platform: '', trust_level: 2 })
  const [creating, setCreating] = useState(false)
  const [createdKey, setCreatedKey] = useState('')
  const [keyCopied, setKeyCopied] = useState(false)

  useEffect(() => {
    loadConnections()
  }, [])

  const loadConnections = async () => {
    try {
      const data = await api.getConnections()
      setConnections(data || [])
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newConn.name.trim() || !newConn.platform.trim()) return

    setCreating(true)
    setError('')

    try {
      const result = await api.createConnection(newConn)
      if (result.api_key) {
        setCreatedKey(result.api_key)
      }
      setConnections((prev) => [...prev, result])
      setNewConn({ name: '', platform: '', trust_level: 2 })
      if (!result.api_key) {
        setShowForm(false)
      }
    } catch (err: any) {
      setError(err.message)
    } finally {
      setCreating(false)
    }
  }

  const handleTrustChange = async (id: string, trust_level: number) => {
    try {
      await api.updateConnection(id, { trust_level })
      setConnections((prev) =>
        prev.map((c) => (c.id === id ? { ...c, trust_level } : c))
      )
    } catch (err: any) {
      setError(err.message)
    }
  }

  const handleDelete = async (id: string, name: string) => {
    if (!window.confirm(`确认删除连接 "${name}"？此操作不可撤销。`)) return

    try {
      await api.deleteConnection(id)
      setConnections((prev) => prev.filter((c) => c.id !== id))
    } catch (err: any) {
      setError(err.message)
    }
  }

  const copyKey = async () => {
    try {
      await navigator.clipboard.writeText(createdKey)
      setKeyCopied(true)
      setTimeout(() => setKeyCopied(false), 2000)
    } catch {
      // Fallback
      const textarea = document.createElement('textarea')
      textarea.value = createdKey
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
      setKeyCopied(true)
      setTimeout(() => setKeyCopied(false), 2000)
    }
  }

  const dismissKey = () => {
    setCreatedKey('')
    setKeyCopied(false)
    setShowForm(false)
  }

  const formatTime = (ts?: string) => {
    if (!ts) return '-'
    try {
      return new Date(ts).toLocaleString('zh-CN')
    } catch {
      return ts
    }
  }

  const getTrustInfo = (level: number) => {
    return TRUST_LEVELS.find((t) => t.value === level) || TRUST_LEVELS[0]
  }

  if (loading) {
    return <div className="page-loading">加载中...</div>
  }

  return (
    <div className="page">
      <div className="page-header">
        <h2>连接管理</h2>
        <button
          className="btn btn-primary"
          onClick={() => {
            setShowForm(true)
            setCreatedKey('')
          }}
        >
          添加连接
        </button>
      </div>

      {error && <div className="alert alert-error">{error}</div>}

      {createdKey && (
        <div className="alert alert-success">
          <div className="key-display">
            <p className="api-key-warning">
              此密钥仅显示一次
            </p>
            <div className="api-key-box">
              <code>{createdKey}</code>
              <button className="btn btn-sm" onClick={copyKey} style={{ marginLeft: 12, color: '#68d391' }}>
                {keyCopied ? '已复制' : '复制'}
              </button>
            </div>
            <button className="btn btn-text" onClick={dismissKey}>
              我已保存，关闭
            </button>
          </div>
        </div>
      )}

      {showForm && !createdKey && (
        <div className="card form-card">
          <h3 className="card-title">新建连接</h3>
          <form onSubmit={handleCreate}>
            <div className="form-row">
              <div className="form-group">
                <label htmlFor="conn-name">名称</label>
                <input
                  id="conn-name"
                  type="text"
                  value={newConn.name}
                  onChange={(e) => setNewConn({ ...newConn, name: e.target.value })}
                  placeholder="例如：我的 Telegram Bot"
                  disabled={creating}
                />
              </div>
              <div className="form-group">
                <label htmlFor="conn-platform">平台</label>
                <select
                  id="conn-platform"
                  value={newConn.platform}
                  onChange={(e) => setNewConn({ ...newConn, platform: e.target.value })}
                  disabled={creating}
                >
                  <option value="">请选择平台</option>
                  <option value="claude">Claude</option>
                  <option value="gpt">GPT</option>
                  <option value="feishu">飞书</option>
                  <option value="other">其他</option>
                </select>
              </div>
              <div className="form-group">
                <label htmlFor="conn-trust">信任等级</label>
                <select
                  id="conn-trust"
                  value={newConn.trust_level}
                  onChange={(e) =>
                    setNewConn({ ...newConn, trust_level: Number(e.target.value) })
                  }
                  disabled={creating}
                >
                  {TRUST_LEVELS.map((t) => (
                    <option key={t.value} value={t.value}>
                      {t.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>
            <div className="form-actions">
              <button type="submit" className="btn btn-primary" disabled={creating}>
                {creating ? '创建中...' : '创建'}
              </button>
              <button
                type="button"
                className="btn"
                onClick={() => setShowForm(false)}
                disabled={creating}
              >
                取消
              </button>
            </div>
          </form>
        </div>
      )}

      {connections.length === 0 ? (
        <div className="empty-state">
          <p>还没有连接</p>
          <p className="empty-hint">添加一个连接来让 Agent 接入不同平台</p>
        </div>
      ) : (
        <div className="table-container">
          <table className="table">
            <thead>
              <tr>
                <th>名称</th>
                <th>平台</th>
                <th>信任等级</th>
                <th>最后使用</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {connections.map((conn) => {
                const trust = getTrustInfo(conn.trust_level)
                return (
                  <tr key={conn.id}>
                    <td className="cell-name">{conn.name}</td>
                    <td>
                      <span className="badge badge-platform">{conn.platform}</span>
                    </td>
                    <td>
                      <select
                        className={`trust-select ${trust.className}`}
                        value={conn.trust_level}
                        onChange={(e) =>
                          handleTrustChange(conn.id, Number(e.target.value))
                        }
                      >
                        {TRUST_LEVELS.map((t) => (
                          <option key={t.value} value={t.value}>
                            {t.label}
                          </option>
                        ))}
                      </select>
                    </td>
                    <td className="cell-time">{formatTime(conn.last_used)}</td>
                    <td>
                      <button
                        className="btn btn-sm btn-danger"
                        onClick={() => handleDelete(conn.id, conn.name)}
                      >
                        删除
                      </button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
