export interface PricingSnapshotState {
  id?: string
  mixed: boolean
}

export function getPricingSnapshotId(value: unknown): string | undefined {
  if (!value || typeof value !== 'object') return undefined
  const id = (value as { pricing_snapshot_id?: unknown }).pricing_snapshot_id
  return typeof id === 'string' ? id.trim() || undefined : undefined
}

export function pricingSnapshotsCompatible(...ids: (string | undefined)[]): boolean {
  // Missing metadata is legacy-compatible, not evidence of a shared baseline.
  return new Set(ids.filter(Boolean)).size <= 1
}

export function getOverviewPricingSnapshot(overview: unknown): PricingSnapshotState {
  const rootId = getPricingSnapshotId(overview)
  const summaryId = overview && typeof overview === 'object'
    ? getPricingSnapshotId((overview as { summary?: unknown }).summary)
    : undefined
  return { id: rootId ?? summaryId, mixed: !pricingSnapshotsCompatible(rootId, summaryId) }
}

export function getAnalysisPricingSnapshot(analysis: unknown): PricingSnapshotState {
  const rootId = getPricingSnapshotId(analysis)
  const breakdownId = analysis && typeof analysis === 'object'
    ? getPricingSnapshotId((analysis as { cost_breakdown?: unknown }).cost_breakdown)
    : undefined
  return { id: rootId ?? breakdownId, mixed: !pricingSnapshotsCompatible(rootId, breakdownId) }
}

export function getEventsPricingSnapshot(page: { events: readonly unknown[]; pricing_snapshot_id?: string }): PricingSnapshotState {
  const ids = [getPricingSnapshotId(page)]
  for (const event of page.events) {
    ids.push(getPricingSnapshotId(event))
    if (event && typeof event === 'object') {
      const selection = (event as { pricing_selection?: { snapshot_id?: unknown } }).pricing_selection
      const id = selection?.snapshot_id
      ids.push(typeof id === 'string' ? id.trim() || undefined : undefined)
    }
  }
  return { id: ids.find(Boolean), mixed: !pricingSnapshotsCompatible(...ids) }
}

export function eventSnapshotsCompatible(current: PricingSnapshotState, incoming: PricingSnapshotState): boolean {
  return !current.mixed && !incoming.mixed && pricingSnapshotsCompatible(current.id, incoming.id)
}
