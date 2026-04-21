import { useEffect, useMemo, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { api, type BillingStatus, type PaidBillingPlanCode } from '../api'
import { useI18n } from '../i18n'
import { billingReasonMessage, formatBillingPrice, formatBillingStorage, resolvePlan } from './BillingShared'

export default function BillingPage() {
  const { locale, tx } = useI18n()
  const location = useLocation()
  const [status, setStatus] = useState<BillingStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState<PaidBillingPlanCode | 'portal' | ''>('')
  const [error, setError] = useState('')

  const reason = useMemo(() => new URLSearchParams(location.search).get('reason'), [location.search])
  const bannerMessage = useMemo(() => billingReasonMessage(reason, tx), [reason, tx])

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
          setError(err?.message || tx('加载 Billing 状态失败', 'Failed to load billing status'))
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
  const usagePercent = status && status.limit_bytes > 0
    ? Math.min(100, Math.round((status.used_bytes / status.limit_bytes) * 100))
    : 0

  const handleCheckout = async (planCode: PaidBillingPlanCode) => {
    if (!status?.can_checkout || busy) return
    setBusy(planCode)
    setError('')
    try {
      const response = await api.createBillingCheckout(planCode)
      window.location.assign(response.checkout_url)
    } catch (err: any) {
      setError(err?.message || tx('创建支付链接失败', 'Failed to create checkout session'))
      setBusy('')
    }
  }

  const handlePortal = async () => {
    if (!status?.can_manage_portal || busy) return
    setBusy('portal')
    setError('')
    try {
      const response = await api.createBillingPortal()
      window.location.assign(response.portal_url)
    } catch (err: any) {
      setError(err?.message || tx('打开订阅管理失败', 'Failed to open billing portal'))
      setBusy('')
    }
  }

  if (loading) {
    return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>
  }

  return (
    <div className="page materials-page">
      <div className="page-header page-header-stack">
        <div>
          <h2>{tx('Billing', 'Billing')}</h2>
          <p className="page-subtitle">
            {tx(
              '查看当前套餐、已用空间和订阅入口。官方托管的升级和续费都会从这里进入。',
              'Review your plan, storage usage, and subscription actions from one place.',
            )}
          </p>
        </div>
      </div>

      {bannerMessage && <div className="alert alert-warn">{bannerMessage}</div>}
      {error && <div className="alert alert-warn">{error}</div>}

      {status && (
        <>
          <div className="billing-summary-card">
            <div className="billing-summary-row">
              <div>
                <div className="billing-kicker">{tx('当前套餐', 'Current plan')}</div>
                <div className="billing-plan-name">{currentPlan?.name || status.current_plan}</div>
                <div className="billing-plan-meta">
                  {formatBillingPrice(currentPlan, locale)} · {formatBillingStorage(status.limit_bytes, locale)}
                </div>
              </div>
              <div className={`billing-status-chip ${status.account_read_only ? 'is-warn' : ''}`}>
                {status.account_read_only
                  ? tx('只读', 'Read-only')
                  : status.entitlement_status === 'grace'
                    ? tx('宽限期', 'Grace period')
                    : tx('正常', 'Active')}
              </div>
            </div>

            <div className="billing-usage-copy">
              {tx('已用空间', 'Storage used')}: {formatBillingStorage(status.used_bytes, locale)} / {formatBillingStorage(status.limit_bytes, locale)}
            </div>
            <div className="billing-meter">
              <div className="billing-meter-fill" style={{ width: `${usagePercent}%` }} />
            </div>
            {status.usage_measured_at && (
              <div className="data-record-secondary">
                {tx('最近统计', 'Last measured')}: {new Date(status.usage_measured_at).toLocaleString(locale === 'zh-CN' ? 'zh-CN' : 'en-US')}
              </div>
            )}

            <div className="billing-actions">
              {status.can_manage_portal && (
                <button className="btn btn-primary" onClick={() => { void handlePortal() }} disabled={busy !== ''}>
                  {busy === 'portal' ? tx('打开中...', 'Opening...') : tx('管理订阅', 'Manage billing')}
                </button>
              )}
              <Link to="/" className="btn">
                {tx('返回概览', 'Back to overview')}
              </Link>
            </div>
          </div>

          <div className="billing-plan-grid">
            {status.plans.map((plan) => {
              const isCurrent = plan.code === status.current_plan
              const checkoutPlanCode = plan.code === 'pro_yearly' ? 'pro_yearly' : 'pro_monthly'
              return (
                <div key={plan.code} className={`billing-plan-card${isCurrent ? ' is-current' : ''}`}>
                  <div className="billing-plan-card-head">
                    <div>
                      <h3 className="card-title">{plan.name}</h3>
                      <div className="billing-plan-price">{formatBillingPrice(plan, locale)}</div>
                    </div>
                    {isCurrent && <span className="billing-status-chip">{tx('当前', 'Current')}</span>}
                  </div>
                  <p className="billing-plan-copy">
                    {tx('存储空间', 'Storage')}: {formatBillingStorage(plan.storage_limit_bytes, locale)}
                  </p>
                  {status.can_checkout && plan.code !== 'free' && (
                    <div className="billing-actions">
                      <button
                        className="btn btn-primary"
                        onClick={() => { void handleCheckout(checkoutPlanCode) }}
                        disabled={busy !== ''}
                      >
                        {busy === plan.code
                          ? tx('跳转中...', 'Redirecting...')
                          : plan.interval === 'year'
                            ? tx('订阅年付', 'Subscribe Yearly')
                            : tx('订阅月付', 'Subscribe Monthly')}
                      </button>
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        </>
      )}
    </div>
  )
}
