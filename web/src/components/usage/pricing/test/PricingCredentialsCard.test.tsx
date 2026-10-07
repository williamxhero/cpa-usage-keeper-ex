// @vitest-environment happy-dom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PricingCredentialsCard } from '../PricingCredentialsCard'

globalThis.IS_REACT_ACT_ENVIRONMENT = true
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))
const credential = { directory_id: 42, name: 'Same friendly name', alias: 'Selected account', provider_type: 'openai', auth_type: 'apikey', endpoint: 'https://synthetic.example', status: 'active', binding_status: 'unbound' }
const response = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

describe('PricingCredentialsCard', () => {
  let root: Root
  const onChanged = vi.fn()
  let container: HTMLDivElement
  const button = (key: string) => Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent === `pricing_credentials.${key}`)!
  const selectCredential = async () => {
    await act(async () => container.querySelector<HTMLButtonElement>('[role="combobox"]')!.click())
    const option = Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent?.includes('Selected account'))!
    await act(async () => option.click())
  }
  beforeEach(() => {
    onChanged.mockReset()
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })
  afterEach(async () => {
    await act(async () => root.unmount())
    document.body.innerHTML = ''
    vi.unstubAllGlobals()
  })
  it('does not expose or fetch the directory for read-only users', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    await act(async () => root.render(<PricingCredentialsCard canManage={false} onChanged={onChanged} />))
    expect(container.textContent).toBe('')
    expect(fetchMock).not.toHaveBeenCalled()
  })
  it('handles denied directory reads without exposing response secrets or controls', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => response({ error: 'synthetic-private-token' }, 403)))
    await act(async () => root.render(<PricingCredentialsCard onChanged={onChanged} />))
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('pricing_credentials.permission_denied')
    expect(container.querySelector('[role="combobox"]')).toBeNull()
    expect(container.textContent).not.toContain('synthetic-private-token')
  })
  it('shows a safe load failure and can retry through refresh', async () => {
    let failing = true
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => failing ? response({ error: 'synthetic-private-token' }, 500) : response({ credentials: String(input).endsWith('credential-subjects') ? [] : [credential] })))
    await act(async () => root.render(<PricingCredentialsCard onChanged={onChanged} />))
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('pricing_credentials.load_failed')
    expect(container.textContent).not.toContain('synthetic-private-token')
    expect(button('register').disabled).toBe(true)
    failing = false
    await act(async () => button('refresh').click())
    expect(container.querySelector('[role="alert"]')).toBeNull()
    await selectCredential()
    expect(button('register').disabled).toBe(false)
  })
  it.each([
    [409, 'conflict'],
    [500, 'save_failed'],
    [403, 'permission_denied'],
  ])('handles save status %s without displaying response secrets or inventing a subject', async (status, errorKey) => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => init?.method === 'POST'
      ? response({ error: 'synthetic-private-token' }, status)
      : response({ credentials: String(input).endsWith('credential-subjects') ? [] : [credential] }))
    vi.stubGlobal('fetch', fetchMock)
    await act(async () => root.render(<PricingCredentialsCard onChanged={onChanged} />))
    await selectCredential()
    await act(async () => button('register').click())
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(`pricing_credentials.${errorKey}`)
    expect(container.textContent).not.toContain('synthetic-private-token')
    expect(container.querySelector('code')).toBeNull()
    expect(onChanged).not.toHaveBeenCalled()
    expect(container.textContent).not.toContain('pricing_credentials.saved')
    if (status === 403) expect(container.querySelector('[role="combobox"]')).toBeNull()
    else {
      await act(async () => button('refresh').click())
      expect(container.querySelector('[role="alert"]')).toBeNull()
      expect(button('register').disabled).toBe(true)
    }
  })
  it('retains a committed subject and disables rebinding when readback fails', async () => {
    let saved = false
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') {
        saved = true
        return response({ ...credential, subject_id: 'cred_committed', binding_status: 'bound' }, 201)
      }
      if (saved) return response({ error: 'synthetic-private-token' }, 500)
      return response({ credentials: String(input).endsWith('credential-subjects') ? [] : [credential] })
    }))
    await act(async () => root.render(<PricingCredentialsCard onChanged={onChanged} />))
    await selectCredential()
    await act(async () => button('register').click())
    expect(onChanged).toHaveBeenCalledOnce()
    expect(container.textContent).toContain('cred_committed')
    expect(container.textContent).toContain('pricing_credentials.saved')
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('pricing_credentials.load_failed')
    expect(container.textContent).not.toContain('synthetic-private-token')
    await act(async () => container.querySelector<HTMLButtonElement>('[role="combobox"]')!.click())
    expect(document.body.querySelector('[role="option"]')?.getAttribute('aria-disabled')).toBe('true')
    expect(button('register').disabled).toBe(true)
  })
  it.each(['mutation', 'readback'])('notifies only for a commit received before %s cancellation', async phase => {
    const pending = Promise.withResolvers<Response>()
    const saved = { ...credential, subject_id: 'cred_committed', binding_status: 'bound' }
    let committed = false
    let signal: AbortSignal | undefined
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') {
        if (phase === 'mutation') { signal = init.signal as AbortSignal; return pending.promise }
        committed = true; return response(saved)
      }
      if (committed) { signal = init?.signal as AbortSignal; return pending.promise.then(result => result.clone()) }
      return response({ credentials: String(url).endsWith('credential-subjects') ? [] : [credential] })
    }))
    await act(async () => root.render(<PricingCredentialsCard onChanged={onChanged} />))
    await selectCredential()
    expect(onChanged).not.toHaveBeenCalled()
    await act(async () => button('register').click())
    expect(onChanged).toHaveBeenCalledTimes(phase === 'readback' ? 1 : 0)
    await act(async () => root.render(null))
    expect(signal?.aborted).toBe(true)
    await act(async () => { pending.resolve(response(phase === 'mutation' ? saved : { credentials: [saved] })); await pending.promise })
    expect(onChanged).toHaveBeenCalledTimes(phase === 'readback' ? 1 : 0)
  })
  it.each(['throw', 'reject', 'pending'])('does not treat callback %s as a failed registration or wait for refresh', async mode => {
    let committed = false
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      const saved = { ...credential, subject_id: 'cred_committed', binding_status: 'bound' }
      if (init?.method === 'POST') { committed = true; return response(saved) }
      return response({ credentials: String(url).endsWith('credential-subjects') ? (committed ? [saved] : []) : [committed ? saved : credential] })
    }))
    onChanged.mockImplementation(() => {
      if (mode === 'throw') throw new Error('refresh failed')
      return mode === 'reject' ? Promise.reject(new Error('refresh failed')) : new Promise<void>(() => {})
    })
    await act(async () => root.render(<PricingCredentialsCard onChanged={onChanged} />))
    await selectCredential()
    await act(async () => button('register').click())
    expect(onChanged).toHaveBeenCalledOnce()
    expect(container.querySelector('[role="alert"]')).toBeNull()
    expect(container.textContent).toContain('pricing_credentials.saved')
    expect(container.textContent).toContain('cred_committed')
  })
  it('aborts the current refresh when unmounted', async () => {
    const signals: AbortSignal[] = []
    let refreshing = false
    vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (refreshing) {
        signals.push(init!.signal as AbortSignal)
        return new Promise<Response>(() => {})
      }
      return response({ credentials: [] })
    }))
    await act(async () => root.render(<PricingCredentialsCard onChanged={onChanged} />))
    refreshing = true
    const refresh = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent === 'pricing_credentials.refresh')!
    await act(async () => refresh.click())
    await act(async () => root.render(null))
    expect(signals.length).toBe(2)
    expect(signals.every(signal => signal.aborted)).toBe(true)
  })
  it('selects an explicit directory reference, submits, and reads the persistent subject', async () => {
    let saved = false
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (init?.method === 'POST') {
        expect(url).toBe('/api/v1/pricing/credentials')
        expect(JSON.parse(String(init.body))).toEqual({ directory_id: 42 })
        expect(init.credentials).toBe('include')
        expect(new Headers(init.headers).get('X-CPA-Usage-Keeper-Request')).toBe('fetch')
        saved = true
        return response({ ...credential, subject_id: 'cred_synthetic', binding_status: 'bound' }, 201)
      }
      return response({ credentials: url.endsWith('credential-subjects') ? (saved ? [{ ...credential, subject_id: 'cred_synthetic', binding_status: 'bound' }] : []) : [{ ...credential, ...(saved ? { subject_id: 'cred_synthetic', binding_status: 'bound' } : {}) }] })
    })
    vi.stubGlobal('fetch', fetchMock)
    await act(async () => { root.render(<PricingCredentialsCard onChanged={onChanged} />) })
    const selector = container.querySelector<HTMLButtonElement>('[role="combobox"]')!
    expect(selector).not.toBeNull()
    await act(async () => selector.click())
    const option = document.body.querySelector<HTMLElement>('[role="option"][data-value="42"]') ?? Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent?.includes('Selected account'))
    expect(option).toBeDefined()
    await act(async () => option!.click())
    const save = Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent === 'pricing_credentials.register')!
    expect(save.disabled).toBe(false)
    await act(async () => save.click())
    expect(onChanged).toHaveBeenCalledOnce()
    expect(container.textContent).toContain('cred_synthetic')
    expect(container.textContent).toContain('Selected account')
    expect(save.disabled).toBe(true)
    expect(container.textContent).toContain('pricing_credentials.no_pricing_change')
  })
})
