import { test, expect } from '@playwright/test'
import { registerUser, loginViaUI, setupUser } from './helpers'

test.describe('Dashboard', () => {
  test('login lands on neuDrive overview', async ({ page, request }) => {
    const user = await registerUser(request)
    await loginViaUI(page, user.email, user.password)
    await expect(page.locator('.sidebar-brand h1')).toHaveText('neuDrive')
    await expect(page.getByRole('heading', { name: '概览' })).toBeVisible()
  })

  test('overview shows current stats and preview cards', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/')

    const dataDrawerButton = page.getByRole('button', { name: '数据文件' })

    await expect(page.getByText('一切正常')).toBeVisible()
    await expect(page.locator('.stat-card').filter({ hasText: /已连接平台|添加平台/ })).toHaveCount(1)
    await expect(page.locator('.stat-card').filter({ hasText: '所有文件' })).toHaveCount(1)
    await expect(page.locator('.stat-card').filter({ hasText: '项目' })).toHaveCount(1)
    await expect(page.locator('.stat-card').filter({ hasText: '技能' })).toHaveCount(1)
    await expect(page.locator('.stat-card').filter({ hasText: 'Memory' })).toHaveCount(1)
    await expect(page.locator('.stat-card').filter({ hasText: '我的资料' })).toHaveCount(1)

    await expect(page.getByRole('heading', { name: '我的资料' })).toBeVisible()
    await expect(page.getByRole('heading', { name: '最近更新' })).toBeVisible()
    await expect(page.getByText('数据管理')).toBeVisible()
    await expect(dataDrawerButton).toHaveAttribute('aria-expanded', 'false')
  })

  test('overview preview links navigate correctly', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/')

    const profileCard = page.locator('.dashboard-card').filter({ has: page.getByRole('heading', { name: '我的资料' }) })
    await profileCard.getByRole('link', { name: '更多' }).click()
    await expect(page).toHaveURL(/\/data\/profile/)

    await page.goto('/')
    const filesCard = page.locator('.dashboard-card').filter({ has: page.getByRole('heading', { name: '最近更新' }) })
    await filesCard.getByRole('link', { name: '文件管理器' }).click()
    await expect(page).toHaveURL(/\/data\/files/)
  })
})

test.describe('Setup Routing', () => {
  test('setup route redirects to web apps and keeps token manager in sidebar', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/setup')
    await expect(page).toHaveURL(/\/setup\/web-apps/)
    await expect(page.getByRole('heading', { name: 'Web / Desktop Apps' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Token 管理' })).toBeVisible()
  })
})

test.describe('Data Navigation', () => {
  test('data drawer expands and collapses with current submenu links', async ({ page, request }) => {
    await setupUser(page, request)

    const dataDrawerButton = page.getByRole('button', { name: '数据文件' })
    const dataSubmenu = page.locator('#data-nav-submenu')

    await expect(dataDrawerButton).toHaveAttribute('aria-expanded', 'false')
    await expect(dataSubmenu).toHaveCount(0)

    await dataDrawerButton.click()
    await expect(dataDrawerButton).toHaveAttribute('aria-expanded', 'true')
    await expect(dataSubmenu).toBeVisible()
    await expect(dataSubmenu.getByRole('link', { name: '文件管理器' })).toBeVisible()
    await expect(dataSubmenu.getByRole('link', { name: '项目' })).toBeVisible()
    await expect(dataSubmenu.getByRole('link', { name: '技能' })).toBeVisible()
    await expect(dataSubmenu.getByRole('link', { name: 'Memory' })).toBeVisible()
    await expect(dataSubmenu.getByRole('link', { name: 'Sync' })).toBeVisible()

    await dataDrawerButton.click()
    await expect(dataDrawerButton).toHaveAttribute('aria-expanded', 'false')
    await expect(dataSubmenu).toHaveCount(0)
  })

  test('legacy routes redirect to current pages', async ({ page, request }) => {
    await setupUser(page, request)

    await page.goto('/info')
    await expect(page).toHaveURL(/\/data\/profile/)

    await page.goto('/projects')
    await expect(page).toHaveURL(/\/data\/projects/)

    await page.goto('/collaborations')
    await expect(page).toHaveURL(/\/$/)
  })
})

test.describe('Sidebar Navigation', () => {
  test('current sidebar links render content without blank pages', async ({ page, request }) => {
    await setupUser(page, request)

    const directLinks = [
      { text: '概览', url: /\/$/, heading: '概览', level: 2 },
      { text: '平台连接', url: /\/connections$/, heading: '连接管理', level: 2 },
      { text: 'Token 管理', url: /\/setup\/tokens$/, heading: 'Token 管理', level: 2 },
    ]
    const dataLinks = [
      { text: '文件管理器', url: /\/data\/files$/, heading: '文件管理器', level: 2 },
      { text: '项目', url: /\/data\/projects$/, heading: '项目', level: 2 },
      { text: '技能', url: /\/data\/skills/, heading: '技能', level: 2 },
      { text: 'Memory', url: /\/data\/memory$/, heading: 'Memory', level: 2 },
      { text: 'Sync', url: /\/data\/sync$/, heading: 'Sync', level: 2 },
    ]

    for (const link of directLinks) {
      await page.getByRole('link', { name: link.text }).click()
      await expect(page).toHaveURL(link.url)
      await expect(page.getByRole('heading', { name: link.heading, level: link.level, exact: true })).toBeVisible()
    }

    await page.getByRole('button', { name: '数据文件' }).click()
    await expect(page.locator('#data-nav-submenu')).toBeVisible()

    for (const link of dataLinks) {
      await page.locator('#data-nav-submenu').getByRole('link', { name: link.text }).click()
      await expect(page).toHaveURL(link.url)
      await expect(page.getByRole('heading', { name: link.heading, level: link.level, exact: true })).toBeVisible()
    }
  })
})
