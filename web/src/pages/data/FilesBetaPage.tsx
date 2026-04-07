import { useEffect, useMemo, useState } from 'react'
import MarkdownPreview from '@uiw/react-markdown-preview'
import type { FileNode } from '../../api'
import { buildAgentDriveMock, listChildren } from './mock/agentdriveMock'

type Namespace = 'all' | 'projects' | 'skills' | 'memory' | 'devices' | 'roles' | 'inbox' | 'notes' | 'root'

function fmtSize(n?: number) {
  if (!n || n <= 0) return '-'
  const u = ['B', 'KB', 'MB', 'GB']
  let i = 0, v = n
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++ }
  return `${v.toFixed(1)} ${u[i]}`
}
function fmtTime(v?: string) { try { return v ? new Date(v).toLocaleString('zh-CN') : '-' } catch { return v || '-' } }

export default function FilesBetaPage() {
  const all = useMemo(() => buildAgentDriveMock(), [])
  const [cur, setCur] = useState<string>('/')
  const [ns] = useState<Namespace>('all')
  const [q, setQ] = useState('')
  const [preview, setPreview] = useState<FileNode | null>(null)

  const rows = useMemo(() => {
    if (q.trim()) {
      const qq = q.trim().toLowerCase()
      return all.filter(n => !n.is_dir && (ns === 'all' || topNs(n.path) === ns) && `${n.name} ${n.path} ${n.kind || ''}`.toLowerCase().includes(qq))
        .sort(byDirThenName)
    }
    return listChildren(all, cur).filter(n => ns === 'all' || topNs(n.path) === ns)
  }, [all, cur, ns, q])

  useEffect(() => { setPreview(null) }, [cur])

  return (
    <div className="page" data-color-mode="light">
      <div className="page-header">
        <div>
          <h2>文件（Beta · 内置假数据）</h2>
          <p className="page-subtitle">像 OneDrive 一样：面包屑 + 表格 + 预览。已融合 Projects / Skills / Memory / Devices / Roles / Inbox / Notes / Root。</p>
        </div>
      </div>

      <div className="page-actions" style={{ marginBottom: 10, gap: 6 }}>
        <Breadcrumbs path={cur} onNavigate={setCur} />
        <div style={{ marginLeft: 'auto', display: 'flex', gap: 6 }}>
          <input value={q} onChange={e => setQ(e.target.value)} placeholder="搜索（关键词/路径/Kind）" />
          <button className="btn" onClick={() => setQ('')}>清除</button>
        </div>
      </div>

      {/* 移除顶部 chips 过滤行以贴近 OneDrive/Dropbox 默认视图 */}

      <div className="dashboard-content-grid" style={{ gridTemplateColumns: '1fr 360px' }}>
        <div className="card">
          <div className="files-thead">
            <div className="files-th files-col-name">名称</div>
            <div className="files-th files-col-size">大小</div>
            <div className="files-th files-col-kind">命名空间</div>
            <div className="files-th files-col-time">最近修改</div>
          </div>
          <div className="files-tbody">
            {rows.length === 0 ? (
              <div className="files-empty">{q ? '无搜索结果' : '该目录暂无内容'}</div>
            ) : rows.map(n => (
              <div key={n.path} className="files-tr" onDoubleClick={() => n.is_dir ? setCur(n.path) : setPreview(n)}>
                <div className="files-td files-col-name">
                  <span className={"file-icon " + (n.is_dir ? 'fi-folder' : (/\.md$/i.test(n.name) ? 'fi-md' : 'fi-file'))} />
                  <span className="file-name">{n.name}{n.kind === 'skill' && <span className="dashboard-inline-chip" style={{ marginLeft: 6 }}>bundle</span>}</span>
                </div>
                <div className="files-td files-col-size">{n.is_dir ? '-' : fmtSize(n.size)}</div>
                <div className="files-td files-col-kind">{topNs(n.path)}</div>
                <div className="files-td files-col-time">{fmtTime(n.updated_at)}</div>
              </div>
            ))}
          </div>
        </div>

        <div className="card">
          <div className="card-header"><div className="card-title">预览</div></div>
          <div className="card-body">
            {!preview ? (
              <div className="data-record-secondary">选择一个 Markdown/文本文件查看预览；双击目录进入。</div>
            ) : (/\.md$/i.test(preview.name) ? (
              <MarkdownPreview source={preview.content || ''} style={{ background: 'transparent' }} />
            ) : preview.content ? (
              <pre style={{ whiteSpace: 'pre-wrap' }}>{preview.content}</pre>
            ) : (
              <div className="data-record-secondary">该文件暂无可预览内容</div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function Breadcrumbs({ path, onNavigate }: { path: string; onNavigate: (p: string) => void }) {
  const parts = path.replace(/^\/+/, '').split('/').filter(Boolean)
  const segs = ['/', ...parts.map((_, i) => '/' + parts.slice(0, i + 1).join('/'))]
  const labels = ['根目录', ...parts]
  return (
    <div className="breadcrumbs">
      {segs.map((seg, i) => (
        <span key={seg}>
          {i > 0 && <span className="breadcrumbs-sep">/</span>}
          <button className="btn btn-sm" onClick={() => onNavigate(seg)}>{labels[i]}</button>
        </span>
      ))}
    </div>
  )
}

function topNs(p: string): Namespace {
  const seg = p.replace(/^\/+/, '').split('/')[0] || ''
  if (['projects','skills','memory','devices','roles','inbox','notes'].includes(seg)) return seg as Namespace
  return 'root'
}

function byDirThenName(a: FileNode, b: FileNode) {
  return a.is_dir === b.is_dir ? a.name.localeCompare(b.name) : a.is_dir ? -1 : 1
}
