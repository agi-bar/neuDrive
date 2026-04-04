import { test, expect } from '@playwright/test'
import { setupUser } from './helpers'

test.describe('Setup Page — Token Management', () => {
  test('shows cloud mode first without auto-generating token', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup')
    await expect(page.getByText('Claude Code 云端模式')).toBeVisible()
    await expect(page.getByText('推荐')).toBeVisible()
    await expect(page.getByText('claude mcp add --transport http agenthub')).toBeVisible()
    await expect(page.getByText(/执行.*\/mcp.*agenthub.*开始认证/)).toBeVisible()
    await expect(page.getByText('暂无 Token')).toBeVisible()
  })

  test('local mode lazily generates stdio token and config', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup')
    await page.getByRole('button', { name: '生成并显示本地模式配置' }).click()
    await expect(page.getByText('agenthub-mcp')).toBeVisible({ timeout: 10000 })
    await expect(page.getByText('--transport stdio')).toBeVisible()
    await expect(page.getByText('mcpServers')).toBeVisible()
    await expect(page.locator('.token-list-name', { hasText: 'Claude Code' })).toBeVisible()
  })

  test('advanced mode lazily generates bearer config', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup')
    await page.getByRole('button', { name: '生成并显示高级模式配置' }).click()
    await expect(page.getByText('Authorization')).toBeVisible({ timeout: 10000 })
    await expect(page.locator('pre').filter({ hasText: /"Authorization": "Bearer aht_/ })).toBeVisible()
    await expect(page.getByText('"type": "http"')).toBeVisible()
    await expect(page.locator('.token-list-name', { hasText: 'MCP HTTP' })).toBeVisible()
  })

  test('GPT Actions config remains visible', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup')
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight))
    await page.waitForTimeout(500)
    await expect(page.getByText('ChatGPT GPT Actions')).toBeVisible()
    await expect(page.getByRole('button', { name: '去创建' })).toBeVisible()
  })

  test('create new token manually', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup')
    await page.waitForLoadState('networkidle')

    // Scroll to token creation form
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight))
    await page.waitForTimeout(500)

    await page.locator('#token-creator input').first().fill('Playwright Token')
    await page.getByRole('button', { name: '生成 Token' }).click()

    await expect(page.getByText('Token 已生成!')).toBeVisible({ timeout: 10000 })
    await expect(page.locator('.token-list-name', { hasText: 'Playwright Token' })).toBeVisible()
    await expect(page.getByText('已有 Token')).toBeVisible()
  })
})
