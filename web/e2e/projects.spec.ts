import { test, expect } from '@playwright/test'
import { setupUser } from './helpers'

test.describe('Projects Page', () => {
  test('create project', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/data/projects')

    await page.getByRole('button', { name: '新建项目' }).click()
    await page.getByPlaceholder('例如：blog-redesign').fill('test-project')
    await page.getByRole('button', { name: '创建' }).click()

    await expect(page.getByText('test-project')).toBeVisible({ timeout: 5000 })
    await expect(page.getByText('进行中')).toBeVisible()
    await expect(page.locator('.project-meta').filter({ hasText: '最后活动：-' })).toHaveCount(0)
  })

  test('project detail shows empty state', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/data/projects')

    // Create
    await page.getByRole('button', { name: '新建项目' }).click()
    await page.getByPlaceholder('例如：blog-redesign').fill('detail-test')
    await page.getByRole('button', { name: '创建' }).click()
    await expect(page.getByText('detail-test')).toBeVisible({ timeout: 5000 })

    // Click to expand detail
    await page.getByText('detail-test').click()
    await page.waitForTimeout(1000)

    await expect(page.getByText('暂无项目详情')).toBeVisible({ timeout: 5000 })
  })

  test('project with logs shows detail', async ({ page, request }) => {
    const user = await setupUser(page, request)
    await page.goto('/data/projects')

    // Create project
    await page.getByRole('button', { name: '新建项目' }).click()
    await page.getByPlaceholder('例如：blog-redesign').fill('logs-test')
    await page.getByRole('button', { name: '创建' }).click()
    await expect(page.getByText('logs-test')).toBeVisible({ timeout: 5000 })

    // Add logs via API (not UI — no log UI exists)
    for (let i = 0; i < 3; i++) {
      await request.post('/api/projects/logs-test/log', {
        headers: { Authorization: `Bearer ${user.token}` },
        data: { source: 'claude', action: 'test', summary: `Log entry ${i + 1}`, tags: ['test'] },
      })
    }

    // Refresh and click project
    await page.reload()
    await page.waitForLoadState('networkidle')
    await page.getByText('logs-test').click()
    await page.waitForTimeout(1500)

    // Should show logs
    await expect(page.getByText('Log entry 1')).toBeVisible({ timeout: 5000 })
  })

  test('summarize project shows context', async ({ page, request }) => {
    const user = await setupUser(page, request)
    await page.goto('/data/projects')

    // Create + add logs
    await page.getByRole('button', { name: '新建项目' }).click()
    await page.getByPlaceholder('例如：blog-redesign').fill('summary-test')
    await page.getByRole('button', { name: '创建' }).click()
    await expect(page.getByText('summary-test')).toBeVisible({ timeout: 5000 })

    for (let i = 0; i < 6; i++) {
      await request.post('/api/projects/summary-test/log', {
        headers: { Authorization: `Bearer ${user.token}` },
        data: { source: 'claude', action: 'wrote', summary: `Article ${i}`, tags: ['writing'] },
      })
    }

    // Trigger summarize via API
    await request.post('/api/projects/summary-test/summarize', {
      headers: { Authorization: `Bearer ${user.token}` },
    })

    // Refresh and check
    await page.reload()
    await page.waitForLoadState('networkidle')
    await page.getByText('summary-test').click()
    await page.waitForTimeout(1500)

    // context.md should be visible
    await expect(page.getByText('context.md')).toBeVisible({ timeout: 5000 })
  })

  test('archive project', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/data/projects')

    await page.getByRole('button', { name: '新建项目' }).click()
    await page.getByPlaceholder('例如：blog-redesign').fill('archive-test')
    await page.getByRole('button', { name: '创建' }).click()
    await expect(page.getByText('archive-test')).toBeVisible({ timeout: 5000 })

    // Archive
    page.on('dialog', d => d.accept())
    await page.getByRole('button', { name: '归档' }).click()
    await page.waitForTimeout(1000)

    await expect(page.getByText('已归档')).toBeVisible({ timeout: 5000 })
  })
})
