import type { PricingCredential, PricingStyle } from '../types'

/** Query dimensions are independently paged; the backend caps each page at 50. */
export interface PricingMatrixQuery {
  credentialPage?: number
  credentialPageSize?: number
  modelPage?: number
  modelPageSize?: number
  credential?: string
  model?: string
}

export interface PricingFourPrices {
  pricing_style: PricingStyle
  prompt_price_per_1m: number
  completion_price_per_1m: number
  cache_read_price_per_1m: number
  cache_write_price_per_1m: number
}

export interface PricingMatrixCell {
  subject_id: string
  credential: PricingCredential | null
  canonical_model_id: string
  covered: boolean
  mode: 'fixed' | 'inherit'
  pricing_style?: PricingStyle
  prompt_price_per_1m?: number | null
  completion_price_per_1m?: number | null
  cache_read_price_per_1m?: number | null
  cache_write_price_per_1m?: number | null
  source?: string
  snapshot_id: string
}

/** Only persisted overrides are returned. Missing cells do not establish availability. */
export interface PricingMatrixResponse {
  credentials: PricingCredential[]
  models: string[]
  cells: PricingMatrixCell[]
  credential_page: number
  credential_page_size: number
  model_page: number
  model_page_size: number
  total_credentials: number
  total_models: number
  snapshot_id: string
}

export interface PricingMatrixCellTarget {
  subject_id: string
  canonical_model_id: string
}

export type PricingMatrixCellInput = PricingMatrixCellTarget & (
  | ({ mode: 'fixed' } & PricingFourPrices)
  | { mode: 'inherit' }
)

export interface PricingMatrixMutationResponse {
  cells: PricingMatrixCell[]
}

export type PricingOfficialSource = 'official' | 'models-dev' | 'litellm' | 'openrouter' | 'legacy-seed'
export type PricingOfficialRefreshSource = 'models-dev' | 'litellm' | 'openrouter'

export interface PricingOfficialEntry {
  id: number
  canonical_model_id: string
  display_model_id: string
  source: PricingOfficialSource
  pricing_style: PricingStyle
  source_url?: string
  fetched_at?: string
  updated_at?: string
  response_hash?: string
  source_version?: string
  prompt_price_per_1m: number | null
  completion_price_per_1m: number | null
  cache_read_price_per_1m: number | null
  cache_write_price_per_1m: number | null
  missing_segments: string[]
  stale: boolean
  stale_at?: string
  aliases?: string[]
}

/** Manual writes require complete prices; null is reserved for upstream missing data. */
export interface PricingOfficialInput extends PricingFourPrices {
  canonical_model_id: string
  display_model_id: string
  source: PricingOfficialSource
  source_url?: string
  aliases?: string[]
}

export interface PricingOfficialResponse {
  pricing: PricingOfficialEntry[]
}

export interface PricingOfficialPreviewResponse {
  preview_id: string
  source_id: PricingOfficialRefreshSource
  source: string
  source_url: string
  fetched_at: string
  response_hash: string
  prices: PricingOfficialEntry[]
  ambiguous_models?: string[]
  stale: boolean
}

export interface PricingCredentialSyncStatus {
  state: 'idle' | 'queued' | 'running' | 'succeeded' | 'failed' | 'unavailable'
  in_flight: boolean
  last_attempt_at?: string
  last_success_at?: string
  last_failure_at?: string
  last_error_code?: string
}
