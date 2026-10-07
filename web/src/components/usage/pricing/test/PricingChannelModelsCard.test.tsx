// @vitest-environment happy-dom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PricingChannelModelsCard } from '../PricingChannelModelsCard'

globalThis.IS_REACT_ACT_ENVIRONMENT = true
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, options?: { count: number }) => options ? `${key}: ${options.count}` : key }) }))
const prefix = 'pricing_channel_models.'
const channel = { id: 'channel_synthetic', name: 'Selected channel', member_subject_ids: ['cred_one', 'cred_two'], multiplier: null, snapshot_id: 'snapshot_synthetic' }
const model = 'synthetic/unpriced-request-model'
const response = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
const dto = (multiplier: number | null, name = model) => ({ channel_id: channel.id, model: name, multiplier, mode: multiplier === null ? 'inherit' : 'multiplier', snapshot_id: 'snapshot_synthetic' })
const modelUrl = `/api/v1/pricing/channels/${channel.id}/model?${new URLSearchParams({ model })}`
const rateFields = ['prompt_price_per_1m', 'completion_price_per_1m', 'cache_read_price_per_1m', 'cache_write_price_per_1m'] as const
const fixedDto = (factor = 1, style: string | undefined = 'openai', name = model) => ({ ...dto(null, name), mode: 'fixed', fixed: { prompt_price_per_1m: factor, completion_price_per_1m: 2 * factor, cache_read_price_per_1m: 3 * factor, cache_write_price_per_1m: 4 * factor, ...(style ? { pricing_style: style } : {}) } })

describe('PricingChannelModelsCard', () => {
  let root: Root
  const onChanged = vi.fn()
  let container: HTMLDivElement
  const button = (key: string) => Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent === prefix + key)!
  const input = () => container.querySelector<HTMLInputElement>(`input[aria-label="${prefix}multiplier"]`)!
  const choose = async (index: number, label: string) => {
    await act(async () => container.querySelectorAll<HTMLElement>('[role="combobox"]')[index].click())
    const option = Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent?.includes(label))!
    expect(option).toBeTruthy()
    await act(async () => option.click())
  }
  const select = async () => { await choose(0, 'Selected channel'); await choose(1, model) }
  const enter = async (value: string) => {
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input(), value)
      input().dispatchEvent(new Event('input', { bubbles: true }))
    })
  }
  const enterRate = async (field: typeof rateFields[number], value: string) => {
    const element = container.querySelector<HTMLInputElement>(`input[aria-label="${prefix}${field}"]`)!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(element, value)
      element.dispatchEvent(new Event('input', { bubbles: true }))
    })
  }
  const enterRates = async (values = ['1', '2', '3', '4']) => {
    for (const [index, field] of rateFields.entries()) await enterRate(field, values[index])
  }
  const chooseSetting = async (key: 'mode' | 'style', label: string) => {
    await act(async () => container.querySelector<HTMLElement>(`[aria-label="${prefix}${key}"]`)!.click())
    const option = Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent?.includes(label))!
    expect(option).toBeTruthy(); await act(async () => option.click())
  }
  const fixedMode = async () => chooseSetting('mode', prefix + 'fixed')
  const setup = (handler: (url: string, init?: RequestInit) => Response | Promise<Response> = () => response(dto(null)), exceptions: ReturnType<typeof dto>[] = []) => {
    const mock = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      const path = String(url)
      if (path.endsWith('/channels')) return response({ channels: [channel] })
      if (path.endsWith('/models/used')) return response({ models: [model] })
      if (path.endsWith('/pricing')) return response({ pricing: [{ model: 'synthetic/baseline' }] })
      if (path.endsWith('/models')) return response({ channel_id: channel.id, models: exceptions, snapshot_id: 'snapshot_synthetic' })
      return handler(path, init)
    })
    vi.stubGlobal('fetch', mock)
    return mock
  }
  beforeEach(() => {
    onChanged.mockReset()
    container = document.createElement('div'); document.body.appendChild(container); root = createRoot(container)
  })
  afterEach(async () => {
    await act(async () => root.unmount())
    document.body.innerHTML = ''; vi.unstubAllGlobals()
  })
  it('does not render or fetch management data for a read-only user', async () => {
    const mock = setup()
    await act(async () => root.render(<PricingChannelModelsCard canManage={false} onChanged={onChanged} />))
    expect(container.textContent).toBe(''); expect(mock).not.toHaveBeenCalled()
  })
  it.each([401, 403])('hides controls on denied initial read %s without upstream bodies', async status => {
    vi.stubGlobal('fetch', vi.fn(async () => response({ error: 'synthetic-private-token' }, status)))
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />))
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(prefix + 'permission_denied')
    expect(container.querySelector('[role="combobox"]')).toBeNull()
    expect(container.textContent).not.toContain('synthetic-private-token')
  })
  it('selects saved stable channels and exact used/catalog/saved models including absent baselines', async () => {
    const mock = setup(() => response(dto(null)), [dto(0.3, 'synthetic/historical')])
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />))
    await act(async () => container.querySelector<HTMLElement>('[role="combobox"]')!.click())
    expect(document.body.textContent).toContain(channel.id)
    expect(document.body.querySelectorAll('[role="option"]')).toHaveLength(1)
    await act(async () => Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent?.includes('Selected channel'))!.click())
    await act(async () => container.querySelectorAll<HTMLElement>('[role="combobox"]')[1].click())
    for (const name of [model, 'synthetic/baseline', 'synthetic/historical']) expect(document.body.textContent).toContain(name)
    await act(async () => Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent === model)!.click())
    expect(mock.mock.calls.some(([url]) => url === modelUrl)).toBe(true)
    expect(container.textContent).toContain(prefix + 'inherited')
    expect(button('clear').disabled).toBe(true)
  })
  it.each([0, 2])('shows only the saved member count %s and never guesses or manages members', async count => {
    const mock = setup()
    mock.mockImplementationOnce(async () => response({ channels: [{ ...channel, member_subject_ids: channel.member_subject_ids.slice(0, count) }] }))
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await choose(0, 'Selected channel')
    expect(container.textContent).toContain(`${prefix}members_help: ${count}`)
    expect(container.textContent).not.toContain('cred_one'); expect(container.textContent).not.toContain('cred_two')
    expect(container.querySelector('input[aria-label="pricing_channels.name"]')).toBeNull()
    expect(mock.mock.calls.some(([url]) => String(url).includes('credential-subjects'))).toBe(false)
    expect(mock.mock.calls.every(([, init]) => !init?.method)).toBe(true)
  })
  it('has an empty channel state without inventing a channel or enabling writes', async () => {
    const mock = setup()
    mock.mockImplementationOnce(async () => response({ channels: [] }))
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />))
    expect(container.textContent).toContain(prefix + 'empty')
    expect(button('save').disabled).toBe(true); expect(button('clear').disabled).toBe(true)
    expect(container.querySelector<HTMLButtonElement>(`[aria-label="${prefix}model"]`)?.disabled).toBe(true)
    expect(mock.mock.calls.some(([url]) => String(url).includes('/model?'))).toBe(false)
  })
  it('saves using PUT and canonical GET readback, refreshes and explicitly deletes to inherit', async () => {
    let saved: number | null = null
    const mock = setup((_url, init) => {
      if (init?.method === 'PUT') { saved = 0.25; return response(dto(0.2)) }
      if (init?.method === 'DELETE') saved = null
      return response(dto(saved))
    })
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select(); await enter('20%')
    await act(async () => button('save').click())
    expect(mock.mock.calls.find(([, init]) => init?.method === 'PUT')?.[1]?.body).toBe(JSON.stringify({ mode: 'multiplier', multiplier: '20%' }))
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
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select(); await enter(value)
    await act(async () => button('save').click())
    expect(mock.mock.calls.find(([, init]) => init?.method === 'PUT')?.[1]?.body).toBe(JSON.stringify({ mode: 'multiplier', multiplier: value }))
    expect(container.querySelector('output')?.textContent).toBe('0'); expect(button('clear').disabled).toBe(false)
  })
  it.each(['', '-1', '+1', '1e2', '20%x', 'NaN', 'Infinity'])('rejects invalid draft %s before mutation', async value => {
    const mock = setup()
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select(); await enter(value)
    await act(async () => button('save').click())
    expect(container.textContent).toContain(prefix + 'invalid_multiplier')
    expect(mock.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false)
  })
  it.each([
    ['PUT', 400, 'invalid_multiplier'], ['PUT', 422, 'invalid_multiplier'], ['PUT', 409, 'conflict'], ['PUT', 500, 'save_failed'], ['PUT', 401, 'permission_denied'], ['PUT', 403, 'permission_denied'],
    ['DELETE', 409, 'conflict'], ['DELETE', 500, 'clear_failed'], ['DELETE', 401, 'permission_denied'], ['DELETE', 403, 'permission_denied'],
  ])('handles mutation %s error %s safely', async (method, status, key) => {
    setup((_url, init) => init?.method === method ? response({ error: 'synthetic-private-token' }, Number(status)) : response(dto(0.3)))
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select(); await enter('1.2x')
    await act(async () => button(method === 'PUT' ? 'save' : 'clear').click())
    expect(container.textContent).toContain(prefix + key); expect(container.textContent).not.toContain('synthetic-private-token')
    expect(onChanged).not.toHaveBeenCalled()
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
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select(); await enter('20%')
    await act(async () => button(method === 'PUT' ? 'save' : 'clear').click())
    expect(container.textContent).toContain(prefix + 'load_failed')
    expect(input().value).toBe(method === 'PUT' ? '0.2' : '')
    expect(onChanged).toHaveBeenCalledOnce()
    expect(container.textContent).toContain(prefix + (method === 'PUT' ? 'saved' : 'cleared'))
    expect(container.textContent).not.toContain('synthetic-private-token')
  })
  it.each([401, 403, 409, 500])('disables writes on model GET error %s and refresh can recover', async status => {
    let failed = true
    setup(() => failed ? response({ error: 'synthetic-private-token' }, status) : response(dto(0.4)))
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select()
    expect(container.textContent).not.toContain('synthetic-private-token'); expect(container.querySelector('output')).toBeNull()
    if (status === 401 || status === 403) expect(container.querySelector('[role="combobox"]')).toBeNull()
    else {
      expect(button('save').disabled).toBe(true); failed = false
      await act(async () => button('refresh').click())
      expect(container.querySelector('output')?.textContent).toBe('0.4')
    }
  })
  it('saves the complete fixed target only, then uses canonical GET rates and style', async () => {
    let committed = false
    const mock = setup((_url, init) => {
      if (init?.method === 'PUT') { committed = true; return response(fixedDto(1)) }
      return response(committed ? fixedDto(2, 'claude') : dto(null))
    })
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select()
    await enter('synthetic-private-token'); await fixedMode(); await enterRates([' 1 ', '2.00', '.3', '4'])
    await chooseSetting('style', 'OpenAI')
    await act(async () => button('save').click())
    expect(mock.mock.calls.find(([, init]) => init?.method === 'PUT')?.[1]?.body).toBe(JSON.stringify({ mode: 'fixed', fixed: { prompt_price_per_1m: '1', completion_price_per_1m: '2.00', cache_read_price_per_1m: '.3', cache_write_price_per_1m: '4', pricing_style: 'openai' } }))
    for (const [index, field] of rateFields.entries()) {
      expect(container.querySelector(`output[aria-label="${prefix}${field}"]`)?.textContent).toBe(String(2 * (index + 1)))
      expect(container.querySelector<HTMLInputElement>(`input[aria-label="${prefix}${field}"]`)?.value).toBe(String(2 * (index + 1)))
    }
    expect(container.textContent).toContain('claude'); expect(container.textContent).toContain(prefix + 'saved')
    expect(container.textContent).not.toContain('synthetic-private-token')
  })
  it('ignores invalid hidden fixed drafts when switching back to multiplier', async () => {
    const mock = setup(() => response(dto(0.2)))
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select(); await fixedMode()
    await enterRate('prompt_price_per_1m', 'NaN')
    await chooseSetting('mode', prefix + 'multiplier'); await enter('20%')
    await act(async () => button('save').click())
    expect(mock.mock.calls.find(([, init]) => init?.method === 'PUT')?.[1]?.body).toBe(JSON.stringify({ mode: 'multiplier', multiplier: '20%' }))
    expect(container.querySelector('output')?.textContent).toBe('0.2')
    expect(container.textContent).not.toContain(prefix + 'invalid_fixed')
  })
  it.each(rateFields.flatMap(field => ['', '-1', 'NaN', 'Infinity', '1e2', '2x', '20%', '+1'].map(value => [field, value] as const)))('rejects incomplete or invalid fixed draft %s=%s without a mutation', async (field, value) => {
    const mock = setup()
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select(); await fixedMode()
    await enterRates(); await chooseSetting('style', 'OpenAI'); await enterRate(field, value)
    await act(async () => button('save').click())
    expect(container.textContent).toContain(prefix + 'invalid_fixed')
    expect(mock.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false)
  })
  it('requires explicit supported fallback without a configured-model baseline, and saves all four zeros as active', async () => {
    let committed = false
    const mock = setup((_url, init) => {
      if (init?.method === 'PUT') committed = true
      return response(committed ? fixedDto(0, 'claude') : dto(null))
    })
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select(); await fixedMode(); await enterRates(['0', '0', '0', '0'])
    await act(async () => button('save').click())
    expect(container.textContent).toContain(prefix + 'style_required')
    expect(mock.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false)
    await chooseSetting('style', 'Claude'); await act(async () => button('save').click())
    expect(container.textContent).not.toContain(prefix + 'inherited')
    expect(button('clear').disabled).toBe(false)
    expect(container.querySelectorAll('output').length).toBe(4)
    for (const output of container.querySelectorAll('output')) expect(output.textContent).toBe('0')
  })
  it('can omit fallback when the configured model has a baseline and does not invent a style', async () => {
    const baseline = 'synthetic/baseline'
    const mock = setup(() => response(dto(null, baseline)))
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await choose(0, 'Selected channel'); await choose(1, baseline)
    await fixedMode(); await enterRates(); await act(async () => button('save').click())
    expect(mock.mock.calls.find(([, init]) => init?.method === 'PUT')?.[1]?.body).toBe(JSON.stringify({ mode: 'fixed', fixed: { prompt_price_per_1m: '1', completion_price_per_1m: '2', cache_read_price_per_1m: '3', cache_write_price_per_1m: '4' } }))
    expect(container.textContent).not.toContain(prefix + 'style_required')
  })
  it.each([[400, 'invalid_fixed'], [422, 'invalid_fixed'], [409, 'conflict'], [500, 'save_failed'], [401, 'permission_denied'], [403, 'permission_denied']] as const)('handles fixed save error %s without exposing private bodies', async (status, key) => {
    setup((_url, init) => init?.method === 'PUT' ? response({ error: 'synthetic-private-token' }, status) : response(fixedDto()))
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select(); await enterRate('cache_write_price_per_1m', '9')
    await act(async () => button('save').click())
    expect(container.textContent).toContain(prefix + key); expect(container.textContent).not.toContain('synthetic-private-token')
    expect(onChanged).not.toHaveBeenCalled()
    expect(container.textContent).not.toContain(prefix + 'saved')
    if (key === 'permission_denied') expect(container.querySelector('[role="combobox"]')).toBeNull()
    else {
      expect(container.querySelector(`output[aria-label="${prefix}cache_write_price_per_1m"]`)?.textContent).toBe('4')
      expect(container.querySelector<HTMLInputElement>(`input[aria-label="${prefix}cache_write_price_per_1m"]`)?.value).toBe('9')
    }
  })
  it('retains committed fixed canonical state after failed GET readback and refresh recovers', async () => {
    let committed = false; let failure = true
    setup((_url, init) => {
      if (init?.method === 'PUT') { committed = true; return response(fixedDto(2)) }
      return committed && failure ? response({ error: 'synthetic-private-token' }, 500) : response(fixedDto(committed ? 3 : 1))
    })
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select()
    await act(async () => button('save').click())
    expect(onChanged).toHaveBeenCalledOnce()
    expect(container.textContent).toContain(prefix + 'load_failed'); expect(container.textContent).toContain(prefix + 'saved')
    expect(container.querySelector<HTMLInputElement>(`input[aria-label="${prefix}prompt_price_per_1m"]`)?.value).toBe('2')
    failure = false; await act(async () => button('refresh').click())
    expect(container.querySelector<HTMLInputElement>(`input[aria-label="${prefix}prompt_price_per_1m"]`)?.value).toBe('3')
    expect(container.textContent).not.toContain('synthetic-private-token')
  })
  it('includes fixed-only historical identifiers in exact model choices', async () => {
    const historical = 'synthetic/historical-fixed-only'
    setup(() => response(fixedDto(1, 'openai', historical)), [fixedDto(1, 'openai', historical)])
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await choose(0, 'Selected channel'); await choose(1, historical)
    expect(container.textContent).toContain(historical); expect(button('clear').disabled).toBe(false)
  })
  it('notifies before readback settles and retains the notification if readback is canceled', async () => {
    const pending = Promise.withResolvers<Response>()
    let committed = false
    let signal: AbortSignal | undefined
    setup((_url, init) => {
      if (init?.method === 'PUT') { committed = true; return response(fixedDto(2)) }
      if (committed) { signal = init?.signal as AbortSignal; return pending.promise }
      return response(fixedDto())
    })
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select()
    await enterRate('prompt_price_per_1m', '2')
    expect(onChanged).not.toHaveBeenCalled()
    await act(async () => button('save').click())
    expect(onChanged).toHaveBeenCalledOnce()
    await act(async () => root.render(null))
    expect(signal?.aborted).toBe(true)
    await act(async () => { pending.resolve(response(fixedDto(2))); await pending.promise })
    expect(onChanged).toHaveBeenCalledOnce()
  })
  it('aborts a fixed mutation on unmount and ignores its late successful response', async () => {
    let resolve!: (value: Response) => void
    const mock = setup((_url, init) => init?.method === 'PUT' ? new Promise<Response>(done => { resolve = done }) : response(fixedDto()))
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select(); await act(async () => button('save').click())
    const signal = mock.mock.calls.find(([, init]) => init?.method === 'PUT')?.[1]?.signal
    await act(async () => root.unmount())
    expect(signal?.aborted).toBe(true)
    await act(async () => resolve(response(fixedDto(2))))
    expect(onChanged).not.toHaveBeenCalled()
    expect(container.textContent).toBe('')
    root = createRoot(container)
  })
  it('reads an active fixed tariff and clears it despite a null multiplier', async () => {
    let cleared = false
    const fixed = { ...dto(null), mode: 'fixed', fixed: { prompt_price_per_1m: 0, completion_price_per_1m: 2, cache_read_price_per_1m: 3, cache_write_price_per_1m: 4, pricing_style: 'openai' } }
    const mock = setup((_url, init) => {
      if (init?.method === 'DELETE') cleared = true
      return response(cleared ? { ...dto(null), mode: 'inherit' } : fixed)
    }, [fixed])
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select()
    expect(container.textContent).toContain(prefix + 'fixed')
    expect(container.textContent).not.toContain(prefix + 'inherited')
    expect(container.querySelector(`output[aria-label="${prefix}prompt_price_per_1m"]`)?.textContent).toBe('0')
    expect(button('clear').disabled).toBe(false)
    await act(async () => button('clear').click())
    expect(mock.mock.calls.some(([, init]) => init?.method === 'DELETE')).toBe(true)
    expect(container.textContent).toContain(prefix + 'inherited')
    expect(button('clear').disabled).toBe(true)
  })
  it('preserves confirmed rates and drafts when refresh GET fails', async () => {
    let failure = false
    setup(() => failure ? response({ error: 'synthetic-private-token' }, 500) : response(fixedDto()))
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select()
    await enterRate('cache_write_price_per_1m', '9'); failure = true
    await act(async () => button('refresh').click())
    expect(container.querySelector(`output[aria-label="${prefix}cache_write_price_per_1m"]`)?.textContent).toBe('4')
    expect(container.querySelector<HTMLInputElement>(`input[aria-label="${prefix}cache_write_price_per_1m"]`)?.value).toBe('9')
    expect(container.textContent).toContain(prefix + 'load_failed'); expect(container.textContent).not.toContain('synthetic-private-token')
  })
  it.each([401, 403])('clears canonical data and drafts when committed readback denies access %s', async status => {
    let committed = false
    setup((_url, init) => {
      if (init?.method === 'PUT') { committed = true; return response(fixedDto(2)) }
      return committed ? response({ error: 'synthetic-private-token' }, status) : response(fixedDto())
    })
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select(); await act(async () => button('save').click())
    expect(container.querySelector('[role="combobox"]')).toBeNull(); expect(container.querySelector('input')).toBeNull()
    expect(container.querySelector('output')).toBeNull(); expect(container.textContent).not.toContain(channel.id)
    expect(container.textContent).not.toContain(model); expect(container.textContent).not.toContain(prefix + 'saved')
    expect(container.textContent).toContain(prefix + 'permission_denied'); expect(container.textContent).not.toContain('synthetic-private-token')
  })
  it('switches models without retaining previous fixed drafts or writing to the previous model', async () => {
    const baseline = 'synthetic/baseline'
    const mock = setup((url, init) => {
      const name = new URL(url, 'http://localhost').searchParams.get('model')!
      if (init?.method === 'PUT') return response(dto(0.4, name))
      return response(name === model ? fixedDto() : dto(null, name))
    })
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select()
    await enterRate('cache_write_price_per_1m', '9'); await choose(1, baseline)
    expect(input().value).toBe(''); expect(container.querySelector('output')).toBeNull()
    await enter('40%'); await act(async () => button('save').click())
    const put = mock.mock.calls.find(([, init]) => init?.method === 'PUT')!
    expect(new URL(String(put[0]), 'http://localhost').searchParams.get('model')).toBe(baseline)
    expect(put[1]?.body).toBe(JSON.stringify({ mode: 'multiplier', multiplier: '40%' }))
  })
  it('switches saved channel IDs and resets exceptions, model selection and drafts', async () => {
    const second = { ...channel, id: 'channel_second', name: 'Second channel', member_subject_ids: [] }
    const mock = setup((url) => response({ ...fixedDto(), channel_id: new URL(url, 'http://localhost').pathname.includes(second.id) ? second.id : channel.id }), [fixedDto(1, 'openai', 'synthetic/historical')])
    mock.mockImplementationOnce(async () => response({ channels: [channel, second] }))
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select()
    await enterRate('cache_write_price_per_1m', '9')
    mock.mockImplementationOnce(async () => response({ channel_id: second.id, models: [], snapshot_id: 'snapshot_synthetic' }))
    await choose(0, second.name)
    expect(container.querySelector('output')).toBeNull(); expect(input().value).toBe('')
    expect(button('save').disabled).toBe(true); expect(container.textContent).toContain(`${prefix}members_help: 0`)
    await act(async () => container.querySelector<HTMLElement>(`[aria-label="${prefix}model"]`)!.click())
    expect(document.body.textContent).not.toContain('synthetic/historical')
    await act(async () => Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent === model)!.click())
    await act(async () => button('save').click())
    const put = mock.mock.calls.find(([, init]) => init?.method === 'PUT')!
    expect(new URL(String(put[0]), 'http://localhost').pathname).toBe(`/api/v1/pricing/channels/${second.id}/model`)
  })
  it('clears a removed selected channel on refresh and disables mutations', async () => {
    const mock = setup(() => response(fixedDto()))
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select()
    mock.mockImplementationOnce(async () => response({ channels: [] }))
    await act(async () => button('refresh').click())
    expect(container.querySelector('output')).toBeNull(); expect(input().value).toBe('')
    expect(button('save').disabled).toBe(true); expect(button('clear').disabled).toBe(true)
    expect(container.textContent).not.toContain(channel.id)
  })
  it('aborts late reads on permission revocation and ignores stale response', async () => {
    let resolve!: (value: Response) => void
    const mock = setup(() => new Promise<Response>(done => { resolve = done }))
    await act(async () => root.render(<PricingChannelModelsCard onChanged={onChanged} />)); await select()
    const signal = mock.mock.calls.find(([url]) => url === modelUrl)?.[1]?.signal
    await act(async () => root.render(<PricingChannelModelsCard canManage={false} onChanged={onChanged} />))
    expect(signal?.aborted).toBe(true)
    await act(async () => resolve(response(dto(0.9))))
    expect(container.textContent).toBe('')
  })
})
