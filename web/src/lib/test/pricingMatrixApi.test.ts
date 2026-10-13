import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ApiError,
  commitOfficialPricingRefresh,
  fetchCredentialPricingMatrix,
  fetchOfficialPricing,
  previewOfficialPricingRefresh,
  refreshPricingCredentials,
  saveOfficialPricingBatch,
  updateCredentialPricingMatrix,
} from '../api'

beforeEach(() => vi.stubGlobal('window', { __APP_BASE_PATH__: '/keeper' }))
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('pricing matrix API', () => {
  it('encodes independent filters and routes key viewers to the scoped GET', async () => {
    const payload = { credentials: [], models: [], cells: [], snapshot_id: 'snap', credential_page: 2, credential_page_size: 20, model_page: 3, model_page_size: 40, total_credentials: 0, total_models: 0 }
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(Response.json(payload))
    await fetchCredentialPricingMatrix({ credentialPage: 2, credentialPageSize: 20, modelPage: 3, modelPageSize: 40, credential: 'north / key', model: 'vendor/model+' }, undefined, true)
    const [url, init] = fetchMock.mock.calls[0]
    const parsed = new URL(String(url), 'http://localhost')
    expect(parsed.pathname).toBe('/keeper/api/v1/key/pricing/credential-model-matrix')
    expect(Object.fromEntries(parsed.searchParams)).toEqual({ credential_page: '2', credential_page_size: '20', model_page: '3', model_page_size: '40', credential: 'north / key', model: 'vendor/model+' })
    expect(init).toMatchObject({ credentials: 'include', cache: 'no-store' })
  })

  it('sends one atomic fixed/inherit batch without altering explicit zero prices', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(Response.json({ cells: [] }))
    const cells = [
      { subject_id: 'cred_a', canonical_model_id: 'vendor/model', mode: 'fixed' as const, pricing_style: 'claude' as const, prompt_price_per_1m: 0, completion_price_per_1m: 2, cache_read_price_per_1m: 0, cache_write_price_per_1m: 4 },
      { subject_id: 'cred_b', canonical_model_id: 'vendor/model', mode: 'inherit' as const },
    ]
    await updateCredentialPricingMatrix(cells)
    const [url, init] = fetchMock.mock.calls[0]
    expect(new URL(String(url), 'http://localhost').pathname).toBe('/keeper/api/v1/pricing/credential-model-matrix')
    expect(init).toMatchObject({ method: 'PUT', credentials: 'include' })
    expect(JSON.parse(String(init?.body))).toEqual({ cells })
  })

  it('uses admin sync endpoints and returns accepted sync status', async () => {
    const status = { state: 'queued', in_flight: true }
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(Response.json(status, { status: 202 }))
    expect(await refreshPricingCredentials()).toEqual(status)
    const [url, init] = fetchMock.mock.calls[0]
    expect(String(url)).toContain('/keeper/api/v1/pricing/credentials/refresh')
    expect(init).toMatchObject({ method: 'POST', credentials: 'include' })
    expect(init?.body).toBeUndefined()
  })
})

describe('official pricing API', () => {
  it('loads the admin list and writes a batch of complete manual prices', async () => {
    const response = { pricing: [] }
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(Response.json(response)).mockResolvedValueOnce(Response.json(response))
    expect(await fetchOfficialPricing()).toEqual(response)
    const input = [{ canonical_model_id: 'vendor/model', display_model_id: 'Model', source: 'official' as const, pricing_style: 'openai' as const, prompt_price_per_1m: 1, completion_price_per_1m: 2, cache_read_price_per_1m: 0, cache_write_price_per_1m: 4 }]
    await saveOfficialPricingBatch(input)
    expect(new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost').pathname).toBe('/keeper/api/v1/pricing/official')
    expect(new URL(String(fetchMock.mock.calls[1][0]), 'http://localhost').pathname).toBe('/keeper/api/v1/pricing/official/batch')
    expect(JSON.parse(String(fetchMock.mock.calls[1][1]?.body))).toEqual({ pricing: input })
  })

  it('requires an explicit preview id for commit and encodes supported sources', async () => {
    const preview = { preview_id: 'preview-id', source_id: 'openrouter', source: 'OpenRouter', source_url: 'https://openrouter.ai/api/v1/models', fetched_at: '2026-10-13T00:00:00Z', response_hash: 'hash', prices: [], stale: false }
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(Response.json(preview)).mockResolvedValueOnce(Response.json({ pricing: [] }))
    await previewOfficialPricingRefresh('openrouter')
    await commitOfficialPricingRefresh(preview.preview_id)
    const parsed = new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost')
    expect(parsed.pathname).toBe('/keeper/api/v1/pricing/official/refresh/preview')
    expect(parsed.searchParams.get('source')).toBe('openrouter')
    expect(JSON.parse(String(fetchMock.mock.calls[1][1]?.body))).toEqual({ preview_id: 'preview-id' })
  })

  it('discards raw backend error text but preserves HTTP status', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(Response.json({ error: 'secret-token / auth path' }, { status: 500 }))
    await expect(fetchOfficialPricing()).rejects.toMatchObject({ name: 'ApiError', status: 500, message: 'Unable to load official pricing' } satisfies Partial<ApiError>)
    expect(fetchMock).toHaveBeenCalledOnce()
  })
})
