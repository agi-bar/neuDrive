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
  test('renders username login form and optional provider actions', async ({ page }) => {
    await mockProviderRoutes(page)
    await page.goto('/login')

    await expect(page.getByText('Log in to neuDrive')).toBeVisible()
    await expect(page.getByText('Use your username or email to access the product.')).toBeVisible()
    await expect(page.getByLabel('Username or email')).toBeVisible()
    await expect(page.getByLabel('Password')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Log in', exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Continue with Pocket ID' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Continue with GitHub' })).toBeVisible()
  })

  test('submits local password login and redirects to requested route', async ({ page }) => {
    await mockProviderRoutes(page)
    await page.goto('/login?redirect=%2Fprojects')
    await page.route('**/api/auth/login', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          access_token: 'access-1',
          refresh_token: 'refresh-1',
          expires_in: 86400,
          user: { id: 'u1', slug: 'alice', display_name: 'Alice' },
        }),
      })
    })
    await page.getByLabel('Username or email').fill('alice')
    await page.getByLabel('Password').fill('playwright1234')
    await page.getByRole('button', { name: 'Log in', exact: true }).click()

    await page.waitForURL(/\/projects$/)
    await expect(page.evaluate(() => localStorage.getItem('token'))).resolves.toBe('access-1')
    await expect(page.evaluate(() => localStorage.getItem('refresh_token'))).resolves.toBe('refresh-1')
  })

  test('clicking Continue with Pocket ID starts Pocket login action', async ({ page }) => {
    const { startCalls } = await mockProviderRoutes(page)
    await page.goto('/login?redirect=%2Foauth%2Fauthorize%3Fclient_id%3Ddemo')
    await page.getByRole('button', { name: 'Continue with Pocket ID' }).click()

    await page.waitForURL('about:blank#pocket-login')
    expect(startCalls).toHaveLength(1)
    expect(startCalls[0]).toEqual({
      provider: 'pocket',
      body: { redirect_url: '/oauth/authorize?client_id=demo', action: 'login' },
    })
  })

  test('clicking Continue with GitHub starts GitHub login action', async ({ page }) => {
    const { startCalls } = await mockProviderRoutes(page)
    await page.goto('/login')
    await page.getByRole('button', { name: 'Continue with GitHub' }).click()

    await page.waitForURL('about:blank#github-login')
    expect(startCalls).toHaveLength(1)
    expect(startCalls[0]).toEqual({
      provider: 'github',
      body: { redirect_url: '/', action: 'login' },
    })
  })

  test('shows provider hints when external providers are unavailable', async ({ page }) => {
    await mockProviderRoutes(page, { pocketEnabled: false, githubEnabled: false })
    await page.goto('/login')

    await expect(page.getByRole('button', { name: 'Log in', exact: true })).toBeEnabled()
    await expect(page.getByText('Pocket ID login is unavailable right now.')).toBeVisible()
    await expect(page.getByText('GitHub login is unavailable right now.')).toBeVisible()
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
