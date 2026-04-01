import { test, expect } from '@playwright/test'
import { setupUser } from './helpers'

test.describe('Setup Page — Token Management', () => {
  test('auto-generates token on first visit', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup')
    await expect(page.getByText('aht_').first()).toBeVisible({ timeout: 10000 })
  })

  test('MCP config visible', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup')
    await expect(page.getByText('claude mcp add')).toBeVisible()
    await expect(page.getByText('mcpServers')).toBeVisible()
  })

  test('HTTP API config visible', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup')
    await expect(page.getByText('Base URL')).toBeVisible()
    await expect(page.getByText('Authorization: Bearer')).toBeVisible()
  })

  test('GPT Actions config visible', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup')
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight))
    await page.waitForTimeout(500)
    await expect(page.getByText('ChatGPT GPT Actions')).toBeVisible()
  })

  test('create new token manually', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup')
    await page.waitForLoadState('networkidle')

    // Scroll to token creation form
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight))
    await page.waitForTimeout(500)

    // Token list should show at least 1 (auto-created)
    await expect(page.getByText('已有 Token')).toBeVisible({ timeout: 5000 })
  })
})
