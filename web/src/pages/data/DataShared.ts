import { type BundleContext, type FileNode, type SkillSummary } from '../../api'
import { getLocaleTag, type AppLocale } from '../../i18n'

const ORDINARY_FILE_EXCLUDED_PREFIXES = [
  '/conversations/',
  '/projects/',
  '/skills/',
  '/memory/',
  '/roles/',
  '/inbox/',
]

const PROFILE_LABELS: Record<string, string> = {
  preferences: '个人偏好',
  relationships: '人际关系',
  principles: '行为准则',
}

const SYSTEM_CONTAINER_PATHS = new Set([
  '/conversations',
  '/inbox',
  '/memory',
  '/memory/profile',
  '/projects',
  '/roles',
  '/skills',
])

function text(locale: AppLocale, zh: string, en: string) {
  return locale === 'zh-CN' ? zh : en
}

const SOURCE_PRIORITY = [
  'manual',
  'upload',
  'claude-code',
  'claude-web',
  'claude-desktop',
  'codex',
  'chatgpt',
  'claude',
  'cursor',
  'windsurf',
  'gemini-cli',
  'gemini',
  'copilot',
  'perplexity',
  'kimi',
  'deepseek',
  'qwen',
  'zhipu',
  'minimax',
  'feishu',
  'open-webui',
  'openai',
  'mcp',
  'summary',
  'scheduler',
  'system',
  'import',
  'agent',
  'neudrive',
  'roles',
  'unknown',
] as const

function hasVisibleContent(node: FileNode) {
  return !node.is_dir && !node.deleted_at
}

function stripMarkdownSuffix(value: string) {
  return value.replace(/\.md$/i, '')
}

function topLevelSegment(path: string) {
  const normalized = path.replace(/^\/+/, '')
  return normalized.split('/')[0] || ''
}

function metadataValue(metadata: Record<string, any> | undefined, key: string) {
  const value = metadata?.[key]
  return typeof value === 'string' ? value.trim() : ''
}

function normalizeSource(value?: string) {
  const normalized = (value || '').trim().toLowerCase().replace(/_/g, '-').replace(/\s+/g, '-').replace(/--+/g, '-')
  if (!normalized) return ''
  if (normalized.startsWith('agent:')) return normalizeSource(normalized.slice('agent:'.length))
  if (normalized.startsWith('platform:')) return normalizeSource(normalized.slice('platform:'.length))

  switch (normalized) {
    case 'browser':
    case 'dashboard':
    case 'manual':
    case 'manually':
    case 'user':
    case 'web':
      return 'manual'
    case 'upload':
    case 'uploaded':
    case 'browser-upload':
    case 'file-upload':
      return 'upload'
    case 'gpt':
    case 'chatgpt-apps':
    case 'chatgpt-actions':
    case 'chatgpt.com':
    case 'chat-openai-com':
    case 'openai-chatgpt':
      return 'chatgpt'
    case 'codex-cli':
    case 'codex-mcp-client':
    case 'openai-codex':
      return 'codex'
    case 'cursor-vscode':
    case 'cursor-desktop':
    case 'cursor-agent':
      return 'cursor'
    case 'gemini-cli-mcp-client':
      return 'gemini-cli'
    case 'github-copilot':
    case 'copilot-chat':
      return 'copilot'
    case 'tongyi':
      return 'qwen'
    case 'chatglm':
    case 'glm':
    case 'bigmodel':
      return 'zhipu'
    case 'lark':
      return 'feishu'
    case 'openwebui':
      return 'open-webui'
    case 'bundle-import':
    case 'full-import':
      return 'import'
    case 'claude-import':
      return 'claude'
    case 'claudecode':
      return 'claude-code'
    case 'claude-connectors':
    case 'claude-connector':
    case 'claudeweb':
    case 'claude.ai':
      return 'claude-web'
    default:
      return normalized
  }
}

export function sourceLabel(source: string | undefined, locale: AppLocale = 'en') {
  switch (normalizeSource(source) || 'unknown') {
    case 'manual':
      return text(locale, '手工填写', 'Manual')
    case 'upload':
      return text(locale, '上传导入', 'Upload')
    case 'claude-code':
      return 'Claude Code'
    case 'claude-web':
      return 'Claude Web'
    case 'claude-desktop':
      return 'Claude Desktop'
    case 'codex':
      return 'Codex'
    case 'chatgpt':
      return 'ChatGPT'
    case 'claude':
      return 'Claude'
    case 'cursor':
      return 'Cursor'
    case 'windsurf':
      return 'Windsurf'
    case 'gemini-cli':
      return 'Gemini CLI'
    case 'gemini':
      return 'Gemini'
    case 'copilot':
      return 'GitHub Copilot'
    case 'perplexity':
      return 'Perplexity'
    case 'kimi':
      return 'Kimi'
    case 'deepseek':
      return 'DeepSeek'
    case 'qwen':
      return 'Qwen'
    case 'zhipu':
      return 'Zhipu GLM'
    case 'minimax':
      return 'MiniMax'
    case 'feishu':
      return locale === 'zh-CN' ? '飞书' : 'Feishu'
    case 'open-webui':
      return 'Open WebUI'
    case 'openai':
      return 'OpenAI'
    case 'mcp':
      return 'MCP'
    case 'summary':
      return text(locale, '摘要生成', 'Summary')
    case 'scheduler':
      return text(locale, '定时任务', 'Scheduler')
    case 'system':
      return text(locale, '系统', 'System')
    case 'import':
      return text(locale, '导入', 'Import')
    case 'agent':
      return 'Agent'
    case 'neudrive':
      return 'neuDrive'
    case 'roles':
      return 'Roles'
    case 'unknown':
      return text(locale, '未标注', 'Unknown')
    default: {
      const value = normalizeSource(source)
      return value
        .split('-')
        .filter(Boolean)
        .map((part) => part.slice(0, 1).toUpperCase() + part.slice(1))
        .join(' ')
    }
  }
}

export function sourceFilterLabel(source: string, locale: AppLocale = 'en') {
  return source === 'all' ? text(locale, '全部来源', 'All sources') : sourceLabel(source, locale)
}

function sourceSortValue(source: string) {
  const normalized = normalizeSource(source) || 'unknown'
  const index = SOURCE_PRIORITY.indexOf(normalized as (typeof SOURCE_PRIORITY)[number])
  return index === -1 ? SOURCE_PRIORITY.length + 1 : index
}

export function fileNodeSource(node: Pick<FileNode, 'source' | 'metadata' | 'path' | 'is_dir'>) {
  const metadata = node.metadata
  const platform = normalizeSource(metadataValue(metadata, 'source_platform'))
  if (platform) return platform
  const direct = normalizeSource(node.source)
  if (direct) return direct
  const explicit = normalizeSource(metadataValue(metadata, 'source'))
  if (explicit) return explicit
  if (metadataValue(metadata, 'capture_mode') === 'archive') return 'upload'
  if (node.is_dir && SYSTEM_CONTAINER_PATHS.has(normalizeHubPath(node.path))) return 'system'
  return ''
}

export function skillSource(skill?: Pick<SkillSummary, 'source'> | null) {
  return normalizeSource(skill?.source)
}

export function projectSource(project: { metadata?: Record<string, any> }) {
  const platform = normalizeSource(metadataValue(project.metadata, 'source_platform'))
  if (platform) return platform
  const explicit = normalizeSource(metadataValue(project.metadata, 'source'))
  if (explicit) return explicit
  if (metadataValue(project.metadata, 'capture_mode') === 'archive') return 'upload'
  return ''
}

export type SourceFilterOption = {
  value: string
  label: string
  count: number
}

export function buildSourceFilterOptions<T>(
  items: T[],
  getSource: (item: T) => string,
  locale: AppLocale = 'en',
): SourceFilterOption[] {
  const counts = new Map<string, number>()
  items.forEach((item) => {
    const value = getSource(item) || 'unknown'
    counts.set(value, (counts.get(value) || 0) + 1)
  })

  return Array.from(counts.entries())
    .map(([value, count]) => ({
      value,
      count,
      label: sourceFilterLabel(value, locale),
    }))
    .sort((left, right) => {
      const priorityDiff = sourceSortValue(left.value) - sourceSortValue(right.value)
      if (priorityDiff !== 0) return priorityDiff
      return left.label.localeCompare(right.label)
    })
}

export function matchesSourceFilter(source: string, filter: string) {
  const normalizedFilter = normalizeSource(filter) || filter
  if (!normalizedFilter || normalizedFilter === 'all') return true
  const normalizedSource = normalizeSource(source)
  if (normalizedFilter === 'unknown') return !normalizedSource
  return normalizedSource === normalizedFilter
}

export function normalizeHubPath(path: string) {
  const normalized = (path || '').trim()
  if (!normalized || normalized === '/') return '/'
  return `/${normalized.replace(/^\/+/, '').replace(/\/+$/, '')}`
}

export function isTextLikeFile(path: string, mimeType?: string) {
  const normalizedMime = (mimeType || '').toLowerCase()
  if (/\.md$/i.test(path)) return true
  if (/\.json$/i.test(path)) return true
  if (normalizedMime.startsWith('text/')) return true
  if (normalizedMime === 'application/json') return true
  return false
}

export function formatDateTime(value?: string, locale: AppLocale = 'en') {
  if (!value) return '-'
  try {
    return new Date(value).toLocaleString(getLocaleTag(locale))
  } catch {
    return value
  }
}

export function summarizeText(value?: string, maxLength = 140, locale: AppLocale = 'en') {
  if (!value) return text(locale, '暂无内容', 'No content yet')
  const normalized = value.replace(/\s+/g, ' ').trim()
  if (!normalized) return text(locale, '暂无内容', 'No content yet')
  if (normalized.length <= maxLength) return normalized
  return `${normalized.slice(0, maxLength).trimEnd()}...`
}

export function summarizeNodeContent(node: FileNode, maxLength = 140, locale: AppLocale = 'en') {
  return summarizeText(node.content, maxLength, locale)
}

export function recentTimestamp(node: FileNode) {
  return new Date(node.updated_at || node.created_at || 0).getTime()
}

export function sortNodesByRecent(entries: FileNode[]) {
  return [...entries].sort((a, b) => recentTimestamp(b) - recentTimestamp(a))
}

export type MaterialsSortKey = 'name' | 'updated_at'
export type MaterialsSortDir = 'asc' | 'desc'

export function getMaterialsSortOptions(locale: AppLocale): Array<{ value: MaterialsSortKey; label: string }> {
  return [
    { value: 'updated_at', label: text(locale, '按时间', 'By time') },
    { value: 'name', label: text(locale, '按名称', 'By name') },
  ]
}

export const MATERIALS_SORT_OPTIONS: Array<{ value: MaterialsSortKey; label: string }> = getMaterialsSortOptions('en')

type SortMaterialsItemsOptions<T> = {
  items: T[]
  sortKey: MaterialsSortKey
  sortDir: MaterialsSortDir
  getName: (item: T) => string
  getUpdatedAt: (item: T) => string | undefined
  groupComparator?: (left: T, right: T) => number
}

export function sortMaterialsItems<T>({
  items,
  sortKey,
  sortDir,
  getName,
  getUpdatedAt,
  groupComparator,
}: SortMaterialsItemsOptions<T>) {
  const multiplier = sortDir === 'asc' ? 1 : -1

  return [...items].sort((left, right) => {
    const groupDiff = groupComparator?.(left, right) || 0
    if (groupDiff !== 0) return groupDiff

    if (sortKey === 'name') {
      return getName(left).localeCompare(getName(right)) * multiplier
    }

    const leftTime = new Date(getUpdatedAt(left) || 0).getTime()
    const rightTime = new Date(getUpdatedAt(right) || 0).getTime()
    if (leftTime !== rightTime) {
      return (leftTime - rightTime) * multiplier
    }
    return getName(left).localeCompare(getName(right))
  })
}

export function normalizeSkillText(value?: string) {
  const text = (value || '').trim()
  if (!text || text === '---') return ''
  return text
}

export function skillBundlePathFromSkillPath(path: string) {
  return normalizeHubPath(path.replace(/\/SKILL\.md$/i, ''))
}

export function skillSummaryDescription(skill?: Pick<SkillSummary, 'description' | 'when_to_use'> | null) {
  if (!skill) return ''
  return normalizeSkillText(skill.description) || normalizeSkillText(skill.when_to_use)
}

export function buildSkillSummaryLookup(skills: SkillSummary[]) {
  return skills.reduce<Record<string, SkillSummary>>((acc, skill) => {
    acc[skillBundlePathFromSkillPath(skill.path)] = skill
    return acc
  }, {})
}

export function skillSummaryForPath(path: string, lookup: Record<string, SkillSummary>) {
  return lookup[normalizeHubPath(path)]
}

export type BundleKind = 'skill' | 'project' | 'conversation'

export type BundleInfo = {
  kind: BundleKind
  name: string
  path?: string
  description?: string
  whenToUse?: string
  status?: string
  readOnly?: boolean
  source?: string
  primaryPath?: string
  logPath?: string
  capabilities?: string[]
  relativePath?: string
}

export type FileTileModel = {
  node: Pick<FileNode, 'path' | 'name' | 'is_dir' | 'kind'>
  subtitle?: string
  description?: string
  path?: string
  source?: string
  footerStart?: string
  footerEnd?: string
}

export type FileTileVariant =
  | 'browser'
  | 'recent'
  | 'memory'
  | 'search'
  | 'bundle-entry'

type BuildFileTileModelOptions = {
  node: FileNode
  variant: FileTileVariant
  currentLabel?: string
  bundleLabel?: string
  locale?: AppLocale
}

function metadataDescription(node: FileNode) {
  const value = typeof node.metadata?.description === 'string' ? node.metadata.description : ''
  return normalizeSkillText(value) || ''
}

function bundleKindValue(node: Pick<FileNode, 'kind' | 'metadata' | 'bundle_context'>): BundleKind | '' {
  if (node.bundle_context?.kind === 'skill' || node.bundle_context?.kind === 'project' || node.bundle_context?.kind === 'conversation') {
    return node.bundle_context.kind
  }
  const bundleKind = metadataValue(node.metadata, 'bundle_kind')
  if (bundleKind === 'skill' || bundleKind === 'project' || bundleKind === 'conversation') return bundleKind
  if (node.kind === 'skill_bundle') return 'skill'
  if (node.kind === 'project_bundle') return 'project'
  if (node.kind === 'conversation_bundle') return 'conversation'
  return ''
}

export function bundleInfoFromContext(context?: BundleContext | null): BundleInfo | null {
  if (!context) return null
  return {
    kind: context.kind,
    name: context.name,
    path: context.path,
    description: normalizeSkillText(context.description) || undefined,
    whenToUse: normalizeSkillText(context.when_to_use) || undefined,
    status: context.status || undefined,
    readOnly: context.read_only === true,
    source: normalizeSource(context.source),
    primaryPath: context.primary_path || undefined,
    logPath: context.log_path || undefined,
    capabilities: Array.isArray(context.capabilities) ? context.capabilities.filter(Boolean) : undefined,
    relativePath: context.relative_path || undefined,
  }
}

export function bundleInfoFromNode(node: Pick<FileNode, 'path' | 'name' | 'kind' | 'metadata' | 'source' | 'is_dir' | 'bundle_context'>): BundleInfo | null {
  if (!node.is_dir) return null
  const fromContext = bundleInfoFromContext(node.bundle_context)
  if (fromContext) return fromContext
  const kind = bundleKindValue(node)
  if (!kind) return null
  return {
    kind,
    name: metadataValue(node.metadata, 'bundle_name') || metadataValue(node.metadata, 'name') || node.name,
    path: normalizeHubPath(node.path),
    description: metadataDescription(node) || undefined,
    whenToUse: metadataValue(node.metadata, 'when_to_use') || undefined,
    status: metadataValue(node.metadata, 'status') || undefined,
    readOnly: node.metadata?.read_only === true,
    source: fileNodeSource(node),
    primaryPath: metadataValue(node.metadata, 'bundle_primary_path') || undefined,
    logPath: metadataValue(node.metadata, 'bundle_log_path') || undefined,
    capabilities: Array.isArray(node.metadata?.bundle_capabilities)
      ? (node.metadata?.bundle_capabilities as string[]).filter(Boolean)
      : undefined,
  }
}

export function isBundleDirectory(node: Pick<FileNode, 'path' | 'name' | 'kind' | 'metadata' | 'source' | 'is_dir' | 'bundle_context'>) {
  return Boolean(bundleInfoFromNode(node))
}

export function bundleDetailMode(currentPath: string, items: FileNode[]) {
  if (currentPath.startsWith('/projects/') && currentPath.split('/').filter(Boolean).length === 2) return true
  if (currentPath.startsWith('/skills/') && items.some((item) => item.name === 'SKILL.md')) return true
  return false
}

export function bundleDescription(bundle?: BundleInfo | null) {
  if (!bundle) return ''
  return normalizeSkillText(bundle.description) || normalizeSkillText(bundle.whenToUse)
}

export function bundleKindLabel(kind: BundleKind, locale: AppLocale) {
  switch (kind) {
    case 'project':
      return text(locale, '项目 Bundle', 'Project Bundle')
    case 'conversation':
      return text(locale, '会话 Bundle', 'Conversation Bundle')
    case 'skill':
    default:
      return text(locale, '技能 Bundle', 'Skill Bundle')
  }
}

function bundleFooterEnd(bundle: BundleInfo, node: FileNode, locale: AppLocale) {
  if (bundle.kind === 'skill') {
    return bundle.readOnly ? text(locale, '只读', 'Read-only') : text(locale, '可编辑', 'Editable')
  }
  if (bundle.kind === 'conversation') {
    return formatDateTime(node.updated_at || node.created_at, locale)
  }
  switch ((bundle.status || '').toLowerCase()) {
    case 'archived':
      return text(locale, '已归档', 'Archived')
    case 'paused':
      return text(locale, '已暂停', 'Paused')
    case 'active':
      return text(locale, '进行中', 'Active')
    default:
      return formatDateTime(node.updated_at || node.created_at, locale)
  }
}

export function bundleStatusLabel(bundle: BundleInfo, locale: AppLocale) {
  if (bundle.kind === 'skill') {
    return bundle.readOnly ? text(locale, '只读', 'Read-only') : text(locale, '可编辑', 'Editable')
  }
  if (bundle.kind === 'conversation') {
    return text(locale, '已归档', 'Archived')
  }
  switch ((bundle.status || '').toLowerCase()) {
    case 'archived':
      return text(locale, '已归档', 'Archived')
    case 'paused':
      return text(locale, '已暂停', 'Paused')
    case 'active':
      return text(locale, '进行中', 'Active')
    default:
      return text(locale, '未知', 'Unknown')
  }
}

export function bundleCapabilityLabel(capability: string, locale: AppLocale = 'en') {
  switch ((capability || '').trim().toLowerCase()) {
    case 'context':
      return 'Context'
    case 'logs':
      return text(locale, '日志', 'Logs')
    case 'artifacts':
      return text(locale, '产物', 'Artifacts')
    case 'instructions':
      return text(locale, '说明', 'Instructions')
    default:
      return capability
        .split(/[_-]+/)
        .filter(Boolean)
        .map((part) => part.slice(0, 1).toUpperCase() + part.slice(1))
        .join(' ')
    }
}

function fileTileDescription(node: FileNode) {
  if (!node.is_dir) return ''
  return bundleDescription(bundleInfoFromNode(node)) || metadataDescription(node)
}

function fileTileName(node: FileNode, variant: FileTileVariant) {
  if (variant === 'memory') {
    return displayNameFromPath(node.path.replace(/^\/memory\//, ''))
  }
  const bundle = bundleInfoFromNode(node)
  if (bundle) return bundle.name
  return node.name
}

function fileTileFooterEnd(node: FileNode, locale: AppLocale) {
  return formatDateTime(node.updated_at || node.created_at, locale)
}

export function buildFileTileModel({
  node,
  variant,
  currentLabel,
  bundleLabel,
  locale = 'en',
}: BuildFileTileModelOptions): FileTileModel {
  const bundle = bundleInfoFromNode(node)
  const bundleCard = variant === 'browser' && Boolean(bundle)
  const resolvedSource = bundle?.source || fileNodeSource(node)

  switch (variant) {
    case 'recent':
      return {
        node,
        path: node.path,
        source: resolvedSource,
        footerStart: fileNamespaceLabel(node.path, locale),
        footerEnd: fileTileFooterEnd(node, locale),
      }
    case 'memory':
      return {
        node: {
          ...node,
          name: fileTileName(node, variant),
        },
        source: resolvedSource,
        subtitle: fileTileFooterEnd(node, locale),
        description: summarizeNodeContent(node, 220, locale),
        path: node.path,
        footerStart: 'memory',
        footerEnd: node.kind || text(locale, '条目', 'Entry'),
      }
    case 'search':
      return {
        node,
        path: node.path,
        source: resolvedSource,
        footerStart: fileNamespaceLabel(node.path, locale),
        footerEnd: fileTileFooterEnd(node, locale),
      }
    case 'bundle-entry':
      return {
        node,
        source: resolvedSource,
        description: fileTileDescription(node) || undefined,
        footerStart: bundleLabel || 'Bundle',
        footerEnd: fileTileFooterEnd(node, locale),
      }
    case 'browser':
    default:
      return {
        node: {
          ...node,
          name: fileTileName(node, variant),
        },
        source: resolvedSource,
        description: fileTileDescription(node) || undefined,
        footerStart: bundleCard && bundle ? bundleKindLabel(bundle.kind, locale) : (currentLabel || text(locale, '根目录', 'Root')),
        footerEnd: bundleCard && bundle
          ? bundleFooterEnd(bundle, node, locale)
          : fileTileFooterEnd(node, locale),
      }
  }
}

export function buildSkillBundleTileModel(skill: SkillSummary, locale: AppLocale = 'en'): FileTileModel {
  const bundlePath = normalizeHubPath(skill.bundle_path || skillBundlePathFromSkillPath(skill.path))
  return {
    node: {
      path: bundlePath,
      name: skill.name,
      is_dir: true,
    },
    source: skillSource(skill),
    description: skillSummaryDescription(skill) || text(locale, '这个 bundle 还没有写描述。', 'This bundle does not have a description yet.'),
    footerStart: text(locale, '技能', 'Skills'),
    footerEnd: skill.read_only ? text(locale, '只读', 'Read-only') : text(locale, '可编辑', 'Editable'),
  }
}

export function isVisibleFileEntry(node: FileNode) {
  return hasVisibleContent(node)
}

export function isOrdinaryFileEntry(node: FileNode) {
  return hasVisibleContent(node) && ORDINARY_FILE_EXCLUDED_PREFIXES.every((prefix) => !node.path.startsWith(prefix))
}

export function isProfileEntry(node: FileNode) {
  return hasVisibleContent(node) && node.path.startsWith('/memory/profile/')
}

export function isProfilePreviewEntry(node: FileNode) {
  return isProfileEntry(node) && !node.path.endsWith('/display_name.md')
}

export function isMemoryEntry(node: FileNode) {
  return hasVisibleContent(node) && node.path.startsWith('/memory/') && !node.path.startsWith('/memory/profile/')
}

export function isSkillDocument(node: FileNode) {
  return hasVisibleContent(node) && node.path.startsWith('/skills/') && node.path.endsWith('/SKILL.md')
}

export function profileLabelFromPath(path: string, locale: AppLocale = 'en') {
  const key = stripMarkdownSuffix(path.split('/').pop() || path)
  if (locale === 'zh-CN') {
    return PROFILE_LABELS[key] || key.replace(/[_-]+/g, ' ')
  }
  const englishLabels: Record<string, string> = {
    preferences: 'Preferences',
    relationships: 'Relationships',
    principles: 'Principles',
  }
  return englishLabels[key] || key.replace(/[_-]+/g, ' ')
}

export function displayNameFromPath(path: string) {
  const normalized = stripMarkdownSuffix(path).replace(/^\/+/, '')
  if (!normalized) return '/'
  return normalized
}

export function encodeHubRoutePath(path: string) {
  return encodeURIComponent(path.replace(/^\/+/, ''))
}

export function dataFileEditorRoute(path: string) {
  return `/data/files/edit/${encodeHubRoutePath(path)}`
}

export function dataFileBrowseRoute(path: string) {
  const normalized = path.replace(/^\/+/, '')
  return normalized ? `/data/files/${encodeHubRoutePath(path)}` : '/data/files'
}

export function normalizeBundleRelativeDir(value?: string | null) {
  return (value || '')
    .trim()
    .replace(/^\/+|\/+$/g, '')
    .split('/')
    .filter(Boolean)
    .join('/')
}

export function bundleBrowsePath(bundlePath: string, relativeDir?: string | null) {
  const root = normalizeHubPath(bundlePath).replace(/\/+$/, '')
  const relative = normalizeBundleRelativeDir(relativeDir)
  return relative ? normalizeHubPath(`${root}/${relative}`) : root
}

export function bundleRelativeDirFromPath(bundlePath: string, targetPath: string) {
  const root = normalizeHubPath(bundlePath).replace(/\/+$/, '')
  const target = normalizeHubPath(targetPath).replace(/\/+$/, '')
  if (target === root) return ''
  const prefix = `${root}/`
  return target.startsWith(prefix) ? target.slice(prefix.length) : ''
}

export function dataSkillBundleRoute(bundleKey: string, relativeDir?: string | null) {
  const base = `/data/skills/${encodeURIComponent(bundleKey)}`
  const relative = normalizeBundleRelativeDir(relativeDir)
  return relative ? `${base}?dir=${encodeURIComponent(relative)}` : base
}

export function dataProjectBundleRoute(projectName: string, relativeDir?: string | null) {
  const base = `/data/projects/${encodeURIComponent(projectName)}`
  const relative = normalizeBundleRelativeDir(relativeDir)
  return relative ? `${base}?dir=${encodeURIComponent(relative)}` : base
}

export function conversationBundleKeyFromPath(path: string) {
  return normalizeHubPath(path).replace(/^\/conversations\/?/, '')
}

export function dataConversationBundleRoute(bundlePath: string, relativeDir?: string | null) {
  const relativeBundlePath = conversationBundleKeyFromPath(bundlePath)
  const base = `/data/conversations/${encodeHubRoutePath(relativeBundlePath)}`
  const relative = normalizeBundleRelativeDir(relativeDir)
  return relative ? `${base}?dir=${encodeURIComponent(relative)}` : base
}

export function fileNamespaceLabel(path: string, locale: AppLocale = 'en') {
  switch (topLevelSegment(path)) {
    case 'conversations':
      return text(locale, '会话', 'Conversations')
    case 'projects':
      return text(locale, '项目', 'Projects')
    case 'skills':
      return text(locale, '技能', 'Skills')
    case 'memory':
      return path.startsWith('/memory/profile/')
        ? text(locale, '我的资料', 'My Profile')
        : 'Memory'
    case 'roles':
      return 'Roles'
    case 'inbox':
      return 'Inbox'
    default:
      return text(locale, '根文件', 'Root Files')
  }
}
