import { useEffect, useState } from 'react'
import { api, type FileNode } from '../../api'
import { fileNamespaceLabel, formatDateTime, isVisibleFileEntry, sortNodesByRecent, summarizeNodeContent } from './DataShared'

export default function DataFilesPage() {
  const [files, setFiles] = useState<FileNode[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    const load = async () => {
      try {
        const snapshot = await api.getTreeSnapshot('/')
        setFiles(sortNodesByRecent(snapshot.entries.filter(isVisibleFileEntry)))
      } catch (err: any) {
        setError(err.message || '加载文件失败')
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
          <h2>所有文件</h2>
          <p className="page-subtitle">这里按最近更新时间汇总展示 Hub 中的全部文件内容，包含项目、技能、Memory、我的资料、设备、Roles、Inbox 和根文件空间。</p>
        </div>
      </div>

      {error && <div className="alert alert-warn">{error}</div>}

      {files.length === 0 ? (
        <div className="empty-state">
          <p>还没有文件内容</p>
          <p className="empty-hint">当 Hub 写入任何文件后，无论属于哪个命名空间，都会出现在这里。</p>
        </div>
      ) : (
        <div className="data-record-list">
          {files.map((file) => (
            <div key={file.path} className="card data-record-item">
              <div className="data-record-head">
                <div className="data-record-title">{file.name}</div>
                <div className="data-inline-list">
                  <span className="dashboard-inline-chip">{fileNamespaceLabel(file.path)}</span>
                  {file.kind && <span className="dashboard-inline-chip">{file.kind}</span>}
                  <span className="data-record-meta">{formatDateTime(file.updated_at || file.created_at)}</span>
                </div>
              </div>
              <div className="data-record-path">{file.path}</div>
              <div className="data-record-preview">{summarizeNodeContent(file, 220)}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
