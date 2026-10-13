// @vitest-environment happy-dom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PricingMatrixCard, type PricingMatrixCardProps } from '../PricingMatrixCard'

const screen = { mobile: false }
vi.mock('react-i18next', () => ({ useTranslation: (namespace = 'translation') => ({ t: (key: string, values?: Record<string, unknown>) => values ? `${namespace}.${key} ${Object.values(values).join(' ')}` : `${namespace}.${key}` }) }))
vi.mock('@/hooks/useMediaQuery', () => ({ useMediaQuery: () => screen.mobile }))
globalThis.IS_REACT_ACT_ENVIRONMENT = true
const subject = (id: string, name: string) => ({ directory_id: 1, subject_id: id, name, alias: name, provider_type: 'openai', auth_type: 'apikey', key_hint: '***safe', status: 'active' as const, binding_status: 'bound' as const })
const credentials = [subject('cred_alpha', 'Synthetic Alpha'), subject('cred_beta', 'Synthetic Beta')]
const models = ['synthetic/complete', 'synthetic/zero', 'synthetic/missing']
const matrix = {
  credentials, models,
  cells: [
    { subject_id: 'cred_alpha', credential: credentials[0], canonical_model_id: models[0], covered: true, mode: 'fixed' as const, pricing_style: 'openai' as const, prompt_price_per_1m: 1, completion_price_per_1m: 2, cache_read_price_per_1m: 3, cache_write_price_per_1m: 4 },
    { subject_id: 'cred_alpha', credential: credentials[0], canonical_model_id: models[1], covered: true, mode: 'fixed' as const, pricing_style: 'claude' as const, prompt_price_per_1m: 0, completion_price_per_1m: 0, cache_read_price_per_1m: 0, cache_write_price_per_1m: 0 },
    { subject_id: 'cred_alpha', credential: credentials[0], canonical_model_id: models[2], covered: true, mode: 'fixed' as const, prompt_price_per_1m: 1, completion_price_per_1m: 2, cache_read_price_per_1m: null, cache_write_price_per_1m: null },
  ], credential_page: 1, credential_page_size: 50, model_page: 1, model_page_size: 50,
  total_credentials: 2, total_models: 3, snapshot_id: 'synthetic_snapshot',
}
const query = { credentialPage: 1, credentialPageSize: 50, modelPage: 1, modelPageSize: 50 }
const baseProps = (overrides: Partial<PricingMatrixCardProps> = {}): PricingMatrixCardProps => ({
  matrix: matrix as unknown as PricingMatrixCardProps['matrix'], query, onQueryChange: vi.fn(), canManage: true,
  onSaveCells: vi.fn(async () => true), ...overrides,
})

describe('PricingMatrixCard', () => {
  let root: Root
  let container: HTMLDivElement
  beforeEach(() => {
    screen.mobile = false
    container = document.createElement('div'); document.body.appendChild(container); root = createRoot(container)
  })
  afterEach(async () => {
    await act(async () => root.unmount())
    document.body.innerHTML = ''
  })
  const render = async (props = baseProps()) => act(async () => root.render(<PricingMatrixCard {...props} />))
  const button = (key: string) => Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent?.includes(`matrix_pricing.${key}`))!
  const click = async (element: HTMLElement) => act(async () => element.click())
  const input = (label: string) => container.querySelector<HTMLInputElement>(`input[aria-label="matrix_pricing.${label}"]`)!
  const enter = async (element: HTMLInputElement, value: string) => act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(element, value)
    element.dispatchEvent(new Event('input', { bubbles: true }))
  })
  const selectCell = async (credential: string, model: string) => click(container.querySelector<HTMLInputElement>(`input[aria-label="matrix_pricing.select_cell ${credential} ${model}"]`)!)

  it('renders a sticky bounded desktop matrix and distinguishes inherited, known zero, and missing segments', async () => {
    await render(baseProps({ canManage: false }))
    expect(container.querySelector('[role="region"]')?.getAttribute('aria-label')).toBe('matrix_pricing.title')
    expect(container.querySelector('thead th')?.className).toContain('credential')
    expect(container.textContent).toContain('matrix_pricing.matrix')
    expect(container.textContent).toContain('matrix_pricing.inherited')
    expect(container.querySelectorAll('output')).toHaveLength(10)
    expect(Array.from(container.querySelectorAll('output')).filter(item => item.textContent === '0')).toHaveLength(4)
    expect(container.querySelector('[role="alert"]')).toBeNull()
    expect(container.querySelectorAll('input[type="checkbox"]')).toHaveLength(0)
    expect(container.textContent).toContain('matrix_pricing.effective_summary_unavailable')
  })

  it('renders readonly summary without any write actions or invoking a writer', async () => {
    const saveCells = vi.fn(async () => true)
    await render(baseProps({ canManage: false, onSaveCells: saveCells, onSync: vi.fn(), onCheckSync: vi.fn() }))
    expect(container.querySelectorAll('button').length).toBeGreaterThan(0)
    expect(container.querySelectorAll('input[type="checkbox"]')).toHaveLength(0)
    expect(container.querySelector('form')).toBeNull()
    expect(container.textContent).not.toContain('matrix_pricing.batch_edit')
    expect(container.textContent).not.toContain('matrix_pricing.sync')
    expect(saveCells).not.toHaveBeenCalled()
  })

  it('validates all four explicit rates and style before sending atomic complete cells', async () => {
    const saveCells = vi.fn(async () => true)
    await render(baseProps({ onSaveCells: saveCells }))
    await selectCell('Synthetic Alpha', models[0])
    await click(button('batch_edit'))
    await enter(input('prompt_price_per_1m'), '0')
    await enter(input('completion_price_per_1m'), '2')
    await enter(input('cache_read_price_per_1m'), '')
    await enter(input('cache_write_price_per_1m'), '4')
    await click(button('save'))
    expect(container.textContent).toContain('matrix_pricing.invalid_prices')
    expect(saveCells).not.toHaveBeenCalled()

    await enter(input('cache_read_price_per_1m'), '3')
    await click(button('save'))
    expect(container.textContent).toContain('matrix_pricing.style_required')
    expect(saveCells).not.toHaveBeenCalled()

    await click(container.querySelector<HTMLElement>('[aria-label="matrix_pricing.style"]')!)
    const style = Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent === 'matrix_pricing.style_openai')!
    await click(style)
    await click(button('save'))
    expect(saveCells).toHaveBeenCalledWith([{
      subject_id: 'cred_alpha', canonical_model_id: models[0], mode: 'fixed', pricing_style: 'openai',
      prompt_price_per_1m: 0, completion_price_per_1m: 2, cache_read_price_per_1m: 3, cache_write_price_per_1m: 4,
    }])
    expect(container.querySelector('form')).toBeNull()
  })

  it('keeps the editor and draft intact after mutation failure', async () => {
    const saveCells = vi.fn(async () => false)
    await render(baseProps({ onSaveCells: saveCells }))
    await selectCell('Synthetic Alpha', models[0]); await click(button('batch_edit'))
    await enter(input('prompt_price_per_1m'), '5'); await enter(input('completion_price_per_1m'), '6'); await enter(input('cache_read_price_per_1m'), '7'); await enter(input('cache_write_price_per_1m'), '8')
    await click(container.querySelector<HTMLElement>('[aria-label="matrix_pricing.style"]')!)
    await click(Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent === 'matrix_pricing.style_claude')!)
    await click(button('save'))
    expect(container.querySelector('form')).not.toBeNull()
    expect(input('prompt_price_per_1m').value).toBe('5')
    expect(container.textContent).toContain('matrix_pricing.save_failed')
  })

  it('clears selected cells atomically with explicit inherit mode', async () => {
    const saveCells = vi.fn(async () => true)
    await render(baseProps({ onSaveCells: saveCells }))
    await selectCell('Synthetic Alpha', models[0]); await click(button('batch_clear'))
    expect(saveCells).toHaveBeenCalledWith([{ subject_id: 'cred_alpha', canonical_model_id: models[0], mode: 'inherit' }])
    expect(container.textContent).toContain('matrix_pricing.cleared')
  })

  it('uses credential-first model cards on narrow viewports and hides the Cartesian table', async () => {
    screen.mobile = true
    await render(baseProps())
    expect(container.querySelector('table')).toBeNull()
    expect(container.querySelector('[aria-label="matrix_pricing.select_credential"]')).not.toBeNull()
    expect(container.textContent).toContain('matrix_pricing.choose_credential')
    await click(container.querySelector<HTMLElement>('[aria-label="matrix_pricing.select_credential"]')!)
    await click(Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent?.includes('Synthetic Alpha'))!)
    expect(container.querySelectorAll('article')).toHaveLength(models.length)
    expect(container.textContent).toContain(models[0])
    expect(container.textContent).toContain('matrix_pricing.model_search')
  })
})
