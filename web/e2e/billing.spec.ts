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
    { code: 'pro_monthly', name: 'Pro Monthly', currency: 'usd', price_cents: 1000, interval: 'month', storage_limit_bytes: 1024 * 1024 * 1024 },
    { code: 'pro_yearly', name: 'Pro Yearly', currency: 'usd', price_cents: 6000, interval: 'year', storage_limit_bytes: 1024 * 1024 * 1024 },
  ],
  can_checkout: true,
  can_manage_portal: false,
}

const monthlyBillingStatus = {
  ...freeBillingStatus,
  current_plan: 'pro_monthly',
  used_bytes: 32 * 1024 * 1024,
  limit_bytes: 1024 * 1024 * 1024,
  can_checkout: false,
  can_manage_portal: true,
}

const yearlyBillingStatus = {
  ...freeBillingStatus,
  current_plan: 'pro_yearly',
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

  test('free users can start monthly checkout', async ({ page, request }) => {
    await enableBillingUI(page, freeBillingStatus)
    await page.route('**/api/billing/checkout', async (route) => {
      await expect(route.request().postDataJSON()).toEqual({ plan_code: 'pro_monthly' })
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ ok: true, checkout_url: '/checkout/monthly' }),
      })
    })
    await page.route('**/checkout/monthly', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'text/html',
        body: '<!doctype html><html><body>mock monthly checkout</body></html>',
      })
    })

    await setupUser(page, request)

    await expect(page.getByRole('link', { name: 'Billing' })).toBeVisible()
    await page.getByRole('link', { name: 'Billing' }).click()
    await expect(page).toHaveURL(/\/billing$/)
    await expect(page.getByText('Storage: 10 MiB')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Subscribe Monthly' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Subscribe Yearly' })).toBeVisible()

    await page.getByRole('button', { name: 'Subscribe Monthly' }).click()
    await expect(page).toHaveURL(/\/checkout\/monthly$/)
  })

  test('free users can start yearly checkout', async ({ page, request }) => {
    await enableBillingUI(page, freeBillingStatus)
    await page.route('**/api/billing/checkout', async (route) => {
      await expect(route.request().postDataJSON()).toEqual({ plan_code: 'pro_yearly' })
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ ok: true, checkout_url: '/checkout/yearly' }),
      })
    })
    await page.route('**/checkout/yearly', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'text/html',
        body: '<!doctype html><html><body>mock yearly checkout</body></html>',
      })
    })

    await setupUser(page, request)
    await page.goto('/billing')

    await page.getByRole('button', { name: 'Subscribe Yearly' }).click()
    await expect(page).toHaveURL(/\/checkout\/yearly$/)
  })

  test('paid monthly users can open the billing portal', async ({ page, request }) => {
    await enableBillingUI(page, monthlyBillingStatus)
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

    await expect(page.getByRole('heading', { name: 'Pro Monthly' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Manage billing' })).toBeVisible()
    await page.getByRole('button', { name: 'Manage billing' }).click()
    await expect(page).toHaveURL(/\/portal\/mock$/)
  })

  test('paid yearly users display the yearly plan', async ({ page, request }) => {
    await enableBillingUI(page, yearlyBillingStatus)
    await setupUser(page, request)
    await page.goto('/billing')

    await expect(page.getByRole('heading', { name: 'Pro Yearly' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Manage billing' })).toBeVisible()
  })

  test('billing success refreshes and shows the current yearly plan', async ({ page, request }) => {
    await enableBillingUI(page, yearlyBillingStatus)
    await setupUser(page, request)
    await page.goto('/billing/success')

    await expect(page.getByText('Upgrade confirmed')).toBeVisible()
    await expect(page.getByText('Pro Yearly')).toBeVisible()
    await expect(page.getByText('1.0 GiB')).toBeVisible()
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
    await page.getByRole('button', { name: 'New memory' }).click()
    await page.getByLabel('File name').fill('overflow-note.md')
    await page.getByRole('button', { name: 'Create' }).click()

    await expect(page).toHaveURL(/\/billing\?reason=quota_exceeded/)
    await expect(page.getByText('Your storage is full.')).toBeVisible()
  })
})
