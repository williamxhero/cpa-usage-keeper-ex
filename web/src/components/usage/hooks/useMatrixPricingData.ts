import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ApiError,
  commitOfficialPricingRefresh,
  fetchCredentialPricingMatrix,
  fetchOfficialPricing,
  fetchPricingCredentialSyncStatus,
  previewOfficialPricingRefresh,
  refreshPricingCredentials,
  saveOfficialPricing,
  saveOfficialPricingBatch,
  updateCredentialPricingMatrix,
} from '@/lib/api'
import type {
  PricingCredentialSyncStatus,
  PricingMatrixCellInput,
  PricingMatrixQuery,
  PricingMatrixResponse,
  PricingOfficialEntry,
  PricingOfficialInput,
  PricingOfficialPreviewResponse,
  PricingOfficialRefreshSource,
  PricingOfficialResponse,
} from '@/lib/types'

export type PricingNotice = (kind: 'success' | 'info' | 'error', message: string) => void

export interface UseMatrixPricingDataOptions {
  enabled?: boolean
  canManage?: boolean
  query?: PricingMatrixQuery
  onAuthRequired?: () => void
  onChanged?: () => void
  onNotice?: PricingNotice
}

export interface UseMatrixPricingDataReturn {
  matrix: PricingMatrixResponse | null
  loading: boolean
  error: string
  saving: boolean
  loadMatrix: () => Promise<void>
  saveCells: (cells: PricingMatrixCellInput[]) => Promise<boolean>
  syncStatus: PricingCredentialSyncStatus | null
  syncLoading: boolean
  syncError: string
  refreshing: boolean
  loadSyncStatus: () => Promise<PricingCredentialSyncStatus | null>
  refreshCredentials: () => Promise<PricingCredentialSyncStatus | null>
  canRead: boolean
  canEdit: boolean
}

export interface UseOfficialPricingDataOptions {
  enabled?: boolean
  canManage?: boolean
  onAuthRequired?: () => void
  onChanged?: () => void
  onNotice?: PricingNotice
}

export interface UseOfficialPricingDataReturn {
  pricing: PricingOfficialEntry[]
  loading: boolean
  error: string
  saving: boolean
  loadOfficialPricing: () => Promise<void>
  saveOfficialPrice: (input: PricingOfficialInput) => Promise<PricingOfficialEntry | null>
  saveOfficialPrices: (inputs: PricingOfficialInput[]) => Promise<PricingOfficialResponse | null>
  preview: PricingOfficialPreviewResponse | null
  previewing: boolean
  previewError: string
  previewOfficialPricing: (source: PricingOfficialRefreshSource) => Promise<PricingOfficialPreviewResponse | null>
  commitOfficialPricing: () => Promise<PricingOfficialResponse | null>
  clearPreview: () => void
  canRead: boolean
  canEdit: boolean
}

const localizedError = (t: (key: string) => string, key: string): string => t(`errors.${key}`)

function useLatest<T>(value: T) {
  const ref = useRef(value)
  useEffect(() => { ref.current = value }, [value])
  return ref
}

export function useMatrixPricingData(options: UseMatrixPricingDataOptions = {}): UseMatrixPricingDataReturn {
  const { enabled = true, canManage = true, query = {} } = options
  const { t } = useTranslation('matrix_pricing')
  const canRead = enabled
  const canEdit = enabled && canManage
  const [matrix, setMatrix] = useState<PricingMatrixResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [syncStatus, setSyncStatus] = useState<PricingCredentialSyncStatus | null>(null)
  const [syncLoading, setSyncLoading] = useState(false)
  const [syncError, setSyncError] = useState('')
  const [refreshing, setRefreshing] = useState(false)
  const matrixRequest = useRef<AbortController | null>(null)
  const statusRequest = useRef<AbortController | null>(null)
  const mutationRequest = useRef<AbortController | null>(null)
  const credentialRefreshRequest = useRef<AbortController | null>(null)
  const authRef = useLatest(options.onAuthRequired)
  const changedRef = useLatest(options.onChanged)
  const noticeRef = useLatest(options.onNotice)
  const enabledRef = useLatest(enabled)
  const canManageRef = useLatest(canManage)
  const queryKey = `${query.credentialPage ?? ''}:${query.credentialPageSize ?? ''}:${query.modelPage ?? ''}:${query.modelPageSize ?? ''}:${query.credential ?? ''}:${query.model ?? ''}`
  const queryRef = useLatest(query)
  const queryKeyRef = useLatest(queryKey)
  const tRef = useLatest(t)

  const notifyChanged = useCallback(() => {
    try { void Promise.resolve(changedRef.current?.()).catch(() => {}) } catch { /* Parent refresh is independent. */ }
  }, [changedRef])
  const authFailure = useCallback((reason: unknown) => {
    if (reason instanceof ApiError && reason.status === 401) authRef.current?.()
  }, [authRef])
  const loadMatrix = useCallback(async () => {
    if (!enabledRef.current) return
    matrixRequest.current?.abort()
    const controller = new AbortController()
    matrixRequest.current = controller
    const requestQueryKey = queryKeyRef.current
    setLoading(true)
    setError('')
    try {
      const result = await fetchCredentialPricingMatrix(queryRef.current, controller.signal, !canManageRef.current)
      if (matrixRequest.current !== controller || queryKeyRef.current !== requestQueryKey) return
      setMatrix(result)
    } catch (reason) {
      if (controller.signal.aborted || matrixRequest.current !== controller) return
      authFailure(reason)
      setError(localizedError(tRef.current, 'load_matrix'))
    } finally {
      if (matrixRequest.current === controller) {
        matrixRequest.current = null
        setLoading(false)
      }
    }
  }, [authFailure, canManageRef, enabledRef, queryKeyRef, queryRef, tRef])
  const loadSyncStatus = useCallback(async () => {
    if (!enabledRef.current || !canManageRef.current) return null
    statusRequest.current?.abort()
    const controller = new AbortController()
    statusRequest.current = controller
    setSyncLoading(true)
    setSyncError('')
    try {
      const status = await fetchPricingCredentialSyncStatus(controller.signal)
      if (statusRequest.current !== controller) return null
      setSyncStatus(status)
      return status
    } catch (reason) {
      if (controller.signal.aborted || statusRequest.current !== controller) return null
      authFailure(reason)
      setSyncError(localizedError(tRef.current, 'load_sync'))
      return null
    } finally {
      if (statusRequest.current === controller) {
        statusRequest.current = null
        setSyncLoading(false)
      }
    }
  }, [authFailure, canManageRef, enabledRef, tRef])
  const saveCells = useCallback(async (cells: PricingMatrixCellInput[]) => {
    if (!enabledRef.current || !canManageRef.current || cells.length === 0) return false
    mutationRequest.current?.abort()
    const controller = new AbortController()
    mutationRequest.current = controller
    setSaving(true)
    try {
      await updateCredentialPricingMatrix(cells, controller.signal)
      if (mutationRequest.current !== controller || !enabledRef.current || !canManageRef.current) return false
      await loadMatrix()
      notifyChanged()
      noticeRef.current?.('success', tRef.current('notifications.matrix_saved'))
      return true
    } catch (reason) {
      if (controller.signal.aborted || mutationRequest.current !== controller) return false
      authFailure(reason)
      const errorKey = reason instanceof ApiError && reason.status === 413 ? 'batch_limit' : 'save_matrix'
      noticeRef.current?.('error', localizedError(tRef.current, errorKey))
      return false
    } finally {
      if (mutationRequest.current === controller) {
        mutationRequest.current = null
        setSaving(false)
      }
    }
  }, [authFailure, canManageRef, enabledRef, loadMatrix, noticeRef, notifyChanged, tRef])
  const refreshCredentials = useCallback(async () => {
    if (!enabledRef.current || !canManageRef.current) return null
    credentialRefreshRequest.current?.abort()
    const controller = new AbortController()
    credentialRefreshRequest.current = controller
    setRefreshing(true)
    try {
      const status = await refreshPricingCredentials(controller.signal)
      if (credentialRefreshRequest.current !== controller || !enabledRef.current || !canManageRef.current) return null
      setSyncStatus(status)
      noticeRef.current?.('success', tRef.current('notifications.credentials_refresh_queued'))
      return status
    } catch (reason) {
      if (controller.signal.aborted || credentialRefreshRequest.current !== controller) return null
      authFailure(reason)
      noticeRef.current?.('error', localizedError(tRef.current, 'refresh_credentials'))
      return null
    } finally {
      if (credentialRefreshRequest.current === controller) {
        credentialRefreshRequest.current = null
        setRefreshing(false)
      }
    }
  }, [authFailure, canManageRef, enabledRef, noticeRef, tRef])

  useEffect(() => {
    if (!enabled) {
      matrixRequest.current?.abort()
      statusRequest.current?.abort()
      mutationRequest.current?.abort()
      credentialRefreshRequest.current?.abort()
      matrixRequest.current = null
      statusRequest.current = null
      mutationRequest.current = null
      credentialRefreshRequest.current = null
      setMatrix(null)
      setSyncStatus(null)
      setLoading(false)
      setSaving(false)
      setSyncLoading(false)
      setRefreshing(false)
      return
    }
    void loadMatrix()
    if (canManage) void loadSyncStatus()
    return () => {
      matrixRequest.current?.abort()
      statusRequest.current?.abort()
      mutationRequest.current?.abort()
      credentialRefreshRequest.current?.abort()
      matrixRequest.current = null
      statusRequest.current = null
      mutationRequest.current = null
      credentialRefreshRequest.current = null
    }
  }, [canManage, enabled, loadMatrix, loadSyncStatus, queryKey])

  return { matrix, loading, error, saving, loadMatrix, saveCells, syncStatus, syncLoading, syncError, refreshing, loadSyncStatus, refreshCredentials, canRead, canEdit }
}

export function useOfficialPricingData(options: UseOfficialPricingDataOptions = {}): UseOfficialPricingDataReturn {
  const { enabled = true, canManage = true } = options
  const { t } = useTranslation('matrix_pricing')
  const canRead = enabled && canManage
  const canEdit = canRead
  const [pricing, setPricing] = useState<PricingOfficialEntry[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [preview, setPreview] = useState<PricingOfficialPreviewResponse | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const [previewError, setPreviewError] = useState('')
  const listRequest = useRef<AbortController | null>(null)
  const previewRequest = useRef<AbortController | null>(null)
  const mutationRequest = useRef<AbortController | null>(null)
  const authRef = useLatest(options.onAuthRequired)
  const changedRef = useLatest(options.onChanged)
  const noticeRef = useLatest(options.onNotice)
  const enabledRef = useLatest(enabled)
  const canManageRef = useLatest(canManage)
  const previewRef = useLatest(preview)
  const tRef = useLatest(t)
  const notifyChanged = useCallback(() => {
    try { void Promise.resolve(changedRef.current?.()).catch(() => {}) } catch { /* Parent refresh is independent. */ }
  }, [changedRef])
  const authFailure = useCallback((reason: unknown) => {
    if (reason instanceof ApiError && reason.status === 401) authRef.current?.()
  }, [authRef])
  const loadOfficialPricing = useCallback(async () => {
    if (!enabledRef.current || !canManageRef.current) {
      setPricing([])
      setError('')
      return
    }
    listRequest.current?.abort()
    const controller = new AbortController()
    listRequest.current = controller
    setLoading(true)
    setError('')
    try {
      const response = await fetchOfficialPricing(controller.signal)
      if (listRequest.current !== controller) return
      setPricing(response.pricing)
    } catch (reason) {
      if (controller.signal.aborted || listRequest.current !== controller) return
      authFailure(reason)
      setError(localizedError(tRef.current, 'load_official'))
    } finally {
      if (listRequest.current === controller) {
        listRequest.current = null
        setLoading(false)
      }
    }
  }, [authFailure, canManageRef, enabledRef, tRef])
  const saveOfficialPrices = useCallback(async (inputs: PricingOfficialInput[]) => {
    if (!enabledRef.current || !canManageRef.current || inputs.length === 0) return null
    mutationRequest.current?.abort()
    const controller = new AbortController()
    mutationRequest.current = controller
    setSaving(true)
    try {
      const response = inputs.length === 1
        ? { pricing: [await saveOfficialPricing(inputs[0], controller.signal)] }
        : await saveOfficialPricingBatch(inputs, controller.signal)
      if (mutationRequest.current !== controller || !enabledRef.current || !canManageRef.current) return null
      await loadOfficialPricing()
      notifyChanged()
      noticeRef.current?.('success', tRef.current('notifications.official_saved'))
      return response
    } catch (reason) {
      if (controller.signal.aborted || mutationRequest.current !== controller) return null
      authFailure(reason)
      noticeRef.current?.('error', localizedError(tRef.current, 'save_official'))
      return null
    } finally {
      if (mutationRequest.current === controller) {
        mutationRequest.current = null
        setSaving(false)
      }
    }
  }, [authFailure, canManageRef, enabledRef, loadOfficialPricing, noticeRef, notifyChanged, tRef])
  const saveOfficialPrice = useCallback(async (input: PricingOfficialInput) => {
    const result = await saveOfficialPrices([input])
    return result?.pricing[0] ?? null
  }, [saveOfficialPrices])
  const previewOfficialPricing = useCallback(async (source: PricingOfficialRefreshSource) => {
    if (!enabledRef.current || !canManageRef.current) return null
    previewRequest.current?.abort()
    const controller = new AbortController()
    previewRequest.current = controller
    setPreviewing(true)
    setPreviewError('')
    try {
      const result = await previewOfficialPricingRefresh(source, controller.signal)
      if (previewRequest.current !== controller) return null
      setPreview(result)
      return result
    } catch (reason) {
      if (controller.signal.aborted || previewRequest.current !== controller) return null
      authFailure(reason)
      setPreviewError(localizedError(tRef.current, 'preview_official'))
      return null
    } finally {
      if (previewRequest.current === controller) {
        previewRequest.current = null
        setPreviewing(false)
      }
    }
  }, [authFailure, canManageRef, enabledRef, tRef])
  const clearPreview = useCallback(() => {
    previewRequest.current?.abort()
    previewRequest.current = null
    setPreview(null)
    setPreviewError('')
    setPreviewing(false)
  }, [])
  const commitOfficialPricing = useCallback(async () => {
    const currentPreview = previewRef.current
    if (!enabledRef.current || !canManageRef.current || !currentPreview) return null
    mutationRequest.current?.abort()
    const controller = new AbortController()
    mutationRequest.current = controller
    setSaving(true)
    try {
      const response = await commitOfficialPricingRefresh(currentPreview.preview_id, controller.signal)
      if (mutationRequest.current !== controller || !enabledRef.current || !canManageRef.current) return null
      setPreview(null)
      await loadOfficialPricing()
      notifyChanged()
      noticeRef.current?.('success', tRef.current('notifications.refresh_committed'))
      return response
    } catch (reason) {
      if (controller.signal.aborted || mutationRequest.current !== controller) return null
      authFailure(reason)
      noticeRef.current?.('error', localizedError(tRef.current, 'commit_official'))
      return null
    } finally {
      if (mutationRequest.current === controller) {
        mutationRequest.current = null
        setSaving(false)
      }
    }
  }, [authFailure, canManageRef, enabledRef, loadOfficialPricing, noticeRef, notifyChanged, previewRef, tRef])

  useEffect(() => {
    if (!canRead) {
      listRequest.current?.abort()
      previewRequest.current?.abort()
      mutationRequest.current?.abort()
      listRequest.current = null
      previewRequest.current = null
      mutationRequest.current = null
      setPricing([])
      setPreview(null)
      setError('')
      setPreviewError('')
      setLoading(false)
      setPreviewing(false)
      setSaving(false)
      return
    }
    void loadOfficialPricing()
    return () => {
      listRequest.current?.abort()
      previewRequest.current?.abort()
      mutationRequest.current?.abort()
      listRequest.current = null
      previewRequest.current = null
      mutationRequest.current = null
    }
  }, [canRead, loadOfficialPricing])

  return { pricing, loading, error, saving, loadOfficialPricing, saveOfficialPrice, saveOfficialPrices, preview, previewing, previewError, previewOfficialPricing, commitOfficialPricing, clearPreview, canRead, canEdit }
}
