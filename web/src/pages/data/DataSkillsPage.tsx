import { useEffect, useState } from 'react'
import { api, type FileNode } from '../../api'
import { formatDateTime, isSkillDocument, sortNodesByRecent, summarizeNodeContent } from './DataShared'

interface SkillItem {
  id: string
  name: string
  path: string
  description: string
  source: string
  whenToUse?: string
  tags: string[]
  updatedAt?: string
}

function formatSkillName(path: string) {
  return path
    .replace(/^\/skills\//, '')
    .replace(/\/SKILL\.md$/i, '')
    .split('/')
    .join(' / ')
}

function buildSkills(entries: FileNode[]): SkillItem[] {
  return sortNodesByRecent(entries.filter(isSkillDocument)).map((entry) => ({
    id: entry.path,
    name: String(entry.metadata?.name || formatSkillName(entry.path)),
    path: entry.path,
    description: String(entry.metadata?.description || summarizeNodeContent(entry, 180)),
    source: String(entry.metadata?.source || 'user'),
    whenToUse: entry.metadata?.when_to_use ? String(entry.metadata.when_to_use) : '',
    tags: Array.isArray(entry.metadata?.tags) ? entry.metadata.tags.map((tag) => String(tag)) : [],
    updatedAt: entry.updated_at || entry.created_at,
  }))
}

export default function DataSkillsPage() {
  const [skills, setSkills] = useState<SkillItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    const load = async () => {
      try {
        const snapshot = await api.getTreeSnapshot('/skills')
        setSkills(buildSkills(snapshot.entries))
      } catch (err: any) {
        setError(err.message || '加载技能失败')
      } finally {
        setLoading(false)
      }
    }

    load()
  }, [])

  if (loading) {
    return <div className="page-loading">加载中...</div>
  }

  return (
    <div className="page">
      <div className="page-header page-header-stack">
        <div>
          <h2>技能</h2>
          <p className="page-subtitle">这里显示 `/skills` 下可直接使用的技能定义，包含系统内置技能和你自己的技能文件。</p>
        </div>
      </div>

      {error && <div className="alert alert-warn">{error}</div>}

      {skills.length === 0 ? (
        <div className="empty-state">
          <p>还没有技能内容</p>
          <p className="empty-hint">导入或创建技能后，会在这里看到对应条目。</p>
        </div>
      ) : (
        <div className="data-record-list">
          {skills.map((skill) => (
            <div key={skill.id} className="card data-record-item">
              <div className="data-record-head">
                <div className="data-record-title">{skill.name}</div>
                <div className="data-record-meta">{formatDateTime(skill.updatedAt)}</div>
              </div>
              <div className="data-record-path">{skill.path}</div>
              <div className="data-record-preview">{skill.description}</div>
              {skill.whenToUse && (
                <div className="data-record-secondary">{skill.whenToUse}</div>
              )}
              <div className="data-inline-list">
                <span className="dashboard-inline-chip">{skill.source}</span>
                {skill.tags.slice(0, 4).map((tag) => (
                  <span key={tag} className="dashboard-inline-chip">{tag}</span>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
