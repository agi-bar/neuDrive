import { Suspense, lazy, useCallback, useDeferredValue, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import type { EditorView } from '@codemirror/view'
import { api, type FileNode } from '../../api'
import { useI18n } from '../../i18n'
import { displayNameFromPath, fileNamespaceLabel, formatDateTime, sourceLabel } from './DataShared'
import '@uiw/react-markdown-preview/markdown.css'

const WorkbenchCodeEditor = lazy(() => import('./WorkbenchCodeEditor'))
const MarkdownPreview = lazy(() => import('@uiw/react-markdown-preview'))

type ViewMode = 'write' | 'split' | 'preview'
type MobileSplitPane = 'editor' | 'preview'
type EditorMutation = {
  from: number
  to: number
  insert: string
  selectionAnchor: number
  selectionHead?: number
}

type LineSelection = {
  from: number
  to: number
  lines: string[]
  selectionFrom: number
  selectionTo: number
  isCollapsedSingleLine: boolean
}

function isMarkdownFilePath(path: string, mimeType?: string) {
  return mimeType === 'text/markdown' || /\.md$/i.test(path)
}

function mimeTypeForPath(path: string, currentMimeType?: string) {
  if (isMarkdownFilePath(path, currentMimeType)) return 'text/markdown'
  return 'text/plain'
}

function toggleWrappedSelection(
  view: EditorView,
  prefix: string,
  suffix: string,
  fallback: string,
  options?: { strictSingleMarker?: boolean },
): EditorMutation {
  const selection = view.state.selection.main
  const doc = view.state.doc
  const selected = doc.sliceString(selection.from, selection.to)
  const hasSelection = selection.from !== selection.to
  const wrappedFrom = selection.from - prefix.length
  const wrappedTo = selection.to + suffix.length
  const before = wrappedFrom >= 0 ? doc.sliceString(wrappedFrom, selection.from) : ''
  const after = wrappedTo <= doc.length ? doc.sliceString(selection.to, wrappedTo) : ''
  let canUnwrap = hasSelection && before === prefix && after === suffix

  if (canUnwrap && options?.strictSingleMarker && prefix === suffix && prefix.length === 1) {
    const beforePrev = wrappedFrom > 0 ? doc.sliceString(wrappedFrom - 1, wrappedFrom) : ''
    const afterNext = wrappedTo < doc.length ? doc.sliceString(wrappedTo, wrappedTo + 1) : ''
    if (beforePrev === prefix || afterNext === suffix) canUnwrap = false
  }

  if (canUnwrap) {
    return {
      from: wrappedFrom,
      to: wrappedTo,
      insert: selected,
      selectionAnchor: wrappedFrom,
      selectionHead: wrappedFrom + selected.length,
    }
  }

  const inner = hasSelection ? selected : fallback
  return {
    from: selection.from,
    to: selection.to,
    insert: `${prefix}${inner}${suffix}`,
    selectionAnchor: selection.from + prefix.length,
    selectionHead: selection.from + prefix.length + inner.length,
  }
}

function getLineSelection(view: EditorView): LineSelection {
  const selection = view.state.selection.main
  const startLine = view.state.doc.lineAt(selection.from)
  const endPos = selection.from === selection.to ? selection.to : Math.max(selection.from, selection.to - 1)
  const endLine = view.state.doc.lineAt(endPos)
  return {
    from: startLine.from,
    to: endLine.to,
    lines: view.state.doc.sliceString(startLine.from, endLine.to).split('\n'),
    selectionFrom: selection.from,
    selectionTo: selection.to,
    isCollapsedSingleLine: selection.from === selection.to && startLine.number === endLine.number,
  }
}

function toggleHeading(view: EditorView, level: 1 | 2 | 3): EditorMutation {
  const ctx = getLineSelection(view)
  const prefix = `${'#'.repeat(level)} `
  const headingPattern = /^(#{1,6}\s+)/
  const allHaveSameHeading = ctx.lines.every((line) => line.startsWith(prefix))
  const insert = ctx.lines
    .map((line) => {
      if (allHaveSameHeading) return line.replace(headingPattern, '')
      if (headingPattern.test(line)) return line.replace(headingPattern, prefix)
      return `${prefix}${line}`
    })
    .join('\n')

  if (ctx.isCollapsedSingleLine) {
    const currentMatch = ctx.lines[0]?.match(headingPattern)
    const currentPrefixLength = currentMatch?.[0]?.length || 0
    const nextCursor = allHaveSameHeading
      ? Math.max(ctx.from, ctx.selectionFrom - currentPrefixLength)
      : ctx.selectionFrom + prefix.length - currentPrefixLength
    return {
      from: ctx.from,
      to: ctx.to,
      insert,
      selectionAnchor: nextCursor,
      selectionHead: nextCursor,
    }
  }

  return {
    from: ctx.from,
    to: ctx.to,
    insert,
    selectionAnchor: ctx.from,
    selectionHead: ctx.from + insert.length,
  }
}

function toggleLinePattern(
  view: EditorView,
  pattern: RegExp,
  addLine: (line: string, index: number) => string,
  defaultPrefixLength: number,
): EditorMutation {
  const ctx = getLineSelection(view)
  const allHavePattern = ctx.lines.every((line) => pattern.test(line))
  const insert = ctx.lines
    .map((line, index) => (allHavePattern ? line.replace(pattern, '') : addLine(line, index)))
    .join('\n')

  if (ctx.isCollapsedSingleLine) {
    const currentMatch = ctx.lines[0]?.match(pattern)
    const currentPrefixLength = currentMatch?.[0]?.length || 0
    const nextCursor = allHavePattern
      ? Math.max(ctx.from, ctx.selectionFrom - currentPrefixLength)
      : ctx.selectionFrom + defaultPrefixLength - currentPrefixLength
    return {
      from: ctx.from,
      to: ctx.to,
      insert,
      selectionAnchor: nextCursor,
      selectionHead: nextCursor,
    }
  }

  return {
    from: ctx.from,
    to: ctx.to,
    insert,
    selectionAnchor: ctx.from,
    selectionHead: ctx.from + insert.length,
  }
}

function toggleCodeBlock(view: EditorView): EditorMutation {
  return toggleWrappedSelection(view, '```text\n', '\n```', 'code')
}

function toggleLink(view: EditorView): EditorMutation {
  const selection = view.state.selection.main
  const doc = view.state.doc
  const selected = doc.sliceString(selection.from, selection.to)

  if (selection.from > 0 && doc.sliceString(selection.from - 1, selection.from) === '[') {
    const after = doc.sliceString(selection.to, Math.min(doc.length, selection.to + 2048))
    const linkSuffix = after.match(/^\]\(([^)]+)\)/)
    if (linkSuffix) {
      return {
        from: selection.from - 1,
        to: selection.to + linkSuffix[0].length,
        insert: selected,
        selectionAnchor: selection.from - 1,
        selectionHead: selection.from - 1 + selected.length,
      }
    }
  }

  const label = selected || 'link text'
  const url = 'https://example.com'
  const insert = `[${label}](${url})`
  return {
    from: selection.from,
    to: selection.to,
    insert,
    selectionAnchor: selection.from + 1,
    selectionHead: selection.from + 1 + label.length,
  }
}

function insertDivider(view: EditorView): EditorMutation {
  const selection = view.state.selection.main
  const insert = selection.from === selection.to ? '\n---\n' : '\n\n---\n'
  return {
    from: selection.to,
    to: selection.to,
    insert,
    selectionAnchor: selection.to + insert.length,
    selectionHead: selection.to + insert.length,
  }
}

export default function DataFileEditorPage() {
  const { locale, tx } = useI18n()
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
  const [renamePath, setRenamePath] = useState(path)
  const [viewMode, setViewMode] = useState<ViewMode>('write')
  const [mobileSplitPane, setMobileSplitPane] = useState<MobileSplitPane>('editor')
  const [infoPanelOpen, setInfoPanelOpen] = useState(() => (typeof window === 'undefined' ? true : window.innerWidth >= 960))
  const [isCompactLayout, setIsCompactLayout] = useState(() => (typeof window === 'undefined' ? false : window.innerWidth < 960))
  const [lastSavedAt, setLastSavedAt] = useState<string>()
  const [isEditorReady, setIsEditorReady] = useState(false)
  const editorViewRef = useRef<EditorView | null>(null)
  const allowNavigationRef = useRef(false)
  const isMarkdownFile = useMemo(() => isMarkdownFilePath(path, node?.mime_type), [node?.mime_type, path])
  const deferredContent = useDeferredValue(content)

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
        setRenamePath(data.path)
        setLastSavedAt(data.updated_at || data.created_at)
      } catch (err: any) {
        setError(err.message || tx('加载文件失败', 'Failed to load file'))
      } finally {
        setLoading(false)
      }
    }
    load()
    return () => { mounted = false }
  }, [path, tx])

  useEffect(() => {
    const onResize = () => {
      const compact = window.innerWidth < 960
      setIsCompactLayout(compact)
      if (!compact) setInfoPanelOpen(true)
    }
    onResize()
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])

  useEffect(() => {
    setViewMode('write')
    setMobileSplitPane('editor')
    setInfoPanelOpen(typeof window === 'undefined' ? true : window.innerWidth >= 960)
    allowNavigationRef.current = false
    setIsEditorReady(false)
  }, [path])

  useEffect(() => {
    if (!isMarkdownFile) setViewMode('write')
  }, [isMarkdownFile])

  const handleSave = useCallback(async () => {
    if (!node) return
    setSaving(true)
    setError('')
    setSuccess('')
    try {
      const saved = await api.writeTree(path, {
        content,
        mimeType: mimeTypeForPath(path, node.mime_type),
        isDir: false,
        metadata: node.metadata,
        expectedVersion: node.version,
        expectedChecksum: node.checksum,
      })
      setNode(saved)
      setRenamePath(saved.path)
      setLastSavedAt(saved.updated_at || new Date().toISOString())
      setSuccess(tx('保存成功', 'Saved'))
    } catch (err: any) {
      const msg = String(err.message || '')
      if (msg.toLowerCase().includes('conflict')) {
        setError(tx('保存失败：版本冲突。请刷新后重试，或手动合并更改。', 'Save failed because of a version conflict. Refresh and try again, or merge the changes manually.'))
      } else if (msg.toLowerCase().includes('read-only')) {
        setError(tx('保存失败：该路径为只读（系统生成或受保护）。建议另存为到 /notes/ 或 /projects/ 路径下，或复制到你自己的 /skills/ 子目录。', 'Save failed because this path is read-only (system-generated or protected). Save to /notes/ or /projects/, or copy it into your own /skills/ subdirectory instead.'))
      } else {
        setError(err.message || tx('保存失败', 'Save failed'))
      }
    } finally {
      setSaving(false)
    }
  }, [content, node, path, tx])

  const handleRename = useCallback(async () => {
    if (!node) return
    const nextPath = renamePath.trim()
    if (!nextPath || nextPath === path) {
      setRenamePath(path)
      return
    }

    setSaving(true)
    setError('')
    setSuccess('')
    try {
      await api.writeTree(nextPath, {
        content,
        mimeType: mimeTypeForPath(nextPath, node.mime_type),
        isDir: false,
        metadata: node.metadata,
      })
      await api.deleteTree(path)
      allowNavigationRef.current = true
      navigate(`/data/files/edit/${encodeURIComponent(nextPath.replace(/^\/+/, ''))}`, { replace: true })
      setSuccess(tx('已重命名', 'Renamed'))
    } catch (err: any) {
      const msg = String(err.message || '')
      if (msg.toLowerCase().includes('read-only')) {
        setError(tx('重命名失败：源路径或目标路径是只读目录。', 'Rename failed because the source or destination path is read-only.'))
      } else {
        setError(err.message || tx('重命名失败', 'Rename failed'))
      }
    } finally {
      setSaving(false)
    }
  }, [content, navigate, node, path, renamePath, tx])

  // 保存快捷键 Cmd/Ctrl+S
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const isSave = (e.key === 's' || e.key === 'S') && (e.metaKey || e.ctrlKey)
      if (isSave) {
        e.preventDefault()
        void handleSave()
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

  useEffect(() => {
    if (!success) return
    const timer = window.setTimeout(() => setSuccess(''), 2600)
    return () => window.clearTimeout(timer)
  }, [success])

  useEffect(() => {
    if (!isDirty) return
    const onDocumentClick = (event: MouseEvent) => {
      if (allowNavigationRef.current) return
      if (event.defaultPrevented) return
      if (event.button !== 0) return
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
      const target = event.target instanceof Element ? event.target.closest('a[href]') : null
      if (!(target instanceof HTMLAnchorElement)) return
      if (target.target && target.target !== '_self') return
      const href = target.getAttribute('href')
      if (!href || href.startsWith('http') || href.startsWith('mailto:') || href.startsWith('tel:')) return
      if (href.startsWith('#')) return
      const confirmed = window.confirm(tx('有未保存的更改，确定要离开吗？', 'You have unsaved changes. Leave anyway?'))
      if (!confirmed) {
        event.preventDefault()
        event.stopPropagation()
        return
      }
      allowNavigationRef.current = true
    }
    document.addEventListener('click', onDocumentClick, true)
    return () => document.removeEventListener('click', onDocumentClick, true)
  }, [isDirty, tx])

  const handleBack = useCallback(() => {
    if (isDirty && !window.confirm(tx('有未保存的更改，确定要离开吗？', 'You have unsaved changes. Leave anyway?'))) return
    allowNavigationRef.current = true
    navigate(-1)
  }, [isDirty, navigate, tx])

  const handleEditorReady = useCallback((view: EditorView | null) => {
    editorViewRef.current = view
    setIsEditorReady(Boolean(view))
  }, [])

  const applySelectionMutation = useCallback((build: (view: EditorView) => EditorMutation) => {
    const view = editorViewRef.current
    if (!view) return
    const next = build(view)
    const selectionHead = next.selectionHead ?? next.selectionAnchor
    view.dispatch({
      changes: { from: next.from, to: next.to, insert: next.insert },
      selection: {
        anchor: next.selectionAnchor,
        head: selectionHead,
      },
      scrollIntoView: true,
    })
    view.focus()
  }, [])

  const title = displayNameFromPath(path)
  const editorFallback = <div className="page-loading" style={{ minHeight: 240 }}>{tx('编辑器加载中...', 'Loading editor...')}</div>
  const previewFallback = <div className="page-loading" style={{ minHeight: 180 }}>{tx('预览加载中...', 'Loading preview...')}</div>
  const effectiveView = effectiveEditorViewMode(isMarkdownFile, viewMode)
  const editorVisible = effectiveView === 'write'
    || effectiveView === 'split' && (!isCompactLayout || mobileSplitPane === 'editor')
  const previewVisible = isMarkdownFile
    && (effectiveView === 'preview' || effectiveView === 'split' && (!isCompactLayout || mobileSplitPane === 'preview'))
  const toolbarDisabled = saving || !isEditorReady || effectiveView === 'preview'
  const savedMetaLabel = lastSavedAt
    ? `${tx('最近保存', 'Last saved')}: ${formatDateTime(lastSavedAt, locale)}`
    : tx('尚未保存', 'Not saved yet')

  if (loading) return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>
  if (!node) {
    return (
      <div className="page materials-page">
        <div className="page-header">
          <h2>{tx('未找到文件', 'File not found')}</h2>
          <div className="page-actions">
            <button className="btn" onClick={() => navigate(-1)}>{tx('返回', 'Back')}</button>
          </div>
        </div>
        {error && <div className="alert alert-error">{error}</div>}
      </div>
    )
  }

  return (
    <div className="page materials-page editor-workbench-page">
      {error && <div className="alert alert-error" role="alert">{error}</div>}
      {success && <div className="alert alert-success" role="status">{success}</div>}

      <div className="editor-workbench-shell">
        <div className="editor-command-bar">
          <div className="editor-command-bar-row">
            <div className="editor-command-title-group">
              <div className="editor-command-eyebrow">{tx('Markdown Workbench', 'Markdown Workbench')}</div>
              <h2 className="editor-command-title">{title}</h2>
              <div className="editor-command-status-row">
                <span className={`editor-status-pill ${isDirty ? 'is-dirty' : 'is-clean'}`}>
                  {isDirty ? tx('未保存更改', 'Unsaved changes') : tx('已保存', 'Saved')}
                </span>
                <span className="dashboard-inline-chip">{fileNamespaceLabel(node.path, locale)}</span>
                <span className="dashboard-inline-chip">{node.kind || (isMarkdownFile ? 'markdown' : 'text')}</span>
                <span className="dashboard-inline-chip">{sourceLabel(node.source, locale)}</span>
              </div>
              <div className="editor-command-meta">{savedMetaLabel}</div>
            </div>
            <div className="editor-command-actions">
              {isMarkdownFile && (
                <div className="materials-toggle-group" role="tablist" aria-label={tx('视图模式', 'View mode')}>
                  <button
                    className={`materials-toggle-item ${effectiveView === 'write' ? 'is-active' : ''}`}
                    onClick={() => setViewMode('write')}
                    type="button"
                  >
                    {tx('写作', 'Write')}
                  </button>
                  <button
                    className={`materials-toggle-item ${effectiveView === 'split' ? 'is-active' : ''}`}
                    onClick={() => setViewMode('split')}
                    type="button"
                  >
                    {tx('分栏', 'Split')}
                  </button>
                  <button
                    className={`materials-toggle-item ${effectiveView === 'preview' ? 'is-active' : ''}`}
                    onClick={() => setViewMode('preview')}
                    type="button"
                  >
                    {tx('预览', 'Preview')}
                  </button>
                </div>
              )}
              <button className="btn" type="button" onClick={() => setInfoPanelOpen((value) => !value)}>
                {infoPanelOpen ? tx('隐藏详情', 'Hide details') : tx('查看详情', 'Details')}
              </button>
              <button className="btn" type="button" onClick={handleBack}>{tx('返回', 'Back')}</button>
              <button className="btn btn-primary" type="button" onClick={() => void handleSave()} disabled={saving}>
                {saving ? tx('保存中…', 'Saving...') : tx('保存', 'Save')}
              </button>
            </div>
          </div>
          {isMarkdownFile && (
            <div className="editor-toolbar-row">
              <div className="editor-toolbar">
                <button className="btn btn-sm" type="button" disabled={toolbarDisabled} onClick={() => applySelectionMutation((view) => toggleHeading(view, 1))}>H1</button>
                <button className="btn btn-sm" type="button" disabled={toolbarDisabled} onClick={() => applySelectionMutation((view) => toggleHeading(view, 2))}>H2</button>
                <button className="btn btn-sm" type="button" disabled={toolbarDisabled} onClick={() => applySelectionMutation((view) => toggleHeading(view, 3))}>H3</button>
                <button className="btn btn-sm" type="button" disabled={toolbarDisabled} onClick={() => applySelectionMutation((view) => toggleWrappedSelection(view, '**', '**', 'bold text'))}>{tx('粗体', 'Bold')}</button>
                <button className="btn btn-sm" type="button" disabled={toolbarDisabled} onClick={() => applySelectionMutation((view) => toggleWrappedSelection(view, '*', '*', 'italic text', { strictSingleMarker: true }))}>{tx('斜体', 'Italic')}</button>
                <button className="btn btn-sm" type="button" disabled={toolbarDisabled} onClick={() => applySelectionMutation(toggleLink)}>{tx('链接', 'Link')}</button>
                <button className="btn btn-sm" type="button" disabled={toolbarDisabled} onClick={() => applySelectionMutation((view) => toggleLinePattern(view, /^>\s/, (line) => `> ${line}`, 2))}>{tx('引用', 'Quote')}</button>
                <button className="btn btn-sm" type="button" disabled={toolbarDisabled} onClick={() => applySelectionMutation((view) => toggleWrappedSelection(view, '`', '`', 'code', { strictSingleMarker: true }))}>{tx('行内代码', 'Inline code')}</button>
                <button className="btn btn-sm" type="button" disabled={toolbarDisabled} onClick={() => applySelectionMutation(toggleCodeBlock)}>{tx('代码块', 'Code block')}</button>
                <button className="btn btn-sm" type="button" disabled={toolbarDisabled} onClick={() => applySelectionMutation((view) => toggleLinePattern(view, /^[-*+]\s/, (line) => `- ${line}`, 2))}>{tx('无序列表', 'Bullet list')}</button>
                <button className="btn btn-sm" type="button" disabled={toolbarDisabled} onClick={() => applySelectionMutation((view) => toggleLinePattern(view, /^\d+\.\s/, (line, index) => `${index + 1}. ${line}`, 3))}>{tx('有序列表', 'Numbered list')}</button>
                <button className="btn btn-sm" type="button" disabled={toolbarDisabled} onClick={() => applySelectionMutation((view) => toggleLinePattern(view, /^- \[[ xX]\]\s/, (line) => `- [ ] ${line}`, 6))}>{tx('任务列表', 'Task list')}</button>
                <button className="btn btn-sm" type="button" disabled={toolbarDisabled} onClick={() => applySelectionMutation(insertDivider)}>{tx('分隔线', 'Divider')}</button>
              </div>
              <div className="editor-toolbar-hint">{tx('快捷键：Cmd/Ctrl + S', 'Shortcut: Cmd/Ctrl + S')}</div>
            </div>
          )}
        </div>

        <div className={`editor-workbench-layout ${infoPanelOpen ? 'is-info-open' : ''} ${isCompactLayout ? 'is-compact' : ''}`}>
          <div className="editor-workbench-main">
            {isCompactLayout && effectiveView === 'split' && isMarkdownFile && (
              <div className="editor-split-mobile-tabs">
                <button
                  className={`materials-toggle-item ${mobileSplitPane === 'editor' ? 'is-active' : ''}`}
                  onClick={() => setMobileSplitPane('editor')}
                  type="button"
                >
                  {tx('编辑', 'Editor')}
                </button>
                <button
                  className={`materials-toggle-item ${mobileSplitPane === 'preview' ? 'is-active' : ''}`}
                  onClick={() => setMobileSplitPane('preview')}
                  type="button"
                >
                  {tx('预览', 'Preview')}
                </button>
              </div>
            )}

            <div className={`editor-stage editor-stage-${effectiveView}`}>
              {editorVisible && (
                <section className="editor-surface editor-surface-editor">
                  <div className="editor-surface-header">
                    <div>
                      <div className="editor-surface-title">{tx('编辑区', 'Editor')}</div>
                      <div className="editor-surface-subtitle">
                        {isMarkdownFile ? tx('支持 Markdown 工具栏和实时排版预览。', 'Markdown tools and live preview are enabled.') : tx('当前文件以纯文本模式编辑。', 'This file is being edited in plain-text mode.')}
                      </div>
                    </div>
                    <div className="editor-surface-meta">{content.length} {tx('字符', 'chars')}</div>
                  </div>
                  <div className="editor-canvas">
                    <Suspense fallback={editorFallback}>
                      <WorkbenchCodeEditor
                        value={content}
                        isMarkdown={isMarkdownFile}
                        onChange={setContent}
                        onReady={handleEditorReady}
                      />
                    </Suspense>
                  </div>
                </section>
              )}

              {previewVisible && (
                <section className="editor-surface editor-surface-preview" data-color-mode="light">
                  <div className="editor-surface-header">
                    <div>
                      <div className="editor-surface-title">{tx('文档预览', 'Preview')}</div>
                      <div className="editor-surface-subtitle">{tx('排版、代码块和引用按最终文档样式渲染。', 'Typography, code blocks, and quotes render in their final document style.')}</div>
                    </div>
                    <div className="editor-surface-meta">{tx('只读', 'Read only')}</div>
                  </div>
                  <div className="editor-preview-shell">
                    <Suspense fallback={previewFallback}>
                      <MarkdownPreview source={deferredContent} className="editor-preview-markdown" style={{ background: 'transparent' }} />
                    </Suspense>
                  </div>
                </section>
              )}
            </div>
          </div>

          {isCompactLayout && infoPanelOpen && <button type="button" className="editor-info-backdrop" onClick={() => setInfoPanelOpen(false)} aria-label={tx('关闭详情面板', 'Close details')} />}

          <aside className={`editor-info-panel ${infoPanelOpen ? 'is-open' : ''}`}>
            <div className="editor-info-panel-header">
              <div>
                <div className="editor-info-eyebrow">{tx('文件信息', 'File details')}</div>
                <h3>{tx('侧边信息', 'Info panel')}</h3>
              </div>
              {isCompactLayout && (
                <button className="btn btn-sm" type="button" onClick={() => setInfoPanelOpen(false)}>
                  {tx('关闭', 'Close')}
                </button>
              )}
            </div>

            <div className="editor-info-section">
              <div className="editor-info-label">{tx('路径', 'Path')}</div>
              <div className="editor-info-value editor-info-path">{node.path}</div>
            </div>

            <div className="editor-info-grid">
              <div className="editor-info-card">
                <div className="editor-info-label">{tx('命名空间', 'Namespace')}</div>
                <div className="editor-info-value">{fileNamespaceLabel(node.path, locale)}</div>
              </div>
              <div className="editor-info-card">
                <div className="editor-info-label">{tx('来源', 'Source')}</div>
                <div className="editor-info-value">{sourceLabel(node.source, locale)}</div>
              </div>
              <div className="editor-info-card">
                <div className="editor-info-label">{tx('类型', 'Type')}</div>
                <div className="editor-info-value">{node.mime_type || (isMarkdownFile ? 'text/markdown' : 'text/plain')}</div>
              </div>
              <div className="editor-info-card">
                <div className="editor-info-label">{tx('最近更新', 'Updated')}</div>
                <div className="editor-info-value">{formatDateTime(node.updated_at || node.created_at, locale)}</div>
              </div>
            </div>

            <div className="editor-info-section">
              <div className="editor-info-label">{tx('重命名文件', 'Rename file')}</div>
              <div className="editor-info-help">{tx('修改路径后会创建新文件并删除旧路径。', 'Renaming creates the file at the new path and deletes the old one.')}</div>
              <div className="form-group">
                <input value={renamePath} onChange={(e) => setRenamePath(e.target.value)} />
              </div>
              <div className="form-actions">
                <button className="btn" type="button" disabled={saving || renamePath === path} onClick={() => setRenamePath(path)}>{tx('还原', 'Reset')}</button>
                <button className="btn btn-primary" type="button" disabled={saving || !renamePath.trim() || renamePath === path} onClick={() => void handleRename()}>
                  {saving ? tx('处理中…', 'Working...') : tx('重命名', 'Rename')}
                </button>
              </div>
            </div>
          </aside>
        </div>
      </div>
    </div>
  )
}

function effectiveEditorViewMode(isMarkdownFile: boolean, viewMode: ViewMode): ViewMode {
  if (!isMarkdownFile) return 'write'
  return viewMode
}
