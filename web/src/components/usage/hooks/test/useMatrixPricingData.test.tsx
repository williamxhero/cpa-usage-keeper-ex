// @vitest-environment happy-dom

import { act, useEffect } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as api from '@/lib/api'
import type { PricingMatrixCellInput, PricingMatrixResponse, PricingOfficialInput, PricingOfficialPreviewResponse } from '@/lib/types'
import { useMatrixPricingData, useOfficialPricingData, type UseMatrixPricingDataOptions, type UseOfficialPricingDataOptions } from '../useMatrixPricingData'

vi.mock('@/lib/api', async (original) => ({
  ...await original<typeof import('@/lib/api')>(),
  fetchCredentialPricingMatrix: vi.fn(), fetchPricingCredentialSyncStatus: vi.fn(),
  updateCredentialPricingMatrix: vi.fn(), refreshPricingCredentials: vi.fn(),
  fetchOfficialPricing: vi.fn(), saveOfficialPricing: vi.fn(), saveOfficialPricingBatch: vi.fn(),
  previewOfficialPricingRefresh: vi.fn(), commitOfficialPricingRefresh: vi.fn(),
}))
const { t } = vi.hoisted(() => ({ t: (key: string) => key }))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t }) }))

const matrix: PricingMatrixResponse = { credentials: [], models: ['model'], cells: [], snapshot_id: 'snapshot', credential_page: 1, credential_page_size: 20, model_page: 1, model_page_size: 20, total_credentials: 0, total_models: 1 }
const rates = { pricing_style: 'openai' as const, prompt_price_per_1m: 0, completion_price_per_1m: 2, cache_read_price_per_1m: 0, cache_write_price_per_1m: 4 }
const cells: PricingMatrixCellInput[] = [{ subject_id: 'cred', canonical_model_id: 'model', mode: 'fixed', ...rates }, { subject_id: 'other', canonical_model_id: 'model', mode: 'inherit' }]
const official: PricingOfficialInput = { canonical_model_id: 'model', display_model_id: 'Model', source: 'official', ...rates }
const preview: PricingOfficialPreviewResponse = { preview_id: 'preview-id', source_id: 'models-dev', source: 'Models.dev', source_url: 'https://models.dev/api.json', fetched_at: '2026-10-13T00:00:00Z', response_hash: 'hash', prices: [], stale: false }

let currentMatrix: ReturnType<typeof useMatrixPricingData>
let currentOfficial: ReturnType<typeof useOfficialPricingData>
function MatrixHarness(props: UseMatrixPricingDataOptions) {
  const result = useMatrixPricingData(props)
  useEffect(() => { currentMatrix = result }, [result])
  return null
}
function OfficialHarness(props: UseOfficialPricingDataOptions) {
  const result = useOfficialPricingData(props)
  useEffect(() => { currentOfficial = result }, [result])
  return null
}
let root: Root
let container: HTMLDivElement
let mounted: boolean
const renderMatrix = async (props: UseMatrixPricingDataOptions = {}) => { await act(async () => root.render(<MatrixHarness {...props} />)) }
const renderOfficial = async (props: UseOfficialPricingDataOptions = {}) => { await act(async () => root.render(<OfficialHarness {...props} />)) }
const unmount = async () => { await act(async () => root.unmount()); mounted = false }

beforeEach(() => {
  globalThis.IS_REACT_ACT_ENVIRONMENT = true
  vi.resetAllMocks()
  vi.mocked(api.fetchCredentialPricingMatrix).mockResolvedValue(matrix)
  vi.mocked(api.fetchPricingCredentialSyncStatus).mockResolvedValue({ state: 'idle', in_flight: false })
  vi.mocked(api.updateCredentialPricingMatrix).mockResolvedValue({ cells: [] })
  vi.mocked(api.refreshPricingCredentials).mockResolvedValue({ state: 'queued', in_flight: true })
  vi.mocked(api.fetchOfficialPricing).mockResolvedValue({ pricing: [] })
  vi.mocked(api.saveOfficialPricingBatch).mockResolvedValue({ pricing: [] })
  vi.mocked(api.previewOfficialPricingRefresh).mockResolvedValue(preview)
  vi.mocked(api.commitOfficialPricingRefresh).mockResolvedValue({ pricing: [] })
  container = document.createElement('div'); document.body.appendChild(container)
  root = createRoot(container); mounted = true
})
afterEach(async () => { if (mounted) await unmount(); container.remove() })

describe('matrix pricing lifecycle and security', () => {
  it('keeps loaders stable and uses the latest authentication callback only for 401', async () => {
    const first = vi.fn(), latest = vi.fn()
    await renderMatrix({ onAuthRequired: first })
    const load = currentMatrix.loadMatrix
    await renderMatrix({ onAuthRequired: latest })
    expect(currentMatrix.loadMatrix).toBe(load)
    expect(api.fetchCredentialPricingMatrix).toHaveBeenCalledOnce()
    vi.mocked(api.fetchCredentialPricingMatrix).mockRejectedValueOnce(new api.ApiError('sensitive backend detail', 401))
    await act(async () => load())
    expect(first).not.toHaveBeenCalled(); expect(latest).toHaveBeenCalledOnce()
    expect(currentMatrix.error).toBe('errors.load_matrix')
    vi.mocked(api.fetchCredentialPricingMatrix).mockRejectedValueOnce(new api.ApiError('sensitive backend detail', 403))
    await act(async () => load())
    expect(latest).toHaveBeenCalledOnce()
  })

  it('routes read-only viewers to the key matrix and prevents all administrative operations', async () => {
    await renderMatrix({ canManage: false })
    expect(api.fetchCredentialPricingMatrix).toHaveBeenCalledWith({}, expect.any(AbortSignal), true)
    expect(currentMatrix.canRead).toBe(true); expect(currentMatrix.canEdit).toBe(false)
    await act(async () => {
      expect(await currentMatrix.saveCells(cells)).toBe(false)
      expect(await currentMatrix.refreshCredentials()).toBeNull()
      expect(await currentMatrix.loadSyncStatus()).toBeNull()
    })
    expect(api.fetchPricingCredentialSyncStatus).not.toHaveBeenCalled()
    expect(api.updateCredentialPricingMatrix).not.toHaveBeenCalled()
    expect(api.refreshPricingCredentials).not.toHaveBeenCalled()
  })

  it('aborts superseded queries and ignores their late responses', async () => {
    const pending = Promise.withResolvers<PricingMatrixResponse>()
    vi.mocked(api.fetchCredentialPricingMatrix).mockReturnValueOnce(pending.promise)
    await renderMatrix({ query: { model: 'first' } })
    const signal = vi.mocked(api.fetchCredentialPricingMatrix).mock.calls[0][1]!
    await renderMatrix({ query: { model: 'second' } })
    expect(signal.aborted).toBe(true)
    await act(async () => pending.resolve({ ...matrix, snapshot_id: 'stale' }))
    expect(currentMatrix.matrix?.snapshot_id).toBe('snapshot')
  })

  it.each(['disable', 'unmount', 'permission'] as const)('aborts reads, writes and credential refresh on %s and ignores late completion', async (change) => {
    const read = Promise.withResolvers<PricingMatrixResponse>()
    const write = Promise.withResolvers<{ cells: [] }>()
    const refresh = Promise.withResolvers<{ state: 'queued'; in_flight: true }>()
    const status = Promise.withResolvers<{ state: 'idle'; in_flight: false }>()
    vi.mocked(api.fetchCredentialPricingMatrix).mockReturnValueOnce(read.promise)
    vi.mocked(api.fetchPricingCredentialSyncStatus).mockReturnValueOnce(status.promise)
    vi.mocked(api.updateCredentialPricingMatrix).mockReturnValueOnce(write.promise)
    vi.mocked(api.refreshPricingCredentials).mockReturnValueOnce(refresh.promise)
    const changed = vi.fn(), notice = vi.fn()
    await renderMatrix({ onChanged: changed, onNotice: notice })
    let saving!: Promise<boolean>, refreshing!: ReturnType<typeof currentMatrix.refreshCredentials>
    act(() => { saving = currentMatrix.saveCells(cells); refreshing = currentMatrix.refreshCredentials() })
    const signals = [vi.mocked(api.fetchCredentialPricingMatrix).mock.calls[0][1], vi.mocked(api.fetchPricingCredentialSyncStatus).mock.calls[0][0], vi.mocked(api.updateCredentialPricingMatrix).mock.calls[0][1], vi.mocked(api.refreshPricingCredentials).mock.calls[0][0]]
    if (change === 'unmount') await unmount()
    else await renderMatrix({ enabled: change !== 'disable', canManage: change !== 'permission', onChanged: changed, onNotice: notice })
    expect(signals.every(signal => signal?.aborted)).toBe(true)
    await act(async () => { read.resolve({ ...matrix, snapshot_id: 'late' }); status.resolve({ state: 'idle', in_flight: false }); write.resolve({ cells: [] }); refresh.resolve({ state: 'queued', in_flight: true }); expect(await saving).toBe(false); expect(await refreshing).toBeNull() })
    expect(changed).not.toHaveBeenCalled(); expect(notice).not.toHaveBeenCalled()
    if (change !== 'unmount') expect(currentMatrix.matrix?.snapshot_id).not.toBe('late')
  })

  it('writes one complete fixed/inherit batch preserving four prices and explicit zeros, then uses the latest callback', async () => {
    const pending = Promise.withResolvers<{ cells: [] }>()
    vi.mocked(api.updateCredentialPricingMatrix).mockReturnValueOnce(pending.promise)
    const first = vi.fn(), latest = vi.fn(), notice = vi.fn()
    await renderMatrix({ onChanged: first, onNotice: notice })
    const save = currentMatrix.saveCells
    let result!: Promise<boolean>
    act(() => { result = save(cells) })
    await renderMatrix({ onChanged: latest, onNotice: notice })
    expect(currentMatrix.saveCells).toBe(save)
    expect(notice).not.toHaveBeenCalled(); expect(latest).not.toHaveBeenCalled()
    await act(async () => { pending.resolve({ cells: [] }); expect(await result).toBe(true) })
    expect(api.updateCredentialPricingMatrix).toHaveBeenCalledExactlyOnceWith(cells, expect.any(AbortSignal))
    expect(api.fetchCredentialPricingMatrix).toHaveBeenCalledTimes(2)
    expect(first).not.toHaveBeenCalled(); expect(latest).toHaveBeenCalledOnce()
    expect(notice).toHaveBeenCalledExactlyOnceWith('success', 'notifications.matrix_saved')
  })

  it.each(['throw', 'reject', 'pending'] as const)('isolates a parent refresh that may %s from a committed matrix save', async (kind) => {
    const changed = vi.fn(() => { if (kind === 'throw') throw new Error('parent'); if (kind === 'reject') return Promise.reject(new Error('parent')); return new Promise<void>(() => {}) })
    await renderMatrix({ onChanged: changed })
    await act(async () => expect(await currentMatrix.saveCells(cells)).toBe(true))
    expect(changed).toHaveBeenCalledOnce(); expect(currentMatrix.saving).toBe(false)
  })

  it.each([401, 403, 413, 500])('redacts failed matrix mutations with status %i without reporting a commit', async (status) => {
    const changed = vi.fn(), notice = vi.fn(), auth = vi.fn()
    await renderMatrix({ onChanged: changed, onNotice: notice, onAuthRequired: auth })
    vi.mocked(api.updateCredentialPricingMatrix).mockRejectedValueOnce(new api.ApiError('secret token / private path', status))
    await act(async () => expect(await currentMatrix.saveCells(cells)).toBe(false))
    expect(changed).not.toHaveBeenCalled()
    expect(notice).toHaveBeenCalledExactlyOnceWith('error', status === 413 ? 'errors.batch_limit' : 'errors.save_matrix')
    expect(auth).toHaveBeenCalledTimes(status === 401 ? 1 : 0)
    expect(currentMatrix.matrix).toEqual(matrix)
  })

  it('does not notify a canceled matrix commit during its readback', async () => {
    const readback = Promise.withResolvers<PricingMatrixResponse>(), changed = vi.fn(), notice = vi.fn()
    await renderMatrix({ onChanged: changed, onNotice: notice })
    vi.mocked(api.fetchCredentialPricingMatrix).mockReturnValueOnce(readback.promise)
    let result!: Promise<boolean>
    await act(async () => { result = currentMatrix.saveCells(cells); await Promise.resolve() })
    await renderMatrix({ enabled: false, onChanged: changed, onNotice: notice })
    await act(async () => { readback.resolve(matrix); expect(await result).toBe(false) })
    expect(changed).not.toHaveBeenCalled(); expect(notice).not.toHaveBeenCalled()
  })

  it('redacts sync status and refresh errors while preserving 401 recovery', async () => {
    const auth = vi.fn(), notice = vi.fn()
    vi.mocked(api.fetchPricingCredentialSyncStatus).mockRejectedValueOnce(new api.ApiError('secret', 401))
    await renderMatrix({ onAuthRequired: auth, onNotice: notice })
    expect(currentMatrix.syncError).toBe('errors.load_sync'); expect(auth).toHaveBeenCalledOnce()
    vi.mocked(api.refreshPricingCredentials).mockRejectedValueOnce(new Error('private network detail'))
    await act(async () => expect(await currentMatrix.refreshCredentials()).toBeNull())
    expect(notice).toHaveBeenCalledExactlyOnceWith('error', 'errors.refresh_credentials')
    expect(auth).toHaveBeenCalledOnce()
  })
})

describe('official pricing lifecycle and security', () => {
  it('does not load, preview or mutate official pricing without administrator capability', async () => {
    await renderOfficial({ canManage: false })
    await act(async () => {
      expect(await currentOfficial.previewOfficialPricing('models-dev')).toBeNull()
      expect(await currentOfficial.saveOfficialPrices([official])).toBeNull()
      expect(await currentOfficial.commitOfficialPricing()).toBeNull()
    })
    expect(api.fetchOfficialPricing).not.toHaveBeenCalled(); expect(api.previewOfficialPricingRefresh).not.toHaveBeenCalled()
    expect(api.saveOfficialPricing).not.toHaveBeenCalled(); expect(api.saveOfficialPricingBatch).not.toHaveBeenCalled(); expect(api.commitOfficialPricingRefresh).not.toHaveBeenCalled()
  })

  it('previews without notifying or writing, then commits only the explicit preview id', async () => {
    const changed = vi.fn(), notice = vi.fn()
    await renderOfficial({ onChanged: changed, onNotice: notice })
    await act(async () => expect(await currentOfficial.commitOfficialPricing()).toBeNull())
    await act(async () => expect(await currentOfficial.previewOfficialPricing('models-dev')).toEqual(preview))
    expect(api.commitOfficialPricingRefresh).not.toHaveBeenCalled(); expect(changed).not.toHaveBeenCalled(); expect(notice).not.toHaveBeenCalled()
    await act(async () => expect(await currentOfficial.commitOfficialPricing()).toEqual({ pricing: [] }))
    expect(api.commitOfficialPricingRefresh).toHaveBeenCalledExactlyOnceWith('preview-id', expect.any(AbortSignal))
    expect(currentOfficial.preview).toBeNull(); expect(changed).toHaveBeenCalledOnce()
    expect(notice).toHaveBeenCalledExactlyOnceWith('success', 'notifications.refresh_committed')
  })

  it('aborts previous-source and cleared previews and ignores late responses', async () => {
    const stale = Promise.withResolvers<PricingOfficialPreviewResponse>()
    vi.mocked(api.previewOfficialPricingRefresh).mockReturnValueOnce(stale.promise)
    await renderOfficial()
    let first!: ReturnType<typeof currentOfficial.previewOfficialPricing>
    act(() => { first = currentOfficial.previewOfficialPricing('models-dev') })
    const signal = vi.mocked(api.previewOfficialPricingRefresh).mock.calls[0][1]!
    await act(async () => currentOfficial.previewOfficialPricing('openrouter'))
    expect(signal.aborted).toBe(true)
    await act(async () => { stale.resolve({ ...preview, preview_id: 'stale' }); expect(await first).toBeNull() })
    expect(currentOfficial.preview?.preview_id).toBe('preview-id')
    const cleared = Promise.withResolvers<PricingOfficialPreviewResponse>()
    vi.mocked(api.previewOfficialPricingRefresh).mockReturnValueOnce(cleared.promise)
    let last!: ReturnType<typeof currentOfficial.previewOfficialPricing>
    act(() => { last = currentOfficial.previewOfficialPricing('litellm') })
    const clearedSignal = vi.mocked(api.previewOfficialPricingRefresh).mock.calls[2][1]!
    act(() => currentOfficial.clearPreview())
    expect(clearedSignal.aborted).toBe(true)
    await act(async () => { cleared.resolve(preview); expect(await last).toBeNull() })
    expect(currentOfficial.preview).toBeNull()
  })

  it.each(['disable', 'permission', 'unmount'] as const)('aborts official reads, previews and writes on %s without late callbacks', async (change) => {
    const read = Promise.withResolvers<{ pricing: [] }>(), write = Promise.withResolvers<{ pricing: [] }>(), pendingPreview = Promise.withResolvers<PricingOfficialPreviewResponse>()
    vi.mocked(api.fetchOfficialPricing).mockReturnValueOnce(read.promise)
    vi.mocked(api.saveOfficialPricingBatch).mockReturnValueOnce(write.promise)
    vi.mocked(api.previewOfficialPricingRefresh).mockReturnValueOnce(pendingPreview.promise)
    const changed = vi.fn(), notice = vi.fn()
    await renderOfficial({ onChanged: changed, onNotice: notice })
    let saving!: ReturnType<typeof currentOfficial.saveOfficialPrices>, previewing!: ReturnType<typeof currentOfficial.previewOfficialPricing>
    act(() => { saving = currentOfficial.saveOfficialPrices([official, { ...official, canonical_model_id: 'other' }]); previewing = currentOfficial.previewOfficialPricing('models-dev') })
    const signals = [vi.mocked(api.fetchOfficialPricing).mock.calls[0][0], vi.mocked(api.saveOfficialPricingBatch).mock.calls[0][1], vi.mocked(api.previewOfficialPricingRefresh).mock.calls[0][1]]
    if (change === 'unmount') await unmount()
    else await renderOfficial({ enabled: change !== 'disable', canManage: change !== 'permission', onChanged: changed, onNotice: notice })
    expect(signals.every(signal => signal?.aborted)).toBe(true)
    await act(async () => { read.resolve({ pricing: [] }); write.resolve({ pricing: [] }); pendingPreview.resolve(preview); expect(await saving).toBeNull(); expect(await previewing).toBeNull() })
    expect(changed).not.toHaveBeenCalled(); expect(notice).not.toHaveBeenCalled()
  })

  it.each(['save', 'commit'] as const)('does not notify a canceled official %s during its readback', async (operation) => {
    const readback = Promise.withResolvers<{ pricing: [] }>(), changed = vi.fn(), notice = vi.fn()
    await renderOfficial({ onChanged: changed, onNotice: notice })
    if (operation === 'commit') await act(async () => currentOfficial.previewOfficialPricing('models-dev'))
    vi.mocked(api.fetchOfficialPricing).mockReturnValueOnce(readback.promise)
    let result!: ReturnType<typeof currentOfficial.saveOfficialPrices>
    await act(async () => { result = operation === 'save' ? currentOfficial.saveOfficialPrices([official, official]) : currentOfficial.commitOfficialPricing(); await Promise.resolve() })
    await renderOfficial({ canManage: false, onChanged: changed, onNotice: notice })
    await act(async () => { readback.resolve({ pricing: [] }); expect(await result).toBeNull() })
    expect(changed).not.toHaveBeenCalled(); expect(notice).not.toHaveBeenCalled()
  })

  it('redacts list, preview and commit errors with latest 401 recovery and preserves failed preview for retry', async () => {
    const first = vi.fn(), latest = vi.fn(), notice = vi.fn(), changed = vi.fn()
    await renderOfficial({ onAuthRequired: first })
    const load = currentOfficial.loadOfficialPricing
    await renderOfficial({ onAuthRequired: latest, onNotice: notice, onChanged: changed })
    expect(currentOfficial.loadOfficialPricing).toBe(load)
    vi.mocked(api.fetchOfficialPricing).mockRejectedValueOnce(new api.ApiError('private list', 401))
    await act(async () => load())
    expect(currentOfficial.error).toBe('errors.load_official'); expect(first).not.toHaveBeenCalled(); expect(latest).toHaveBeenCalledOnce()
    vi.mocked(api.previewOfficialPricingRefresh).mockRejectedValueOnce(new api.ApiError('private preview', 403))
    await act(async () => currentOfficial.previewOfficialPricing('models-dev'))
    expect(currentOfficial.previewError).toBe('errors.preview_official'); expect(latest).toHaveBeenCalledOnce()
    await act(async () => currentOfficial.previewOfficialPricing('models-dev'))
    vi.mocked(api.commitOfficialPricingRefresh).mockRejectedValueOnce(new api.ApiError('private commit', 401))
    await act(async () => expect(await currentOfficial.commitOfficialPricing()).toBeNull())
    expect(notice).toHaveBeenCalledExactlyOnceWith('error', 'errors.commit_official'); expect(latest).toHaveBeenCalledTimes(2)
    expect(currentOfficial.preview).toEqual(preview); expect(changed).not.toHaveBeenCalled()
  })

  it('preserves complete zero-price single and atomic batch payloads and isolates rejected parent refresh', async () => {
    const changed = vi.fn(() => Promise.reject(new Error('parent refresh'))), notice = vi.fn()
    await renderOfficial({ onChanged: changed, onNotice: notice })
    await act(async () => currentOfficial.saveOfficialPrices([official]))
    expect(api.saveOfficialPricing).toHaveBeenCalledExactlyOnceWith(official, expect.any(AbortSignal))
    const inputs = [official, { ...official, canonical_model_id: 'other' }]
    await act(async () => currentOfficial.saveOfficialPrices(inputs))
    expect(api.saveOfficialPricingBatch).toHaveBeenCalledExactlyOnceWith(inputs, expect.any(AbortSignal))
    expect(changed).toHaveBeenCalledTimes(2); expect(currentOfficial.saving).toBe(false)
    vi.mocked(api.saveOfficialPricingBatch).mockRejectedValueOnce(new Error('secret network path'))
    await act(async () => expect(await currentOfficial.saveOfficialPrices(inputs)).toBeNull())
    expect(changed).toHaveBeenCalledTimes(2); expect(notice).toHaveBeenLastCalledWith('error', 'errors.save_official')
  })
})
