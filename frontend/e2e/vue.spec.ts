// Legacy scaffold e2e removed; see agent.spec.ts for chat/models flows.
import { expect, test } from '@playwright/test'

test('root redirects to chat shell', async ({ page }) => {
  await page.route('**/api/agent/models', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: '0',
        message: '',
        data: { defaultModelId: 'default', models: [] },
      }),
    })
  })
  await page.goto('/')
  await expect(page).toHaveURL(/\/chat$/)
})
