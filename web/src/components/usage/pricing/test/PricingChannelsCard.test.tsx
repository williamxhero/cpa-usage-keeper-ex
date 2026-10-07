// @vitest-environment happy-dom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PricingChannelsCard } from '../PricingChannelsCard'
import type { PricingChannel } from '@/lib/types'

globalThis.IS_REACT_ACT_ENVIRONMENT = true
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))
const response = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
const subjects = ['a', 'b', 'shared'].map(id => ({ directory_id: id, subject_id: `cred_${id}`, name: `OpenAI ${id}`, provider_type: 'openai', auth_type: 'apikey', status: 'active', binding_status: id === 'shared' ? 'ambiguous' : 'bound' }))
const channel = (multiplier: number | null = .2): PricingChannel => ({ id: 'chan_a', name: 'Channel A', member_subject_ids: ['cred_a'], multiplier, snapshot_id: 'snapshot_synthetic' })
const base = '/api/v1/pricing/channels'

describe('PricingChannelsCard', () => {
  let root: Root
  const onChanged = vi.fn()
  let container: HTMLDivElement
  const button = (key: string, scope: ParentNode = container) => Array.from(scope.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent === `pricing_channels.${key}`)!
  const input = (key: string) => container.querySelector<HTMLInputElement>(`input[aria-label="pricing_channels.${key}"]`)!
  const enter = async (key: string, text: string) => {
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input(key), text)
      input(key).dispatchEvent(new Event('input', { bubbles: true }))
    })
  }
  const choose = async (key: string, text: string) => {
    await act(async () => container.querySelector<HTMLElement>(`[aria-label="pricing_channels.${key}"]`)!.click())
    const option = Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent?.includes(text))!
    expect(option).toBeDefined()
    await act(async () => option.click())
  }
  const selectChannel = () => choose('select', 'Channel A')
  const mockSaved = (saved = channel()) => {
    const fetchMock = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: subjects })
      if (String(url) === base) return response({ channels: [saved] })
      if (String(url).endsWith('/default') && init?.method === 'DELETE') return response({ ...saved, multiplier: null })
      return response(saved)
    })
    vi.stubGlobal('fetch', fetchMock)
    return fetchMock
  }
  beforeEach(() => {
    onChanged.mockReset()
    container = document.createElement('div'); document.body.appendChild(container); root = createRoot(container)
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  })
  afterEach(async () => {
    await act(async () => root.unmount())
    document.body.innerHTML = ''; vi.unstubAllGlobals(); vi.restoreAllMocks()
  })
  it('does not render or fetch management data for read-only access', async () => {
    const fetchMock = mockSaved()
    await act(async () => root.render(<PricingChannelsCard canManage={false} onChanged={onChanged} />))
    expect(container.textContent).toBe(''); expect(fetchMock).not.toHaveBeenCalled()
  })
  it.each([401, 403])('hides all sensitive controls on denied reads %s', async status => {
    vi.stubGlobal('fetch', vi.fn(async () => response({ error: 'synthetic-secret' }, status)))
    await act(async () => root.render(<PricingChannelsCard onChanged={onChanged} />))
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('pricing_channels.permission_denied')
    expect(container.querySelector('input')).toBeNull(); expect(container.querySelector('[role="combobox"]')).toBeNull()
    expect(container.textContent).not.toContain('synthetic-secret')
  })
  it('creates two independent same-provider channels through explicit safe member selection, renames, refreshes and reloads', async () => {
    const saved: PricingChannel[] = []
    const mutations: unknown[] = []
    const fetchMock = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: subjects })
      if (String(url) === base && init?.method === 'POST') {
        const data = JSON.parse(String(init.body)); mutations.push(data)
        const item = { ...data, id: `chan_${saved.length}`, multiplier: null, snapshot_id: 'snapshot_synthetic' }
        saved.push(item); return response(item)
      }
      if (String(url) === base) return response({ channels: saved })
      const item = saved.find(row => String(url).endsWith(row.id))!
      if (init?.method === 'PUT') { mutations.push(JSON.parse(String(init.body))); Object.assign(item, JSON.parse(String(init.body))) }
      return response(item)
    })
    vi.stubGlobal('fetch', fetchMock)
    await act(async () => root.render(<PricingChannelsCard onChanged={onChanged} />))
    expect(container.querySelectorAll('li')).toHaveLength(0)
    expect(button('add_member').disabled).toBe(true)
    await enter('name', 'Channel A')
    await choose('member_select', 'OpenAI a')
    expect(container.querySelectorAll('li')).toHaveLength(0) // Selection is not membership until Add.
    await act(async () => button('add_member').click())
    await act(async () => button('create').click())
    expect(mutations[0]).toEqual({ name: 'Channel A', member_subject_ids: ['cred_a'] })
    expect(container.textContent).toContain('chan_0')
    await choose('select', 'pricing_channels.new')
    await enter('name', 'Channel B')
    await act(async () => container.querySelector<HTMLElement>('[aria-label="pricing_channels.member_select"]')!.click())
    for (const text of ['OpenAI a', 'OpenAI shared']) {
      const option = Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent?.includes(text))!
      expect(option.getAttribute('aria-disabled')).toBe('true')
    }
    const b = Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent?.includes('OpenAI b'))!
    await act(async () => b.click())
    await act(async () => button('add_member').click())
    await act(async () => button('create').click())
    expect(mutations[1]).toEqual({ name: 'Channel B', member_subject_ids: ['cred_b'] })
    await enter('name', 'Renamed B')
    await act(async () => button('save_channel').click())
    expect(saved[1].id).toBe('chan_1'); expect(saved[1].name).toBe('Renamed B'); expect(saved[1].member_subject_ids).toEqual(['cred_b'])
    await act(async () => button('refresh').click())
    expect(onChanged).toHaveBeenCalledTimes(3)
    expect(input('name').value).toBe('Renamed B')
    await act(async () => root.unmount()); root = createRoot(container)
    await act(async () => root.render(<PricingChannelsCard onChanged={onChanged} />))
    await choose('select', 'Renamed B')
    expect(input('name').value).toBe('Renamed B'); expect(container.textContent).toContain('cred_b')
    expect(fetchMock.mock.calls.some(([url, init]) => String(url) === `${base}/chan_1` && !init?.method)).toBe(true)
  })
  it('removes members explicitly and requires a nonblank name without guessing memberships', async () => {
    const fetchMock = mockSaved()
    await act(async () => root.render(<PricingChannelsCard onChanged={onChanged} />)); await selectChannel()
    await act(async () => container.querySelector<HTMLButtonElement>('[aria-label="pricing_channels.remove_member cred_a"]')!.click())
    expect(container.querySelectorAll('li')).toHaveLength(0)
    await enter('name', '  '); await act(async () => button('save_channel').click())
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('pricing_channels.invalid_name')
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false)
    await enter('name', 'Empty channel'); await act(async () => button('save_channel').click())
    expect(JSON.parse(String(fetchMock.mock.calls.find(([, init]) => init?.method === 'PUT')![1]!.body))).toEqual({ name: 'Empty channel', member_subject_ids: [] })
  })
  it.each([['0.2', .2], ['0.2x', .2], ['20%', .2], ['0', 0], ['1X', 1], ['120%', 1.2]])('saves %s and displays server canonical active multiplier %s', async (text, canonical) => {
    let saved = channel(null)
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: subjects })
      if (String(url) === base) return response({ channels: [saved] })
      if (init?.method === 'PUT') { expect(JSON.parse(String(init.body))).toEqual({ multiplier: text }); saved = channel(Number(canonical)) }
      return response(saved)
    }))
    await act(async () => root.render(<PricingChannelsCard onChanged={onChanged} />)); await selectChannel(); await enter('multiplier', ` ${text} `)
    await act(async () => button('save_default').click())
    expect(input('multiplier').value).toBe(String(canonical)); expect(container.querySelector('output')?.textContent).toBe(String(canonical))
    expect(container.textContent).toContain('pricing_channels.active'); expect(button('clear').disabled).toBe(false)
  })
  it.each(['', '-1', '+1', '1e2', '20%x', 'NaN', 'Infinity'])('rejects invalid multiplier %s without sending a write', async text => {
    const fetchMock = mockSaved(); await act(async () => root.render(<PricingChannelsCard onChanged={onChanged} />)); await selectChannel()
    await enter('multiplier', text); await act(async () => button('save_default').click())
    expect(container.textContent).toContain('pricing_channels.invalid_multiplier')
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false)
    expect(container.querySelector('output')?.textContent).toBe('0.2')
  })
  it('clears to inheritance and uses real GET readback rather than a local calculation', async () => {
    let saved = channel(.2)
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: subjects })
      if (String(url) === base) return response({ channels: [saved] })
      if (init?.method === 'PUT') { saved = channel(.25); return response(channel(.2)) }
      if (init?.method === 'DELETE') saved = channel(null)
      return response(saved)
    }))
    await act(async () => root.render(<PricingChannelsCard onChanged={onChanged} />)); await selectChannel(); await enter('multiplier', '20%')
    await act(async () => button('save_default').click())
    expect(container.querySelector('output')?.textContent).toBe('0.25')
    await act(async () => button('clear').click())
    expect(input('multiplier').value).toBe(''); expect(container.querySelector('output')).toBeNull()
    expect(container.textContent).toContain('pricing_channels.inherited'); expect(button('clear').disabled).toBe(true)
  })
  it.each(['channel', 'default', 'clear'])('retains committed %s configuration on readback failure', async kind => {
    let committed = false
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: subjects })
      if (String(url) === base) return response({ channels: [channel()] })
      if (init?.method) { committed = true; return response({ ...channel(kind === 'clear' ? null : .3), name: 'Committed name' }) }
      return committed ? response({ error: 'synthetic-secret' }, 500) : response(channel())
    }))
    await act(async () => root.render(<PricingChannelsCard onChanged={onChanged} />)); await selectChannel(); await enter('name', 'Committed name'); await enter('multiplier', '.3')
    await act(async () => button(kind === 'channel' ? 'save_channel' : kind === 'clear' ? 'clear' : 'save_default').click())
    expect(input('name').value).toBe('Committed name'); expect(input('multiplier').value).toBe(kind === 'clear' ? '' : '0.3')
    expect(onChanged).toHaveBeenCalledOnce()
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('pricing_channels.load_failed')
    expect(container.textContent).not.toContain('synthetic-secret')
  })
  it.each([409, 500, 401, 403])('handles mutation error %s safely, without publishing a success', async status => {
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: subjects })
      if (String(url) === base) return response({ channels: [channel()] })
      return init?.method ? response({ error: 'synthetic-secret' }, status) : response(channel())
    }))
    await act(async () => root.render(<PricingChannelsCard onChanged={onChanged} />)); await selectChannel(); await enter('name', 'Renamed')
    await act(async () => button('save_channel').click())
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(`pricing_channels.${status === 409 ? 'conflict' : status === 500 ? 'save_failed' : 'permission_denied'}`)
    expect(onChanged).not.toHaveBeenCalled()
    expect(container.textContent).not.toContain('synthetic-secret'); expect(container.textContent).not.toContain('pricing_channels.saved')
    if (status === 401 || status === 403) expect(container.querySelector('input')).toBeNull()
    else expect(container.querySelector('output')?.textContent).toBe('0.2')
  })
  it.each(['mutation', 'readback'])('notifies only for a commit received before %s cancellation', async phase => {
    const pending = Promise.withResolvers<Response>()
    let committed = false
    let signal: AbortSignal | undefined
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: subjects })
      if (String(url) === base) return response({ channels: [channel()] })
      if (init?.method === 'PUT') {
        if (phase === 'mutation') { signal = init.signal as AbortSignal; return pending.promise }
        committed = true; return response(channel(.3))
      }
      if (committed) { signal = init?.signal as AbortSignal; return pending.promise }
      return response(channel())
    }))
    await act(async () => root.render(<PricingChannelsCard onChanged={onChanged} />)); await selectChannel(); await enter('multiplier', '30%')
    expect(onChanged).not.toHaveBeenCalled()
    await act(async () => button('save_default').click())
    expect(onChanged).toHaveBeenCalledTimes(phase === 'readback' ? 1 : 0)
    await act(async () => root.render(null))
    expect(signal?.aborted).toBe(true)
    await act(async () => { pending.resolve(response(channel(.3))); await pending.promise })
    expect(onChanged).toHaveBeenCalledTimes(phase === 'readback' ? 1 : 0)
  })
  it('sends dependency confirmation only after the explicit deletion dialog is confirmed', async () => {
    const fetchMock = mockSaved(); fetchMock.mockImplementation(async (url, init) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: subjects })
      if (String(url) === base) return response({ channels: [channel()] })
      if (init?.method === 'DELETE') return new Response(null, { status: 204 })
      return response(channel())
    })
    await act(async () => root.render(<PricingChannelsCard onChanged={onChanged} />)); await selectChannel()
    await act(async () => button('delete').click())
    expect(document.body.textContent).toContain('pricing_channels.delete_help')
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'DELETE')).toBe(false)
    const cancel = Array.from(document.body.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent === 'common.cancel')!
    await act(async () => cancel.click())
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'DELETE')).toBe(false)
    await act(async () => button('delete').click())
    await act(async () => button('confirm_delete', document.body).click())
    expect(fetchMock.mock.calls.filter(([, init]) => init?.method === 'DELETE').map(([url]) => String(url))).toEqual([`${base}/chan_a?confirm_dependencies=true`])
    expect(onChanged).toHaveBeenCalledOnce()
    expect(input('name').value).toBe(''); expect(container.textContent).toContain('pricing_channels.deleted')
  })
})
