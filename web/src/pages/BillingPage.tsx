import { useEffect, useMemo, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { api, type BillingStatus, type PaidBillingPlanCode } from '../api'
import { useI18n } from '../i18n'
import { billingReasonMessage, formatBillingPrice, formatBillingStorage, resolvePlan } from './BillingShared'
import { formatDateTime } from './data/DataShared'

export default function BillingPage() {
  const { locale, tx } = useI18n()
  const location = useLocation()
  const [status, setStatus] = useState<BillingStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState<PaidBillingPlanCode | 'portal' | 'redeem' | ''>('')
  const [error, setError] = useState('')
  const [redeemCode, setRedeemCode] = useState('')
  const [redeemFeedback, setRedeemFeedback] = useState('')
  const [showCoupon, setShowCoupon] = useState(false)
  const reason = useMemo(() => new URLSearchParams(location.search).get('reason'), [location.search])
  const bannerMessage = useMemo(() => billingReasonMessage(reason, tx), [reason, tx])

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      setStatus(await api.getBillingStatus())
    } catch (err: any) {
      setError(err?.message || tx('加载 Billing 状态失败', 'Failed to load billing status'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const checkout = async (plan: PaidBillingPlanCode) => {
    if (!status?.can_checkout) return
    setBusy(plan)
    setError('')
    try {
      const response = await api.createBillingCheckout(plan)
      window.location.assign(response.checkout_url)
    } catch (err: any) {
      setError(err?.message || tx('创建支付链接失败', 'Failed to create checkout session'))
      setBusy('')
    }
  }

  const openPortal = async () => {
    if (!status?.can_manage_portal) return
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

  const redeem = async () => {
    const code = redeemCode.trim()
    if (!code) return
    setBusy('redeem')
    setRedeemFeedback('')
    try {
      const response = await api.redeemBillingCode(code)
      setStatus(response.status)
      setRedeemCode('')
      setRedeemFeedback(tx('兑换成功。', 'Promo code redeemed.'))
    } catch (err: any) {
      setRedeemFeedback(err?.message || tx('兑换失败。', 'Redeem failed.'))
    } finally {
      setBusy('')
    }
  }

  if (loading) return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>

  const current = resolvePlan(status?.plans || [], status?.current_plan || 'free')
  const isPaid = status && status.current_plan !== 'free'
  const isPromo = status?.access_source === 'promo_code'
  const yearly = status?.plans.find((plan) => plan.code === 'pro_yearly')
  const monthly = status?.plans.find((plan) => plan.code === 'pro_monthly')
  const usagePercent = status && status.limit_bytes > 0 ? Math.min(100, Math.round((status.used_bytes / status.limit_bytes) * 100)) : 0

  return (
    <div className="page billing-page-new">
      {bannerMessage && <div className="alert alert-warn">{bannerMessage}</div>}
      {error && <div className="alert alert-warn">{error}</div>}

      {!status && (
        <div className="empty-action-state">
          <p>{tx('支付网关在当前部署不可用。开源核心功能仍可继续使用。', 'Billing gateway is unavailable in this deployment. Core features remain available.')}</p>
          <Link to="/onboarding" className="btn btn-primary">{tx('进入接入向导', 'Open onboarding')}</Link>
        </div>
      )}

      {status && !isPaid && !isPromo && (
        <>
          <div className="page-header compact-header">
            <div>
              <h2>{tx('Upgrade to Pro', 'Upgrade to Pro')}</h2>
              <p className="page-subtitle">Free plan · 10 MiB storage · Manual sync</p>
            </div>
          </div>

          <section className="billing-upgrade-layout">
            <article className="plan-option-card featured">
              <span className="recommended-chip">Recommended</span>
              <h3>Pro Yearly</h3>
              <div className="pricing-price">{formatBillingPrice(yearly, locale)}</div>
              <p>Save 50% with yearly</p>
              <ul>
                <li>1 GiB storage</li>
                <li>Auto sync</li>
                <li>More storage</li>
                <li>Git backup</li>
                <li>Priority import</li>
              </ul>
              <button className="btn btn-primary btn-block" disabled={busy !== '' || !status.can_checkout} onClick={() => { void checkout('pro_yearly') }}>
                {busy === 'pro_yearly' ? tx('跳转中...', 'Redirecting...') : tx('年付升级', 'Upgrade yearly')}
              </button>
            </article>

            <article className="plan-option-card">
              <h3>Pro Monthly</h3>
              <div className="pricing-price">{formatBillingPrice(monthly, locale)}</div>
              <p>{tx('按月使用 Pro。', 'Use Pro month to month.')}</p>
              <button className="btn btn-outline btn-block" disabled={busy !== '' || !status.can_checkout} onClick={() => { void checkout('pro_monthly') }}>
                {busy === 'pro_monthly' ? tx('跳转中...', 'Redirecting...') : tx('月付', 'Pay monthly')}
              </button>
            </article>
          </section>
        </>
      )}

      {status && (isPaid || isPromo) && (
        <>
          <div className="page-header compact-header">
            <div>
              <h2>{tx('Plan & Billing', 'Plan & Billing')}</h2>
              <p className="page-subtitle">{tx('管理当前套餐、续费和账单入口。', 'Manage your current plan, renewal, and billing access.')}</p>
            </div>
            <div className="page-actions">
              {status.can_manage_portal && (
                <button className="btn btn-primary" disabled={busy !== ''} onClick={() => { void openPortal() }}>
                  {busy === 'portal' ? tx('打开中...', 'Opening...') : tx('管理支付', 'Manage payment')}
                </button>
              )}
            </div>
          </div>

          <section className="billing-pro-summary">
            <div className="billing-pro-row">
              <span>{tx('Current plan', 'Current plan')}</span>
              <strong>{isPromo ? 'Pro Promo' : current?.name || status.current_plan}</strong>
            </div>
            <div className="billing-pro-row">
              <span>{tx('Renewal', 'Renewal')}</span>
              <strong>{status.current_period_end ? formatDateTime(status.current_period_end, locale) : isPromo && status.promo?.ends_at ? formatDateTime(status.promo.ends_at, locale) : '-'}</strong>
            </div>
            <div className="billing-pro-row">
              <span>{tx('Storage', 'Storage')}</span>
              <strong>{formatBillingStorage(status.used_bytes, locale)} / {formatBillingStorage(status.limit_bytes, locale)}</strong>
            </div>
            <div className="billing-meter">
              <div className="billing-meter-fill" style={{ width: `${usagePercent}%` }} />
            </div>
            <div className="page-actions">
              {status.can_manage_portal && <button className="btn btn-outline" disabled={busy !== ''} onClick={() => { void openPortal() }}>{tx('Download invoices', 'Download invoices')}</button>}
              <a className="btn btn-outline" href="mailto:support@neudrive.ai">{tx('需要更多空间？联系我们', 'Need more storage? Contact us')}</a>
            </div>
          </section>
        </>
      )}

      {status && (
        <section className="coupon-section">
          <button className="btn-text" onClick={() => setShowCoupon((value) => !value)}>{tx('Have a coupon?', 'Have a coupon?')}</button>
          {showCoupon && (
            <div className="coupon-form">
              <input className="input" value={redeemCode} placeholder={tx('输入兑换码', 'Enter promo code')} onChange={(event) => setRedeemCode(event.target.value)} />
              <button className="btn btn-outline" disabled={busy !== '' || !redeemCode.trim()} onClick={() => { void redeem() }}>{busy === 'redeem' ? tx('兑换中...', 'Redeeming...') : tx('兑换', 'Redeem')}</button>
              {redeemFeedback && <div className="alert alert-warn">{redeemFeedback}</div>}
            </div>
          )}
        </section>
      )}
    </div>
  )
}
