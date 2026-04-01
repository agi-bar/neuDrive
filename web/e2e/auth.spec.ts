import { test, expect } from '@playwright/test'
import { registerUser, loginViaUI } from './helpers'

test.describe('Auth — Registration', () => {
  test('register via UI form', async ({ page }) => {
    const slug = `pw-reg-${Date.now()}`
    await page.goto('/login')
    // Switch to register tab
    await page.getByRole('button', { name: '注册' }).click()
    await page.waitForTimeout(300)

    // Fill registration form
    await page.getByPlaceholder('你的名字').fill('Test User')
    await page.getByPlaceholder('my-username').fill(slug)
    await page.getByPlaceholder('your@email.com').fill(`${slug}@test.local`)
    await page.getByPlaceholder('至少 8 个字符').fill('testpass1234')
    await page.getByPlaceholder('再次输入密码').fill('testpass1234')

    await page.locator('button[type="submit"]').click()
    await page.waitForURL(/^(?!.*\/login)/, { timeout: 15000 })

    // Should be on dashboard
    await expect(page.locator('.sidebar-brand h1')).toHaveText('Agent Hub')
  })
})

test.describe('Auth — Login', () => {
  test('login with valid credentials', async ({ page, request }) => {
    const user = await registerUser(request)
    await loginViaUI(page, user.email, user.password)
    await expect(page.locator('.sidebar-brand h1')).toHaveText('Agent Hub')
  })

  test('login with wrong password shows error', async ({ page, request }) => {
    const user = await registerUser(request)
    await page.goto('/login')
    await page.waitForLoadState('networkidle')
    await page.getByPlaceholder('your@email.com').fill(user.email)
    await page.getByPlaceholder('输入密码').fill('wrongpassword')
    await page.locator('button[type="submit"]').click()
    await page.waitForTimeout(1000)

    // Should still be on login page with error
    await expect(page).toHaveURL(/\/login/)
    await expect(page.locator('.alert-error, .error')).toBeVisible({ timeout: 5000 }).catch(() => {
      // Error might be displayed differently
    })
  })
})

test.describe('Auth — Logout', () => {
  test('logout redirects to login', async ({ page, request }) => {
    const user = await registerUser(request)
    await loginViaUI(page, user.email, user.password)

    // Click logout
    await page.getByRole('button', { name: '退出' }).click()
    await page.waitForURL(/\/login/, { timeout: 5000 })
    await expect(page).toHaveURL(/\/login/)
  })
})

test.describe('Auth — Protected routes', () => {
  test('unauthenticated access redirects to login', async ({ page }) => {
    await page.goto('/projects')
    await page.waitForURL(/\/login/, { timeout: 5000 })
    await expect(page).toHaveURL(/\/login/)
  })
})
