// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { DualCosts, PriceEstimate, PricingSelection } from '@/lib/types';
import { DualCostsDisplay, PricingExplanation } from '../DualCosts';
import { fetchUsageEvents } from '@/lib/api';
import { StatCards } from '../StatCards';
vi.mock('react-i18next', async (importOriginal) => ({ ...await importOriginal<typeof import('react-i18next')>(), useTranslation: () => ({ t: (key: string) => key }) }));
const estimate = (amount: number | null, status: PriceEstimate['status'] = amount === null ? 'unavailable' : 'complete'): PriceEstimate => ({ total_cost_usd: amount, uncached_input_cost_usd: amount ?? 0, output_cost_usd: 0, cache_read_cost_usd: 0, cache_write_cost_usd: 0, complete: status === 'complete', has_known: amount !== null, status });
let container: HTMLDivElement; let root: Root;
beforeEach(() => { (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true; container = document.createElement('div'); document.body.append(container); root = createRoot(container); });
afterEach(() => { act(() => root.unmount()); container.remove(); vi.unstubAllGlobals(); });
describe('authoritative dual cost display', () => {
  it('shows both server summary amounts inside the existing Overview cost tile', () => {
    const costs = { configured: estimate(30), reference: estimate(10) };
    act(() => root.render(<StatCards usage={{ usage: { total_requests: 1, success_count: 1, failure_count: 0, total_tokens: 100 }, summary: { rpm: 1, tpm: 100, input_tokens: 100, cache_read_tokens: 0, cache_creation_tokens: 0, reasoning_tokens: 0, total_cost: 999, cost_available: false, dual_costs: costs } }} loading={false} dailyAverageUsage={null} reserveDailyAverage={false} sparklines={{ requests: null, tokens: null, rpm: null, tpm: null, cacheReadRate: null, cost: null }} />));
    expect(container.querySelector('[data-cost-estimate="configured"]')?.textContent).toContain('$30.00');
    expect(container.querySelector('[data-cost-estimate="reference"]')?.textContent).toContain('$10.00');
    expect(container.textContent).not.toContain('$999');
    expect(container.textContent).not.toContain('usage_stats.cost_need_price');
  });
  it('preserves authoritative states and amounts across the actual API boundary', async () => {
    const costs: DualCosts = { configured: estimate(2, 'partial'), reference: estimate(10) };
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ pricing_snapshot_id: 'snapshot-a', events: [{ dual_costs: costs }], total_count: 1, page: 1, page_size: 50, total_pages: 1 })));
    vi.stubGlobal('fetch', fetchMock);
    const response = await fetchUsageEvents({ range: '24h' }, undefined, { apiKeyId: 'safe-id' });
    act(() => root.render(<DualCostsDisplay costs={response.events[0].dual_costs} />));
    expect(String(fetchMock.mock.calls[0][0])).toContain('api_key_id=safe-id');
    expect(container.textContent).toContain('$2.00'); expect(container.textContent).toContain('$10.00');
    expect(container.textContent).toContain('cost_estimates.partial');
  });
  it('keeps a zero multiplier unavailable without a baseline, while fixed zero is known', () => {
    const missing = { ...estimate(null), unavailable_reason: 'missing_baseline' };
    const selection: PricingSelection = { snapshot_id: 'a', scope: 'credential_model', mode: 'multiplier', multiplier: 0, dual_costs: { configured: missing, reference: missing } };
    act(() => root.render(<PricingExplanation selection={selection} />));
    expect(container.textContent).not.toContain('cost_estimates.known_zero');
    expect(container.textContent).toContain('cost_estimates.reasons.missing_baseline');
    act(() => root.render(<PricingExplanation selection={{ ...selection, mode: 'fixed', fixed: { prompt_price_per_1m: 0, completion_price_per_1m: 0, cache_read_price_per_1m: 0, cache_write_price_per_1m: 0, pricing_style: 'claude' }, pricing_style: 'claude', dual_costs: { configured: estimate(0), reference: missing } }} />));
    expect(container.querySelector('[data-cost-estimate="configured"]')?.textContent).toContain('cost_estimates.known_zero');
    expect(container.querySelector('[data-cost-estimate="reference"]')?.textContent).toContain('cost_estimates.unavailable');
    for (const key of ['prompt', 'completion', 'cache_read', 'cache_write']) expect(container.textContent).toContain(`cost_estimates.${key}_price_per_1m`);
    expect(container.textContent).toContain('claude');
  });
  it.each([[30, 10], [2, 10], [3, 10], [0, 10], [3, null], [null, null]])('displays independent backend amounts %s/%s', (configured, reference) => {
    act(() => root.render(<DualCostsDisplay costs={{ configured: estimate(configured), reference: estimate(reference) }} configuredFallback={999} />));
    const groups = container.querySelectorAll('[data-cost-estimate]');
    expect(groups).toHaveLength(2);
    expect(groups[0].textContent).toContain('cost_estimates.configured');
    expect(groups[1].textContent).toContain('cost_estimates.reference');
    for (const [index, value] of [configured, reference].entries()) {
      expect(groups[index].textContent).toContain(value === null ? 'cost_estimates.unavailable' : `$${value.toFixed(2)}`);
    }
    expect(container.textContent).not.toContain('999');
    if (configured === 0) expect(groups[0].textContent).toContain('cost_estimates.known_zero');
  });
  it('distinguishes partial known subtotal from fully unknown and maps safe reasons', () => {
    const costs: DualCosts = { configured: { ...estimate(2, 'partial'), unavailable_reason: 'retained_evidence_incomplete' }, reference: { ...estimate(null), unavailable_reason: 'missing_baseline' } };
    act(() => root.render(<DualCostsDisplay costs={costs} />));
    expect(container.textContent).toContain('$2.00');
    expect(container.textContent).toContain('cost_estimates.partial');
    expect(container.textContent).toContain('cost_estimates.reasons.retained_evidence_incomplete');
    expect(container.textContent).toContain('cost_estimates.reasons.missing_baseline');
  });
  it('never fabricates a baseline or completeness on an older server', () => {
    act(() => root.render(<DualCostsDisplay configuredFallback={2} />));
    expect(container.textContent).toContain('$2.00');
    expect(container.textContent).toContain('cost_estimates.not_provided');
    expect(container.textContent).not.toContain('cost_estimates.complete');
  });
  it('renders an expandable explicit safe explanation, including independent models and replacement', () => {
    const selection: PricingSelection = { snapshot_id: 'synthetic_snapshot', scope: 'credential_model', mode: 'multiplier', subject_id: 'synthetic_subject', subject_name: 'Safe subject', channel_id: 'synthetic_channel', channel_name: 'Safe channel', selected_model: 'selected', selected_by: 'model', baseline_model: 'baseline', baseline_by: 'model_alias', multiplier: 0, legacy_adjustments_replaced: true, legacy_model_multiplier: 2, legacy_rule_multiplier: 3, final_multiplier: 0, matched_rules: [{ key: 'safe-rule', multiplier: 3 }], dual_costs: { configured: estimate(0), reference: estimate(10) } };
    Object.assign(selection, { lookup_key: 'SECRET_LOOKUP', api_key: 'SECRET_API', auth_path: 'SECRET_PATH', error: 'SECRET_BODY' });
    act(() => root.render(<PricingExplanation selection={selection} />));
    expect(container.querySelector('details > summary')).not.toBeNull();
    expect(container.textContent).toContain('Safe subject'); expect(container.textContent).toContain('Safe channel');
    expect(container.textContent).toContain('selected'); expect(container.textContent).toContain('baseline');
    expect(container.textContent).toContain('cost_estimates.replacement_warning'); expect(container.textContent).toContain('safe-rule');
    expect(container.textContent).not.toContain('SECRET');
  });
  it('expands pricing evidence without toggling its surrounding clickable request row', () => {
    const onRowClick = vi.fn();
    act(() => root.render(<div onClick={onRowClick}><PricingExplanation selection={{ snapshot_id: 'a', scope: 'legacy', mode: 'legacy' }} /></div>));
    act(() => container.querySelector('summary')!.dispatchEvent(new MouseEvent('click', { bubbles: true })));
    expect(onRowClick).not.toHaveBeenCalled();
    expect(container.querySelector('details')?.open).toBe(true);
  });
  it('does not render arbitrary reason, scope, style, or matched-rule condition values', () => {
    const selection = { snapshot_id: 'safe', scope: 'SECRET_SCOPE', mode: 'SECRET_MODE', pricing_style: 'SECRET_STYLE', unavailable_reason: 'SECRET_REASON', matched_rules: [{ key: 'rule', multiplier: 2, condition: 'SECRET_CONDITION' }], dual_costs: { configured: { ...estimate(null), unavailable_reason: 'SECRET_REASON' }, reference: estimate(null) } } as unknown as PricingSelection;
    act(() => root.render(<PricingExplanation selection={selection} />));
    expect(container.textContent).not.toContain('SECRET');
  });
});
