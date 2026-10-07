// @vitest-environment happy-dom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PricingCredentialModelsCard } from '../PricingCredentialModelsCard'

globalThis.IS_REACT_ACT_ENVIRONMENT = true
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))
const prefix = 'pricing_credential_models.'
const subject = { directory_id: 42, subject_id: 'cred_synthetic', name: 'Friendly synthetic name', alias: 'Selected account', provider_type: 'openai', auth_type: 'apikey', endpoint: 'https://synthetic.example', status: 'active', binding_status: 'bound' }
const model = 'synthetic/unpriced-request-model'
const response = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
const dto = (multiplier: number | null, name = model) => ({ subject_id: subject.subject_id, model: name, multiplier, snapshot_id: 'snapshot_synthetic' })
const modelUrl = `/api/v1/pricing/credentials/${subject.subject_id}/model?${new URLSearchParams({ model })}`

describe('PricingCredentialModelsCard', () => {
  let root: Root
  let container: HTMLDivElement
  const button = (key: string) => Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent === prefix + key)!
  const input = () => container.querySelector<HTMLInputElement>(`input[aria-label="${prefix}multiplier"]`)!
  const choose = async (index: number, label: string) => {
    await act(async () => container.querySelectorAll<HTMLElement>('[role="combobox"]')[index].click())
    const option = Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent?.includes(label))!
    expect(option).toBeTruthy()
    await act(async () => option.click())
  }
  const select = async () => { await choose(0, 'Selected account'); await choose(1, model) }
  const enter = async (value: string) => {
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input(), value)
      input().dispatchEvent(new Event('input', { bubbles: true }))
    })
  }
  const setup = (handler: (url: string, init?: RequestInit) => Response | Promise<Response> = () => response(dto(null)), exceptions: ReturnType<typeof dto>[] = []) => {
    const mock = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      const path = String(url)
      if (path.endsWith('credential-subjects')) return response({ credentials: [subject, { ...subject, subject_id: undefined, alias: 'Unregistered' }] })
      if (path.endsWith('/models/used')) return response({ models: [model] })
      if (path.endsWith('/pricing')) return response({ pricing: [{ model: 'synthetic/baseline' }] })
      if (path.endsWith('/models')) return response({ subject_id: subject.subject_id, models: exceptions, snapshot_id: 'snapshot_synthetic' })
      return handler(path, init)
    })
    vi.stubGlobal('fetch', mock)
    return mock
  }
  beforeEach(() => {
    container = document.createElement('div'); document.body.appendChild(container); root = createRoot(container)
  })
  afterEach(async () => {
    await act(async () => root.unmount())
    document.body.innerHTML = ''; vi.unstubAllGlobals()
  })
  it('does not render or fetch management data for a read-only user', async () => {
    const mock = setup()
    await act(async () => root.render(<PricingCredentialModelsCard canManage={false} />))
    expect(container.textContent).toBe(''); expect(mock).not.toHaveBeenCalled()
  })
  it.each([401, 403])('hides controls on denied initial read %s without upstream bodies', async status => {
    vi.stubGlobal('fetch', vi.fn(async () => response({ error: 'synthetic-private-token' }, status)))
    await act(async () => root.render(<PricingCredentialModelsCard />))
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(prefix + 'permission_denied')
    expect(container.querySelector('[role="combobox"]')).toBeNull()
    expect(container.textContent).not.toContain('synthetic-private-token')
  })
  it('selects only registered safe credentials and exact used/catalog/saved models including absent baselines', async () => {
    const mock = setup(() => response(dto(null)), [dto(0.3, 'synthetic/historical')])
    await act(async () => root.render(<PricingCredentialModelsCard />))
    await act(async () => container.querySelector<HTMLElement>('[role="combobox"]')!.click())
    expect(document.body.textContent).not.toContain('Unregistered')
    expect(document.body.textContent).toContain(subject.endpoint)
    await act(async () => Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent?.includes('Selected account'))!.click())
    await act(async () => container.querySelectorAll<HTMLElement>('[role="combobox"]')[1].click())
    for (const name of [model, 'synthetic/baseline', 'synthetic/historical']) expect(document.body.textContent).toContain(name)
    await act(async () => Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent === model)!.click())
    expect(mock.mock.calls.some(([url]) => url === modelUrl)).toBe(true)
    expect(container.textContent).toContain(prefix + 'inherited')
    expect(button('clear').disabled).toBe(true)
  })
  it('saves using PUT and canonical GET readback, refreshes and explicitly deletes to inherit', async () => {
    let saved: number | null = null
    const mock = setup((_url, init) => {
      if (init?.method === 'PUT') { saved = 0.25; return response(dto(0.2)) }
      if (init?.method === 'DELETE') saved = null
      return response(dto(saved))
    })
    await act(async () => root.render(<PricingCredentialModelsCard />)); await select(); await enter('20%')
    await act(async () => button('save').click())
    expect(mock.mock.calls.find(([, init]) => init?.method === 'PUT')?.[1]?.body).toBe(JSON.stringify({ multiplier: '20%' }))
    expect(container.querySelector('output')?.textContent).toBe('0.25'); expect(input().value).toBe('0.25')
    saved = 0
    await act(async () => button('refresh').click())
    expect(container.querySelector('output')?.textContent).toBe('0'); expect(button('clear').disabled).toBe(false)
    await act(async () => button('clear').click())
    expect(mock.mock.calls.some(([, init]) => init?.method === 'DELETE')).toBe(true)
    expect(container.querySelector('output')).toBeNull(); expect(input().value).toBe('')
    expect(container.textContent).toContain(prefix + 'inherited'); expect(container.textContent).toContain(prefix + 'cleared')
  })
  it.each(['0', '1', '.2X', '120%'])('saves active valid grammar %s without computing costs', async value => {
    const mock = setup(() => response(dto(0)))
    await act(async () => root.render(<PricingCredentialModelsCard />)); await select(); await enter(value)
    await act(async () => button('save').click())
    expect(mock.mock.calls.find(([, init]) => init?.method === 'PUT')?.[1]?.body).toBe(JSON.stringify({ multiplier: value }))
    expect(container.querySelector('output')?.textContent).toBe('0'); expect(button('clear').disabled).toBe(false)
  })
  it.each(['', '-1', '+1', '1e2', '20%x', 'NaN', 'Infinity'])('rejects invalid draft %s before mutation', async value => {
    const mock = setup()
    await act(async () => root.render(<PricingCredentialModelsCard />)); await select(); await enter(value)
    await act(async () => button('save').click())
    expect(container.textContent).toContain(prefix + 'invalid_multiplier')
    expect(mock.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false)
  })
  it.each([
    ['PUT', 400, 'invalid_multiplier'], ['PUT', 409, 'conflict'], ['PUT', 500, 'save_failed'], ['PUT', 401, 'permission_denied'], ['PUT', 403, 'permission_denied'],
    ['DELETE', 409, 'conflict'], ['DELETE', 500, 'clear_failed'], ['DELETE', 401, 'permission_denied'], ['DELETE', 403, 'permission_denied'],
  ])('handles mutation %s error %s safely', async (method, status, key) => {
    setup((_url, init) => init?.method === method ? response({ error: 'synthetic-private-token' }, Number(status)) : response(dto(0.3)))
    await act(async () => root.render(<PricingCredentialModelsCard />)); await select(); await enter('1.2x')
    await act(async () => button(method === 'PUT' ? 'save' : 'clear').click())
    expect(container.textContent).toContain(prefix + key); expect(container.textContent).not.toContain('synthetic-private-token')
    expect(container.textContent).not.toContain(prefix + 'saved'); expect(container.textContent).not.toContain(prefix + 'cleared')
    if (key === 'permission_denied') expect(container.querySelector('[role="combobox"]')).toBeNull()
    else { expect(container.querySelector('output')?.textContent).toBe('0.3'); expect(input().value).toBe('1.2x') }
  })
  it.each(['PUT', 'DELETE'])('retains committed %s when GET readback fails', async method => {
    let committed = false
    setup((_url, init) => {
      if (init?.method === method) { committed = true; return response(dto(method === 'PUT' ? 0.2 : null)) }
      return committed ? response({ error: 'synthetic-private-token' }, 500) : response(dto(0.3))
    })
    await act(async () => root.render(<PricingCredentialModelsCard />)); await select(); await enter('20%')
    await act(async () => button(method === 'PUT' ? 'save' : 'clear').click())
    expect(container.textContent).toContain(prefix + 'load_failed')
    expect(input().value).toBe(method === 'PUT' ? '0.2' : '')
    expect(container.textContent).toContain(prefix + (method === 'PUT' ? 'saved' : 'cleared'))
    expect(container.textContent).not.toContain('synthetic-private-token')
  })
  it.each([401, 403, 409, 500])('disables writes on model GET error %s and refresh can recover', async status => {
    let failed = true
    setup(() => failed ? response({ error: 'synthetic-private-token' }, status) : response(dto(0.4)))
    await act(async () => root.render(<PricingCredentialModelsCard />)); await select()
    expect(container.textContent).not.toContain('synthetic-private-token'); expect(container.querySelector('output')).toBeNull()
    if (status === 401 || status === 403) expect(container.querySelector('[role="combobox"]')).toBeNull()
    else {
      expect(button('save').disabled).toBe(true); failed = false
      await act(async () => button('refresh').click())
      expect(container.querySelector('output')?.textContent).toBe('0.4')
    }
  })
  it('aborts late reads on permission revocation and ignores stale response', async () => {
    let resolve!: (value: Response) => void
    const mock = setup(() => new Promise<Response>(done => { resolve = done }))
    await act(async () => root.render(<PricingCredentialModelsCard />)); await select()
    const signal = mock.mock.calls.find(([url]) => url === modelUrl)?.[1]?.signal
    await act(async () => root.render(<PricingCredentialModelsCard canManage={false} />))
    expect(signal?.aborted).toBe(true)
    await act(async () => resolve(response(dto(0.9))))
    expect(container.textContent).toBe('')
  })
})
