import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import type { PricingMatrixCellInput, PricingOfficialInput } from '../types'

const headerValue = (init: RequestInit | undefined, name: string) => new Headers(init?.headers).get(name)
const rates = { pricing_style: 'claude' as const, prompt_price_per_1m: 0, completion_price_per_1m: 2, cache_read_price_per_1m: 0, cache_write_price_per_1m: 4 }
const cells: PricingMatrixCellInput[] = [{ subject_id: 'cred', canonical_model_id: 'vendor/model', mode: 'fixed', ...rates }, { subject_id: 'other', canonical_model_id: 'vendor/model', mode: 'inherit' }]
const price: PricingOfficialInput = { canonical_model_id: 'vendor/model', display_model_id: 'Model', source: 'official', ...rates }
const clients: [string, (signal?: AbortSignal) => Promise<unknown>, string, string][] = [
  ['matrix read', signal => api.fetchCredentialPricingMatrix({}, signal), 'GET', '/pricing/credential-model-matrix'],
  ['key matrix read', signal => api.fetchCredentialPricingMatrix({}, signal, true), 'GET', '/key/pricing/credential-model-matrix'],
  ['matrix write', signal => api.updateCredentialPricingMatrix(cells, signal), 'PUT', '/pricing/credential-model-matrix'],
  ['sync status', api.fetchPricingCredentialSyncStatus, 'GET', '/pricing/credentials/sync-status'],
  ['credential refresh', api.refreshPricingCredentials, 'POST', '/pricing/credentials/refresh'],
  ['official list', api.fetchOfficialPricing, 'GET', '/pricing/official'],
  ['official single write', signal => api.saveOfficialPricing(price, signal), 'PUT', '/pricing/official'],
  ['official batch write', signal => api.saveOfficialPricingBatch([price], signal), 'PUT', '/pricing/official/batch'],
  ['official preview', signal => api.previewOfficialPricingRefresh('models-dev', signal), 'GET', '/pricing/official/refresh/preview'],
  ['official commit', signal => api.commitOfficialPricingRefresh('explicit-preview', signal), 'POST', '/pricing/official/refresh/commit'],
]

beforeEach(() => vi.stubGlobal('window', { __APP_BASE_PATH__: '/keeper/' }))
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('retained pricing API boundary', () => {
  it('does not export retired management clients', () => {
    for (const name of [
      'fetchPricingSyncPreview', 'updatePricing', 'updatePricingBatch', 'deletePricing', 'bindPricingCredential',
      'fetchPricingIdentityState', 'migratePricingIdentity', 'correctPricingIdentity',
      'fetchPricingCredentialDefault', 'savePricingCredentialDefault', 'clearPricingCredentialDefault',
      'fetchPricingChannels', 'fetchPricingChannel', 'savePricingChannel', 'deletePricingChannel',
      'savePricingChannelDefault', 'clearPricingChannelDefault', 'fetchPricingChannelModels', 'fetchPricingChannelModel',
      'savePricingChannelModel', 'savePricingChannelFixed', 'clearPricingChannelModel',
      'fetchPricingCredentialModels', 'fetchPricingCredentialModel', 'savePricingCredentialModel',
      'savePricingCredentialFixed', 'clearPricingCredentialModel', 'fetchPricingRules', 'replacePricingRules',
    ]) expect(api).not.toHaveProperty(name)
  })

  it.each([
    ['compatibility pricing', api.fetchPricing, '/pricing'],
    ['used models', api.fetchUsedModels, '/models/used'],
    ['credential directory', api.fetchPricingCredentials, '/pricing/credentials'],
    ['credential subjects', api.fetchCredentialPricingSubjects, '/pricing/credential-subjects'],
  ] as const)('retains %s read with base path, credentials and cancellation', async (_name, load, path) => {
    const body = { pricing: [], models: [], credentials: [] }
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(Response.json(body))
    const signal = new AbortController().signal
    expect(await load(signal)).toEqual(body)
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe(`/keeper/api/v1${path}`)
    expect(init).toMatchObject({ credentials: 'include', signal })
    expect(init?.method ?? 'GET').toBe('GET')
    expect(init?.body).toBeUndefined()
    expect(headerValue(init, 'X-CPA-Usage-Keeper-Request')).toBeNull()
    if (path.startsWith('/pricing/')) expect(init?.cache).toBe('no-store')
  })

  it.each(clients)('retains %s credentials, cancellation and mutation-only request intent', async (_name, call, method, path) => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(Response.json({}))
    const signal = new AbortController().signal
    await call(signal)
    const [url, init] = fetchMock.mock.calls[0]
    expect(new URL(String(url), 'http://localhost').pathname).toBe(`/keeper/api/v1${path}`)
    expect(init).toMatchObject({ credentials: 'include', signal })
    expect(init?.method ?? 'GET').toBe(method)
    expect(headerValue(init, 'X-CPA-Usage-Keeper-Request')).toBe(method === 'GET' ? null : 'fetch')
    if (method === 'GET') { expect(init?.cache).toBe('no-store'); expect(init?.body).toBeUndefined() }
    else if (method === 'PUT' || path.endsWith('/commit')) expect(headerValue(init, 'Content-Type')).toBe('application/json')
  })

  it('preserves complete atomic fixed/inherit and official four-price payloads including explicit zeros', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async () => Response.json({}))
    await api.updateCredentialPricingMatrix(cells)
    await api.saveOfficialPricing(price)
    await api.saveOfficialPricingBatch([price])
    expect(fetchMock).toHaveBeenCalledTimes(3)
    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toEqual({ cells })
    expect(JSON.parse(String(fetchMock.mock.calls[1][1]?.body))).toEqual(price)
    expect(JSON.parse(String(fetchMock.mock.calls[2][1]?.body))).toEqual({ pricing: [price] })
  })

  it.each(['models-dev', 'litellm', 'openrouter'] as const)('previews %s without committing until the explicit preview id is submitted', async source => {
    const body = { preview_id: 'preview-id', source_id: source, prices: [] }
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(Response.json(body)).mockResolvedValueOnce(Response.json({ pricing: [] }))
    const signal = new AbortController().signal
    expect(await api.previewOfficialPricingRefresh(source, signal)).toEqual(body)
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost').searchParams.get('source')).toBe(source)
    await api.commitOfficialPricingRefresh(body.preview_id, signal)
    expect(JSON.parse(String(fetchMock.mock.calls[1][1]?.body))).toEqual({ preview_id: 'preview-id' })
  })

  it.each([400, 401, 403, 404, 409, 413, 422, 500])('preserves HTTP status %i while discarding backend details for all retained management clients', async status => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async () => Response.json({ error: 'secret-token / private-auth-path' }, { status }))
    for (const [, call] of clients) {
      const request = call()
      await expect(request).rejects.toBeInstanceOf(api.ApiError)
      await expect(request).rejects.toMatchObject({ status, message: expect.stringMatching(/^Unable to /) })
      await expect(request).rejects.not.toHaveProperty('message', expect.stringContaining('secret-token'))
      await expect(request).rejects.not.toHaveProperty('message', expect.stringContaining('private-auth-path'))
    }
    expect(fetchMock).toHaveBeenCalledTimes(clients.length)
  })

  it('propagates abort cancellation instead of returning a successful matrix result', async () => {
    const controller = new AbortController()
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation((_url, init) => new Promise((_resolve, reject) => {
      init?.signal?.addEventListener('abort', () => reject(new DOMException('Canceled', 'AbortError')), { once: true })
    }))
    const request = api.fetchCredentialPricingMatrix({}, controller.signal)
    controller.abort()
    await expect(request).rejects.toMatchObject({ name: 'AbortError' })
    expect(fetchMock.mock.calls[0][1]?.signal).toBe(controller.signal)
  })

  it('clears the embed fallback session on pricing 401 while keeping safe errors and credentials', async () => {
    const tokens = new Map([['cpa_usage_keeper_embed_session', 'synthetic-embed-token']])
    const sessionStorage = { getItem: (key: string) => tokens.get(key) ?? null, removeItem: vi.fn((key: string) => tokens.delete(key)) }
    vi.stubGlobal('window', { __APP_BASE_PATH__: '/keeper/', location: { search: '?embed=cpamc' }, sessionStorage })
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(Response.json({ error: 'sensitive-auth-detail' }, { status: 401 }))
    await expect(api.fetchCredentialPricingMatrix()).rejects.toMatchObject({ status: 401, message: 'Unable to load pricing matrix' })
    expect(fetchMock.mock.calls[0][1]?.credentials).toBe('include')
    expect(headerValue(fetchMock.mock.calls[0][1], 'X-CPA-Usage-Keeper-Embed-Session')).toBe('synthetic-embed-token')
    expect(sessionStorage.removeItem).toHaveBeenCalledExactlyOnceWith('cpa_usage_keeper_embed_session')
    expect(tokens.size).toBe(0)
  })
})
