// @vitest-environment happy-dom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PricingCredentialDefaultsCard } from '../PricingCredentialDefaultsCard'

globalThis.IS_REACT_ACT_ENVIRONMENT = true
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))
const subject = { directory_id: 42, subject_id: 'cred_synthetic', name: 'Same friendly name', alias: 'Selected account', provider_type: 'openai', auth_type: 'apikey', endpoint: 'https://synthetic.example', status: 'active', binding_status: 'bound' }
const response = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
const defaultUrl = '/api/v1/pricing/credentials/cred_synthetic/default'
const dto = (multiplier: number | null) => ({ subject_id: subject.subject_id, multiplier, snapshot_id: 'snapshot_synthetic' })

describe('PricingCredentialDefaultsCard', () => {
  let root: Root
  const onChanged = vi.fn()
  let container: HTMLDivElement
  const button = (key: string) => Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent === `pricing_credential_defaults.${key}`)!
  const input = () => container.querySelector<HTMLInputElement>('input[aria-label="pricing_credential_defaults.multiplier"]')!
  const selectSubject = async (name = 'Selected account') => {
    await act(async () => container.querySelector<HTMLElement>('[role="combobox"]')!.click())
    const option = Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent?.includes(name))!
    await act(async () => option.click())
  }
  const enter = async (value: string) => {
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input(), value)
      input().dispatchEvent(new Event('input', { bubbles: true }))
    })
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
  it('does not render or fetch management data for a read-only user', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    await act(async () => root.render(<PricingCredentialDefaultsCard canManage={false} onChanged={onChanged} />))
    expect(container.textContent).toBe('')
    expect(fetchMock).not.toHaveBeenCalled()
  })
  it.each([401, 403])('hides all controls on denied subject-list read %s without showing upstream secrets', async status => {
    vi.stubGlobal('fetch', vi.fn(async () => response({ error: 'synthetic-private-token' }, status)))
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('pricing_credential_defaults.permission_denied')
    expect(container.querySelector('[role="combobox"]')).toBeNull()
    expect(container.querySelector('input')).toBeNull()
    expect(container.textContent).not.toContain('synthetic-private-token')
  })
  it('retries a failed subject-list read and does not invent a configuration', async () => {
    let failing = true
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL) => {
      if (failing) return response({ error: 'synthetic-private-token' }, 500)
      return String(url).endsWith('credential-subjects') ? response({ credentials: [subject] }) : response(dto(null))
    }))
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('pricing_credential_defaults.load_failed')
    expect(button('save').disabled).toBe(true)
    expect(container.textContent).not.toContain('synthetic-private-token')
    failing = false
    await act(async () => button('refresh').click())
    await selectSubject()
    expect(container.querySelector('[role="alert"]')).toBeNull()
    expect(container.textContent).toContain('pricing_credential_defaults.inherited')
  })
  it.each([401, 403, 409, 500])('handles selected-default load error %s without permitting writes', async status => {
    let failing = true
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL) => String(url).endsWith('credential-subjects')
      ? response({ credentials: [subject] }) : failing ? response({ error: 'synthetic-private-token' }, status) : response(dto(0.4))))
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    await selectSubject()
    const key = status === 401 || status === 403 ? 'permission_denied' : status === 409 ? 'conflict' : 'load_failed'
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(`pricing_credential_defaults.${key}`)
    expect(container.textContent).not.toContain('synthetic-private-token')
    expect(container.querySelector('output')).toBeNull()
    if (status === 401 || status === 403) expect(container.querySelector('[role="combobox"]')).toBeNull()
    else {
      expect(button('save').disabled).toBe(true)
      expect(button('clear').disabled).toBe(true)
      failing = false
      await act(async () => button('refresh').click())
      expect(container.querySelector('output')?.textContent).toBe('0.4')
      expect(button('save').disabled).toBe(false)
    }
  })
  it.each([
    ['PUT', 409, 'conflict'], ['PUT', 500, 'save_failed'], ['PUT', 401, 'permission_denied'], ['PUT', 403, 'permission_denied'],
    ['DELETE', 409, 'conflict'], ['DELETE', 500, 'clear_failed'], ['DELETE', 401, 'permission_denied'], ['DELETE', 403, 'permission_denied'],
  ])('handles %s error %s without exposing secrets or a success notice', async (method, status, key) => {
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: [subject] })
      return init?.method === method ? response({ error: 'synthetic-private-token' }, Number(status)) : response(dto(0.3))
    }))
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    await selectSubject()
    await enter('0.2x')
    await act(async () => button(method === 'PUT' ? 'save' : 'clear').click())
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(`pricing_credential_defaults.${key}`)
    expect(container.textContent).not.toContain('synthetic-private-token')
    expect(onChanged).not.toHaveBeenCalled()
    expect(container.textContent).not.toContain('pricing_credential_defaults.saved')
    expect(container.textContent).not.toContain('pricing_credential_defaults.cleared')
    if (key === 'permission_denied') {
      expect(container.querySelector('[role="combobox"]')).toBeNull()
      expect(container.querySelector('output')).toBeNull()
    } else {
      expect(container.querySelector('output')?.textContent).toBe('0.3')
      expect(input().value).toBe('0.2x')
    }
  })
  it.each(['PUT', 'DELETE'])('retains committed %s canonical state when GET readback fails', async method => {
    let committed = false
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: [subject] })
      if (init?.method === method) {
        committed = true
        return response(dto(method === 'PUT' ? 0.2 : null))
      }
      return committed ? response({ error: 'synthetic-private-token' }, 500) : response(dto(0.3))
    }))
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    await selectSubject()
    await enter('20%')
    await act(async () => button(method === 'PUT' ? 'save' : 'clear').click())
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('pricing_credential_defaults.load_failed')
    expect(container.textContent).not.toContain('synthetic-private-token')
    expect(container.textContent).toContain(`pricing_credential_defaults.${method === 'PUT' ? 'saved' : 'cleared'}`)
    expect(input().value).toBe(method === 'PUT' ? '0.2' : '')
    expect(method === 'PUT' ? container.querySelector('output')?.textContent : container.querySelector('output')).toBe(method === 'PUT' ? '0.2' : null)
    expect(onChanged).toHaveBeenCalledOnce()
    if (method === 'DELETE') expect(container.textContent).toContain('pricing_credential_defaults.inherited')
  })
  it('notifies before readback settles and retains the notification if readback is canceled', async () => {
    const pending = Promise.withResolvers<Response>()
    let committed = false
    let signal: AbortSignal | undefined
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: [subject] })
      if (init?.method === 'PUT') { committed = true; return response(dto(.2)) }
      if (committed) { signal = init?.signal as AbortSignal; return pending.promise }
      return response(dto(.3))
    }))
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    await selectSubject(); await enter('20%')
    expect(onChanged).not.toHaveBeenCalled()
    await act(async () => button('save').click())
    expect(onChanged).toHaveBeenCalledOnce()
    await act(async () => root.render(null))
    expect(signal?.aborted).toBe(true)
    await act(async () => { pending.resolve(response(dto(.2))); await pending.promise })
    expect(onChanged).toHaveBeenCalledOnce()
  })
  it('uses the GET readback rather than calculating a local canonical value', async () => {
    let saved = false
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: [subject] })
      if (init?.method === 'PUT') {
        saved = true
        return response(dto(0.2))
      }
      return response(dto(saved ? 0.25 : null))
    }))
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    await selectSubject()
    await enter('20%')
    await act(async () => button('save').click())
    expect(onChanged).toHaveBeenCalledOnce()
    expect(input().value).toBe('0.25')
    expect(container.querySelector('output')?.textContent).toBe('0.25')
  })
  it.each([
    ['0.2', 0.2], ['0.2x', 0.2], ['0.2X', 0.2], ['20%', 0.2], ['1.2x', 1.2], ['120%', 1.2],
    ['.2', 0.2], ['1.', 1], ['0x', 0], ['0%', 0], ['1X', 1],
  ])('submits valid text %s and displays canonical %s as active', async (text, canonical) => {
    let saved = false
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: [subject] })
      if (init?.method === 'PUT') {
        expect(JSON.parse(String(init.body))).toEqual({ multiplier: text })
        saved = true
      }
      return response(dto(saved ? Number(canonical) : null))
    }))
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    await selectSubject()
    await enter(` ${text} `)
    await act(async () => button('save').click())
    expect(input().value).toBe(String(canonical))
    expect(container.querySelector('output')?.textContent).toBe(String(canonical))
    expect(container.textContent).toContain('pricing_credential_defaults.active')
    expect(container.textContent).not.toContain('pricing_credential_defaults.inherited')
  })
  it('selects same-named credentials by safe subject ID, not raw identity or directory reference', async () => {
    const second = { ...subject, subject_id: 'cred_second', directory_id: 43 }
    const fetchMock = vi.fn(async (url: RequestInfo | URL) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: [{ ...subject, lookup_key: 'synthetic-private-token', identity: 'synthetic-raw-identity', api_key: 'synthetic-key' }, second, { ...subject, subject_id: undefined, alias: 'Not registered' }] })
      expect(String(url)).toBe('/api/v1/pricing/credentials/cred_second/default')
      return response({ ...dto(0.7), subject_id: second.subject_id })
    })
    vi.stubGlobal('fetch', fetchMock)
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    await act(async () => container.querySelector<HTMLElement>('[role="combobox"]')!.click())
    const options = Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]'))
    expect(options).toHaveLength(2)
    expect(options[0].textContent).toContain('cred_synthetic')
    expect(options[1].textContent).toContain('cred_second')
    expect(document.body.textContent).not.toMatch(/synthetic-private-token|synthetic-raw-identity|synthetic-key|Not registered/)
    await act(async () => options[1].click())
    expect(container.querySelector('output')?.textContent).toBe('0.7')
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('/usage/identities'))).toBe(false)
  })
  it.each([['0.0000001', 0.0000001], ['1000000000000000000000', 1e21]])('keeps canonical %s editable as plain decimal rather than rejected exponent text', async (text, canonical) => {
    const fetchMock = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: [subject] })
      if (init?.method === 'PUT') expect(JSON.parse(String(init.body))).toEqual({ multiplier: text })
      return response(dto(Number(canonical)))
    })
    vi.stubGlobal('fetch', fetchMock)
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    await selectSubject()
    expect(input().value).toBe(text)
    await act(async () => button('save').click())
    expect(container.textContent).toContain('pricing_credential_defaults.saved')
    expect(container.textContent).not.toContain('pricing_credential_defaults.invalid_multiplier')
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(true)
  })
  it('aborts a pending save on permission revocation and requires fresh selection after permission returns', async () => {
    const pending = Promise.withResolvers<Response>()
    let signal: AbortSignal | undefined
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: [subject] })
      if (init?.method === 'PUT') {
        signal = init.signal as AbortSignal
        return pending.promise
      }
      return response(dto(0.3))
    }))
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    await selectSubject()
    await enter('0.9x')
    await act(async () => button('save').click())
    expect(button('save').disabled).toBe(true)
    expect(button('clear').disabled).toBe(true)
    expect(signal?.aborted).toBe(false)
    await act(async () => root.render(<PricingCredentialDefaultsCard canManage={false} onChanged={onChanged} />))
    expect(signal?.aborted).toBe(true)
    expect(container.textContent).toBe('')
    await act(async () => {
      pending.resolve(response(dto(0.9)))
      await pending.promise
    })
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    expect(container.querySelector('output')).toBeNull()
    expect(button('save').disabled).toBe(true)
    expect(input().value).toBe('')
    expect(onChanged).not.toHaveBeenCalled()
    expect(container.textContent).not.toContain('pricing_credential_defaults.saved')
    await selectSubject()
    expect(container.querySelector('output')?.textContent).toBe('0.3')
  })
  it.each(['GET', 'PUT', 'DELETE'])('aborts pending default %s on unmount and ignores its late response', async method => {
    const pending = Promise.withResolvers<Response>()
    let signal: AbortSignal | undefined
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: [subject] })
      if ((init?.method ?? 'GET') === method) {
        signal = init?.signal as AbortSignal
        return pending.promise
      }
      return response(dto(0.3))
    }))
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    await selectSubject()
    if (method !== 'GET') {
      await enter('0.2x')
      await act(async () => button(method === 'PUT' ? 'save' : 'clear').click())
    }
    expect(signal?.aborted).toBe(false)
    expect(button('save').disabled).toBe(true)
    await act(async () => root.render(null))
    expect(signal?.aborted).toBe(true)
    await act(async () => {
      pending.resolve(response(dto(0.2)))
      await pending.promise
    })
    expect(onChanged).not.toHaveBeenCalled()
    expect(container.textContent).toBe('')
  })
  it('shows a safe network error without overwriting saved state', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: [subject] })
      if (init?.method === 'PUT') throw new Error('synthetic-private-token')
      return response(dto(0.3))
    }))
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    await selectSubject()
    await enter('0.2x')
    await act(async () => button('save').click())
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('pricing_credential_defaults.save_failed')
    expect(container.querySelector('output')?.textContent).toBe('0.3')
    expect(container.textContent).not.toContain('synthetic-private-token')
  })
  it.each([400, 422])('shows a field error for backend unsafe numeric rejection %s and keeps the last saved value', async status => {
    vi.stubGlobal('fetch', vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: [subject] })
      if (init?.method === 'PUT') return response({ error: 'synthetic-private-token' }, status)
      return response(dto(0.3))
    }))
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    await selectSubject()
    await enter('9'.repeat(200))
    await act(async () => button('save').click())
    expect(input().getAttribute('aria-invalid')).toBe('true')
    expect(container.textContent).toContain('pricing_credential_defaults.invalid_multiplier')
    expect(container.textContent).not.toContain('synthetic-private-token')
    expect(container.querySelector('output')?.textContent).toBe('0.3')
    expect(input().value).toBe('9'.repeat(200))
    await enter('0.2x')
    expect(input().getAttribute('aria-invalid')).not.toBe('true')
  })
  it.each([0, 1])('treats %s as active and clears only through explicit DELETE to inheritance', async initial => {
    let current: number | null = initial
    const fetchMock = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: [subject] })
      if (init?.method === 'DELETE') {
        expect(String(url)).toBe(defaultUrl)
        expect(init.body).toBeUndefined()
        expect(init.credentials).toBe('include')
        expect(new Headers(init.headers).get('X-CPA-Usage-Keeper-Request')).toBe('fetch')
        current = null
      }
      return response(dto(current))
    })
    vi.stubGlobal('fetch', fetchMock)
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    await selectSubject()
    expect(input().value).toBe(String(initial))
    expect(container.querySelector('output')?.textContent).toBe(String(initial))
    expect(container.textContent).toContain('pricing_credential_defaults.active')
    expect(container.textContent).not.toContain('pricing_credential_defaults.inherited')
    expect(button('clear').disabled).toBe(false)
    await enter('')
    await act(async () => button('save').click())
    expect(container.querySelector('output')?.textContent).toBe(String(initial))
    await act(async () => button('clear').click())
    expect(container.textContent).toContain('pricing_credential_defaults.inherited')
    expect(container.textContent).toContain('pricing_credential_defaults.cleared')
    expect(container.textContent).toContain('pricing_credential_defaults.history_warning')
    expect(container.querySelector('output')).toBeNull()
    expect(input().value).toBe('')
    expect(input().getAttribute('aria-invalid')).not.toBe('true')
    expect(button('clear').disabled).toBe(true)
    expect(fetchMock.mock.calls.filter(([url, init]) => String(url) === defaultUrl && !init?.method)).toHaveLength(2)
    await act(async () => button('refresh').click())
    expect(container.textContent).toContain('pricing_credential_defaults.inherited')
  })
  it.each(['', '   ', '-1', '-0', '+1', 'NaN', 'Infinity', 'Inf', '1e3', '0x10', '20%x', '0.2x%', '0.2xx', '1 x', '1 .2', 'abc', '1,2', '.', '9'.repeat(310)])('rejects invalid text %j without changing the saved override', async value => {
    const fetchMock = vi.fn(async (url: RequestInfo | URL) => String(url).endsWith('credential-subjects') ? response({ credentials: [subject] }) : response(dto(0.3)))
    vi.stubGlobal('fetch', fetchMock)
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    await selectSubject()
    await enter(value)
    await act(async () => button('save').click())
    expect(input().getAttribute('aria-invalid')).toBe('true')
    expect(container.textContent).toContain('pricing_credential_defaults.invalid_multiplier')
    expect(container.querySelector('output')?.textContent).toBe('0.3')
    expect(fetchMock.mock.calls).toHaveLength(2)
    expect(onChanged).not.toHaveBeenCalled()
    expect(container.textContent).not.toContain('pricing_credential_defaults.saved')
  })
  it('selects a registered subject, saves text, reads the canonical value, and reloads it', async () => {
    let saved = false
    const fetchMock = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('credential-subjects')) return response({ credentials: [subject] })
      expect(String(url)).toBe(defaultUrl)
      if (init?.method === 'PUT') {
        expect(JSON.parse(String(init.body))).toEqual({ multiplier: '20%' })
        expect(init.credentials).toBe('include')
        expect(new Headers(init.headers).get('X-CPA-Usage-Keeper-Request')).toBe('fetch')
        saved = true
      }
      return response(dto(saved ? 0.2 : null))
    })
    vi.stubGlobal('fetch', fetchMock)
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    expect(button('save').disabled).toBe(true)
    await selectSubject()
    expect(container.textContent).toContain('pricing_credential_defaults.inherited')
    expect(input().value).toBe('')
    expect(button('clear').disabled).toBe(true)
    await enter(' 20% ')
    await act(async () => button('save').click())
    expect(input().value).toBe('0.2')
    expect(container.querySelector('output')?.textContent).toBe('0.2')
    expect(container.textContent).toContain('pricing_credential_defaults.active')
    expect(container.textContent).toContain('pricing_credential_defaults.saved')
    expect(container.textContent).toContain('pricing_credential_defaults.history_warning')
    expect(container.textContent).toContain('pricing_credential_defaults.replacement_warning')
    expect(fetchMock.mock.calls.filter(([url, init]) => String(url) === defaultUrl && !init?.method)).toHaveLength(2)
    await act(async () => root.render(null))
    await act(async () => root.render(<PricingCredentialDefaultsCard onChanged={onChanged} />))
    await selectSubject()
    expect(input().value).toBe('0.2')
  })
})
