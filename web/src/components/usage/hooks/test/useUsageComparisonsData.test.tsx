// @vitest-environment happy-dom
import { act, useEffect } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { useUsageComparisonsData, type UseUsageComparisonsDataOptions } from '../useUsageComparisonsData';
let container: HTMLDivElement; let root: Root; let result: ReturnType<typeof useUsageComparisonsData>;
const requests: Array<{ url: string; signal?: AbortSignal; resolve: (response: Response) => void }> = [];
function Probe({ options }: { options: UseUsageComparisonsDataOptions }) {
  const current = useUsageComparisonsData(options);
  useEffect(() => { result = current; });
  return null;
}
const render = async (options: UseUsageComparisonsDataOptions) => { await act(async () => { root.render(<Probe options={options} />); }); };
const respond = async (index: number, id: string) => { await act(async () => { requests[index].resolve(new Response(JSON.stringify({ models: [], pricing_snapshot_id: id }))); }); };
beforeEach(() => {
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  requests.length = 0;
  vi.stubGlobal('fetch', vi.fn((url: string, init?: RequestInit) => new Promise<Response>(resolve => requests.push({ url: String(url), signal: init?.signal ?? undefined, resolve }))));
  container = document.createElement('div'); document.body.append(container); root = createRoot(container);
});
afterEach(() => { act(() => root.unmount()); container.remove(); vi.unstubAllGlobals(); });
it('pins comparisons to exact range/custom boundaries/APIKey and viewer scope, including while disabled', async () => {
  const options: UseUsageComparisonsDataOptions = { range: 'custom', customUnit: 'hour', customStart: '2026-05-01T01:00:00+08:00', customEnd: '2026-05-01T08:00:00+08:00', apiKeyId: 'key-a' };
  await render(options); await respond(0, 'snapshot-a');
  expect(result.currentComparisons?.pricing_snapshot_id).toBe('snapshot-a');
  for (const changed of [{ apiKeyId: 'key-b' }, { customStart: '2026-05-01T02:00:00+08:00' }, { customEnd: '2026-05-01T09:00:00+08:00' }, { range: '24h' as const }, { keyViewer: true }]) {
    await render({ ...options, ...changed, enabled: false });
    expect(result.currentComparisons).toBeNull();
    expect(result.comparisons?.pricing_snapshot_id).toBe('snapshot-a');
  }
  await render({ ...options, enabled: false });
  expect(result.currentComparisons?.pricing_snapshot_id).toBe('snapshot-a');
  expect(requests).toHaveLength(1);
  expect(requests[0].url).toContain('api_key_id=key-a');
});
it('cancels superseded comparison loads and never commits a late mismatched snapshot', async () => {
  await render({ range: '24h', apiKeyId: 'key-a' });
  await render({ range: '24h', apiKeyId: 'key-b' });
  expect(requests[0].signal?.aborted).toBe(true);
  await respond(1, 'snapshot-b'); await respond(0, 'snapshot-a');
  expect(result.currentComparisons?.pricing_snapshot_id).toBe('snapshot-b');
  expect(result.loading).toBe(false);
});
