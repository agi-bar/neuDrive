import { test, expect } from '@playwright/test'
import { setupUser } from './helpers'

test.describe('Collaborations Route', () => {
  test('legacy collaborations route redirects to overview', async ({ page, request }) => {
    await setupUser(page, request)
    await page.goto('/collaborations')
    await expect(page).toHaveURL(/\/$/)
    await expect(page.getByRole('heading', { name: '概览' })).toBeVisible()
    await expect(page.locator('.sidebar-brand h1')).toHaveText('neuDrive')
  })
})
