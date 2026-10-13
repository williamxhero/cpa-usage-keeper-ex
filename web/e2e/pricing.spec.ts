import { expect, test, type Page } from '@playwright/test'
import { SyntheticApi } from './fixtures'

const openAdmin = async (page: Page, api = new SyntheticApi()) => {
  await api.install(page, 'admin')
  await page.goto('/settings')
  await expect(page.getByRole('region', { name: 'Pricing matrix' }).getByRole('table')).toBeVisible()
  return api
}
const fillRates = async (container: ReturnType<Page['getByRole']>, values = ['11', '12', '13', '14']) => {
  for (const [label, value] of [['Ordinary input', values[0]], ['Output', values[1]], ['Cache read', values[2]], ['Cache write', values[3]]]) {
    await container.getByLabel(label).fill(value!)
  }
}

test('admin matrix shows complete, known-zero, and missing rates; edit, save, clear, bulk, and failure remain synthetic', async ({ page }) => {
  const api = await openAdmin(page)
  const matrix = page.getByRole('region', { name: 'Pricing matrix' })
  await expect(matrix.getByRole('columnheader', { name: 'synthetic/complete' })).toBeVisible()
  await expect(matrix.getByRole('columnheader', { name: 'synthetic/zero' })).toBeVisible()
  await expect(matrix.getByRole('columnheader', { name: 'synthetic/partial' })).toBeVisible()
  await expect(matrix.getByText('3', { exact: true })).toBeVisible()
  await expect(matrix.getByText('4', { exact: true })).toBeVisible()
  await expect(matrix.getByText('Not provided').first()).toBeVisible()

  await page.getByRole('button', { name: 'Edit Synthetic Alpha / synthetic/complete' }).click()
  const editor = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Edit complete prices' }) })
  await fillRates(editor)
  await editor.getByRole('button', { name: 'Save prices' }).click()
  await expect.poll(() => api.matrix.find((cell) => cell.canonical_model_id === 'synthetic/complete')?.prompt_price_per_1m).toBe(11)

  await page.getByRole('button', { name: 'Edit Synthetic Alpha / synthetic/complete' }).click()
  const clearEditor = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Edit complete prices' }) })
  await clearEditor.getByRole('button', { name: 'Clear to official inheritance' }).click()
  await expect.poll(() => api.matrix.find((cell) => cell.canonical_model_id === 'synthetic/complete')?.mode).toBe('inherit')

  await page.getByRole('checkbox', { name: 'Select Synthetic Alpha / synthetic/zero' }).check()
  await page.getByRole('checkbox', { name: 'Select Synthetic Alpha / synthetic/partial' }).check()
  await page.getByRole('button', { name: 'Set selected prices' }).click()
  const bulk = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Set prices for 2 cells' }) })
  await fillRates(bulk, ['5', '6', '7', '8'])
  await bulk.getByRole('button', { name: 'Pricing style' }).click()
  await page.getByRole('option', { name: 'OpenAI' }).click()
  await bulk.getByRole('button', { name: 'Save prices' }).click()
  await expect.poll(() => api.matrix.find((cell) => cell.canonical_model_id === 'synthetic/zero')?.prompt_price_per_1m).toBe(5)
  await page.getByRole('checkbox', { name: 'Select Synthetic Alpha / synthetic/zero' }).check()
  await page.getByRole('button', { name: 'Clear selected overrides' }).click()
  await expect.poll(() => api.matrix.find((cell) => cell.canonical_model_id === 'synthetic/zero')?.mode).toBe('inherit')

  api.failNextMutation = true
  await page.getByRole('button', { name: 'Edit Synthetic Alpha / synthetic/partial' }).click()
  const failed = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Edit complete prices' }) })
  await fillRates(failed, ['21', '22', '23', '24'])
  await failed.getByRole('button', { name: 'Save prices' }).click()
  await expect(page.getByText(/Unable to save prices/i)).toBeVisible()
  await expect(page.getByText('raw-body-must-not-render')).toHaveCount(0)
  await api.expectNoUnexpected()
})

test('official refresh requires a separate explicit commit and errors do not expose raw response bodies', async ({ page }) => {
  const api = await openAdmin(page)
  await page.getByRole('button', { name: 'Preview official price refresh' }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Confirm official price refresh' })).toBeVisible()
  expect(api.requests.some((request) => request.path === '/pricing/official/refresh/commit')).toBe(false)
  await page.getByRole('button', { name: 'Confirm official price refresh' }).click()
  await expect.poll(() => api.requests.filter((request) => request.path === '/pricing/official/refresh/commit').length).toBe(1)

  api.failNextMutation = true
  await page.getByRole('button', { name: 'Edit baseline prices' }).click()
  const editor = page.getByRole('dialog')
  await fillRates(editor, ['1', '2', '3', '4'])
  await editor.getByRole('button', { name: 'Save baseline prices' }).click()
  await expect(page.getByText('raw-body-must-not-render')).toHaveCount(0)
  await api.expectNoUnexpected()
})

test('key-pricing is readonly and only reads the viewer matrix endpoint', async ({ page }) => {
  const api = new SyntheticApi()
  await api.install(page, 'api_key_viewer')
  await page.goto('/key-pricing')
  await expect(page.getByRole('region', { name: 'Pricing matrix' }).getByRole('table')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Edit Synthetic Alpha / synthetic/complete' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Preview official price refresh' })).toHaveCount(0)
  await expect(page.getByText('Read-only pricing summary')).toBeVisible()
  expect(api.requests.some((request) => request.method !== 'GET')).toBe(false)
  expect(api.requests.some((request) => request.path === '/pricing/official' || request.path.includes('/refresh/'))).toBe(false)
  expect(api.requests.filter((request) => request.path === '/key/pricing/credential-model-matrix').length).toBeGreaterThan(0)
  await api.expectNoUnexpected()
})

test('matrix 401 clears the session and returns to login', async ({ page }) => {
  const api = await openAdmin(page)
  api.failNextMatrixWith401 = true
  await page.reload()
  await expect(page.locator('[data-keeper-page="login"]')).toBeVisible()
  await api.expectNoUnexpected()
})

for (const [locale, title] of [['en', 'Pricing matrix'], ['zh', '价格矩阵'], ['zh-TW', '價格矩陣']]) {
  test(`pricing matrix renders in ${locale}`, async ({ page }) => {
    const api = new SyntheticApi()
    await api.install(page, 'admin', locale)
    await page.goto('/settings')
    await expect(page.getByRole('heading', { name: title })).toBeVisible()
    await api.expectNoUnexpected()
  })
}

test('matrix scroll area and sticky header work on desktop; mobile can select, edit, save, and clear', async ({ page }) => {
  const api = await openAdmin(page)
  const region = page.getByRole('region', { name: 'Pricing matrix' })
  const scroll = await region.evaluate((node) => ({ overflow: getComputedStyle(node).overflowX, width: node.clientWidth, content: node.scrollWidth }))
  expect(scroll.overflow).toMatch(/auto|scroll/)
  expect(scroll.content).toBeGreaterThan(scroll.width)
  const sticky = await region.locator('thead th').first().evaluate((node) => getComputedStyle(node).position)
  expect(sticky).toBe('sticky')

  await page.setViewportSize({ width: 375, height: 812 })
  const credential = page.getByRole('button', { name: 'Select a credential' })
  await expect(credential).toBeVisible()
  await credential.click()
  await page.getByRole('option', { name: /Synthetic Beta/ }).click()
  await page.getByRole('button', { name: 'Edit Synthetic Beta / synthetic/complete' }).click()
  const editor = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Edit complete prices' }) })
  await fillRates(editor, ['31', '32', '33', '34'])
  await editor.getByRole('button', { name: 'Save prices' }).click()
  await page.getByRole('button', { name: 'Edit Synthetic Beta / synthetic/complete' }).click()
  await page.locator('form').filter({ has: page.getByRole('heading', { name: 'Edit complete prices' }) }).getByRole('button', { name: 'Clear to official inheritance' }).click()
  await api.expectNoUnexpected()
})
