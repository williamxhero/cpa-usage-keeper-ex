// @vitest-environment happy-dom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PricingMatrixResponse } from '@/lib/types';
import { PricingMatrixCard } from '../PricingMatrixCard';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
const screen = { mobile: false };
vi.mock('@/hooks/useMediaQuery', () => ({ useMediaQuery: () => screen.mobile }));
const credential = { directory_id: 1, subject_id: 'stable-subject', name: 'Long Credential Name', alias: '', key_hint: 'sk-a....safe', provider_type: 'codex', auth_type: 'apikey', status: 'active' as const, binding_status: 'bound' as const };
const matrix = {
  credentials: [credential], models: ['complete', 'partial', 'unknown'], cells: [],
  effective_cells: [
    { subject_id: 'stable-subject', canonical_model_id: 'complete', credential: null, mode: 'inherit', covered: false, status: 'complete', source: 'official', pricing_style: 'claude', prompt_price_per_1m: 2, completion_price_per_1m: 10, cache_read_price_per_1m: .1, cache_write_price_per_1m: 2.5, snapshot_id: 'pinned' },
    { subject_id: 'stable-subject', canonical_model_id: 'partial', credential: null, mode: 'inherit', covered: false, status: 'partial', source: 'openrouter', prompt_price_per_1m: 0, snapshot_id: 'pinned' },
    { subject_id: 'stable-subject', canonical_model_id: 'unknown', credential: null, mode: 'inherit', covered: false, status: 'unavailable', source: 'unavailable', snapshot_id: 'pinned' },
  ], credential_page: 1, credential_page_size: 50, model_page: 1, model_page_size: 50, total_credentials: 1, total_models: 3, snapshot_id: 'pinned',
} as PricingMatrixResponse;
let root: Root; let container: HTMLDivElement;
beforeEach(() => { globalThis.IS_REACT_ACT_ENVIRONMENT = true; screen.mobile = false; container = document.createElement('div'); document.body.append(container); root = createRoot(container); });
afterEach(async () => { await act(async () => root.unmount()); container.remove(); });
describe('matrix effective price usability', () => {
  const render = async (data = matrix, canManage = false) => { await act(async () => root.render(<PricingMatrixCard matrix={data} query={{}} onQueryChange={vi.fn()} canManage={canManage} onSaveCells={vi.fn(async () => true)} />)); };
  it('renders server inherited values, known zero, partial and unavailable without guessing', async () => {
    await render();
    expect(Array.from(container.querySelectorAll('output')).map(item => item.textContent)).toEqual(['2', '10', '0.1', '2.5', '0']);
    expect(container.textContent).toContain('inherited');
    expect(container.textContent).toContain('partial');
    expect(container.textContent).toContain('unavailable');
    expect(container.textContent).toContain('official');
    expect(container.textContent).toContain('openrouter');
    expect(container.textContent).not.toContain('effective_summary_unavailable');
    expect(container.querySelector('form')).toBeNull();
  });
  it('prefills inherited prices while keeping missing segments empty', async () => {
    await render(matrix, true);
    await act(async () => container.querySelectorAll<HTMLButtonElement>('button[aria-label^="edit_cell"]')[0].click());
    expect(container.querySelector<HTMLInputElement>('input[aria-label="cache_read_price_per_1m"]')?.value).toBe('0.1');
    await act(async () => Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent === 'cancel')!.click());
    await act(async () => container.querySelectorAll<HTMLButtonElement>('button[aria-label^="edit_cell"]')[1].click());
    expect(container.querySelector<HTMLInputElement>('input[aria-label="prompt_price_per_1m"]')?.value).toBe('0');
    expect(container.querySelector<HTMLInputElement>('input[aria-label="cache_read_price_per_1m"]')?.value).toBe('');
  });
  it('prefills a single batch selection and only common prices for mixed targets', async () => {
    await render(matrix, true);
    await act(async () => container.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')[0].click());
    const batchEdit = () => Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent === 'batch_edit')!.click();
    await act(async () => batchEdit());
    expect(container.querySelector<HTMLInputElement>('input[aria-label="cache_read_price_per_1m"]')?.value).toBe('0.1');
    await act(async () => Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent === 'cancel')!.click());
    await act(async () => container.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')[1].click());
    await act(async () => batchEdit());
    expect(container.querySelector<HTMLInputElement>('input[aria-label="prompt_price_per_1m"]')?.value).toBe('');
    expect(container.querySelector<HTMLInputElement>('input[aria-label="cache_read_price_per_1m"]')?.value).toBe('');
  });
  it('uses genuine alias before key hint before a safely abbreviated name', async () => {
    await render();
    expect(container.querySelector('tbody th')?.textContent).toContain('sk-a....safe');
    expect(container.querySelector('tbody th')?.textContent).not.toContain('Long Credential Name');
    await render({ ...matrix, credentials: [{ ...credential, alias: 'Real Alias' }] });
    expect(container.querySelector('tbody th')?.textContent).toContain('Real Alias');
    await render({ ...matrix, credentials: [{ ...credential, key_hint: '' }] });
    expect(container.querySelector('tbody th')?.textContent).toContain('Long....Name');
    expect(container.querySelector('tbody th')?.textContent).not.toContain('Long Credential Name');
  });
  it('shows the same effective prices and key hint on mobile cards', async () => {
    screen.mobile = true;
    await render();
    await act(async () => container.querySelector<HTMLElement>('[aria-label="select_credential"]')!.click());
    await act(async () => document.querySelector<HTMLElement>('[role="option"]')!.click());
    expect(container.querySelector('table')).toBeNull();
    expect(container.querySelectorAll('article')).toHaveLength(3);
    expect(container.querySelectorAll('output')).toHaveLength(5);
    expect(container.textContent).toContain('sk-a....safe');
  });
});
