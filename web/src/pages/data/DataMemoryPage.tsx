import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, type FileNode } from '../../api'
import MaterialsSectionToolbar from '../../components/MaterialsSectionToolbar'
import FileMaterialsTile from '../../components/FileMaterialsTile'
import { MATERIALS_SORT_OPTIONS, buildFileTileModel, dataFileEditorRoute, isMemoryEntry, type MaterialsSortDir, type MaterialsSortKey, sortMaterialsItems } from './DataShared'

function ensureMemoryFilename(value: string) {
  const trimmed = value.trim().replace(/^\/+/, '')
  if (!trimmed) return ''
  return /\.md$/i.test(trimmed) ? trimmed : `${trimmed}.md`
}

function memoryTitleFromFilename(filename: string) {
  return filename.replace(/\.md$/i, '').replace(/[-_]+/g, ' ').trim() || 'New Memory'
}

export default function DataMemoryPage() {
  const [entries, setEntries] = useState<FileNode[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showNewForm, setShowNewForm] = useState(false)
  const [newEntryName, setNewEntryName] = useState('memory-note.md')
  const [creating, setCreating] = useState(false)
  const [sortKey, setSortKey] = useState<MaterialsSortKey>('updated_at')
  const [sortDir, setSortDir] = useState<MaterialsSortDir>('desc')
  const navigate = useNavigate()

  useEffect(() => {
    const load = async () => {
      try {
        const snapshot = await api.getTreeSnapshot('/memory')
        setEntries(snapshot.entries.filter(isMemoryEntry))
      } catch (err: any) {
        setError(err.message || '加载 Memory 失败')
      } finally {
        setLoading(false)
      }
    }

    void load()
  }, [])

  const sortedEntries = useMemo(
    () =>
      sortMaterialsItems({
        items: entries,
        sortKey,
        sortDir,
        getName: (entry) => entry.name,
        getUpdatedAt: (entry) => entry.updated_at || entry.created_at,
      }),
    [entries, sortDir, sortKey],
  )

  const handleCreateMemory = async (event: React.FormEvent) => {
    event.preventDefault()
    const filename = ensureMemoryFilename(newEntryName)
    if (!filename) return

    setCreating(true)
    setError('')
    try {
      const path = `/memory/${filename}`
      const title = memoryTitleFromFilename(filename)
      await api.writeTree(path, {
        content: `# ${title}\n\n`,
        mimeType: 'text/markdown',
      })
      setShowNewForm(false)
      setNewEntryName('memory-note.md')
      const snapshot = await api.getTreeSnapshot('/memory')
      setEntries(snapshot.entries.filter(isMemoryEntry))
      navigate(dataFileEditorRoute(path))
    } catch (err: any) {
      setError(err.message || '新建 Memory 失败')
    } finally {
      setCreating(false)
    }
  }

  if (loading) {
    return <div className="page-loading">加载中...</div>
  }

  return (
    <div className="page materials-page">
      <section className="materials-hero">
        <div className="materials-hero-copy">
          <div className="materials-kicker">Agent Hub Data</div>
          <h2 className="materials-title">Memory</h2>
          <p className="materials-subtitle">这里显示 <code>/memory</code> 下的记忆内容，不包含“我的资料”使用的 <code>/memory/profile</code> 条目。</p>
        </div>
      </section>

      {error && <div className="alert alert-warn">{error}</div>}

      {showNewForm && (
        <div className="materials-panel form-card">
          <div className="materials-section-head">
            <div>
              <h3 className="materials-section-title">新建 Memory</h3>
              <p className="materials-section-copy">创建一个新的 markdown 记忆条目，保存后会直接进入编辑器。</p>
            </div>
          </div>
          <form onSubmit={handleCreateMemory}>
            <div className="form-group">
              <label htmlFor="memory-name">文件名称</label>
              <input
                id="memory-name"
                type="text"
                value={newEntryName}
                onChange={(event) => setNewEntryName(event.target.value)}
                placeholder="例如：travel-notes.md"
                disabled={creating}
                autoFocus
              />
            </div>
            <div className="form-actions">
              <button type="submit" className="btn btn-primary" disabled={creating}>
                {creating ? '创建中...' : '创建'}
              </button>
              <button type="button" className="btn" onClick={() => setShowNewForm(false)} disabled={creating}>
                取消
              </button>
            </div>
          </form>
        </div>
      )}

      <section className="materials-section">
        <div className="materials-section-head">
          <div>
            <h3 className="materials-section-title">Recent Memory</h3>
            <p className="materials-section-copy">统一按时间或名称整理可见的 memory 条目。</p>
          </div>
          <MaterialsSectionToolbar
            count={entries.length}
            sortKey={sortKey}
            sortOptions={MATERIALS_SORT_OPTIONS}
            sortDir={sortDir}
            onSortKeyChange={(value) => setSortKey(value as MaterialsSortKey)}
            onSortDirToggle={() => setSortDir((value) => (value === 'desc' ? 'asc' : 'desc'))}
          >
            <button className="btn btn-sm materials-toolbar-control" onClick={() => setShowNewForm((value) => !value)}>
              {showNewForm ? '取消新建' : '新建 Memory'}
            </button>
          </MaterialsSectionToolbar>
        </div>

        {entries.length === 0 ? (
          <div className="empty-state">
            <p>还没有 Memory 内容</p>
            <p className="empty-hint">Agent 写入记忆后，会在这里看到对应条目。</p>
          </div>
        ) : (
          <div className="materials-grid materials-grid-wide">
          {sortedEntries.map((entry) => (
            (() => {
              const tile = buildFileTileModel({ node: entry, variant: 'memory' })
              return (
                <FileMaterialsTile
                  key={entry.path}
                  node={tile.node}
                  subtitle={tile.subtitle}
                  description={tile.description}
                  path={tile.path}
                  footerStart={tile.footerStart}
                  footerEnd={tile.footerEnd}
                  onOpen={() => navigate(dataFileEditorRoute(entry.path))}
                />
              )
            })()
          ))}
          </div>
        )}
      </section>
    </div>
  )
}
