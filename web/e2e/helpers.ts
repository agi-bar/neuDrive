import { Page } from '@playwright/test'

export async function mockPublicConfig(page: Page, overrides: Record<string, any> = {}) {
  await page.route('**/api/config', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        ok: true,
        data: {
          git_mirror_execution_mode: 'local',
          github_app_enabled: false,
          github_app_slug: '',
          github_client_id: '',
          github_enabled: false,
          local_mode: false,
          storage: 'sqlite',
          system_settings_enabled: true,
          ...overrides,
        },
      }),
    })
  })
}

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
    refreshToken: body.refresh_token,
    userId: body.user?.id,
  }
}

async function installBrowserSession(page: Page, accessToken: string, refreshToken?: string) {
  await page.evaluate(({ accessToken: token, refreshToken: refresh }) => {
    localStorage.setItem('token', token)
    if (refresh) {
      localStorage.setItem('refresh_token', refresh)
    } else {
      localStorage.removeItem('refresh_token')
    }
  }, { accessToken, refreshToken })
}

// Establish a browser session without depending on the login page form fields.
export async function loginViaUI(page: Page, email: string, password: string) {
  if (page.url() === 'about:blank' || !page.url().includes('/login')) {
    await page.goto('/login')
    await page.waitForLoadState('networkidle')
  }

  const redirect = new URL(page.url()).searchParams.get('redirect') || '/'
  const auth = await page.evaluate(async ({ email, password }) => {
    const res = await fetch('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password }),
    })
    const body = await res.json().catch(() => null)
    if (!res.ok) {
      throw new Error(body?.message || body?.error || res.statusText || 'login failed')
    }
    return body && body.ok === true && body.data !== undefined ? body.data : body
  }, { email, password })

  await installBrowserSession(page, auth.access_token, auth.refresh_token)
  await page.goto(redirect)
  await page.waitForURL(/^(?!.*\/login)/, { timeout: 15000 })
  await page.waitForLoadState('networkidle')
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
