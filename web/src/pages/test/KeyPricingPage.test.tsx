// @vitest-environment happy-dom

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  matrixProps: [] as Array<Record<string, unknown>>,
  officialProps: [] as Array<Record<string, unknown>>,
}));

vi.mock('@/features/key-viewer/KeyViewerShell', () => ({
  KeyViewerShell: ({ children, onRefresh }: { children: React.ReactNode; onRefresh: () => void }) => (
    <div><button type="button" data-refresh onClick={onRefresh}>refresh</button>{children}</div>
  ),
}));

vi.mock('@/components/usage/pricing/PricingMatrixSettings', () => ({
  PricingMatrixSettings: (props: Record<string, unknown>) => {
    mocks.matrixProps.push(props);
    return <div data-matrix />;
  },
}));

vi.mock('@/components/usage/pricing/PricingOfficialCard', () => ({
  PricingOfficialCard: (props: Record<string, unknown>) => {
    mocks.officialProps.push(props);
    return <div data-official />;
  },
}));

import { KeyPricingPage } from '../KeyPricingPage';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('KeyPricingPage', () => {
  let container: HTMLDivElement;
  let root: ReturnType<typeof createRoot>;

  beforeEach(() => {
    mocks.matrixProps.length = 0;
    mocks.officialProps.length = 0;
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
  });

  it('mounts pricing cards read-only and propagates refresh and auth handling', async () => {
    const onAuthRequired = vi.fn();
    await act(async () => root.render(<KeyPricingPage onNavigate={() => undefined} onAuthRequired={onAuthRequired} />));
    expect(container.querySelector('[data-matrix]')).not.toBeNull();
    expect(container.querySelector('[data-official]')).not.toBeNull();
    expect(mocks.matrixProps.at(-1)).toMatchObject({ canManage: false, onAuthRequired, refreshRevision: 0 });
    expect(mocks.officialProps.at(-1)).toMatchObject({ canManage: false, onAuthRequired, refreshRevision: 0 });

    await act(async () => container.querySelector<HTMLButtonElement>('[data-refresh]')!.click());
    expect(mocks.matrixProps.at(-1)).toMatchObject({ canManage: false, refreshRevision: 1 });
    expect(mocks.officialProps.at(-1)).toMatchObject({ canManage: false, refreshRevision: 1 });
  });
});
