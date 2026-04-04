import { test, expect } from '@playwright/test'
import { setupUser } from './helpers'

test.describe('Setup Page — Token Management', () => {
  test('shows cloud mode tabs first without auto-generating token', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup')
    const cloudTabs = page.locator('[aria-label="云端模式平台"]')
    await expect(page.getByText('云端模式（浏览器授权）')).toBeVisible()
    await expect(page.getByText('推荐')).toBeVisible()
    await expect(page.getByText('默认添加到全局配置')).toBeVisible()
    await expect(cloudTabs.getByRole('tab', { name: 'Claude' })).toHaveAttribute('aria-selected', 'true')
    await expect(cloudTabs.getByRole('tab', { name: 'Codex' })).toBeVisible()
    await expect(page.locator('pre').filter({ hasText: /claude mcp add -s user --transport http agenthub/ })).toBeVisible()
    await expect(page.locator('ol.setup-steps').getByText(/\/mcp/)).toBeVisible()
    await expect(page.getByText('暂无 Token')).toBeVisible()
  })

  test('cloud mode can switch to codex instructions', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup')
    const cloudTabs = page.locator('[aria-label="云端模式平台"]')
    await cloudTabs.getByRole('tab', { name: 'Codex' }).click()
    await expect(cloudTabs.getByRole('tab', { name: 'Codex' })).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByRole('heading', { name: 'Codex CLI' })).toBeVisible()
    await expect(page.locator('pre').filter({ hasText: /codex mcp add agenthub --url/ })).toBeVisible()
    await expect(page.locator('pre').filter({ hasText: 'codex mcp login agenthub' })).toBeVisible()
    await expect(page.locator('pre').filter({ hasText: 'codex mcp list' })).toBeVisible()
  })

  test('local mode supports Claude and Codex tabs with lazy token generation', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup')
    const localTabs = page.locator('[aria-label="本地模式平台"]')
    await page.getByRole('button', { name: '生成并显示本地模式配置' }).click()
    await expect(localTabs.getByRole('tab', { name: 'Claude' })).toHaveAttribute('aria-selected', 'true')
    await expect(page.locator('pre').filter({ hasText: /claude mcp add -s user agenthub[\s\S]*--transport stdio[\s\S]*agenthub-mcp/ })).toBeVisible({ timeout: 10000 })
    await expect(page.locator('pre').filter({ hasText: /"mcpServers"/ })).toBeVisible()
    await expect(page.locator('.token-list-name', { hasText: 'Claude Code' })).toBeVisible()

    await localTabs.getByRole('tab', { name: 'Codex' }).click()
    await expect(localTabs.getByRole('tab', { name: 'Codex' })).toHaveAttribute('aria-selected', 'true')
    await expect(page.locator('pre').filter({ hasText: /codex mcp add agenthub -- agenthub-mcp --token aht_/ })).toBeVisible()
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

  test('create and rename token manually', async ({ page, request }) => {
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

    await page.getByRole('button', { name: '改名' }).click()
    await page.locator('.token-inline-input').fill('Playwright Token Renamed')
    await page.getByRole('button', { name: '保存' }).click()

    await expect(page.locator('.token-list-name', { hasText: 'Playwright Token Renamed' })).toBeVisible()
  })
})
