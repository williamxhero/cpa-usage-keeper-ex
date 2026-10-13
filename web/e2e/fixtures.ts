import { expect, type Page, type Route } from '@playwright/test'

const credentials = Array.from({ length: 12 }, (_, index) => ({
  directory_id: index + 1,
  subject_id: `subject-${index + 1}`,
  name: `Synthetic ${['Alpha', 'Beta', 'Gamma', 'Delta', 'Epsilon', 'Zeta', 'Eta', 'Theta', 'Iota', 'Kappa', 'Lambda', 'Mu'][index]}`,
  provider_type: 'openai',
  auth_type: 'api_key',
  status: 'active' as const,
  binding_status: 'bound' as const,
}))
const models = ['synthetic/complete', 'synthetic/zero', 'synthetic/partial', ...Array.from({ length: 7 }, (_, i) => `synthetic/model-${String(i + 4).padStart(2, '0')}`)]
const pricing = (model: string, prompt: number | null, completion: number | null, read: number | null, write: number | null) => ({
  subject_id: 'subject-1', credential: null, canonical_model_id: model, covered: true, mode: 'fixed' as const, pricing_style: 'openai' as const,
  prompt_price_per_1m: prompt, completion_price_per_1m: completion, cache_read_price_per_1m: read, cache_write_price_per_1m: write, snapshot_id: 'synthetic-snapshot',
})

export const matrixSeed = [
  pricing('synthetic/complete', 1, 2, 3, 4),
  pricing('synthetic/zero', 0, 0, 0, 0),
  pricing('synthetic/partial', 1, 2, null, null),
]

export class SyntheticApi {
  readonly unexpected: string[] = []
  readonly requests: { method: string; path: string; body?: unknown }[] = []
  matrix = structuredClone(matrixSeed)
  official: unknown[] = [{
    id: 1, canonical_model_id: 'official-complete', display_model_id: 'Official Complete', source: 'official', pricing_style: 'openai',
    prompt_price_per_1m: 1, completion_price_per_1m: 2, cache_read_price_per_1m: 3, cache_write_price_per_1m: 4, missing_segments: [], stale: false,
  }]
  failNextMutation = false
  failNextMatrixWith401 = false
  authStatus = 200

  async install(page: Page, role: 'admin' | 'api_key_viewer' = 'admin', locale = 'en') {
    await page.addInitScript((language) => localStorage.setItem('cpa-usage-keeper-language', language), locale)
    await page.route('**/*', async (route) => {
      const url = new URL(route.request().url())
      if (url.hostname === '127.0.0.1' || url.hostname === 'localhost') return route.continue()
      this.unexpected.push(`external ${route.request().method()} ${url.origin}${url.pathname}`)
      await route.abort()
    })
    await page.route('**/api/**', async (route) => this.handle(route, role))
  }

  private async handle(route: Route, role: 'admin' | 'api_key_viewer') {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^.*\/api\/v1/, '')
    const method = request.method()
    let body: unknown
    try { body = request.postDataJSON() } catch { /* no JSON request body */ }
    this.requests.push({ method, path, body })
    const respond = (payload: unknown, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(payload) })

    if (path === '/auth/session' && method === 'GET') {
      return respond(this.authStatus === 200 ? { authenticated: true, role, ...(role === 'api_key_viewer' ? { api_key: { display_key: 'sk-synthetic' } } : {}) } : { authenticated: false }, this.authStatus)
    }
    if (path === '/status' && method === 'GET') return respond({ running: true, sync_running: false, timezone: 'UTC' })
    if (path === '/version' && method === 'GET') return respond({ version: 'synthetic', updateCheckEnabled: false })
    if (path === '/usage/api-keys/options' && method === 'GET') return respond({ options: [] })
    if (path === '/usage/api-keys/settings' && method === 'GET') return respond({ items: [] })
    if (path === '/auth/sessions' && method === 'GET') return respond({ sessions: [] })
    if (method === 'GET' && ['/usage/overview', '/usage/overview/comparisons', '/usage/events', '/usage/overview/realtime', '/usage/analysis', '/usage/analysis/latency'].includes(path)) return respond({})
    if (path === '/pricing/credentials/sync-status' && method === 'GET') return respond({ state: 'idle', in_flight: false })
    if (path === '/pricing/credential-model-matrix' || path === '/key/pricing/credential-model-matrix') {
      if (method === 'GET') {
        if (this.failNextMatrixWith401) { this.failNextMatrixWith401 = false; return respond({}, 401) }
        const credential = url.searchParams.get('credential')
        const model = url.searchParams.get('model')
        return respond({ credentials, models, cells: this.matrix.filter((cell) => (!credential || cell.subject_id === credential) && (!model || cell.canonical_model_id.includes(model))), credential_page: 1, credential_page_size: 50, model_page: 1, model_page_size: 50, total_credentials: credentials.length, total_models: models.length, snapshot_id: 'synthetic-snapshot' })
      }
      if (method === 'PUT' && role === 'admin') {
        if (this.failNextMutation) { this.failNextMutation = false; return respond({ detail: 'secret token raw-body-must-not-render' }, 500) }
        const cells = (body as { cells?: Record<string, unknown>[] })?.cells ?? []
        for (const input of cells) {
          this.matrix = this.matrix.filter((cell) => !(cell.subject_id === input.subject_id && cell.canonical_model_id === input.canonical_model_id))
          this.matrix.push(input.mode === 'inherit' ? { ...input, credential: null, covered: true, mode: 'inherit', snapshot_id: 'synthetic-snapshot' } as typeof this.matrix[number] : { ...input, credential: null, covered: true, snapshot_id: 'synthetic-snapshot' } as typeof this.matrix[number])
        }
        return respond({ cells: this.matrix })
      }
    }
    if (path === '/pricing/official' && method === 'GET' && role === 'admin') return respond({ pricing: this.official })
    if (path === '/pricing/official' && method === 'PUT' && role === 'admin') {
      if (this.failNextMutation) { this.failNextMutation = false; return respond({ detail: 'secret token raw-body-must-not-render' }, 500) }
      const input = body as Record<string, unknown>
      this.official = [{ ...input, id: 1, missing_segments: [], stale: false }]
      return respond(this.official[0])
    }
    if (path === '/pricing/official/batch' && method === 'PUT' && role === 'admin') return respond({ pricing: this.official })
    if (path === '/pricing/official/refresh/preview' && method === 'GET' && role === 'admin') return respond({ preview_id: 'synthetic-preview', source_id: 'models-dev', source: 'Synthetic catalog', source_url: 'https://example.invalid/synthetic', fetched_at: '2026-01-01T00:00:00Z', response_hash: 'synthetic-hash', prices: this.official, stale: false })
    if (path === '/pricing/official/refresh/commit' && method === 'POST' && role === 'admin') return respond({ pricing: this.official })

    this.unexpected.push(`${method} ${path}`)
    return respond({ error: 'unexpected synthetic API request' }, 599)
  }

  async expectNoUnexpected() {
    expect(this.unexpected, `Unexpected API/external requests: ${this.unexpected.join(', ')}`).toEqual([])
  }
}
