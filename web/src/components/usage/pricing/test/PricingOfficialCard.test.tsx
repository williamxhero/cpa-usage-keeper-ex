// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PricingOfficialCard } from '../PricingOfficialCard';
import type { PricingOfficialEntry, PricingOfficialInput, PricingOfficialPreviewResponse } from '@/lib/types';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;
vi.mock('react-i18next', () => ({ useTranslation: (namespace: string) => ({ t: (key: string, values?: Record<string, unknown>) => key === 'official_page' ? `Page ${values?.page}` : `${namespace}.${key}`, i18n: { language: 'en' } }) }));
const hook = vi.hoisted(() => ({
  loadOfficialPricing: vi.fn(), saveOfficialPrice: vi.fn(), saveOfficialPrices: vi.fn(),
  previewOfficialPricing: vi.fn(), commitOfficialPricing: vi.fn(), clearPreview: vi.fn(),
}));
vi.mock('@/components/usage/hooks/useMatrixPricingData', () => ({
  useOfficialPricingData: (options: { canManage: boolean }) => ({
    pricing: options.canManage ? data.rows : [], loading: false, error: '', saving: false,
    loadOfficialPricing: hook.loadOfficialPricing, saveOfficialPrice: hook.saveOfficialPrice,
    saveOfficialPrices: hook.saveOfficialPrices, preview: data.preview, previewing: false, previewError: '',
    previewOfficialPricing: hook.previewOfficialPricing, commitOfficialPricing: hook.commitOfficialPricing,
    clearPreview: hook.clearPreview, canRead: options.canManage, canEdit: options.canManage,
  }),
}));

const base = (id: number, override: Partial<PricingOfficialEntry> = {}): PricingOfficialEntry => ({
  id, canonical_model_id: `provider/model-${id}`, display_model_id: `Model ${id}`, source: 'official',
  pricing_style: 'openai', prompt_price_per_1m: 0, completion_price_per_1m: 2,
  cache_read_price_per_1m: null, cache_write_price_per_1m: 4, missing_segments: ['cache_read_price_per_1m'],
  stale: false, updated_at: '2025-01-02T00:00:00Z', fetched_at: '2025-01-03T00:00:00Z', ...override,
});
const preview: PricingOfficialPreviewResponse = {
  preview_id: 'preview-safe-id', source_id: 'models-dev', source: 'models-dev', source_url: 'https://example.invalid',
  fetched_at: '2025-01-03T00:00:00Z', response_hash: 'b'.repeat(64), prices: [base(2)], ambiguous_models: [], stale: false,
};
const data = { rows: [base(1), base(2, { stale: true, missing_segments: [] })], preview: null as PricingOfficialPreviewResponse | null };

describe('PricingOfficialCard', () => {
  let root: Root;
  let container: HTMLDivElement;
  const button = (key: string, scope: ParentNode = container) => Array.from(scope.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent?.includes(`matrix_pricing.${key}`))!;
  const setInput = async (element: HTMLInputElement, text: string) => act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(element, text);
    element.dispatchEvent(new Event('input', { bubbles: true }));
  });
  beforeEach(() => {
    data.rows = [base(1), base(2, { stale: true, missing_segments: [] })]; data.preview = null;
    Object.values(hook).forEach(fn => fn.mockReset());
    container = document.createElement('div'); document.body.appendChild(container); root = createRoot(container);
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
  });
  afterEach(async () => { await act(async () => root.unmount()); document.body.innerHTML = ''; vi.restoreAllMocks(); });

  it('does not fetch admin prices or render writes for readonly access', async () => {
    await act(async () => root.render(<PricingOfficialCard />));
    expect(container.textContent).toContain('matrix_pricing.official_read_limit');
    expect(container.querySelector('table')).toBeNull();
    expect(container.querySelector('input')).toBeNull();
    expect(hook.loadOfficialPricing).not.toHaveBeenCalled();
    expect(hook.previewOfficialPricing).not.toHaveBeenCalled();
  });

  it('renders zero distinctly from missing and maps safe matcher reasons', async () => {
    data.rows[0] = base(1, { unavailable_reason: 'no_match' } as Partial<PricingOfficialEntry>);
    await act(async () => root.render(<PricingOfficialCard canManage />));
    const row = container.querySelector('tbody tr')!;
    expect(row.textContent).toContain('$0');
    expect(row.textContent).toContain('matrix_pricing.official_missing_price');
    expect(row.textContent).toContain('matrix_pricing.official_no_match');
    expect(row.textContent).not.toContain('unavailable_reason');
  });

  it('renders only validated version and response-hash metadata', async () => {
    const responseHash = 'a'.repeat(64);
    data.rows = [
      base(1, { source_version: '2026-10-01', response_hash: responseHash }),
      base(2, { source_version: 'SECRET_VERSION', response_hash: 'SECRET_HASH' }),
    ];
    await act(async () => root.render(<PricingOfficialCard canManage />));
    const rows = container.querySelectorAll('tbody tr');
    expect(rows[0].textContent).toContain('matrix_pricing.official_source_version: 2026-10-01');
    expect(rows[0].textContent).toContain(`matrix_pricing.official_response_hash: ${responseHash}`);
    expect(rows[1].textContent).not.toContain('SECRET');
  });

  it('submits bounded search and pagination over returned rows', async () => {
    data.rows = Array.from({ length: 27 }, (_, index) => base(index + 1));
    await act(async () => root.render(<PricingOfficialCard canManage />));
    expect(container.querySelectorAll('tbody tr')).toHaveLength(25);
    expect(container.textContent).toContain('Page 1');
    await act(async () => button('official_next').click());
    expect(container.querySelectorAll('tbody tr')).toHaveLength(2);
    expect(container.textContent).toContain('Page 2');
    const input = container.querySelector<HTMLInputElement>('input[aria-label="matrix_pricing.official_search"]')!;
    await setInput(input, 'model-27');
    await act(async () => button('official_search_action').click());
    expect(container.querySelectorAll('tbody tr')).toHaveLength(1);
    expect(container.querySelector('tbody')?.textContent).toContain('provider/model-27');
  });

  it('saves all four manual prices while retaining explicit zero', async () => {
    hook.saveOfficialPrice.mockResolvedValue(base(1));
    await act(async () => root.render(<PricingOfficialCard canManage />));
    await act(async () => button('official_edit').click());
    const values = ['0', '2.5', '3', '4'];
    const inputs = Array.from(document.body.querySelectorAll<HTMLInputElement>('.modal input'));
    expect(inputs).toHaveLength(4);
    for (const [index, value] of values.entries()) await setInput(inputs[index], value);
    const saveButton = Array.from(document.body.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent?.includes('matrix_pricing.official_save'))!;
    await act(async () => saveButton.click());
    const sent = hook.saveOfficialPrice.mock.calls[0][0] as PricingOfficialInput;
    expect([sent.prompt_price_per_1m, sent.completion_price_per_1m, sent.cache_read_price_per_1m, sent.cache_write_price_per_1m]).toEqual([0, 2.5, 3, 4]);
  });

  it('previews a refresh before sending the separate explicit commit', async () => {
    data.preview = preview;
    hook.previewOfficialPricing.mockResolvedValue(preview);
    hook.commitOfficialPricing.mockResolvedValue({ pricing: [base(1)] });
    await act(async () => root.render(<PricingOfficialCard canManage />));
    expect(document.body.textContent).toContain(`matrix_pricing.official_response_hash: ${preview.response_hash}`);
    expect(document.body.textContent).not.toContain(preview.source_url);
    await act(async () => button('official_refresh_preview').click());
    expect(hook.previewOfficialPricing).toHaveBeenCalledWith('models-dev');
    expect(hook.commitOfficialPricing).not.toHaveBeenCalled();
    const commitButton = Array.from(document.body.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent?.includes('matrix_pricing.official_commit_refresh'))!;
    expect(commitButton).toBeDefined();
    await act(async () => commitButton.click());
    expect(hook.commitOfficialPricing).toHaveBeenCalledOnce();
  });
});
