import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, type FileNode, type SkillSummary } from '../../api'
import MaterialsSectionToolbar from '../../components/MaterialsSectionToolbar'
import FileMaterialsTile from '../../components/FileMaterialsTile'
import ResourceActionMenu from '../../components/ResourceActionMenu'
import ResourceConfirmDialog from '../../components/ResourceConfirmDialog'
import SourceFilterBar from '../../components/SourceFilterBar'
import useResourceCardMenu from '../../hooks/useResourceCardMenu'
import useTreeDeleteDialog from '../../hooks/useTreeDeleteDialog'
import { useI18n } from '../../i18n'
import {
  getMaterialsSortOptions,
  buildFileTileModel,
  buildSourceFilterOptions,
  buildSkillBundleTileModel,
  dataFileBrowseRoute,
  dataFileEditorRoute,
  fileNodeSource,
  matchesSourceFilter,
  skillSource,
  sourceLabel,
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
  const { locale, tx } = useI18n()
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
  const [sourceFilter, setSourceFilter] = useState('all')
  const { activeMenuId, closeMenu, isMenuOpen, toggleMenu } = useResourceCardMenu()

  const load = useCallback(async () => {
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
      closeMenu()
      if (!currentBundlePath) {
        setSelectedBundlePath(null)
      }
      setSelectedEntryPath(null)
    } catch (err: any) {
      setError(err.message || tx('加载技能失败', 'Failed to load skills'))
    } finally {
      setLoading(false)
    }
  }, [closeMenu, currentBundlePath, tx])

  const {
    closeDialog: closeDeleteDialog,
    confirmDelete,
    dialog: deleteDialog,
    requestDelete,
    submitting: deleteSubmitting,
  } = useTreeDeleteDialog({ tx, onDeleted: load })

  useEffect(() => {
    void load()
  }, [load])

  const currentSkill = currentBundlePath
    ? skills.find((skill) => skill.bundlePath === currentBundlePath) || null
    : null
  const selectedBundle = selectedBundlePath
    ? skills.find((skill) => skill.bundlePath === selectedBundlePath) || null
    : null
  const selectedDeletePath = currentBundlePath ? selectedEntryPath : selectedBundle?.bundlePath || null
  const canDeleteSelection = Boolean(selectedDeletePath && !(currentBundlePath ? currentSkill?.read_only : selectedBundle?.read_only))

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
  const filteredSkills = useMemo(
    () => sortedSkills.filter((skill) => matchesSourceFilter(skillSource(skill), sourceFilter)),
    [sortedSkills, sourceFilter],
  )
  const skillSourceOptions = useMemo(
    () => buildSourceFilterOptions(skills, skillSource, locale),
    [locale, skills],
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
  const filteredBundleEntries = useMemo(
    () => sortedBundleEntries.filter((entry) => matchesSourceFilter(fileNodeSource(entry), sourceFilter)),
    [sortedBundleEntries, sourceFilter],
  )
  const bundleSourceOptions = useMemo(
    () => buildSourceFilterOptions(bundleEntries, fileNodeSource, locale),
    [bundleEntries, locale],
  )

  const openBundleDetail = (bundleId: string) => {
    closeMenu()
    navigate(`/data/skills/${encodeRoutePath(bundleId)}`)
  }

  const openFileEditor = (path: string) => {
    closeMenu()
    navigate(dataFileEditorRoute(path))
  }

  const openFolder = (path: string) => {
    closeMenu()
    navigate(dataFileBrowseRoute(path))
  }

  const handleDownloadZip = useCallback(async (path: string) => {
    closeMenu()
    try {
      await api.downloadTreeZip(path)
    } catch (err: any) {
      setError(err.message || tx('下载 ZIP 失败', 'Failed to download ZIP'))
    }
  }, [closeMenu, tx])

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
        metadata: { source: 'manual' },
      })
      setShowNewForm(false)
      setNewBundleName('new-skill')
      navigate(dataFileEditorRoute(path))
    } catch (err: any) {
      setError(err.message || tx('新建技能失败', 'Failed to create skill'))
    } finally {
      setCreating(false)
    }
  }

  const sortOptions = getMaterialsSortOptions(locale)

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (deleteDialog || activeMenuId) return
      if (event.key === 'Escape') {
        if (currentBundlePath) setSelectedEntryPath(null)
        else setSelectedBundlePath(null)
        return
      }
      if (event.key === 'Delete' && selectedDeletePath && canDeleteSelection) {
        event.preventDefault()
        void requestDelete([selectedDeletePath])
        return
      }
      if (event.key !== 'Enter') return
      if (currentBundlePath && selectedEntryPath) {
        const entry = bundleEntries.find((item) => item.path === selectedEntryPath)
        if (!entry) return
        if (entry.is_dir) {
          openFolder(entry.path)
          return
        }
        if (isEditableFile(entry)) {
          openFileEditor(entry.path)
        }
        return
      }
      if (!currentBundlePath && selectedBundle) {
        openBundleDetail(selectedBundle.bundleId)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [activeMenuId, bundleEntries, canDeleteSelection, currentBundlePath, deleteDialog, openBundleDetail, openFileEditor, openFolder, requestDelete, selectedBundle, selectedDeletePath, selectedEntryPath])

  if (loading) {
    return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>
  }

  if (currentBundlePath) {
    return (
      <div className="page materials-page">
        <section className="materials-hero">
          <div className="materials-hero-copy">
            <nav aria-label={tx('面包屑', 'Breadcrumbs')} className="materials-breadcrumbs">
              <button className="btn-text" onClick={() => navigate('/data/skills')}>{tx('技能', 'Skills')}</button>
              <span className="breadcrumbs-sep">/</span>
              <span>{bundleRoute || 'bundle'}</span>
            </nav>
            <div className="materials-kicker">neuDrive Data</div>
            <h2 className="materials-title">{currentSkill?.name || bundleRoute}</h2>
            <p className="materials-subtitle">{skillSummaryDescription(currentSkill) || tx('这个技能 bundle 里的文件现在和文件管理器使用同一套卡片展示。', 'Files in this skill bundle now use the same card layout as the file browser.')}</p>
          </div>
        </section>

        {error && <div className="alert alert-warn">{error}</div>}
        {!error && !currentSkill && (
          <div className="alert alert-warn">{tx('没有找到这个 skill bundle。', 'This skill bundle could not be found.')}</div>
        )}

        {currentSkill && (
          <section className="materials-section">
            <div className="materials-section-head">
              <div>
                <h3 className="materials-section-title">{tx('Bundle 内容', 'Bundle contents')}</h3>
                <p className="materials-section-copy">{tx('这个 bundle 里的文件和文件夹按同一套文件卡片规则展示。', 'Files and folders in this bundle are displayed with the same file card rules.')}</p>
              </div>
              <MaterialsSectionToolbar
                count={filteredBundleEntries.length}
                sortKey={sortKey}
                sortOptions={sortOptions}
                sortDir={sortDir}
                onSortKeyChange={(value) => setSortKey(value as MaterialsSortKey)}
                onSortDirToggle={() => setSortDir((value) => (value === 'desc' ? 'asc' : 'desc'))}
              >
                <button
                  className="btn btn-sm materials-toolbar-control is-danger"
                  disabled={!canDeleteSelection}
                  onClick={() => {
                    if (selectedDeletePath) void requestDelete([selectedDeletePath])
                  }}
                >
                  {tx('删除', 'Delete')}
                </button>
              </MaterialsSectionToolbar>
            </div>

            {(bundleSourceOptions.length > 1 || sourceFilter !== 'all') && (
              <SourceFilterBar options={bundleSourceOptions} value={sourceFilter} onChange={setSourceFilter} />
            )}

            {filteredBundleEntries.length === 0 ? (
              <p className="dashboard-empty-copy">{tx('这个 bundle 目录目前还是空的。', 'This bundle is currently empty.')}</p>
            ) : (
              <div className="materials-grid">
                {filteredBundleEntries.map((entry) => {
                  const tile = buildFileTileModel({
                    node: entry,
                    variant: 'skill-bundle-entry',
                    bundleLabel: currentSkill?.name || bundleRoute,
                    locale,
                  })
                  return (
                    <FileMaterialsTile
                      key={entry.path}
                      node={tile.node}
                      subtitle={tile.subtitle}
                      description={tile.description}
                      extraPills={tile.source ? <span className="materials-tile-pill materials-source-pill">{sourceLabel(tile.source, locale)}</span> : undefined}
                      path={tile.path}
                      footerStart={tile.footerStart}
                      footerEnd={tile.footerEnd}
                      selected={selectedEntryPath === entry.path}
                      menuOpen={isMenuOpen(entry.path)}
                      menuButtonAriaLabel={tx(`打开 ${entry.name} 的工具菜单`, `Open tools menu for ${entry.name}`)}
                      menuPanel={(
                        <ResourceActionMenu
                          items={[
                            ...((entry.is_dir || isEditableFile(entry))
                              ? [{
                                  key: 'open',
                                  label: entry.is_dir ? tx('进入目录', 'Open folder') : tx('打开文件', 'Open file'),
                                  onSelect: () => {
                                    closeMenu()
                                    if (entry.is_dir) {
                                      openFolder(entry.path)
                                    } else {
                                      openFileEditor(entry.path)
                                    }
                                  },
                                }]
                              : []),
                            {
                              key: 'download',
                              label: tx('下载 ZIP', 'Download ZIP'),
                              onSelect: () => {
                                void handleDownloadZip(entry.path)
                              },
                            },
                            {
                              key: 'select',
                              label: selectedEntryPath === entry.path ? tx('取消选中', 'Unselect') : tx('加入选择', 'Select'),
                              onSelect: () => {
                                closeMenu()
                                setSelectedEntryPath((value) => value === entry.path ? null : entry.path)
                              },
                            },
                            ...(currentSkill?.read_only
                              ? []
                              : [{
                                  key: 'delete',
                                  label: tx('删除', 'Delete'),
                                  tone: 'danger' as const,
                                  onSelect: () => {
                                    closeMenu()
                                    void requestDelete([entry.path])
                                  },
                                }]),
                          ]}
                        />
                      )}
                      onMenuToggle={() => toggleMenu(entry.path)}
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
          <div className="materials-kicker">neuDrive Data</div>
          <h2 className="materials-title">{tx('技能', 'Skills')}</h2>
          <p className="materials-subtitle">{tx('按 skill bundle 展示 ', 'Show skills in ')}<code>/skills</code>{tx(' 下的技能。一个文件夹就是一个 skill，点开后再看 bundle 详情。', '. Each folder is a skill. Open it to inspect bundle details.')}</p>
        </div>
      </section>

      {error && <div className="alert alert-warn">{error}</div>}

      {showNewForm && (
        <div className="materials-panel form-card">
          <div className="materials-section-head">
            <div>
              <h3 className="materials-section-title">{tx('新建技能', 'New skill')}</h3>
              <p className="materials-section-copy">{tx('创建一个新的 skill bundle，并直接进入 ', 'Create a new skill bundle and jump straight into the ')}<code>SKILL.md</code>{tx(' 编辑器。', ' editor.')}</p>
            </div>
          </div>
          <form onSubmit={handleCreateSkill}>
            <div className="form-group">
              <label htmlFor="skill-name">{tx('技能名称', 'Skill name')}</label>
              <input
                id="skill-name"
                type="text"
                value={newBundleName}
                onChange={(event) => setNewBundleName(event.target.value)}
                placeholder={tx('例如：meeting-notes', 'For example: meeting-notes')}
                disabled={creating}
                autoFocus
              />
            </div>
            <div className="form-actions">
              <button type="submit" className="btn btn-primary" disabled={creating}>
                {creating ? tx('创建中...', 'Creating...') : tx('创建', 'Create')}
              </button>
              <button type="button" className="btn" onClick={() => setShowNewForm(false)} disabled={creating}>
                {tx('取消', 'Cancel')}
              </button>
            </div>
          </form>
        </div>
      )}

      <section className="materials-section">
        <div className="materials-section-head">
          <div>
            <h3 className="materials-section-title">Skill Bundles</h3>
            <p className="materials-section-copy">{tx('统一按时间或名称浏览 skill bundle 卡片。', 'Browse skill bundle cards by time or name.')}</p>
          </div>
          <MaterialsSectionToolbar
            count={filteredSkills.length}
            sortKey={sortKey}
            sortOptions={sortOptions}
            sortDir={sortDir}
            onSortKeyChange={(value) => setSortKey(value as MaterialsSortKey)}
            onSortDirToggle={() => setSortDir((value) => (value === 'desc' ? 'asc' : 'desc'))}
          >
            <button className="btn btn-sm materials-toolbar-control" onClick={() => setShowNewForm((value) => !value)}>
              {showNewForm ? tx('取消新建', 'Close form') : tx('新建技能', 'New skill')}
            </button>
            <button
              className="btn btn-sm materials-toolbar-control is-danger"
              disabled={!canDeleteSelection}
              onClick={() => {
                if (selectedDeletePath) void requestDelete([selectedDeletePath])
              }}
            >
              {tx('删除', 'Delete')}
            </button>
          </MaterialsSectionToolbar>
        </div>

        {(skillSourceOptions.length > 1 || sourceFilter !== 'all') && (
          <SourceFilterBar options={skillSourceOptions} value={sourceFilter} onChange={setSourceFilter} />
        )}

        {filteredSkills.length === 0 ? (
          <div className="empty-state">
            <p>{tx('还没有技能内容', 'No skills yet')}</p>
            <p className="empty-hint">{tx('导入或创建 skill bundle 后，会在这里看到对应文件夹。', 'Imported or newly created skill bundles will appear here.')}</p>
          </div>
        ) : (
          <div className="materials-grid">
            {filteredSkills.map((skill) => {
              const tile = buildSkillBundleTileModel(skill, locale)
              return (
                <FileMaterialsTile
                  key={skill.path}
                  node={tile.node}
                  subtitle={tile.subtitle}
                  description={tile.description}
                  extraPills={tile.source ? <span className="materials-tile-pill materials-source-pill">{sourceLabel(tile.source, locale)}</span> : undefined}
                  path={tile.path}
                  footerStart={tile.footerStart}
                  footerEnd={tile.footerEnd}
                  selected={selectedBundlePath === tile.node.path}
                  menuOpen={isMenuOpen(skill.bundlePath)}
                  menuButtonAriaLabel={tx(`打开 ${skill.name} 的工具菜单`, `Open tools menu for ${skill.name}`)}
                  menuPanel={(
                    <ResourceActionMenu
                      items={[
                        {
                          key: 'open',
                          label: tx('进入 bundle', 'Open bundle'),
                          onSelect: () => {
                            closeMenu()
                            openBundleDetail(skill.bundleId)
                          },
                        },
                        {
                          key: 'download',
                          label: tx('下载 ZIP', 'Download ZIP'),
                          onSelect: () => {
                            void handleDownloadZip(skill.bundlePath)
                          },
                        },
                        {
                          key: 'select',
                          label: selectedBundlePath === tile.node.path ? tx('取消选中', 'Unselect') : tx('加入选择', 'Select'),
                          onSelect: () => {
                            closeMenu()
                            setSelectedBundlePath((value) => value === tile.node.path ? null : tile.node.path)
                          },
                        },
                        ...(!skill.read_only
                          ? [{
                              key: 'delete',
                              label: tx('删除', 'Delete'),
                              tone: 'danger' as const,
                              onSelect: () => {
                                closeMenu()
                                void requestDelete([skill.bundlePath])
                              },
                            }]
                          : []),
                      ]}
                    />
                  )}
                  onMenuToggle={() => toggleMenu(skill.bundlePath)}
                  onSelect={() => setSelectedBundlePath(tile.node.path)}
                  onOpen={() => openBundleDetail(skill.bundleId)}
                />
              )
            })}
          </div>
        )}
      </section>

      <ResourceConfirmDialog
        open={Boolean(deleteDialog)}
        kicker={tx('删除确认', 'Delete confirmation')}
        title={deleteDialog?.nonEmptyDirectories.length ? tx('这些目录不是空的', 'These folders are not empty') : tx('确认删除选中条目', 'Confirm deletion')}
        description={deleteDialog?.nonEmptyDirectories.length
          ? tx('确认后会递归删除其中所有可写文件和文件夹。只读内容不会被删除，可能会继续保留。', 'Continuing will recursively delete all writable files and folders inside. Read-only content will not be deleted and may remain in place.')
          : tx('这个操作会删除选中的技能文件或 bundle，且不可撤销。', 'This will delete the selected skill file or bundle and cannot be undone.')}
        cancelLabel={tx('取消', 'Cancel')}
        confirmLabel={deleteSubmitting ? tx('删除中...', 'Deleting...') : tx('确认删除', 'Delete')}
        tone="danger"
        submitting={deleteSubmitting}
        onCancel={closeDeleteDialog}
        onConfirm={() => void confirmDelete()}
      />
    </div>
  )
}
