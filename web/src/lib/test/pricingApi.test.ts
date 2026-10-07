import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, clearPricingChannelModel, correctPricingIdentity, deletePricing, fetchPricingChannelModel, fetchPricingChannelModels, fetchPricingIdentityState, fetchPricingRules, fetchPricingSyncPreview, migratePricingIdentity, replacePricingRules, savePricingChannelFixed, savePricingChannelModel, updatePricing, updatePricingBatch } from '../api'

const headerValue = (init: RequestInit | undefined, name: string): string | null => (
  new Headers(init?.headers).get(name)
)

beforeEach(() => {
  vi.stubGlobal('window', { __APP_BASE_PATH__: undefined })
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('identity association API client', () => {
  it('uses exact opaque references, bounded safe payload fields, credentials and cancellation', async () => {
    vi.stubGlobal('window', { __APP_BASE_PATH__: '/keeper' })
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async () => Response.json({}))
    const signal = new AbortController().signal
    await fetchPricingIdentityState(signal)
    expect(fetchMock.mock.calls[0]).toEqual(['/keeper/api/v1/pricing/identity-bindings', expect.objectContaining({ cache: 'no-store', credentials: 'include', signal })])
    const migration = { subject_id: 'cred_a', directory_ref: 'selection_new', snapshot_id: 'snapshot_old', confirmed: true, raw_identity: 'never-serialize' }
    await migratePricingIdentity(migration, signal)
    const [url, init] = fetchMock.mock.calls[1]
    expect(url).toBe('/keeper/api/v1/pricing/identity-bindings/migrate')
    expect(init).toMatchObject({ credentials: 'include', method: 'POST', signal })
    expect(JSON.parse(String(init?.body))).toEqual({ subject_id: 'cred_a', directory_ref: 'selection_new', snapshot_id: 'snapshot_old', confirmed: true })
    await correctPricingIdentity('binding/opaque', { expected_subject_id: 'cred_a', target_subject_id: 'cred_b', action: 'rebind', snapshot_id: 'snapshot_new', confirmed: true }, signal)
    const [correctionURL, correction] = fetchMock.mock.calls[2]
    expect(correctionURL).toBe('/keeper/api/v1/pricing/identity-bindings/binding%2Fopaque/correction')
    expect(correction).toMatchObject({ credentials: 'include', method: 'PUT', signal })
    expect(JSON.parse(String(correction?.body))).toEqual({ expected_subject_id: 'cred_a', target_subject_id: 'cred_b', action: 'rebind', snapshot_id: 'snapshot_new', confirmed: true })
  })
})

describe('pricing API client', () => {
  it.each([undefined, 'litellm'] as const)('requests the selected pricing source %s with cancellation', async (source) => {
    vi.stubGlobal('window', { __APP_BASE_PATH__: '/keeper' })
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true, json: async () => ({}) } as Response)
    const signal = new AbortController().signal
    await fetchPricingSyncPreview(source, signal)
    const [rawURL, init] = fetchMock.mock.calls[0]
    const url = new URL(String(rawURL), 'http://localhost')
    expect(url.pathname).toBe('/keeper/api/v1/pricing/sync/preview')
    expect(url.searchParams.get('source')).toBe(source ?? 'models-dev')
    expect(init).toMatchObject({ credentials: 'include', cache: 'no-store', signal })
  })

  it('updates one model through the pricing endpoint without sending a pricing snapshot', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(Response.json({
      model: 'openai/gpt-4.1',
      pricing_style: 'openai',
      prompt_price_per_1m: 3,
      completion_price_per_1m: 15,
      cache_read_price_per_1m: 0.3,
      cache_write_price_per_1m: 0,
      price_multiplier: 1,
    }))

    await updatePricing('openai/gpt-4.1', {
      pricing_style: 'openai',
      prompt_price_per_1m: 3,
      completion_price_per_1m: 15,
      cache_read_price_per_1m: 0.3,
      cache_write_price_per_1m: 0,
      price_multiplier: 1,
    })

    const [url, init] = fetchMock.mock.calls[0]
    const parsed = new URL(String(url), 'http://localhost')
    const body = JSON.parse(String(init?.body))

    expect(parsed.pathname).toBe('/api/v1/pricing')
    expect(init).toMatchObject({ credentials: 'include', method: 'PUT' })
    expect(headerValue(init, 'Content-Type')).toBe('application/json')
    expect(body).toEqual({
      model: 'openai/gpt-4.1',
      pricing_style: 'openai',
      prompt_price_per_1m: 3,
      completion_price_per_1m: 15,
      cache_read_price_per_1m: 0.3,
      cache_write_price_per_1m: 0,
      price_multiplier: 1,
    })
  })

  it('deletes one model through the pricing endpoint without sending a request body', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response())

    await deletePricing('openai/gpt-4.1')

    const [url, init] = fetchMock.mock.calls[0]
    const parsed = new URL(String(url), 'http://localhost')

    expect(parsed.pathname).toBe('/api/v1/pricing')
    expect(parsed.searchParams.get('model')).toBe('openai/gpt-4.1')
    expect(init).toMatchObject({ credentials: 'include', method: 'DELETE' })
    expect(init?.body).toBeUndefined()
  })

  it('updates multiple model prices in one batch request', async () => {
	const pricing = [
	  { model: 'model-a', pricing_style: 'openai' as const, prompt_price_per_1m: 2, completion_price_per_1m: 0, cache_read_price_per_1m: 0, cache_write_price_per_1m: 0, price_multiplier: 1 },
	  { model: 'model-b', pricing_style: 'openai' as const, prompt_price_per_1m: 3, completion_price_per_1m: 0, cache_read_price_per_1m: 0, cache_write_price_per_1m: 0, price_multiplier: 1 },
	]
	const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(Response.json({ pricing }))

	await updatePricingBatch(pricing)

	expect(fetchMock).toHaveBeenCalledTimes(1)
	const [rawURL, init] = fetchMock.mock.calls[0]
	expect(new URL(String(rawURL), 'http://localhost').pathname).toBe('/api/v1/pricing/batch')
	expect(init).toMatchObject({ credentials: 'include', method: 'PUT' })
	expect(JSON.parse(String(init?.body))).toEqual({ pricing })
  })
})

describe('channel model pricing API', () => {
  const channelId = 'channel/synthetic +?#中文'
  const model = 'provider/Exact Model+?#中文'
  const canonical = { channel_id: channelId, model, multiplier: 0, mode: 'multiplier', snapshot_id: 'snapshot_synthetic' }
  const fixed = { prompt_price_per_1m: '0', completion_price_per_1m: '2.00', cache_read_price_per_1m: '.3', cache_write_price_per_1m: '4', pricing_style: 'claude' as const }

  it('loads saved exceptions with encoded stable channel ID, base path and cancellation', async () => {
    vi.stubGlobal('window', { __APP_BASE_PATH__: '/keeper' })
    const body = { channel_id: channelId, models: [canonical], snapshot_id: 'snapshot_synthetic' }
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(Response.json(body))
    const signal = new AbortController().signal
    expect(await fetchPricingChannelModels(channelId, signal)).toEqual(body)
    expect(fetchMock.mock.calls[0][0]).toBe(`/keeper/api/v1/pricing/channels/${encodeURIComponent(channelId)}/models`)
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ credentials: 'include', signal, cache: 'no-store' })
  })

  it.each(['GET', 'PUT', 'FIXED', 'DELETE'])('uses exact model query and canonical response for %s', async method => {
    vi.stubGlobal('window', { __APP_BASE_PATH__: '/keeper' })
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(Response.json(canonical))
    const signal = new AbortController().signal
    const saved = method === 'GET' ? await fetchPricingChannelModel(channelId, model, signal)
      : method === 'PUT' ? await savePricingChannelModel(channelId, model, '20%', signal)
        : method === 'FIXED' ? await savePricingChannelFixed(channelId, model, fixed, signal)
          : await clearPricingChannelModel(channelId, model, signal)
    expect(saved).toEqual(canonical)
    const [rawURL, init] = fetchMock.mock.calls[0]
    const url = new URL(String(rawURL), 'http://localhost')
    expect(url.pathname).toBe(`/keeper/api/v1/pricing/channels/${encodeURIComponent(channelId)}/model`)
    expect([...url.searchParams.entries()]).toEqual([['model', model]])
    expect(init).toMatchObject({ credentials: 'include', signal })
    if (method === 'GET') {
      expect(init?.cache).toBe('no-store'); expect(init?.body).toBeUndefined()
    } else {
      expect(init?.method).toBe(method === 'FIXED' ? 'PUT' : method)
      expect(headerValue(init, 'X-CPA-Usage-Keeper-Request')).toBe('fetch')
      if (method === 'DELETE') expect(init?.body).toBeUndefined()
      else {
        expect(headerValue(init, 'Content-Type')).toBe('application/json')
        expect(JSON.parse(String(init?.body))).toEqual(method === 'FIXED' ? { mode: 'fixed', fixed } : { mode: 'multiplier', multiplier: '20%' })
      }
    }
  })

  it.each([400, 401, 403, 404, 409, 422, 500])('preserves status %s for safe localized handling', async status => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async () => Response.json({ error: 'synthetic failure' }, { status }))
    const calls = [
      () => fetchPricingChannelModels(channelId), () => fetchPricingChannelModel(channelId, model),
      () => savePricingChannelModel(channelId, model, '0'), () => savePricingChannelFixed(channelId, model, fixed),
      () => clearPricingChannelModel(channelId, model),
    ]
    for (const call of calls) {
      const result = call()
      await expect(result).rejects.toBeInstanceOf(ApiError)
      await expect(result).rejects.toMatchObject({ status })
    }
    expect(fetchMock).toHaveBeenCalledTimes(5)
  })
})

describe('pricing rules API', () => {
  it('loads rules through a query parameter so model names may contain slashes', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(Response.json({ model: 'openai/gpt-5.6', rules: [] }))
    const signal = new AbortController().signal

    await fetchPricingRules('openai/gpt-5.6', signal)

    const [rawURL, init] = fetchMock.mock.calls[0]
    const url = new URL(String(rawURL), 'http://localhost')
    expect(url.pathname).toBe('/api/v1/pricing/rules')
    expect(url.searchParams.get('model')).toBe('openai/gpt-5.6')
    expect(init).toMatchObject({ credentials: 'include', signal, cache: 'no-store' })
  })

  it('replaces the complete rule set and preserves an explicit empty array', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(Response.json({ model: 'model-a', rules: [] }))

    await replacePricingRules({ model: 'model-a', rules: [] })

    const [rawURL, init] = fetchMock.mock.calls[0]
    expect(new URL(String(rawURL), 'http://localhost').pathname).toBe('/api/v1/pricing/rules')
    expect(init).toMatchObject({ credentials: 'include', method: 'PUT' })
    expect(headerValue(init, 'Content-Type')).toBe('application/json')
    expect(headerValue(init, 'X-CPA-Usage-Keeper-Request')).toBe('fetch')
    expect(init?.body).toBe(JSON.stringify({ model: 'model-a', rules: [] }))
  })
})
