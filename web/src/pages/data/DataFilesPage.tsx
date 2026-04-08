import { useEffect, useState } from 'react'
import { api, type FileNode } from '../../api'
import { fileNamespaceLabel, formatDateTime, isVisibleFileEntry, sortNodesByRecent } from './DataShared'
import { useNavigate } from 'react-router-dom'

export default function DataFilesPage() {
  const [files, setFiles] = useState<FileNode[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const navigate = useNavigate()
  const [creating, setCreating] = useState(false)
  const [createPath, setCreatePath] = useState('/notes/new-note.md')
  const [createContent, setCreateContent] = useState('# 新文档\n\n')
  const [busyPath, setBusyPath] = useState<string | null>(null)

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

  const refresh = async () => {
    setLoading(true)
    setError('')
    try {
      const snapshot = await api.getTreeSnapshot('/')
      setFiles(sortNodesByRecent(snapshot.entries.filter(isVisibleFileEntry)))
    } catch (err: any) {
      setError(err.message || '加载文件失败')
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = async (goEdit = true) => {
    try {
      setBusyPath(createPath)
      await api.writeTree(createPath, {
        content: createContent,
        mimeType: createPath.toLowerCase().endsWith('.md') ? 'text/markdown' : 'text/plain',
      })
      if (goEdit) {
        navigate(`/data/files/edit/${encodeURIComponent(createPath.replace(/^\/+/, ''))}`)
      } else {
        await refresh()
        setCreating(false)
      }
    } catch (err: any) {
      alert(`创建失败：${err.message || err}`)
    } finally {
      setBusyPath(null)
    }
  }

  const handleDelete = async (path: string) => {
    if (!confirm(`确定删除该文件吗？\n${path}`)) return
    try {
      setBusyPath(path)
      await api.deleteTree(path)
      await refresh()
    } catch (err: any) {
      const msg = String(err.message || '')
      if (msg.includes('read-only')) {
        alert('删除失败：该路径为只读（系统文件或受保护区域）。')
      } else {
        alert(`删除失败：${msg}`)
      }
    } finally {
      setBusyPath(null)
    }
  }

  return (
    <div className="page">
      <div className="page-header page-header-stack">
        <div>
          <h2>最近更新</h2>
          <p className="page-subtitle">这里按更新时间列出最近改过的 Hub 文档。点击文件名会直接进入编辑器。</p>
        </div>
        <div className="page-actions">
          {!creating ? (
            <button className="btn" onClick={() => setCreating(true)}>新建文件</button>
          ) : (
            <button className="btn" onClick={() => setCreating(false)}>取消</button>
          )}
        </div>
      </div>

      {error && <div className="alert alert-warn">{error}</div>}
      <div className="alert" style={{ background: '#fffbeb', border: '1px solid #fde68a' }}>
        小提示：部分系统路径是只读的（例如内置技能/设备配置等）。如遇“path is read-only”，请改在 <code>/notes/</code>、<code>/projects/</code> 或你自己的 <code>/skills/</code> 子目录下创建/编辑。
      </div>

      {creating && (
        <div className="card">
          <div className="form-row">
            <div className="form-group">
              <label>路径</label>
              <input value={createPath} onChange={e => setCreatePath(e.target.value)} placeholder="/notes/new-note.md" />
            </div>
            <div className="form-group">
              <label>操作</label>
              <div className="form-actions">
                <button className="btn" disabled={!!busyPath} onClick={() => handleCreate(false)}>仅创建</button>
                <button className="btn btn-primary" disabled={!!busyPath} onClick={() => handleCreate(true)}>创建并编辑</button>
              </div>
            </div>
          </div>
          <div className="form-group">
            <label>初始内容（可选）</label>
            <textarea rows={6} value={createContent} onChange={e => setCreateContent(e.target.value)} />
          </div>
        </div>
      )}

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
                <button
                  type="button"
                  className="btn-text data-record-title-button"
                  onClick={() => navigate(`/data/files/edit/${encodeURIComponent(file.path.replace(/^\/+/, ''))}`)}
                >
                  {file.name}
                </button>
                <div className="data-inline-list">
                  <span className="dashboard-inline-chip">{fileNamespaceLabel(file.path)}</span>
                  {file.kind && <span className="dashboard-inline-chip">{file.kind}</span>}
                  <span className="data-record-meta">{formatDateTime(file.updated_at || file.created_at)}</span>
                </div>
              </div>
              <div className="data-record-path">{file.path}</div>
              <div className="form-actions" style={{ marginTop: 10 }}>
                {!file.is_dir && (
                  <button className="btn btn-sm btn-danger" disabled={busyPath === file.path} onClick={() => handleDelete(file.path)}>删除</button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
