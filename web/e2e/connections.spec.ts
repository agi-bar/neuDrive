import { test, expect } from '@playwright/test'
import { registerOAuthApp, setupUser } from './helpers'

test.describe('Connections Page', () => {
  test('shows setup entry cards and empty state for a new account', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/connections')

    await expect(page.getByRole('heading', { name: '连接管理' })).toBeVisible()
    await expect(page.getByRole('button', { name: '打开 Web / Desktop Apps', exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: '打开 CLI Apps', exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: '打开 Local Mode', exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: '打开 Advanced', exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: '打开 ChatGPT GPT Actions', exact: true })).toBeVisible()
    await expect(page.getByText('还没有连接')).toBeVisible()
  })

  test('API-created manual connection appears and can be deleted from the UI', async ({ page, request }) => {
    const user = await setupUser(page, request)

    await request.post('/api/connections', {
      headers: { Authorization: `Bearer ${user.token}` },
      data: { name: 'My Claude', type: 'claude', trust_level: 4 },
    })

    await page.goto('/connections')
    await page.waitForLoadState('networkidle')

    const row = page.getByRole('row', { name: /My Claude/ })
    await expect(row).toBeVisible()
    await expect(row.getByText(/API Key · ahk_/)).toBeVisible()
    await expect(row.getByRole('button', { name: '删除' })).toBeVisible()

    page.on('dialog', dialog => dialog.accept())
    await row.getByRole('button', { name: '删除' }).click()
    await expect(page.getByText('还没有连接')).toBeVisible({ timeout: 5000 })
  })

  test('manual connection persists after refresh', async ({ page, request }) => {
    const user = await setupUser(page, request)

    await request.post('/api/connections', {
      headers: { Authorization: `Bearer ${user.token}` },
      data: { name: 'Persist Test', type: 'gpt', trust_level: 3 },
    })

    await page.goto('/connections')
    await page.waitForLoadState('networkidle')
    await page.reload()
    await page.waitForLoadState('networkidle')

    const row = page.getByRole('row', { name: /Persist Test/ })
    await expect(row).toBeVisible({ timeout: 5000 })
    await expect(row.getByText('GPT', { exact: true })).toBeVisible()
  })

  test('shows Claude Connector OAuth grant in connection management', async ({ page, request }) => {
    const user = await setupUser(page, request)
    const { response, clientID, redirectURI } = await registerOAuthApp(request, user.token, {
      name: 'Claude Connector',
    })
    const scope = 'admin'

    expect(response.ok()).toBeTruthy()

    const authRes = await request.post('/oauth/authorize', {
      form: {
        client_id: clientID,
        redirect_uri: redirectURI,
        scope,
        state: 'pw-test',
        action: 'approve',
        _token: user.token,
      },
      maxRedirects: 0,
      failOnStatusCode: false,
    })
    expect(authRes.status()).toBe(302)

    await page.goto('/connections')
    await page.waitForLoadState('networkidle')

    const row = page.getByRole('row', { name: /Claude Connector/ })
    await expect(row).toBeVisible()
    await expect(row.getByText(/OAuth \/ MCP/)).toBeVisible()
    await expect(row.getByRole('button', { name: '撤销授权' })).toBeVisible()
  })
})
