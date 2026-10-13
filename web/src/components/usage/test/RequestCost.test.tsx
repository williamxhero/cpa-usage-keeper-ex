// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PriceEstimate, PricingSelection } from '@/lib/types';
import { RequestCost } from '../RequestCost';

vi.mock('react-i18next', async (importOriginal) => ({ ...await importOriginal<typeof import('react-i18next')>(), useTranslation: () => ({ t: (key: string) => key }) }));
const estimate = (amount: number | null, status: PriceEstimate['status'] = amount === null ? 'unavailable' : 'complete'): PriceEstimate => ({ total_cost_usd: amount, uncached_input_cost_usd: amount ?? 0, output_cost_usd: 0, cache_read_cost_usd: 0, cache_write_cost_usd: 0, complete: status === 'complete', has_known: amount !== null, status });
let container: HTMLDivElement;
let root: Root;
beforeEach(() => {
  globalThis.IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement('div'); document.body.append(container); root = createRoot(container);
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); });
describe('request list configured cost', () => {
  it.each([[2, 'complete', '$2.00'], [0, 'complete', '$0'], [2, 'partial', '$2.00'], [null, 'unavailable', '—']] as const)('shows only one amount for %s/%s', async (amount, status, text) => {
    await act(async () => root.render(<RequestCost costs={{ configured: estimate(amount, status), reference: estimate(99) }} configuredFallback={999} />));
    expect(container.textContent).toBe(text);
    expect(container.textContent).not.toContain('reference');
    expect(container.querySelector('[title]')?.getAttribute('title')).toContain(`cost_estimates.${status}`);
    expect(container.querySelectorAll('[data-request-cost]')).toHaveLength(1);
  });
  it('prefers authoritative selection estimates and shows dual totals only once in details', async () => {
    const costs = { configured: estimate(2, 'partial'), reference: estimate(99) };
    const selection = { dual_costs: costs, snapshot_id: 'selection-snapshot' } as PricingSelection;
    await act(async () => root.render(<RequestCost selection={selection} configuredFallback={999} />));
    expect(container.textContent).toBe('$2.00');
    await act(async () => container.querySelector<HTMLButtonElement>('button')!.click());
    const dialog = document.querySelector('[role="dialog"]');
    expect(dialog?.querySelectorAll('[data-cost-estimate="configured"]')).toHaveLength(1);
    expect(dialog?.querySelectorAll('[data-cost-estimate="reference"]')).toHaveLength(1);
    expect(dialog?.textContent).toContain('selection-snapshot');
  });
  it('uses only a finite explicitly available legacy fallback', async () => {
    await act(async () => root.render(<RequestCost configuredFallback={0} />));
    expect(container.textContent).toBe('$0');
    await act(async () => root.render(<RequestCost configuredFallback={null} />));
    expect(container.textContent).toBe('—');
  });
  it('keeps source and both prices in an on-demand detail dialog, not the cell', async () => {
    const costs = { configured: { ...estimate(2, 'partial'), source: 'credential_matrix', snapshot_id: 'safe-snapshot' }, reference: estimate(99) };
    await act(async () => root.render(<RequestCost costs={costs} />));
    expect(document.querySelector('[role="dialog"]')).toBeNull();
    await act(async () => container.querySelector<HTMLButtonElement>('button')!.click());
    expect(container.textContent).toBe('$2.00');
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain('$99.00');
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain('safe-snapshot');
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain('cost_estimates.partial');
  });
});
