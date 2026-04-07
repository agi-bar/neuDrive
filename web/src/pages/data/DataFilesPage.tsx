import { useEffect, useMemo, useState } from 'react'
import { api, type FileNode } from '../../api'
import { fileNamespaceLabel, formatDateTime, isVisibleFileEntry, sortNodesByRecent, summarizeNodeContent } from './DataShared'
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
  const [renameFor, setRenameFor] = useState<string | null>(null)
  const [renameTo, setRenameTo] = useState('')

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

  const handleBeginRename = (path: string) => {
    setRenameFor(path)
    setRenameTo(path)
  }

  const handleRename = async (fromPath: string) => {
    if (!renameTo || renameTo === fromPath) {
      setRenameFor(null)
      return
    }
    try {
      setBusyPath(fromPath)
      const file = files.find(f => f.path === fromPath)
      const content = file?.content || ''
      await api.writeTree(renameTo, {
        content,
        mimeType: (file?.mime_type) || (renameTo.toLowerCase().endsWith('.md') ? 'text/markdown' : 'text/plain'),
      })
      await api.deleteTree(fromPath)
      await refresh()
      setRenameFor(null)
    } catch (err: any) {
      const msg = String(err.message || '')
      if (msg.includes('read-only')) {
        alert('重命名失败：目标或源路径为只读。')
      } else {
        alert(`重命名失败：${msg}`)
      }
    } finally {
      setBusyPath(null)
    }
  }

  return (
    <div className="page">
      <div className="page-header page-header-stack">
        <div>
          <h2>所有文件</h2>
          <p className="page-subtitle">这里按最近更新时间汇总展示 Hub 中的全部文件内容，包含项目、技能、Memory、我的资料、设备、Roles、Inbox 和根文件空间。</p>
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
                <div className="data-record-title">{file.name}</div>
                <div className="data-inline-list">
                  <span className="dashboard-inline-chip">{fileNamespaceLabel(file.path)}</span>
                  {file.kind && <span className="dashboard-inline-chip">{file.kind}</span>}
                  <span className="data-record-meta">{formatDateTime(file.updated_at || file.created_at)}</span>
                </div>
              </div>
              <div className="data-record-path">{file.path}</div>
              <div className="data-record-preview">{summarizeNodeContent(file, 220)}</div>
              <div className="form-actions" style={{ marginTop: 10 }}>
                {!file.is_dir && (file.path.toLowerCase().endsWith('.md') || (file.mime_type || '').startsWith('text/')) && (
                  <button
                    className="btn btn-sm"
                    onClick={() => navigate(`/data/files/edit/${encodeURIComponent(file.path.replace(/^\/+/, ''))}`)}
                  >
                    编辑
                  </button>
                )}
                {!file.is_dir && (
                  <button className="btn btn-sm" disabled={busyPath === file.path} onClick={() => handleBeginRename(file.path)}>重命名</button>
                )}
                {!file.is_dir && (
                  <button className="btn btn-sm btn-danger" disabled={busyPath === file.path} onClick={() => handleDelete(file.path)}>删除</button>
                )}
              </div>
              {renameFor === file.path && (
                <div className="form-row" style={{ marginTop: 8 }}>
                  <div className="form-group" style={{ gridColumn: '1 / span 2' }}>
                    <label>新路径</label>
                    <input value={renameTo} onChange={e => setRenameTo(e.target.value)} />
                  </div>
                  <div className="form-group">
                    <label>&nbsp;</label>
                    <div className="form-actions">
                      <button className="btn btn-sm" disabled={busyPath === file.path} onClick={() => setRenameFor(null)}>取消</button>
                      <button className="btn btn-sm btn-primary" disabled={busyPath === file.path} onClick={() => handleRename(file.path)}>保存重命名</button>
                    </div>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
