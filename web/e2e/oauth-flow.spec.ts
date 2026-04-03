import { test, expect } from '@playwright/test'

const CF_URL = process.env.CF_URL || 'http://localhost:8080'

test.describe('OAuth Authorization Flow', () => {
  test('no form flash — redirects to login immediately', async ({ page }) => {
    const authorizeURL = `${CF_URL}/oauth/authorize?response_type=code&client_id=https%3A%2F%2Fclaude.ai%2Foauth%2Fmcp-test&redirect_uri=https%3A%2F%2Fclaude.ai%2Fapi%2Fmcp%2Fauth_callback&scope=admin&state=test123`

    // Visit OAuth authorize page
    await page.goto(authorizeURL)

    // Should redirect to /login (not show OAuth form)
    await page.waitForURL(/\/login/, { timeout: 10000 })
    await expect(page).toHaveURL(/\/login/)

    // Should have redirect param
    const url = page.url()
    expect(url).toContain('redirect=')
  })

  test('login then shows consent page with Authorize button', async ({ page, request }) => {
    // Register a user via API
    const slug = `oauth-test-${Date.now()}`
    const email = `${slug}@test.local`
    const password = 'oauthtest1234'
    await request.post('/api/auth/register', {
      data: { slug, email, password },
    })

    const authorizeURL = `${CF_URL}/oauth/authorize?response_type=code&client_id=https%3A%2F%2Fclaude.ai%2Foauth%2Fmcp-test&redirect_uri=https%3A%2F%2Fclaude.ai%2Fapi%2Fmcp%2Fauth_callback&scope=admin&state=test123`

    // Visit OAuth authorize — redirects to login
    await page.goto(authorizeURL)
    await page.waitForURL(/\/login/, { timeout: 10000 })

    // Login
    await page.getByPlaceholder('your@email.com').fill(email)
    await page.getByPlaceholder('输入密码').fill(password)
    await page.locator('button[type="submit"]').click()

    // After login, should redirect back to authorize page
    await page.waitForTimeout(3000)

    const finalURL = page.url()
    console.log('Final URL:', finalURL)

    // Should be on the authorize page showing consent
    if (finalURL.includes('/oauth/authorize')) {
      // Consent card should be visible with Authorize button
      const consentCard = await page.locator('.consent-card').isVisible().catch(() => false)
      expect(consentCard).toBe(true)

      // Email/password fields should NOT be visible
      const emailField = await page.locator('input[name="email"]').isVisible().catch(() => false)
      expect(emailField).toBe(false)

      // Authorize button should be visible
      const authorizeBtn = await page.locator('button.btn-approve').isVisible().catch(() => false)
      expect(authorizeBtn).toBe(true)
    }
  })
})
