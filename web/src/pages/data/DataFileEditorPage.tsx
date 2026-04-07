import { useEffect, useMemo, useState, useCallback } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, type FileNode } from '../../api'
import { displayNameFromPath, fileNamespaceLabel, formatDateTime } from './DataShared'
import MDEditor from '@uiw/react-md-editor'
import '@uiw/react-md-editor/markdown-editor.css'
import '@uiw/react-markdown-preview/markdown.css'

export default function DataFileEditorPage() {
  const params = useParams()
  const navigate = useNavigate()
  const raw = params['*'] || ''
  const path = useMemo(() => {
    const decoded = decodeURIComponent(raw)
    return decoded.startsWith('/') ? decoded : `/${decoded}`
  }, [raw])

  const [node, setNode] = useState<FileNode | null>(null)
  const [content, setContent] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  useEffect(() => {
    let mounted = true
    const load = async () => {
      setLoading(true)
      setError('')
      setSuccess('')
      try {
        const data = await api.getTree(path)
        if (!mounted) return
        setNode(data)
        setContent(data.content || '')
      } catch (err: any) {
        setError(err.message || '加载文件失败')
      } finally {
        setLoading(false)
      }
    }
    load()
    return () => { mounted = false }
  }, [path])

  const handleSave = useCallback(async () => {
    if (!node) return
    setSaving(true)
    setError('')
    setSuccess('')
    try {
      const saved = await api.writeTree(path, {
        content,
        mimeType: node.mime_type || (path.toLowerCase().endsWith('.md') ? 'text/markdown' : 'text/plain'),
        isDir: false,
        expectedVersion: node.version,
        expectedChecksum: node.checksum,
      })
      setNode(saved)
      setSuccess('保存成功')
    } catch (err: any) {
      const msg = String(err.message || '')
      if (msg.toLowerCase().includes('conflict')) {
        setError('保存失败：版本冲突。请刷新后重试，或手动合并更改。')
      } else if (msg.toLowerCase().includes('read-only')) {
        setError('保存失败：该路径为只读（系统生成或受保护）。建议另存为到 /notes/ 或 /projects/ 路径下，或复制到你自己的 /skills/ 子目录。')
      } else {
        setError(err.message || '保存失败')
      }
    } finally {
      setSaving(false)
    }
  }, [node, path, content])

  // 保存快捷键 Cmd/Ctrl+S
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const isSave = (e.key === 's' || e.key === 'S') && (e.metaKey || e.ctrlKey)
      if (isSave) {
        e.preventDefault()
        handleSave()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [handleSave])

  const isDirty = node ? (content !== (node.content || '')) : false

  // 离开页未保存提示（刷新/关闭）
  useEffect(() => {
    const beforeUnload = (e: BeforeUnloadEvent) => {
      if (!isDirty) return
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', beforeUnload)
    return () => window.removeEventListener('beforeunload', beforeUnload)
  }, [isDirty])

  const title = displayNameFromPath(path)

  if (loading) return <div className="page-loading">加载中...</div>
  if (!node) {
    return (
      <div className="page">
        <div className="page-header">
          <h2>未找到文件</h2>
          <div className="page-actions">
            <button className="btn" onClick={() => navigate(-1)}>返回</button>
          </div>
        </div>
        {error && <div className="alert alert-error">{error}</div>}
      </div>
    )
  }

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h2>编辑：{title}</h2>
          <p className="page-subtitle">
            <span className="dashboard-inline-chip">{fileNamespaceLabel(node.path)}</span>
            {node.kind && <span className="dashboard-inline-chip" style={{ marginLeft: 8 }}>{node.kind}</span>}
            <span className="data-record-meta" style={{ marginLeft: 8 }}>最近更新：{formatDateTime(node.updated_at || node.created_at)}</span>
          </p>
          <div className="data-record-path">{node.path}</div>
        </div>
        <div className="page-actions">
          <button
            className="btn"
            onClick={() => {
              if (isDirty && !confirm('有未保存的更改，确定要离开吗？')) return
              navigate(-1)
            }}
          >返回</button>
          <button className="btn btn-primary" onClick={handleSave} disabled={saving}>{saving ? '保存中…' : '保存'}</button>
        </div>
      </div>

      {error && <div className="alert alert-error" role="alert">{error}</div>}
      {success && <div className="alert alert-success" role="status">{success}</div>}

      <div className="card" data-color-mode="light">
        <MDEditor
          value={content}
          onChange={(v) => setContent(v || '')}
          preview="edit"
          height={520}
          visibleDragbar={false}
        />
      </div>
      <div className="card" data-color-mode="light" style={{ marginTop: 12 }}>
        <MDEditor.Markdown source={content} style={{ background: 'transparent' }} />
      </div>
    </div>
  )
}
