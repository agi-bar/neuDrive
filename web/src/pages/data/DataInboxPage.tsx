import { useEffect, useState } from 'react'
import { api, type InboxMessageRecord } from '../../api'
import { formatDateTime, summarizeText } from './DataShared'

type InboxStatus = 'incoming' | 'read' | 'archived'

const STATUS_LABELS: Record<InboxStatus, string> = {
  incoming: '待处理',
  read: '已读',
  archived: '已归档',
}

export default function DataInboxPage() {
  const [status, setStatus] = useState<InboxStatus>('incoming')
  const [messages, setMessages] = useState<InboxMessageRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    const load = async () => {
      setLoading(true)
      setError('')
      try {
        const data = await api.getInbox('assistant', status)
        setMessages([...data].sort((a, b) => {
          const aTime = new Date(a.created_at || 0).getTime()
          const bTime = new Date(b.created_at || 0).getTime()
          return bTime - aTime
        }))
      } catch (err: any) {
        setError(err.message || '加载 Inbox 失败')
      } finally {
        setLoading(false)
      }
    }

    load()
  }, [status])

  return (
    <div className="page">
      <div className="page-header page-header-stack">
        <div>
          <h2>Inbox</h2>
          <p className="page-subtitle">当前默认查看 `assistant` 角色的收件箱消息，并可切换待处理、已读和已归档状态。</p>
        </div>
      </div>

      <div className="setup-tabs" role="tablist" aria-label="Inbox 状态">
        {(['incoming', 'read', 'archived'] as InboxStatus[]).map((item) => (
          <button
            key={item}
            type="button"
            role="tab"
            className={`setup-tab ${status === item ? 'setup-tab-active' : ''}`}
            aria-selected={status === item}
            onClick={() => setStatus(item)}
          >
            {STATUS_LABELS[item]}
          </button>
        ))}
      </div>

      {error && <div className="alert alert-warn">{error}</div>}

      {loading ? (
        <div className="page-loading">加载中...</div>
      ) : messages.length === 0 ? (
        <div className="empty-state">
          <p>{STATUS_LABELS[status]}中没有消息</p>
          <p className="empty-hint">切换状态或等待新的 agent 消息进入收件箱。</p>
        </div>
      ) : (
        <div className="data-record-list">
          {messages.map((message) => (
            <div key={message.id} className="card data-record-item">
              <div className="data-record-head">
                <div className="data-record-title">{message.subject || '无主题消息'}</div>
                <div className="data-inline-list">
                  {message.priority && <span className="dashboard-inline-chip">{message.priority}</span>}
                  {message.action_required && <span className="dashboard-inline-chip">需处理</span>}
                </div>
              </div>
              <div className="data-record-secondary">来自：{message.from_address}</div>
              <div className="data-record-preview">{summarizeText(message.body, 240)}</div>
              <div className="data-record-meta">{formatDateTime(message.created_at)}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
