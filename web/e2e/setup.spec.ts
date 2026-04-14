import { test, expect } from '@playwright/test'
import { setupUser } from './helpers'

test.describe('Setup Page — Token Management', () => {
  test('redirects /setup to web apps guide with Claude tab active by default', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup')
    await expect(page).toHaveURL(/\/setup\/web-apps/)
    const webTabs = page.locator('[aria-label="Web / Desktop Apps 平台"]')
    await expect(page.getByRole('heading', { name: 'Web / Desktop Apps' })).toBeVisible()
    await expect(webTabs.getByRole('tab', { name: 'Claude' })).toHaveAttribute('aria-selected', 'true')
    await expect(webTabs.getByRole('tab', { name: 'ChatGPT' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Claude Connectors' })).toBeVisible()
    await expect(page.getByText('Claude Connectors 列表')).toBeVisible()
    await expect(page.getByRole('link', { name: 'Token 管理' })).toBeVisible()
  })

  test('web apps guide can switch to ChatGPT instructions', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup/web-apps')
    const webTabs = page.locator('[aria-label="Web / Desktop Apps 平台"]')
    await webTabs.getByRole('tab', { name: 'ChatGPT' }).click()
    await expect(webTabs.getByRole('tab', { name: 'ChatGPT' })).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByRole('heading', { name: 'ChatGPT Apps' })).toBeVisible()
    await expect(page.getByText('Settings -> Apps', { exact: true })).toBeVisible()
    await expect(page.getByText('ChatGPT Apps 设置')).toBeVisible()
  })

  test('cloud mode can switch to codex instructions', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup/cloud')
    const cloudTabs = page.locator('[aria-label="CLI Apps 平台"]')
    await expect(page.getByRole('heading', { name: 'CLI Apps' })).toBeVisible()
    await expect(page.getByText('推荐', { exact: true })).toBeVisible()
    await expect(page.getByText('适合 Claude Code、Codex CLI、Gemini CLI 和 Cursor Agent')).toBeVisible()
    await cloudTabs.getByRole('tab', { name: 'Codex' }).click()
    await expect(cloudTabs.getByRole('tab', { name: 'Codex' })).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByRole('heading', { name: 'Codex CLI' })).toBeVisible()
    await expect(page.locator('pre').filter({ hasText: /codex mcp add neudrive --url/ })).toBeVisible()
    await expect(page.locator('pre').filter({ hasText: 'codex mcp login neudrive' })).toBeVisible()
    await expect(page.locator('pre').filter({ hasText: 'codex mcp list' })).toBeVisible()
  })

  test('local mode supports Claude and Codex tabs without auto-generating a token', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup/local')
    const localTabs = page.locator('[aria-label="本地模式平台"]')
    await page.getByRole('button', { name: '查看本地模式配置' }).click()
    await expect(localTabs.getByRole('tab', { name: 'Claude' })).toHaveAttribute('aria-selected', 'true')
    await expect(page.locator('pre').filter({ hasText: 'export NEUDRIVE_TOKEN=<YOUR_NEUDRIVE_TOKEN>' })).toBeVisible()
    await expect(page.locator('pre').filter({ hasText: /claude mcp add -s user neudrive -- neudrive-mcp --token-env NEUDRIVE_TOKEN/ })).toBeVisible()
    await expect(page.locator('pre').filter({ hasText: /"mcpServers"/ })).toBeVisible()
    await expect(page.locator('pre').filter({ hasText: /"--token-env",\s*"NEUDRIVE_TOKEN"/ })).toBeVisible()
    await expect(page.locator('.token-list-name', { hasText: 'Claude Code' })).toHaveCount(0)

    await page.getByRole('button', { name: '创建本模式 Token' }).click()
    await expect(page.locator('pre').filter({ hasText: /export NEUDRIVE_TOKEN=ndt_/ })).toBeVisible()

    await localTabs.getByRole('tab', { name: 'Codex' }).click()
    await expect(localTabs.getByRole('tab', { name: 'Codex' })).toHaveAttribute('aria-selected', 'true')
    await expect(page.locator('pre').filter({ hasText: /codex mcp add neudrive -- neudrive-mcp --token-env NEUDRIVE_TOKEN/ })).toBeVisible()

    await page.goto('/setup/tokens')
    await expect(page.locator('.token-list-name', { hasText: 'Claude Code' })).toBeVisible({ timeout: 10000 })
  })

  test('advanced mode shows template first and only creates a token on demand', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup/advanced')
    await page.getByRole('button', { name: '查看高级模式配置' }).click()
    await expect(page.locator('pre').filter({ hasText: 'export NEUDRIVE_TOKEN=<YOUR_NEUDRIVE_TOKEN>' })).toBeVisible()
    await expect(page.locator('pre').filter({ hasText: /codex mcp add neudrive --url .* --bearer-token-env-var NEUDRIVE_TOKEN/ })).toBeVisible()
    await expect(page.locator('pre').filter({ hasText: /"Authorization": "Bearer <YOUR_NEUDRIVE_TOKEN>"/ })).toBeVisible()
    await expect(page.locator('.token-list-name', { hasText: 'MCP HTTP' })).toHaveCount(0)

    await page.getByRole('button', { name: '创建本模式 Token' }).click()
    await expect(page.locator('pre').filter({ hasText: /export NEUDRIVE_TOKEN=ndt_/ })).toBeVisible({ timeout: 10000 })
    await expect(page.locator('pre').filter({ hasText: /"Authorization": "Bearer ndt_/ })).toBeVisible({ timeout: 10000 })
    await expect(page.getByText('"type": "http"')).toBeVisible()

    await page.goto('/setup/tokens')
    await expect(page.locator('.token-list-name', { hasText: 'MCP HTTP' })).toBeVisible()
  })

  test('GPT Actions config remains visible', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup/gpt-actions')
    await expect(page.getByText('ChatGPT GPT Actions')).toBeVisible()
    await expect(page.getByRole('button', { name: '前往 Token 管理' })).toBeVisible()
  })

  test('create and rename token manually', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup/tokens')
    await page.waitForLoadState('networkidle')
    await expect(page.getByRole('heading', { name: 'Token 管理', level: 2 })).toBeVisible()

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
