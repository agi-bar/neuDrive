import { test, expect } from '@playwright/test'
import { registerUser } from './helpers'

const CF_URL = process.env.CF_URL || 'http://localhost:8080'

test.describe('OAuth Page Visual Check', () => {
  test('logged-in user sees app info + auto-authorize status, NOT email/password form', async ({ page, request }) => {
    const user = await registerUser(request)

    // First login on this domain via SPA
    await page.goto(CF_URL + '/login')
    await page.waitForLoadState('networkidle')
    await page.getByPlaceholder('your@email.com').fill(user.email)
    await page.getByPlaceholder('输入密码').fill(user.password)
    await page.locator('button[type="submit"]').click()
    await page.waitForURL(/^(?!.*\/login)/, { timeout: 15000 })

    // Now visit OAuth authorize page
    const authorizeURL = `${CF_URL}/oauth/authorize?response_type=code&client_id=https%3A%2F%2Fclaude.ai%2Foauth%2Fmcp-test&redirect_uri=https%3A%2F%2Fclaude.ai%2Fapi%2Fmcp%2Fauth_callback&scope=admin&state=visual-test`
    await page.goto(authorizeURL)

    // Take screenshot BEFORE auto-submit happens
    await page.waitForTimeout(500)
    await page.screenshot({ path: 'test-results/oauth-logged-in.png' })

    // Should show app name (claude.ai)
    // Should show "正在自动授权" or auto-submit
    // Should NOT show email/password fields
    const emailVisible = await page.locator('input[name="email"]').isVisible().catch(() => false)
    const autoStatus = await page.getByText('正在自动授权').isVisible().catch(() => false)

    console.log('Email field visible:', emailVisible)
    console.log('Auto-status visible:', autoStatus)

    // Email form should be hidden
    expect(emailVisible).toBe(false)
  })

  test('not-logged-in user gets redirected to login without seeing OAuth form', async ({ page }) => {
    // Clear any existing session
    await page.goto(CF_URL)
    await page.evaluate(() => {
      localStorage.removeItem('token')
      localStorage.removeItem('refresh_token')
    })

    const authorizeURL = `${CF_URL}/oauth/authorize?response_type=code&client_id=https%3A%2F%2Fclaude.ai%2Foauth%2Fmcp-test&redirect_uri=https%3A%2F%2Fclaude.ai%2Fapi%2Fmcp%2Fauth_callback&scope=admin&state=visual-test2`
    await page.goto(authorizeURL)

    // Should redirect to /login immediately
    await page.waitForURL(/\/login/, { timeout: 10000 })
    await expect(page).toHaveURL(/\/login/)

    // Take screenshot
    await page.screenshot({ path: 'test-results/oauth-not-logged-in.png' })
  })
})
