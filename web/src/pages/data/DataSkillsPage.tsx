import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, type FileNode, type SkillSummary } from '../../api'
import MaterialsSectionToolbar from '../../components/MaterialsSectionToolbar'
import FileMaterialsTile from '../../components/FileMaterialsTile'
import {
  MATERIALS_SORT_OPTIONS,
  buildFileTileModel,
  buildSkillBundleTileModel,
  dataFileBrowseRoute,
  dataFileEditorRoute,
  type MaterialsSortDir,
  type MaterialsSortKey,
  skillBundlePathFromSkillPath,
  skillSummaryDescription,
  sortMaterialsItems,
} from './DataShared'

type SkillBundle = SkillSummary & {
  bundleId: string
  bundlePath: string
  created_at?: string
  updated_at?: string
}

function bundleIdFromSkillPath(path: string) {
  return skillBundlePathFromSkillPath(path).replace(/^\/skills\/?/, '')
}

function encodeRoutePath(path: string) {
  return path
    .split('/')
    .filter(Boolean)
    .map((segment) => encodeURIComponent(segment))
    .join('/')
}

function isEditableFile(entry: FileNode) {
  if (entry.is_dir) return false
  const mimeType = entry.mime_type || ''
  return /\.md$/i.test(entry.name) || mimeType.startsWith('text/')
}

function normalizeBundleName(value: string) {
  return value.trim().replace(/^\/+|\/+$/g, '').replace(/\s+/g, '-')
}

function skillStarterMarkdown(name: string) {
  return `---
name: ${name}
description:
---

# ${name}

## Use when

Describe when this skill should be used.

## Avoid when

Describe when this skill should not be used.
`
}

export default function DataSkillsPage() {
  const navigate = useNavigate()
  const params = useParams()
  const bundleRoute = (params['*'] || '')
    .split('/')
    .filter(Boolean)
    .map((segment) => decodeURIComponent(segment))
    .join('/')
  const currentBundlePath = bundleRoute ? `/skills/${bundleRoute}` : ''

  const [skills, setSkills] = useState<SkillBundle[]>([])
  const [bundleEntries, setBundleEntries] = useState<FileNode[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selectedBundlePath, setSelectedBundlePath] = useState<string | null>(null)
  const [selectedEntryPath, setSelectedEntryPath] = useState<string | null>(null)
  const [showNewForm, setShowNewForm] = useState(false)
  const [newBundleName, setNewBundleName] = useState('new-skill')
  const [creating, setCreating] = useState(false)
  const [sortKey, setSortKey] = useState<MaterialsSortKey>('updated_at')
  const [sortDir, setSortDir] = useState<MaterialsSortDir>('desc')

  useEffect(() => {
    const load = async () => {
      setLoading(true)
      setError('')
      try {
        const [skillData, skillsRoot, currentBundle] = await Promise.all([
          api.getSkills(),
          api.getTree('/skills'),
          currentBundlePath ? api.getTree(currentBundlePath) : Promise.resolve<FileNode | null>(null),
        ])

        const folderLookup = (skillsRoot.children || []).reduce<Record<string, FileNode>>((acc, child) => {
          acc[child.path] = child
          return acc
        }, {})

        const bundles = skillData
          .filter((skill) => skill.path.startsWith('/skills/'))
          .map((skill) => {
            const bundlePath = skillBundlePathFromSkillPath(skill.path)
            const folder = folderLookup[bundlePath]
            return {
              ...skill,
              bundleId: bundleIdFromSkillPath(skill.path),
              bundlePath,
              created_at: folder?.created_at,
              updated_at: folder?.updated_at || folder?.created_at,
            }
          })

        setSkills(bundles)
        setBundleEntries(currentBundle?.children || [])
        if (!currentBundlePath) {
          setSelectedBundlePath(null)
        }
        setSelectedEntryPath(null)
      } catch (err: any) {
        setError(err.message || '加载技能失败')
      } finally {
        setLoading(false)
      }
    }

    void load()
  }, [currentBundlePath])

  const currentSkill = currentBundlePath
    ? skills.find((skill) => skill.bundlePath === currentBundlePath) || null
    : null

  const sortedSkills = useMemo(
    () =>
      sortMaterialsItems({
        items: skills,
        sortKey,
        sortDir,
        getName: (skill) => skill.name,
        getUpdatedAt: (skill) => skill.updated_at || skill.created_at,
      }),
    [skills, sortDir, sortKey],
  )

  const sortedBundleEntries = useMemo(
    () =>
      sortMaterialsItems({
        items: bundleEntries,
        sortKey,
        sortDir,
        getName: (entry) => entry.name,
        getUpdatedAt: (entry) => entry.updated_at || entry.created_at,
        groupComparator: (left, right) => {
          if (left.name === 'SKILL.md' && right.name !== 'SKILL.md') return -1
          if (right.name === 'SKILL.md' && left.name !== 'SKILL.md') return 1
          if (left.is_dir !== right.is_dir) return left.is_dir ? -1 : 1
          return 0
        },
      }),
    [bundleEntries, sortDir, sortKey],
  )

  const openBundleDetail = (bundleId: string) => {
    navigate(`/data/skills/${encodeRoutePath(bundleId)}`)
  }

  const openFileEditor = (path: string) => {
    navigate(dataFileEditorRoute(path))
  }

  const openFolder = (path: string) => {
    navigate(dataFileBrowseRoute(path))
  }

  const handleCreateSkill = async (event: FormEvent) => {
    event.preventDefault()
    const bundleName = normalizeBundleName(newBundleName)
    if (!bundleName) return

    setCreating(true)
    setError('')
    try {
      const path = `/skills/${bundleName}/SKILL.md`
      await api.writeTree(path, {
        content: skillStarterMarkdown(bundleName),
        mimeType: 'text/markdown',
      })
      setShowNewForm(false)
      setNewBundleName('new-skill')
      navigate(dataFileEditorRoute(path))
    } catch (err: any) {
      setError(err.message || '新建技能失败')
    } finally {
      setCreating(false)
    }
  }

  if (loading) {
    return <div className="page-loading">加载中...</div>
  }

  if (currentBundlePath) {
    return (
      <div className="page materials-page">
        <section className="materials-hero">
          <div className="materials-hero-copy">
            <nav aria-label="面包屑" className="materials-breadcrumbs">
              <button className="btn-text" onClick={() => navigate('/data/skills')}>技能</button>
              <span className="breadcrumbs-sep">/</span>
              <span>{bundleRoute || 'bundle'}</span>
            </nav>
            <div className="materials-kicker">Agent Hub Data</div>
            <h2 className="materials-title">{currentSkill?.name || bundleRoute}</h2>
            <p className="materials-subtitle">{skillSummaryDescription(currentSkill) || '这个技能 bundle 里的文件现在和文件管理器使用同一套卡片展示。'}</p>
          </div>
        </section>

        {error && <div className="alert alert-warn">{error}</div>}
        {!error && !currentSkill && (
          <div className="alert alert-warn">没有找到这个 skill bundle。</div>
        )}

        {currentSkill && (
          <section className="materials-section">
            <div className="materials-section-head">
              <div>
                <h3 className="materials-section-title">Bundle 内容</h3>
                <p className="materials-section-copy">这个 bundle 里的文件和文件夹按同一套文件卡片规则展示。</p>
              </div>
              <MaterialsSectionToolbar
                count={bundleEntries.length}
                sortKey={sortKey}
                sortOptions={MATERIALS_SORT_OPTIONS}
                sortDir={sortDir}
                onSortKeyChange={(value) => setSortKey(value as MaterialsSortKey)}
                onSortDirToggle={() => setSortDir((value) => (value === 'desc' ? 'asc' : 'desc'))}
              />
            </div>

            {bundleEntries.length === 0 ? (
              <p className="dashboard-empty-copy">这个 bundle 目录目前还是空的。</p>
            ) : (
              <div className="materials-grid">
                {sortedBundleEntries.map((entry) => {
                  const tile = buildFileTileModel({
                    node: entry,
                    variant: 'skill-bundle-entry',
                    bundleLabel: currentSkill?.name || bundleRoute,
                  })
                  return (
                    <FileMaterialsTile
                      key={entry.path}
                      node={tile.node}
                      subtitle={tile.subtitle}
                      description={tile.description}
                      path={tile.path}
                      footerStart={tile.footerStart}
                      footerEnd={tile.footerEnd}
                      selected={selectedEntryPath === entry.path}
                      onSelect={() => setSelectedEntryPath(entry.path)}
                      onOpen={entry.is_dir ? () => openFolder(entry.path) : (isEditableFile(entry) ? () => openFileEditor(entry.path) : undefined)}
                    />
                  )
                })}
              </div>
            )}
          </section>
        )}
      </div>
    )
  }

  return (
    <div className="page materials-page">
      <section className="materials-hero">
        <div className="materials-hero-copy">
          <div className="materials-kicker">Agent Hub Data</div>
          <h2 className="materials-title">技能</h2>
          <p className="materials-subtitle">按 skill bundle 展示 <code>/skills</code> 下的技能。一个文件夹就是一个 skill，点开后再看 bundle 详情。</p>
        </div>
      </section>

      {error && <div className="alert alert-warn">{error}</div>}

      {showNewForm && (
        <div className="materials-panel form-card">
          <div className="materials-section-head">
            <div>
              <h3 className="materials-section-title">新建技能</h3>
              <p className="materials-section-copy">创建一个新的 skill bundle，并直接进入 <code>SKILL.md</code> 编辑器。</p>
            </div>
          </div>
          <form onSubmit={handleCreateSkill}>
            <div className="form-group">
              <label htmlFor="skill-name">技能名称</label>
              <input
                id="skill-name"
                type="text"
                value={newBundleName}
                onChange={(event) => setNewBundleName(event.target.value)}
                placeholder="例如：meeting-notes"
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
            <h3 className="materials-section-title">Skill Bundles</h3>
            <p className="materials-section-copy">统一按时间或名称浏览 skill bundle 卡片。</p>
          </div>
          <MaterialsSectionToolbar
            count={skills.length}
            sortKey={sortKey}
            sortOptions={MATERIALS_SORT_OPTIONS}
            sortDir={sortDir}
            onSortKeyChange={(value) => setSortKey(value as MaterialsSortKey)}
            onSortDirToggle={() => setSortDir((value) => (value === 'desc' ? 'asc' : 'desc'))}
          >
            <button className="btn btn-sm materials-toolbar-control" onClick={() => setShowNewForm((value) => !value)}>
              {showNewForm ? '取消新建' : '新建技能'}
            </button>
          </MaterialsSectionToolbar>
        </div>

        {skills.length === 0 ? (
          <div className="empty-state">
            <p>还没有技能内容</p>
            <p className="empty-hint">导入或创建 skill bundle 后，会在这里看到对应文件夹。</p>
          </div>
        ) : (
          <div className="materials-grid">
            {sortedSkills.map((skill) => {
              const tile = buildSkillBundleTileModel(skill)
              return (
                <FileMaterialsTile
                  key={skill.path}
                  node={tile.node}
                  subtitle={tile.subtitle}
                  description={tile.description}
                  path={tile.path}
                  footerStart={tile.footerStart}
                  footerEnd={tile.footerEnd}
                  selected={selectedBundlePath === tile.node.path}
                  onSelect={() => setSelectedBundlePath(tile.node.path)}
                  onOpen={() => openBundleDetail(skill.bundleId)}
                />
              )
            })}
          </div>
        )}
      </section>
    </div>
  )
}
