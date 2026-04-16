import { test, expect } from '@playwright/test'
import { registerUser, loginViaUI, setupUser } from './helpers'

test.describe('Import & Export', () => {
  test('export JSON from dashboard', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/')

    // Click JSON export
    const [download] = await Promise.all([
      page.waitForEvent('download', { timeout: 10000 }).catch(() => null),
      page.getByRole('button', { name: '导出数据 (JSON)' }).click(),
    ])

    // Either a download started or success message shown
    const hasMsg = await page.getByText('已开始下载').isVisible({ timeout: 3000 }).catch(() => false)
    expect(download !== null || hasMsg).toBeTruthy()
  })

  test('import data then verify on dashboard', async ({ page, request }) => {
    const user = await registerUser(request)

    // Import skill via API
    await request.post('/api/import/skill', {
      headers: { Authorization: `Bearer ${user.token}` },
      data: {
        name: 'pw-test-skill',
        files: { 'SKILL.md': '# Test Skill\nPlaywright imported skill' },
      },
    })

    // Login and check dashboard
    await loginViaUI(page, user.email, user.password)
    await page.goto('/')
    await page.waitForLoadState('networkidle')

    // Dashboard should show non-zero stats after import
    const statsText = await page.locator('.stat-value').allTextContents()
    const hasNonZero = statsText.some(v => parseInt(v) > 0)
    expect(hasNonZero).toBeTruthy()
  })

  test('import profile then verify on info page', async ({ page, request }) => {
    const user = await registerUser(request)

    // Import profile via API
    await request.post('/api/import/profile', {
      headers: { Authorization: `Bearer ${user.token}` },
      data: {
        preferences: 'Imported preference via Playwright test',
        relationships: 'Carol is a colleague',
      },
    })

    // Login and check info page
    await loginViaUI(page, user.email, user.password)
    await page.goto('/info')
    await page.waitForLoadState('networkidle')

    // Preferences textarea should have imported content
    const prefValue = await page.locator('textarea').nth(0).inputValue()
    expect(prefValue).toContain('Imported preference')

    const relValue = await page.locator('textarea').nth(1).inputValue()
    expect(relValue).toContain('Carol')
  })
})
