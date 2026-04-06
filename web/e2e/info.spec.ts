import { test, expect } from '@playwright/test'
import { setupUser } from './helpers'

test.describe('Info Page — Profile Persistence', () => {
  test('save preferences and verify after reload', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/data/profile')
    await page.waitForLoadState('networkidle')

    await page.locator('textarea').nth(0).fill('偏好简洁代码，Go 优先')
    await page.getByRole('button', { name: '保存所有配置' }).click()
    await expect(page.getByText('已保存')).toBeVisible({ timeout: 5000 })

    await page.reload()
    await page.waitForLoadState('networkidle')
    expect(await page.locator('textarea').nth(0).inputValue()).toContain('偏好简洁代码')
  })

  test('save relationships and verify after reload', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/data/profile')
    await page.waitForLoadState('networkidle')

    await page.locator('textarea').nth(1).fill('Alice 是产品经理')
    await page.getByRole('button', { name: '保存所有配置' }).click()
    await expect(page.getByText('已保存')).toBeVisible({ timeout: 5000 })

    await page.reload()
    await page.waitForLoadState('networkidle')
    expect(await page.locator('textarea').nth(1).inputValue()).toContain('Alice')
  })

  test('save principles and verify after reload', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/data/profile')
    await page.waitForLoadState('networkidle')

    await page.locator('textarea').nth(2).fill('先做再说，最小可行')
    await page.getByRole('button', { name: '保存所有配置' }).click()
    await expect(page.getByText('已保存')).toBeVisible({ timeout: 5000 })

    await page.reload()
    await page.waitForLoadState('networkidle')
    expect(await page.locator('textarea').nth(2).inputValue()).toContain('先做再说')
  })

  test('save all three with single button', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/data/profile')
    await page.waitForLoadState('networkidle')

    await page.locator('textarea').nth(0).fill('偏好 TypeScript')
    await page.locator('textarea').nth(1).fill('Bob 是设计师')
    await page.locator('textarea').nth(2).fill('代码即文档')

    // Single save-all button
    await page.getByRole('button', { name: '保存所有配置' }).click()
    await expect(page.getByText('已保存')).toBeVisible({ timeout: 5000 })

    await page.reload()
    await page.waitForLoadState('networkidle')

    expect(await page.locator('textarea').nth(0).inputValue()).toContain('TypeScript')
    expect(await page.locator('textarea').nth(1).inputValue()).toContain('Bob')
    expect(await page.locator('textarea').nth(2).inputValue()).toContain('代码即文档')
  })

  test('vault section exists in page', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/data/profile')
    await page.waitForLoadState('networkidle')

    // Scroll vault section into view
    const vault = page.getByRole('heading', { name: '安全存储' })
    await vault.scrollIntoViewIfNeeded()
    await expect(vault).toBeVisible({ timeout: 5000 })
  })
})
