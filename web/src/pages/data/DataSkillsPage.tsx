import { useEffect, useState } from 'react'
import { api, type SkillSummary } from '../../api'

export default function DataSkillsPage() {
  const [skills, setSkills] = useState<SkillSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    const load = async () => {
      try {
        const data = await api.getSkills()
        setSkills(data)
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
            <div key={skill.path} className="card data-record-item">
              <div className="data-record-head">
                <div className="data-record-title">{skill.name}</div>
                <div className="data-record-meta">{skill.read_only ? '只读' : skill.source}</div>
              </div>
              <div className="data-record-path">{skill.path}</div>
              <div className="data-record-preview">{skill.description || '暂无描述'}</div>
              {skill.when_to_use && (
                <div className="data-record-secondary">{skill.when_to_use}</div>
              )}
              <div className="data-inline-list">
                <span className="dashboard-inline-chip">{skill.source}</span>
                {(skill.tags || []).slice(0, 4).map((tag) => (
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
