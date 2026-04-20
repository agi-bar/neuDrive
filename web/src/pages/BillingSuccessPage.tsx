import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type BillingStatus } from '../api'
import { useI18n } from '../i18n'
import { formatBillingStorage, resolvePlan } from './BillingShared'

export default function BillingSuccessPage() {
  const { locale, tx } = useI18n()
  const [status, setStatus] = useState<BillingStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      setLoading(true)
      setError('')
      try {
        const nextStatus = await api.getBillingStatus()
        if (!cancelled) {
          setStatus(nextStatus)
        }
      } catch (err: any) {
        if (!cancelled) {
          setError(err?.message || tx('刷新套餐状态失败', 'Failed to refresh billing status'))
        }
      } finally {
        if (!cancelled) {
          setLoading(false)
        }
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [tx])

  const currentPlan = resolvePlan(status?.plans || [], status?.current_plan || 'free')

  if (loading) {
    return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>
  }

  return (
    <div className="page materials-page">
      <div className="billing-success-card">
        <div className="status-banner">
          <span className="status-icon status-ok">&#10003;</span>
          <span className="status-text">{tx('升级成功', 'Upgrade confirmed')}</span>
        </div>

        <h2>{tx('谢谢，你的套餐状态已经刷新。', 'Thanks, your billing status has been refreshed.')}</h2>
        <p className="page-subtitle">
          {tx(
            '如果 Stripe 已经确认付款，你的可用空间会按新套餐显示。',
            'Once Stripe confirms the payment, your storage allowance will reflect the new plan.',
          )}
        </p>

        {error && <div className="alert alert-warn">{error}</div>}

        {status && (
          <div className="sync-login-summary">
            <div className="sync-login-summary-row">
              <span>{tx('当前套餐', 'Current plan')}</span>
              <strong>{currentPlan?.name || status.current_plan}</strong>
            </div>
            <div className="sync-login-summary-row">
              <span>{tx('可用空间', 'Storage limit')}</span>
              <strong>{formatBillingStorage(status.limit_bytes, locale)}</strong>
            </div>
          </div>
        )}

        <div className="billing-actions">
          <Link to="/billing" className="btn btn-primary">
            {tx('查看 Billing', 'Open billing')}
          </Link>
          <Link to="/" className="btn">
            {tx('回到概览', 'Back to overview')}
          </Link>
        </div>
      </div>
    </div>
  )
}
