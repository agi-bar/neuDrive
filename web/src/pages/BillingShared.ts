import type { BillingPlan } from '../api'

type Locale = 'zh-CN' | 'en'
type TranslationFn = (zh: string, en: string) => string

export function formatBillingStorage(bytes: number, locale: Locale) {
  if (!Number.isFinite(bytes) || bytes <= 0) return `0 ${locale === 'zh-CN' ? 'B' : 'B'}`
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let value = bytes
  let unitIndex = 0
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex += 1
  }
  return `${value.toFixed(value >= 10 || unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`
}

export function formatBillingPrice(plan: BillingPlan | undefined, locale: Locale) {
  if (!plan) return locale === 'zh-CN' ? '未配置' : 'Unavailable'
  if (plan.price_cents <= 0) return locale === 'zh-CN' ? '免费' : 'Free'
  const currency = (plan.currency || 'usd').toUpperCase()
  const value = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency,
    maximumFractionDigits: 2,
  }).format(plan.price_cents / 100)
  const interval = plan.interval === 'month'
    ? (locale === 'zh-CN' ? '月' : 'month')
    : plan.interval === 'year'
      ? (locale === 'zh-CN' ? '年' : 'year')
    : plan.interval
  return `${value} / ${interval}`
}

export function resolvePlan(plans: BillingPlan[], code: string) {
  return plans.find((plan) => plan.code === code)
}

export function billingReasonMessage(reason: string | null, tx: TranslationFn) {
  switch (reason) {
    case 'quota_exceeded':
      return tx(
        '当前存储空间已经达到上限。升级套餐或删除部分内容后，再继续写入。',
        'Your storage is full. Upgrade or remove some content before writing again.',
      )
    case 'account_read_only':
      return tx(
        '当前账户已超出存储上限，暂时进入只读状态。升级套餐或清理内容后可恢复写入。',
        'This account is over its storage limit and is temporarily read-only. Upgrade or clean up content to write again.',
      )
    default:
      return ''
  }
}
