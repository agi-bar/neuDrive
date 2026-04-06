import { useEffect, useState } from 'react'
import { api, type FileNode } from '../../api'
import { displayNameFromPath, formatDateTime, isMemoryEntry, sortNodesByRecent, summarizeNodeContent } from './DataShared'

export default function DataMemoryPage() {
  const [entries, setEntries] = useState<FileNode[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    const load = async () => {
      try {
        const snapshot = await api.getTreeSnapshot('/memory')
        setEntries(sortNodesByRecent(snapshot.entries.filter(isMemoryEntry)))
      } catch (err: any) {
        setError(err.message || '加载 Memory 失败')
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
          <h2>Memory</h2>
          <p className="page-subtitle">这里显示 `/memory` 下的记忆内容，不包含“我的资料”使用的 `/memory/profile` 条目。</p>
        </div>
      </div>

      {error && <div className="alert alert-warn">{error}</div>}

      {entries.length === 0 ? (
        <div className="empty-state">
          <p>还没有 Memory 内容</p>
          <p className="empty-hint">Agent 写入记忆后，会在这里看到对应条目。</p>
        </div>
      ) : (
        <div className="data-record-list">
          {entries.map((entry) => (
            <div key={entry.path} className="card data-record-item">
              <div className="data-record-head">
                <div className="data-record-title">{displayNameFromPath(entry.path.replace(/^\/memory\//, ''))}</div>
                <div className="data-record-meta">{formatDateTime(entry.updated_at || entry.created_at)}</div>
              </div>
              <div className="data-record-path">{entry.path}</div>
              <div className="data-record-preview">{summarizeNodeContent(entry, 220)}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
