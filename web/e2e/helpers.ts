import { Page } from '@playwright/test'

// Register a unique user via API and return credentials.
export async function registerUser(request: any) {
  const slug = `pw-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`
  const email = `${slug}@test.local`
  const password = 'playwright1234'

  const res = await request.post('/api/auth/register', {
    data: { slug, email, password },
  })
  const body = await res.json()
  return {
    slug,
    email,
    password,
    token: body.access_token,
    userId: body.user?.id,
  }
}

// Login via the UI login form.
export async function loginViaUI(page: Page, email: string, password: string) {
  await page.goto('/login')
  await page.waitForLoadState('networkidle')
  await page.getByPlaceholder('your@email.com').fill(email)
  await page.getByPlaceholder('输入密码').fill(password)
  await page.locator('button[type="submit"]').click()
  await page.waitForURL(/^(?!.*\/login)/, { timeout: 15000 })
}

// Register + login in one step.
export async function setupUser(page: Page, request: any) {
  const user = await registerUser(request)
  await loginViaUI(page, user.email, user.password)
  return user
}

export async function registerOAuthApp(request: any, token: string, overrides: {
  name?: string
  redirectURI?: string
  scopes?: string[]
  description?: string
} = {}) {
  const redirectURI = overrides.redirectURI || 'https://claude.ai/api/mcp/auth_callback'
  const response = await request.post('/api/oauth/apps', {
    headers: { Authorization: `Bearer ${token}` },
    data: {
      name: overrides.name || 'Claude Connector',
      redirect_uris: [redirectURI],
      scopes: overrides.scopes || ['admin'],
      description: overrides.description || 'Playwright OAuth test app',
    },
  })
  const body = await response.json()
  const payload = body?.data || body
  return {
    response,
    clientID: payload.client_id,
    redirectURI,
    raw: payload,
  }
}
