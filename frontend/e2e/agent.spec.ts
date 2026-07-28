import { expect, test } from '@playwright/test'

const catalog = {
  code: '0',
  message: '',
  data: {
    defaultModelId: 'default',
    models: [
      {
        id: 'default',
        displayName: 'Default Model',
        provider: 'mock',
        model: 'mock-1',
        available: true,
        capabilities: ['chat', 'streaming', 'tools'],
      },
      {
        id: 'alt',
        displayName: 'Alt Model',
        provider: 'mock',
        model: 'mock-2',
        available: true,
        capabilities: ['chat', 'streaming'],
      },
    ],
  },
}

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.clear()
  })

  await page.route('**/api/agent/models', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(catalog),
    })
  })

  await page.route('**/api/agent/chat/stream', async (route) => {
    const body = [
      'event: message',
      'data: {"content":"Hel"}',
      '',
      'event: message',
      'data: {"content":"lo from stream"}',
      '',
      'event: done',
      'data: {"modelId":"default"}',
      '',
      '',
    ].join('\n')
    await route.fulfill({
      status: 200,
      headers: {
        'content-type': 'text/event-stream',
        'cache-control': 'no-cache',
      },
      body,
    })
  })

  await page.route('**/api/agent/chat', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: '0',
        message: '',
        data: {
          modelId: 'default',
          message: { role: 'assistant', content: 'json fallback' },
        },
      }),
    })
  })
})

test('navigates chat/models and selects a model', async ({ page }) => {
  await page.goto('/')
  await expect(page).toHaveURL(/\/chat$/)
  await expect(page.getByTestId('chat-view')).toBeVisible()

  await page.getByRole('menuitem', { name: /模型/ }).click()
  await expect(page).toHaveURL(/\/models$/)
  await expect(page.getByTestId('models-view')).toBeVisible()
  await expect(page.getByTestId('model-status-panel')).toBeVisible()
  await expect(page.getByTestId('config-status-note')).toContainText('不是实时网络探测')

  // Element Plus custom namespace classes should appear
  await expect(page.locator('.tael-table, [class*="tael-"]').first()).toBeVisible()

  await page.getByTestId('select-alt').click()
  await expect(page.getByText('Alt Model').first()).toBeVisible()

  await page.getByRole('menuitem', { name: /对话/ }).click()
  await expect(page.getByTestId('model-tag')).toContainText('Alt Model')
})

test('streams assistant reply in a single bubble', async ({ page }) => {
  await page.goto('/chat')
  await expect(page.getByTestId('chat-view')).toBeVisible()

  await page.getByTestId('chat-input').fill('hello agent')
  await page.getByTestId('send-button').click()

  await expect(page.getByTestId('message-list')).toContainText('hello agent')
  await expect(page.getByTestId('message-list')).toContainText('Hello from stream')
  // single assistant bubble
  await expect(page.locator('[data-testid="message-assistant"]')).toHaveCount(1)
})

test('unknown routes redirect to chat', async ({ page }) => {
  await page.goto('/does-not-exist')
  await expect(page).toHaveURL(/\/chat$/)
})
