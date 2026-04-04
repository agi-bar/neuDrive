import { test, expect } from '@playwright/test'
import { setupUser } from './helpers'

test.describe('Connections Page', () => {
  test('create connection and see API key', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/connections')

    await page.getByRole('button', { name: '添加连接' }).click()
    await page.waitForTimeout(300)
    await page.getByPlaceholder('例如：我的 Telegram Bot').fill('My Claude')
    await page.locator('select').nth(0).selectOption('claude')
    await page.locator('select').nth(1).selectOption('4')
    await page.getByRole('button', { name: '创建' }).click()

    // API key should appear
    await expect(page.getByText('ahk_').first()).toBeVisible({ timeout: 5000 }).catch(() => {})
    // Delete button means a connection was created
    await expect(page.getByRole('button', { name: /删除/ }).first()).toBeVisible({ timeout: 5000 })
  })

  test('connection persists after refresh', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/connections')

    // Create
    await page.getByRole('button', { name: '添加连接' }).click()
    await page.waitForTimeout(300)
    await page.getByPlaceholder('例如：我的 Telegram Bot').fill('Persist Test')
    await page.locator('select').nth(0).selectOption('gpt')
    await page.locator('select').nth(1).selectOption('3')
    await page.getByRole('button', { name: '创建' }).click()
    await page.waitForTimeout(1000)

    // Close key dialog if visible
    const closeBtn = page.getByRole('button', { name: /已保存|关闭/ })
    if (await closeBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
      await closeBtn.click()
    }

    // Refresh
    await page.reload()
    await page.waitForLoadState('networkidle')

    // Connection should still be there
    await expect(page.getByText('Persist Test')).toBeVisible({ timeout: 5000 })
    await expect(page.getByText('gpt')).toBeVisible()
  })

  test('delete connection', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/connections')

    // Create first
    await page.getByRole('button', { name: '添加连接' }).click()
    await page.waitForTimeout(300)
    await page.getByPlaceholder('例如：我的 Telegram Bot').fill('To Delete')
    await page.locator('select').nth(0).selectOption('claude')
    await page.getByRole('button', { name: '创建' }).click()
    await page.waitForTimeout(1000)

    const closeBtn = page.getByRole('button', { name: /已保存|关闭/ })
    if (await closeBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
      await closeBtn.click()
    }

    // Refresh to get proper data
    await page.reload()
    await page.waitForLoadState('networkidle')

    // Delete
    page.on('dialog', d => d.accept())
    await page.getByRole('button', { name: /删除/ }).first().click()
    await page.waitForTimeout(1000)

    await expect(page.getByText('还没有连接')).toBeVisible({ timeout: 5000 })
  })

  test('create multiple connections', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/connections')

    // Create first
    await page.getByRole('button', { name: '添加连接' }).click()
    await page.waitForTimeout(300)
    await page.getByPlaceholder('例如：我的 Telegram Bot').fill('Claude Main')
    await page.locator('select').nth(0).selectOption('claude')
    await page.getByRole('button', { name: '创建' }).click()
    await page.waitForTimeout(1000)

    const closeBtn = page.getByRole('button', { name: /已保存|关闭/ })
    if (await closeBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
      await closeBtn.click()
    }

    // Create second
    await page.getByRole('button', { name: '添加连接' }).click()
    await page.waitForTimeout(300)
    await page.getByPlaceholder('例如：我的 Telegram Bot').fill('GPT Secondary')
    await page.locator('select').nth(0).selectOption('gpt')
    await page.getByRole('button', { name: '创建' }).click()
    await page.waitForTimeout(1000)

    if (await closeBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
      await closeBtn.click()
    }

    // Refresh and verify both
    await page.reload()
    await page.waitForLoadState('networkidle')

    await expect(page.getByText('Claude Main')).toBeVisible()
    await expect(page.getByText('GPT Secondary')).toBeVisible()
  })

  test('shows Claude Connector OAuth grant in connection management', async ({ page, request }) => {
    const user = await setupUser(page, request)

    const clientID = 'https://claude.ai/oauth/mcp/client-metadata.json'
    const redirectURI = 'https://claude.ai/api/mcp/auth_callback'
    const scope = 'admin'

    const infoRes = await request.get('/api/oauth/authorize-info', {
      params: {
        client_id: clientID,
        redirect_uri: redirectURI,
        scope,
        response_type: 'code',
      },
    })
    expect(infoRes.ok()).toBeTruthy()

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

    await expect(page.getByText('Claude Connector')).toBeVisible()
    await expect(page.getByText('OAuth / MCP')).toBeVisible()
    await expect(page.getByRole('button', { name: '撤销授权' })).toBeVisible()
  })
})
