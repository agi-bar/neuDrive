import { test, expect } from '@playwright/test'
import { registerUser, loginViaUI, mockPublicConfig } from './helpers'

function authProvidersResponse(pocketEnabled = true, githubEnabled = true) {
  return {
    ok: true,
    data: [
      { id: 'github', kind: 'oauth2', display_name: 'GitHub', enabled: githubEnabled },
      { id: 'pocket', kind: 'oidc', display_name: 'Pocket ID', enabled: pocketEnabled },
    ],
  }
}

async function mockProviderRoutes(page: any, options?: { pocketEnabled?: boolean; githubEnabled?: boolean }) {
  const startCalls: Array<{ provider: string; body: any }> = []

  await mockPublicConfig(page)

  await page.route('**/api/auth/providers', async (route: any) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(authProvidersResponse(options?.pocketEnabled ?? true, options?.githubEnabled ?? true)),
    })
  })

  await page.route('**/api/auth/providers/*/start', async (route: any) => {
    const requestURL = new URL(route.request().url())
    const provider = requestURL.pathname.split('/')[4]
    const body = JSON.parse(route.request().postData() || '{}')
    startCalls.push({ provider, body })

    let authorizationURL = 'about:blank#unknown'
    if (provider === 'github') {
      authorizationURL = 'about:blank#github-login'
    } else if (body.action === 'signup') {
      authorizationURL = 'about:blank#pocket-signup'
    } else {
      authorizationURL = 'about:blank#pocket-login'
    }

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ ok: true, data: { authorization_url: authorizationURL } }),
    })
  })

  return { startCalls }
}

test.describe('Auth — Login Page', () => {
  test('renders split layout with fixed button order', async ({ page }) => {
    await mockProviderRoutes(page)
    await page.goto('/login')

    await expect(page.locator('.login-shell')).toBeVisible()
    await expect(page.locator('.login-hero')).toBeVisible()
    await expect(page.locator('.login-panel-card')).toBeVisible()
    await expect(page.locator('.login-hero-title')).toHaveText('neuDrive')
    await expect(page.locator('.login-hero-slogan')).toHaveText('One hub for all your AI agents')
    await expect(page.locator('.login-hero-subtitle')).toHaveText('Identity, memory, skills, and connections in one place.')

    const buttons = page.locator('.login-actions button')
    await expect(buttons).toHaveCount(3)
    await expect(buttons.nth(0)).toHaveText('Login')
    await expect(buttons.nth(1)).toHaveText('Sign up')
    await expect(buttons.nth(2)).toHaveText('Login with GitHub')
  })

  test('clicking Login starts Pocket login action', async ({ page }) => {
    const { startCalls } = await mockProviderRoutes(page)
    await page.goto('/login?redirect=%2Foauth%2Fauthorize%3Fclient_id%3Ddemo')
    await page.getByRole('button', { name: 'Login', exact: true }).click()

    await page.waitForURL('about:blank#pocket-login')
    expect(startCalls).toHaveLength(1)
    expect(startCalls[0]).toEqual({
      provider: 'pocket',
      body: { redirect_url: '/oauth/authorize?client_id=demo', action: 'login' },
    })
  })

  test('clicking Sign up starts Pocket signup action', async ({ page }) => {
    const { startCalls } = await mockProviderRoutes(page)
    await page.goto('/login?redirect=%2Foauth%2Fauthorize%3Fclient_id%3Ddemo')
    await page.getByRole('button', { name: 'Sign up' }).click()

    await page.waitForURL('about:blank#pocket-signup')
    expect(startCalls).toHaveLength(1)
    expect(startCalls[0]).toEqual({
      provider: 'pocket',
      body: { redirect_url: '/oauth/authorize?client_id=demo', action: 'signup' },
    })
  })

  test('clicking Login with GitHub starts GitHub login action', async ({ page }) => {
    const { startCalls } = await mockProviderRoutes(page)
    await page.goto('/login')
    await page.getByRole('button', { name: 'Login with GitHub' }).click()

    await page.waitForURL('about:blank#github-login')
    expect(startCalls).toHaveLength(1)
    expect(startCalls[0]).toEqual({
      provider: 'github',
      body: { redirect_url: '/', action: 'login' },
    })
  })

  test('disables buttons and shows provider hints when providers are unavailable', async ({ page }) => {
    await mockProviderRoutes(page, { pocketEnabled: false, githubEnabled: false })
    await page.goto('/login')

    await expect(page.getByRole('button', { name: 'Login', exact: true })).toBeDisabled()
    await expect(page.getByRole('button', { name: 'Sign up' })).toBeDisabled()
    await expect(page.getByRole('button', { name: 'Login with GitHub' })).toBeDisabled()
    await expect(page.locator('.login-provider-note')).toContainText([
      'Pocket ID login and signup are unavailable right now.',
      'GitHub login is unavailable right now.',
    ])
  })
})

test.describe('Auth — Logout', () => {
  test('logout redirects to login', async ({ page, request }) => {
    await mockPublicConfig(page)
    const user = await registerUser(request)
    await loginViaUI(page, user.email, user.password)

    await page.getByRole('button', { name: 'Sign out' }).click()
    await page.waitForURL(/\/login/, { timeout: 5000 })
    await expect(page).toHaveURL(/\/login/)
  })
})

test.describe('Auth — Protected routes', () => {
  test('unauthenticated access redirects to login', async ({ page }) => {
    await mockPublicConfig(page)
    await page.goto('/projects')
    await page.waitForURL(/\/login/, { timeout: 5000 })
    await expect(page).toHaveURL(/\/login/)
  })
})
