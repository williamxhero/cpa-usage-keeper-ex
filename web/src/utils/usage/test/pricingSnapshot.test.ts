import { describe, expect, it } from 'vitest'
import { eventSnapshotsCompatible, getEventsPricingSnapshot, getOverviewPricingSnapshot, getPricingSnapshotId, pricingSnapshotsCompatible } from '../pricingSnapshot'

describe('pricing snapshot compatibility', () => {
  it('treats absent metadata as legacy-compatible without inventing a baseline', () => {
    expect(getEventsPricingSnapshot({ events: [{}] })).toEqual({ id: undefined, mixed: false })
    expect(pricingSnapshotsCompatible(undefined, 'snapshot-a')).toBe(true)
    expect(getPricingSnapshotId({ pricing_snapshot_id: '  ' })).toBeUndefined()
  })

  it('uses page, event and selection IDs as authoritative evidence', () => {
    expect(getEventsPricingSnapshot({ pricing_snapshot_id: 'a', events: [{ pricing_selection: { snapshot_id: 'b' } }] })).toEqual({ id: 'a', mixed: true })
    expect(getEventsPricingSnapshot({ events: [{ pricing_snapshot_id: 'a', pricing_selection: { snapshot_id: 'b' } }] }).mixed).toBe(true)
    expect(getEventsPricingSnapshot({ events: [{ pricing_selection: { snapshot_id: ' a ' } }] })).toEqual({ id: 'a', mixed: false })
    expect(getEventsPricingSnapshot({ events: [{ pricing_snapshot_id: 'a' }, { pricing_snapshot_id: 'b' }] }).mixed).toBe(true)
  })

  it('treats the Overview summary ID as authoritative even when the root is absent', () => {
    expect(getOverviewPricingSnapshot({ summary: { pricing_snapshot_id: 'a' } })).toEqual({ id: 'a', mixed: false })
    expect(getOverviewPricingSnapshot({ pricing_snapshot_id: 'a', summary: { pricing_snapshot_id: 'b' } })).toEqual({ id: 'a', mixed: true })
  })

  it('allows unchanged IDs and rejects cross-snapshot or internally mixed pages', () => {
    expect(eventSnapshotsCompatible({ id: 'a', mixed: false }, { id: 'a', mixed: false })).toBe(true)
    expect(eventSnapshotsCompatible({ id: 'a', mixed: false }, { id: 'b', mixed: false })).toBe(false)
    expect(eventSnapshotsCompatible({ mixed: false }, { id: 'a', mixed: true })).toBe(false)
  })
})
