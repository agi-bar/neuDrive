import { type FileNode, type SkillSummary } from '../../api'

const ORDINARY_FILE_EXCLUDED_PREFIXES = [
  '/projects/',
  '/skills/',
  '/memory/',
  '/devices/',
  '/roles/',
  '/inbox/',
]

const PROFILE_LABELS: Record<string, string> = {
  preferences: '个人偏好',
  relationships: '人际关系',
  principles: '行为准则',
}

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

export function normalizeHubPath(path: string) {
  const normalized = (path || '').trim()
  if (!normalized || normalized === '/') return '/'
  return `/${normalized.replace(/^\/+/, '').replace(/\/+$/, '')}`
}

export function formatDateTime(value?: string) {
  if (!value) return '-'
  try {
    return new Date(value).toLocaleString('zh-CN')
  } catch {
    return value
  }
}

export function summarizeText(value?: string, maxLength = 140) {
  if (!value) return '暂无内容'
  const normalized = value.replace(/\s+/g, ' ').trim()
  if (!normalized) return '暂无内容'
  if (normalized.length <= maxLength) return normalized
  return `${normalized.slice(0, maxLength).trimEnd()}...`
}

export function summarizeNodeContent(node: FileNode, maxLength = 140) {
  return summarizeText(node.content, maxLength)
}

export function recentTimestamp(node: FileNode) {
  return new Date(node.updated_at || node.created_at || 0).getTime()
}

export function sortNodesByRecent(entries: FileNode[]) {
  return [...entries].sort((a, b) => recentTimestamp(b) - recentTimestamp(a))
}

export type MaterialsSortKey = 'name' | 'updated_at'
export type MaterialsSortDir = 'asc' | 'desc'

export const MATERIALS_SORT_OPTIONS: Array<{ value: MaterialsSortKey; label: string }> = [
  { value: 'updated_at', label: '按时间' },
  { value: 'name', label: '按名称' },
]

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

export type FileTileModel = {
  node: Pick<FileNode, 'path' | 'name' | 'is_dir' | 'kind'>
  subtitle?: string
  description?: string
  path?: string
  footerStart?: string
  footerEnd?: string
}

export type FileTileVariant =
  | 'browser'
  | 'recent'
  | 'memory'
  | 'search'
  | 'skill-bundle-entry'

type BuildFileTileModelOptions = {
  node: FileNode
  variant: FileTileVariant
  currentLabel?: string
  bundleLabel?: string
  skillLookup?: Record<string, SkillSummary>
}

function metadataDescription(node: FileNode) {
  const value = typeof node.metadata?.description === 'string' ? node.metadata.description : ''
  return normalizeSkillText(value) || ''
}

function fileTileDescription(node: FileNode, skillLookup: Record<string, SkillSummary> = {}) {
  if (!node.is_dir) return ''
  return skillSummaryDescription(skillSummaryForPath(node.path, skillLookup)) || metadataDescription(node)
}

function fileTileName(node: FileNode, variant: FileTileVariant) {
  if (variant === 'memory') {
    return displayNameFromPath(node.path.replace(/^\/memory\//, ''))
  }
  return node.name
}

function fileTileFooterEnd(node: FileNode) {
  return formatDateTime(node.updated_at || node.created_at)
}

export function buildFileTileModel({
  node,
  variant,
  currentLabel,
  bundleLabel,
  skillLookup = {},
}: BuildFileTileModelOptions): FileTileModel {
  const skillSummary = skillSummaryForPath(node.path, skillLookup)
  const skillBundleCard = variant === 'browser' && node.is_dir && Boolean(skillSummary)

  switch (variant) {
    case 'recent':
      return {
        node,
        path: node.path,
        footerStart: fileNamespaceLabel(node.path),
        footerEnd: fileTileFooterEnd(node),
      }
    case 'memory':
      return {
        node: {
          ...node,
          name: fileTileName(node, variant),
        },
        subtitle: fileTileFooterEnd(node),
        description: summarizeNodeContent(node, 220),
        path: node.path,
        footerStart: 'memory',
        footerEnd: node.kind || 'entry',
      }
    case 'search':
      return {
        node,
        path: node.path,
        footerStart: fileNamespaceLabel(node.path),
        footerEnd: fileTileFooterEnd(node),
      }
    case 'skill-bundle-entry':
      return {
        node,
        description: fileTileDescription(node, skillLookup) || undefined,
        footerStart: bundleLabel || 'bundle',
        footerEnd: fileTileFooterEnd(node),
      }
    case 'browser':
    default:
      return {
        node,
        description: fileTileDescription(node, skillLookup) || undefined,
        footerStart: skillBundleCard ? 'skills' : (currentLabel || '根目录'),
        footerEnd: skillBundleCard ? (skillSummary?.read_only ? '只读' : '可编辑') : fileTileFooterEnd(node),
      }
  }
}

export function buildSkillBundleTileModel(skill: SkillSummary): FileTileModel {
  return {
    node: {
      path: skillBundlePathFromSkillPath(skill.path),
      name: skill.name,
      is_dir: true,
    },
    description: skillSummaryDescription(skill) || '这个 bundle 还没有写描述。',
    footerStart: 'skills',
    footerEnd: skill.read_only ? '只读' : '可编辑',
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

export function profileLabelFromPath(path: string) {
  const key = stripMarkdownSuffix(path.split('/').pop() || path)
  return PROFILE_LABELS[key] || key.replace(/[_-]+/g, ' ')
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

export function fileNamespaceLabel(path: string) {
  switch (topLevelSegment(path)) {
    case 'projects':
      return '项目'
    case 'skills':
      return '技能'
    case 'memory':
      return path.startsWith('/memory/profile/') ? '我的资料' : 'Memory'
    case 'devices':
      return '设备'
    case 'roles':
      return 'Roles'
    case 'inbox':
      return 'Inbox'
    default:
      return '根文件'
  }
}
