import { type FileNode } from '../../api'

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
