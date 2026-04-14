import { test, expect } from '@playwright/test'
import { setupUser } from './helpers'

function projectTile(page: any, name: string) {
  return page.locator('.materials-tile').filter({ hasText: name }).first()
}

async function selectProject(page: any, name: string) {
  await projectTile(page, name).click({ position: { x: 12, y: 12 } })
}

test.describe('Projects Page', () => {
  test('create project', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/data/projects')

    await page.getByRole('button', { name: '新建项目' }).click()
    await page.getByPlaceholder('例如：blog-redesign').fill('test-project')
    await page.getByRole('button', { name: '创建' }).click()

    await expect(projectTile(page, 'test-project')).toBeVisible({ timeout: 5000 })
    await expect(page.getByText('进行中')).toBeVisible()
  })

  test('selected project shows empty detail state', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/data/projects')

    await page.getByRole('button', { name: '新建项目' }).click()
    await page.getByPlaceholder('例如：blog-redesign').fill('detail-test')
    await page.getByRole('button', { name: '创建' }).click()
    await expect(projectTile(page, 'detail-test')).toBeVisible({ timeout: 5000 })

    await selectProject(page, 'detail-test')
    await expect(page.getByText('暂无项目详情')).toBeVisible({ timeout: 5000 })
  })

  test('project logs appear in selected detail panel', async ({ page, request }) => {
    const user = await setupUser(page, request)
    await page.goto('/data/projects')

    await page.getByRole('button', { name: '新建项目' }).click()
    await page.getByPlaceholder('例如：blog-redesign').fill('logs-test')
    await page.getByRole('button', { name: '创建' }).click()
    await expect(projectTile(page, 'logs-test')).toBeVisible({ timeout: 5000 })

    for (let i = 0; i < 3; i++) {
      await request.post('/api/projects/logs-test/log', {
        headers: { Authorization: `Bearer ${user.token}` },
        data: { source: 'claude', action: 'test', summary: `Log entry ${i + 1}`, tags: ['test'] },
      })
    }

    await page.reload()
    await page.waitForLoadState('networkidle')
    await selectProject(page, 'logs-test')
    const logsPanel = page.locator('.materials-panel').filter({
      has: page.getByRole('heading', { name: '最近日志', level: 4, exact: true }),
    })
    await expect(logsPanel).toBeVisible({ timeout: 5000 })
    await expect(logsPanel.locator('.summary').filter({ hasText: 'Log entry 1' }).first()).toBeVisible()
  })

  test('summarized project shows context in detail panel', async ({ page, request }) => {
    const user = await setupUser(page, request)
    await page.goto('/data/projects')

    await page.getByRole('button', { name: '新建项目' }).click()
    await page.getByPlaceholder('例如：blog-redesign').fill('summary-test')
    await page.getByRole('button', { name: '创建' }).click()
    await expect(projectTile(page, 'summary-test')).toBeVisible({ timeout: 5000 })

    for (let i = 0; i < 6; i++) {
      await request.post('/api/projects/summary-test/log', {
        headers: { Authorization: `Bearer ${user.token}` },
        data: { source: 'claude', action: 'wrote', summary: `Article ${i}`, tags: ['writing'] },
      })
    }
    await request.post('/api/projects/summary-test/summarize', {
      headers: { Authorization: `Bearer ${user.token}` },
    })

    await page.reload()
    await page.waitForLoadState('networkidle')
    await selectProject(page, 'summary-test')
    await expect(page.getByText('context.md')).toBeVisible({ timeout: 5000 })
  })

  test('archive project from detail panel', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/data/projects')

    await page.getByRole('button', { name: '新建项目' }).click()
    await page.getByPlaceholder('例如：blog-redesign').fill('archive-test')
    await page.getByRole('button', { name: '创建' }).click()
    await expect(projectTile(page, 'archive-test')).toBeVisible({ timeout: 5000 })

    await selectProject(page, 'archive-test')
    await page.getByRole('button', { name: '归档项目' }).click()
    await page.getByRole('button', { name: '确认归档' }).click()

    await expect(page.getByText('已归档')).toBeVisible({ timeout: 5000 })
  })
})
