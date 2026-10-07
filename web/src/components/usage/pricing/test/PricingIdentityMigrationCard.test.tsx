// @vitest-environment happy-dom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PricingIdentityMigrationCard } from '../PricingIdentityMigrationCard'
import type { PricingCredential, PricingIdentityState } from '@/lib/types'

globalThis.IS_REACT_ACT_ENVIRONMENT = true
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))
const base = '/api/v1/pricing/identity-bindings'
const response = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
const credential = (name: string, subject?: string, status: PricingCredential['binding_status'] = 'bound'): PricingCredential => ({ directory_id: 1, name, subject_id: subject, provider_type: 'openai', auth_type: 'apikey', status: 'active', binding_status: status })
const initial = (): PricingIdentityState => ({
  snapshot_id: 'snapshot_old',
  subjects: [{ ...credential('Historical A', 'cred_a'), status: 'stale', binding_status: 'stale' }, credential('Account B', 'cred_b')],
  directory: [
    { ref: 'selection_new', credential: credential('New account', undefined, 'unbound') },
    { ref: 'selection_occupied', credential: credential('Occupied account', 'cred_b') },
    { ref: 'selection_shared', credential: credential('Shared account', undefined, 'ambiguous') },
    { ref: 'selection_unknown', credential: credential('Unknown account', undefined, 'unknown') },
  ],
  bindings: [{ ref: 'binding_old', subject_id: 'cred_a', enabled: true, credential: { ...credential('Historical A', 'cred_a'), status: 'stale', binding_status: 'stale' } }],
})

describe('PricingIdentityMigrationCard', () => {
  let root: Root
  const onChanged = vi.fn()
  let container: HTMLDivElement
  const button = (key: string, scope: ParentNode = container) => Array.from(scope.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent === `pricing_identity_migration.${key}`)!
  const choose = async (key: string, text: string) => {
    await act(async () => container.querySelector<HTMLElement>(`[aria-label="pricing_identity_migration.${key}"]`)!.click())
    const option = Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent?.includes(text))!
    expect(option).toBeDefined()
    await act(async () => option.click())
  }
  const setup = () => {
    const state = initial()
    const mutations: { method: string; url: string; body: Record<string, unknown> }[] = []
    const fetchMock = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method) {
        const body = JSON.parse(String(init.body))
        mutations.push({ method: init.method, url: String(url), body })
        state.snapshot_id = `snapshot_${mutations.length}`
        if (String(url).endsWith('/migrate')) {
          state.bindings.push({ ref: 'binding_new', subject_id: String(body.subject_id), enabled: true, credential: credential('New account', String(body.subject_id)) })
          state.directory[0].credential = credential('New account', String(body.subject_id))
          return response({ binding_ref: 'binding_new', subject_id: body.subject_id, enabled: true, snapshot_id: state.snapshot_id })
        }
        const binding = state.bindings.find(item => String(url).includes(item.ref))!
        binding.enabled = body.action === 'rebind'
        binding.subject_id = String(body.target_subject_id ?? binding.subject_id)
        binding.credential = { ...binding.credential, subject_id: binding.subject_id, binding_status: binding.enabled ? 'bound' : 'unbound' }
        return response({ binding_ref: binding.ref, subject_id: binding.subject_id, enabled: binding.enabled, snapshot_id: state.snapshot_id })
      }
      return response(state)
    })
    vi.stubGlobal('fetch', fetchMock)
    return { state, mutations, fetchMock }
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
  it('does not fetch or render for read-only access', async () => {
    const { fetchMock } = setup()
    await act(async () => root.render(<PricingIdentityMigrationCard canManage={false} onChanged={onChanged} />))
    expect(container.textContent).toBe(''); expect(fetchMock).not.toHaveBeenCalled()
  })
  it.each([401, 403])('hides management data and does not display upstream error text on denied reads %s', async status => {
    vi.stubGlobal('fetch', vi.fn(async () => response({ error: 'synthetic-private-token' }, status)))
    await act(async () => root.render(<PricingIdentityMigrationCard onChanged={onChanged} />))
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('pricing_identity_migration.permission_denied')
    expect(container.querySelector('[role="combobox"]')).toBeNull()
    expect(container.textContent).not.toContain('synthetic-private-token')
  })
  it('requires safe explicit subject and directory selection, blocks occupied/shared/unknown, then confirms migration and reads canonical state', async () => {
    const { mutations, fetchMock } = setup()
    await act(async () => root.render(<PricingIdentityMigrationCard onChanged={onChanged} />))
    expect(button('migrate').disabled).toBe(true)
    await choose('subject', 'Historical A')
    await act(async () => container.querySelector<HTMLElement>('[aria-label="pricing_identity_migration.directory"]')!.click())
    for (const label of ['Occupied account', 'Shared account', 'Unknown account']) {
      const option = Array.from(document.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent?.includes(label))!
      expect(option.getAttribute('aria-disabled')).toBe('true')
    }
    await act(async () => Array.from(document.querySelectorAll<HTMLElement>('[role="option"]')).find(item => item.textContent?.includes('New account'))!.click())
    expect(onChanged).not.toHaveBeenCalled()
    expect(mutations).toHaveLength(0)
    await act(async () => button('migrate').click())
    expect(document.body.textContent).toContain('pricing_identity_migration.migrate_warning')
    expect(onChanged).not.toHaveBeenCalled()
    expect(mutations).toHaveLength(0)
    await act(async () => Array.from(document.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent === 'common.cancel')!.click())
    expect(onChanged).not.toHaveBeenCalled()
    expect(mutations).toHaveLength(0)
    await act(async () => button('migrate').click())
    await act(async () => button('confirm', document).click())
    expect(mutations).toEqual([{ method: 'POST', url: `${base}/migrate`, body: { subject_id: 'cred_a', directory_ref: 'selection_new', snapshot_id: 'snapshot_old', confirmed: true } }])
    expect(onChanged).toHaveBeenCalledOnce()
    expect(container.querySelector('output')?.textContent).toContain('binding_new · cred_a')
    expect(button('migrate').disabled).toBe(true)
    expect(fetchMock.mock.calls.filter(([, init]) => !init?.method)).toHaveLength(2)
    await choose('binding', 'binding_new')
    expect(container.textContent).toContain('New account')
    await act(async () => root.unmount()); root = createRoot(container)
    await act(async () => root.render(<PricingIdentityMigrationCard onChanged={onChanged} />))
    await choose('binding', 'binding_old')
    expect(container.textContent).toContain('Historical A') // Migration retains the old saved binding.
  })
  it('keeps correction separate, confirms unbind and rebind to a different owner, using opaque references and current expected ownership', async () => {
    const { mutations } = setup()
    await act(async () => root.render(<PricingIdentityMigrationCard onChanged={onChanged} />))
    await choose('binding', 'binding_old')
    await act(async () => button('unbind').click())
    expect(document.body.textContent).toContain('pricing_identity_migration.correction_warning')
    expect(document.body.textContent).toContain('pricing_identity_migration.unbind_detail')
    await act(async () => button('confirm', document).click())
    expect(mutations[0]).toEqual({ method: 'PUT', url: `${base}/binding_old/correction`, body: { expected_subject_id: 'cred_a', action: 'unbind', snapshot_id: 'snapshot_old', confirmed: true } })
    await choose('binding', 'binding_old')
    expect(button('unbind').disabled).toBe(true)
    await choose('target', 'Account B')
    expect(mutations).toHaveLength(1)
    await act(async () => button('rebind').click())
    expect(document.body.textContent).toContain('pricing_identity_migration.rebind_detail')
    await act(async () => button('confirm', document).click())
    expect(mutations[1].body).toEqual({ expected_subject_id: 'cred_a', target_subject_id: 'cred_b', action: 'rebind', snapshot_id: 'snapshot_1', confirmed: true })
    expect(onChanged).toHaveBeenCalledTimes(2)
    expect(container.querySelector('output')?.textContent).toContain('binding_old · cred_b')
  })
  it('refreshes a stale/conflicting selection and requires a new confirmation without retry or raw errors', async () => {
    const s = initial()
    const fetchMock = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method) return response({ error: 'synthetic-private-token' }, 409)
      return response(s)
    })
    vi.stubGlobal('fetch', fetchMock)
    await act(async () => root.render(<PricingIdentityMigrationCard onChanged={onChanged} />))
    await choose('subject', 'Historical A'); await choose('directory', 'New account')
    await act(async () => button('migrate').click()); await act(async () => button('confirm', document).click())
    expect(onChanged).not.toHaveBeenCalled()
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('pricing_identity_migration.conflict')
    expect(container.textContent).not.toContain('synthetic-private-token')
    expect(button('migrate').disabled).toBe(true)
    expect(fetchMock.mock.calls.filter(([, init]) => !!init?.method)).toHaveLength(1)
    expect(fetchMock.mock.calls.filter(([, init]) => !init?.method)).toHaveLength(2)
  })
  it('retains the committed receipt after readback failure and disables obsolete selections until refresh', async () => {
    const s = initial(); let committed = false
    vi.stubGlobal('fetch', vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method) { committed = true; return response({ binding_ref: 'binding_new', subject_id: 'cred_a', enabled: true, snapshot_id: 'snapshot_new' }) }
      return committed ? response({ error: 'synthetic-private-token' }, 500) : response(s)
    }))
    await act(async () => root.render(<PricingIdentityMigrationCard onChanged={onChanged} />))
    await choose('subject', 'Historical A'); await choose('directory', 'New account')
    await act(async () => button('migrate').click()); await act(async () => button('confirm', document).click())
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('pricing_identity_migration.readback_failed')
    expect(onChanged).toHaveBeenCalledOnce()
    expect(container.querySelector('output')?.textContent).toContain('binding_new · cred_a')
    expect(container.textContent).not.toContain('synthetic-private-token')
    expect(button('migrate').disabled).toBe(true); expect(button('unbind').disabled).toBe(true)
  })
  it.each(['mutation', 'readback'])('notifies only for a commit received before %s cancellation', async phase => {
    const pending = Promise.withResolvers<Response>()
    const receipt = { binding_ref: 'binding_new', subject_id: 'cred_a', enabled: true, snapshot_id: 'snapshot_new' }
    let committed = false
    let signal: AbortSignal | undefined
    vi.stubGlobal('fetch', vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method) {
        if (phase === 'mutation') { signal = init.signal as AbortSignal; return pending.promise }
        committed = true; return response(receipt)
      }
      if (committed) { signal = init?.signal as AbortSignal; return pending.promise }
      return response(initial())
    }))
    await act(async () => root.render(<PricingIdentityMigrationCard onChanged={onChanged} />))
    await choose('subject', 'Historical A'); await choose('directory', 'New account')
    await act(async () => button('migrate').click())
    expect(onChanged).not.toHaveBeenCalled()
    await act(async () => button('confirm', document).click())
    expect(onChanged).toHaveBeenCalledTimes(phase === 'readback' ? 1 : 0)
    await act(async () => root.render(<PricingIdentityMigrationCard canManage={false} onChanged={onChanged} />))
    expect(signal?.aborted).toBe(true)
    await act(async () => { pending.resolve(response(phase === 'mutation' ? receipt : initial())); await pending.promise })
    expect(onChanged).toHaveBeenCalledTimes(phase === 'readback' ? 1 : 0)
  })
  it('discards controls and pending confirmation when permissions change', async () => {
    setup()
    await act(async () => root.render(<PricingIdentityMigrationCard onChanged={onChanged} />))
    await choose('subject', 'Historical A'); await choose('directory', 'New account')
    await act(async () => button('migrate').click())
    await act(async () => root.render(<PricingIdentityMigrationCard canManage={false} onChanged={onChanged} />))
    expect(container.textContent).toBe(''); expect(document.querySelector('[role="dialog"]')).toBeNull()
  })
})
