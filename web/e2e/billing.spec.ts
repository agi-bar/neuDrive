import { expect, test, type Page } from '@playwright/test'
import { setupUser } from './helpers'

const freeBillingStatus = {
  current_plan: 'free',
  entitlement_status: 'active',
  used_bytes: 3 * 1024 * 1024,
  limit_bytes: 10 * 1024 * 1024,
  usage_measured_at: '2026-04-19T12:00:00Z',
  account_read_only: false,
  plans: [
    { code: 'free', name: 'Free', currency: 'usd', price_cents: 0, interval: 'month', storage_limit_bytes: 10 * 1024 * 1024 },
    { code: 'pro', name: 'Pro', currency: 'usd', price_cents: 3000, interval: 'month', storage_limit_bytes: 1024 * 1024 * 1024 },
  ],
  can_checkout: true,
  can_manage_portal: false,
}

const proBillingStatus = {
  ...freeBillingStatus,
  current_plan: 'pro',
  used_bytes: 32 * 1024 * 1024,
  limit_bytes: 1024 * 1024 * 1024,
  can_checkout: false,
  can_manage_portal: true,
}

async function enableBillingUI(page: Page, status: typeof freeBillingStatus) {
  await page.route('**/api/config', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        ok: true,
        data: {
          billing_enabled: true,
          system_settings_enabled: false,
        },
      }),
    })
  })

  await page.route('**/api/billing/status', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(status),
    })
  })
}

test.describe('Billing UI', () => {
  test('billing stays hidden when feature flag is off', async ({ page, request }) => {
    await setupUser(page, request)

    await expect(page.getByRole('link', { name: 'Billing' })).toHaveCount(0)
    await page.goto('/billing')
    await expect(page).toHaveURL(/\/$/)
  })

  test('free users can open billing and start checkout', async ({ page, request }) => {
    await enableBillingUI(page, freeBillingStatus)
    await page.route('**/api/billing/checkout', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ ok: true, checkout_url: '/checkout/mock' }),
      })
    })
    await page.route('**/checkout/mock', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'text/html',
        body: '<!doctype html><html><body>mock checkout</body></html>',
      })
    })

    await setupUser(page, request)

    await expect(page.getByRole('link', { name: 'Billing' })).toBeVisible()
    await page.getByRole('link', { name: 'Billing' }).click()
    await expect(page).toHaveURL(/\/billing$/)
    await expect(page.getByText('存储空间: 10 MiB')).toBeVisible()
    await expect(page.getByRole('button', { name: '升级到 Pro' })).toBeVisible()

    await page.getByRole('button', { name: '升级到 Pro' }).click()
    await expect(page).toHaveURL(/\/checkout\/mock$/)
  })

  test('paid users can open the billing portal', async ({ page, request }) => {
    await enableBillingUI(page, proBillingStatus)
    await page.route('**/api/billing/portal', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ ok: true, portal_url: '/portal/mock' }),
      })
    })
    await page.route('**/portal/mock', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'text/html',
        body: '<!doctype html><html><body>mock portal</body></html>',
      })
    })

    await setupUser(page, request)
    await page.goto('/billing')

    await expect(page.getByRole('button', { name: '管理订阅' })).toBeVisible()
    await page.getByRole('button', { name: '管理订阅' }).click()
    await expect(page).toHaveURL(/\/portal\/mock$/)
  })

  test('quota errors redirect the app into billing', async ({ page, request }) => {
    await enableBillingUI(page, freeBillingStatus)
    await page.route('**/api/tree/**', async (route) => {
      if (route.request().method() !== 'PUT') {
        await route.continue()
        return
      }
      await route.fulfill({
        status: 403,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 'quota_exceeded',
          message: 'storage quota exceeded',
          plan: 'free',
          used_bytes: 10 * 1024 * 1024,
          limit_bytes: 10 * 1024 * 1024,
          upgrade_url: '/billing',
        }),
      })
    })

    await setupUser(page, request)
    await page.goto('/data/memory')
    await page.getByRole('button', { name: '新建 Memory' }).click()
    await page.getByLabel('文件名称').fill('overflow-note.md')
    await page.getByRole('button', { name: '创建' }).click()

    await expect(page).toHaveURL(/\/billing\?reason=quota_exceeded/)
    await expect(page.getByText('当前存储空间已经达到上限')).toBeVisible()
  })
})
