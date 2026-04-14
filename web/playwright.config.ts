import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,
  retries: 0,
  use: {
    baseURL: process.env.NEUDRIVE_TEST_URL || 'http://localhost:8080',
    // E2E specs currently assert against Chinese copy across the app.
    locale: 'zh-CN',
    headless: true,
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
  ],
})
