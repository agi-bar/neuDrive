import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, type BillingStatus, type PaidBillingPlanCode } from '../api'
import { useI18n } from '../i18n'
import { formatBillingStorage } from './BillingShared'

interface PlanGatePageProps {
  billingEnabled?: boolean
}

export default function PlanGatePage({ billingEnabled = false }: PlanGatePageProps) {
  const { locale, tx } = useI18n()
  const navigate = useNavigate()
  const [status, setStatus] = useState<BillingStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState<PaidBillingPlanCode | ''>('')
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      if (!billingEnabled) {
        setLoading(false)
        return
      }
      try {
        const next = await api.getBillingStatus()
        if (!cancelled) setStatus(next)
      } catch {
        if (!cancelled) setStatus(null)
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [billingEnabled])

  const startCheckout = async (plan: PaidBillingPlanCode) => {
    if (!status?.can_checkout) {
      navigate('/onboarding', { replace: true })
      return
    }
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

  const continueFree = () => {
    localStorage.removeItem('neudrive.postSignupIntent')
    localStorage.setItem('neudrive.planGateSeen', '1')
    navigate('/onboarding', { replace: true })
  }

  if (loading) {
    return <div className="page-loading">{tx('加载中...', 'Loading...')}</div>
  }

  const canCheckout = !!status?.can_checkout
  const freeLimit = status?.plans.find((plan) => plan.code === 'free')?.storage_limit_bytes || 10 * 1024 * 1024
  const proLimit = status?.plans.find((plan) => plan.code === 'pro_yearly')?.storage_limit_bytes || 1024 * 1024 * 1024

  return (
    <div className="page app-narrow-page">
      <section className="plan-gate">
        <div className="plan-gate-head">
          <p className="materials-kicker">Choose your plan</p>
          <h2>{tx('选择你想怎样使用 neuDrive。', 'Choose how you want to use neuDrive.')}</h2>
          <p>
            {tx(
              '如果你需要更多存储、自动同步和备份，推荐使用 Pro。',
              'Pro is recommended if you need more storage, auto sync, and backup.',
            )}
          </p>
          {!billingEnabled && (
            <div className="alert alert-warn">
              {tx('当前是开源核心部署，支付网关未启用。你可以直接进入接入向导。', 'This open-source core deployment does not have the hosted billing gateway enabled. You can continue to setup.')}
            </div>
          )}
          {billingEnabled && !canCheckout && (
            <div className="alert alert-warn">
              {tx('支付网关当前不可用或你的账户已拥有付费权益。', 'Billing gateway is unavailable or this account already has paid access.')}
            </div>
          )}
          {error && <div className="alert alert-warn">{error}</div>}
        </div>

        <div className="plan-gate-grid">
          <article className="plan-option-card featured">
            <span className="recommended-chip">Recommended</span>
            <h3>Pro Yearly</h3>
            <div className="pricing-price">$60 / year</div>
            <p>{tx('Save 50% · 约 $5/月', 'Save 50% · about $5/month')}</p>
            <ul>
              <li>{formatBillingStorage(proLimit, locale)} storage</li>
              <li>Auto sync</li>
              <li>Git backup</li>
              <li>Priority import</li>
            </ul>
            <button className="btn btn-primary btn-block" disabled={busy !== ''} onClick={() => { void startCheckout('pro_yearly') }}>
              {busy === 'pro_yearly' ? tx('跳转中...', 'Redirecting...') : tx('年付 Pro', 'Start Pro yearly')}
            </button>
          </article>

          <article className="plan-option-card">
            <h3>Pro Monthly</h3>
            <div className="pricing-price">$10 / month</div>
            <p>{tx('先按月验证工作流。', 'Validate the workflow month to month.')}</p>
            <button className="btn btn-outline btn-block" disabled={busy !== ''} onClick={() => { void startCheckout('pro_monthly') }}>
              {busy === 'pro_monthly' ? tx('跳转中...', 'Redirecting...') : tx('月付 Pro', 'Start monthly')}
            </button>
          </article>
        </div>

        <button className="plan-free-link" type="button" onClick={continueFree}>
          {tx('继续使用 Free', 'Continue with Free')} · {formatBillingStorage(freeLimit, locale)} storage
        </button>
        <p className="login-note"><Link to="/">{tx('稍后再选', 'Decide later')}</Link></p>
      </section>
    </div>
  )
}
