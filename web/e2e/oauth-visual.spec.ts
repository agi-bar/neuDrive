import { test, expect } from '@playwright/test'
import { registerUser } from './helpers'

const CF_URL = process.env.CF_URL || 'http://localhost:8080'

test.describe('OAuth Consent Page (SPA)', () => {
  test('logged-in user sees app info + Authorize button, no auto-submit', async ({ page, request }) => {
    const user = await registerUser(request)

    // Login via SPA
    await page.goto(CF_URL + '/login')
    await page.waitForLoadState('networkidle')
    await page.getByPlaceholder('your@email.com').fill(user.email)
    await page.getByPlaceholder('输入密码').fill(user.password)
    await page.locator('button[type="submit"]').click()
    await page.waitForURL(/^(?!.*\/login)/, { timeout: 15000 })

    // Visit OAuth authorize page
    const authorizeURL = `${CF_URL}/oauth/authorize?response_type=code&client_id=https%3A%2F%2Fclaude.ai%2Foauth%2Fmcp-test&redirect_uri=https%3A%2F%2Fclaude.ai%2Fapi%2Fmcp%2Fauth_callback&scope=admin&state=visual-test`
    await page.goto(authorizeURL)

    // Wait for SPA to render consent page
    await page.waitForSelector('.oauth-card', { timeout: 10000 })

    // Take screenshot
    await page.screenshot({ path: 'test-results/oauth-logged-in.png' })

    // 1. Should still be on the authorize page (NOT auto-redirected)
    expect(page.url()).toContain('/oauth/authorize')

    // 2. Authorize button should be visible
    const authorizeBtn = page.locator('.oauth-btn-approve')
    await expect(authorizeBtn).toBeVisible()

    // 3. Deny button should be visible
    const denyBtn = page.locator('.oauth-btn-deny')
    await expect(denyBtn).toBeVisible()

    // 4. App info should be visible
    const appInfo = page.locator('.oauth-app-info')
    await expect(appInfo).toBeVisible()

    // 5. User status should show logged-in
    const userStatus = page.locator('.oauth-user-status')
    await expect(userStatus).toBeVisible()
    const statusText = await userStatus.textContent()
    expect(statusText).toContain('Logged in as')

    // 6. No email/password fields
    const emailField = await page.locator('input[name="email"]').isVisible().catch(() => false)
    expect(emailField).toBe(false)

    console.log('Status:', statusText)
    console.log('URL:', page.url())
  })

  test('not-logged-in user gets redirected to login', async ({ page }) => {
    // Clear any existing session
    await page.goto(CF_URL)
    await page.evaluate(() => {
      localStorage.removeItem('token')
      localStorage.removeItem('refresh_token')
    })

    const authorizeURL = `${CF_URL}/oauth/authorize?response_type=code&client_id=https%3A%2F%2Fclaude.ai%2Foauth%2Fmcp-test&redirect_uri=https%3A%2F%2Fclaude.ai%2Fapi%2Fmcp%2Fauth_callback&scope=admin&state=visual-test2`
    await page.goto(authorizeURL)

    // Should redirect to /login
    await page.waitForURL(/\/login/, { timeout: 10000 })
    await expect(page).toHaveURL(/\/login/)

    // Should have redirect param
    const url = page.url()
    expect(url).toContain('redirect=')

    await page.screenshot({ path: 'test-results/oauth-not-logged-in.png' })
  })

  test('clicking Authorize submits and redirects to callback', async ({ page, request }) => {
    const user = await registerUser(request)

    // Login first
    await page.goto(CF_URL + '/login')
    await page.waitForLoadState('networkidle')
    await page.getByPlaceholder('your@email.com').fill(user.email)
    await page.getByPlaceholder('输入密码').fill(user.password)
    await page.locator('button[type="submit"]').click()
    await page.waitForURL(/^(?!.*\/login)/, { timeout: 15000 })

    // Visit OAuth authorize page
    const authorizeURL = `${CF_URL}/oauth/authorize?response_type=code&client_id=https%3A%2F%2Fclaude.ai%2Foauth%2Fmcp-test&redirect_uri=https%3A%2F%2Fclaude.ai%2Fapi%2Fmcp%2Fauth_callback&scope=admin&state=click-test`
    await page.goto(authorizeURL)

    // Wait for consent page
    await page.waitForSelector('.oauth-card', { timeout: 10000 })
    await expect(page.locator('.oauth-btn-approve')).toBeVisible()

    // Click Authorize
    await page.locator('.oauth-btn-approve').click()

    // Should redirect to callback URL (claude.ai)
    await page.waitForTimeout(3000)
    const finalURL = page.url()
    console.log('Final URL:', finalURL)

    // Should have left the authorize page
    expect(finalURL).toContain('claude.ai')
  })

  test('login flow: login → redirects back to OAuth → shows consent', async ({ page, request }) => {
    const user = await registerUser(request)

    // Clear session
    await page.goto(CF_URL)
    await page.evaluate(() => {
      localStorage.removeItem('token')
      localStorage.removeItem('refresh_token')
    })

    // Go to OAuth authorize (should redirect to login)
    const authorizeURL = `${CF_URL}/oauth/authorize?response_type=code&client_id=https%3A%2F%2Fclaude.ai%2Foauth%2Fmcp-test&redirect_uri=https%3A%2F%2Fclaude.ai%2Fapi%2Fmcp%2Fauth_callback&scope=admin&state=flow-test`
    await page.goto(authorizeURL)
    await page.waitForURL(/\/login/, { timeout: 10000 })

    // Login
    await page.getByPlaceholder('your@email.com').fill(user.email)
    await page.getByPlaceholder('输入密码').fill(user.password)
    await page.locator('button[type="submit"]').click()

    // After login, should go back to OAuth authorize (NOT dashboard)
    await page.waitForURL(/\/oauth\/authorize/, { timeout: 15000 })

    // Consent page should appear
    await page.waitForSelector('.oauth-card', { timeout: 10000 })
    await expect(page.locator('.oauth-btn-approve')).toBeVisible()

    // Should NOT have gone through dashboard
    console.log('URL after login:', page.url())
    expect(page.url()).toContain('/oauth/authorize')
  })
})
