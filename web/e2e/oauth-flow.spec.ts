import { test, expect } from '@playwright/test'
import { registerOAuthApp } from './helpers'

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

    const { response, clientID, redirectURI } = await registerOAuthApp(request, await (async () => {
      const loginRes = await request.post('/api/auth/login', {
        data: { email, password },
      })
      const body = await loginRes.json()
      return body.access_token
    })(), {
      name: 'OAuth Flow Test App',
    })
    expect(response.ok()).toBeTruthy()

    const authorizeURL = `${CF_URL}/oauth/authorize?response_type=code&client_id=${encodeURIComponent(clientID)}&redirect_uri=${encodeURIComponent(redirectURI)}&scope=admin&state=test123`

    // Visit OAuth authorize — redirects to login
    await page.goto(authorizeURL)
    await page.waitForURL(/\/login/, { timeout: 10000 })

    // Login
    await page.getByPlaceholder('your@email.com').fill(email)
    await page.getByPlaceholder('输入密码').fill(password)
    await page.locator('button[type="submit"]').click()

    await page.waitForURL(/\/oauth\/authorize/, { timeout: 15000 })
    await expect(page.locator('.oauth-card')).toBeVisible()
    await expect(page.locator('.oauth-btn-approve')).toBeVisible()
    await expect(page.locator('.oauth-btn-deny')).toBeVisible()
    await expect(page.locator('.oauth-app-info')).toBeVisible()

    const emailField = await page.locator('input[name="email"]').isVisible().catch(() => false)
    expect(emailField).toBe(false)
  })
})
