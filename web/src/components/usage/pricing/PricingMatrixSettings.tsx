import { useCallback, useEffect, useRef, useState } from 'react';
import type { PricingMatrixQuery } from '@/lib/types';
import { useMatrixPricingData, type PricingNotice } from '../hooks/useMatrixPricingData';
import { PricingMatrixCard } from './PricingMatrixCard';

export interface PricingMatrixSettingsProps {
  canManage?: boolean;
  onAuthRequired?: () => void;
  onChanged?: () => void;
  onNotice?: PricingNotice;
  refreshRevision?: number;
}

export function PricingMatrixSettings({ canManage = false, onAuthRequired, onChanged, onNotice, refreshRevision }: PricingMatrixSettingsProps) {
  const [query, setQuery] = useState<PricingMatrixQuery>({ credentialPage: 1, credentialPageSize: 50, modelPage: 1, modelPageSize: 50 });
  const data = useMatrixPricingData({ enabled: true, canManage, query, onAuthRequired, onChanged, onNotice });
  const { loadMatrix, loadSyncStatus } = data;
  const initialRevision = useRef(refreshRevision);
  const reload = useCallback(() => { void loadMatrix(); }, [loadMatrix]);
  useEffect(() => {
    if (initialRevision.current === refreshRevision) return;
    initialRevision.current = refreshRevision;
    void loadMatrix();
    if (canManage) void loadSyncStatus();
  }, [canManage, loadMatrix, loadSyncStatus, refreshRevision]);
  return <PricingMatrixCard
    matrix={data.matrix}
    query={query}
    onQueryChange={setQuery}
    canManage={data.canEdit}
    loading={data.loading}
    saving={data.saving}
    error={Boolean(data.error)}
    onReload={reload}
    onSaveCells={data.saveCells}
    syncStatus={data.syncStatus}
    syncLoading={data.syncLoading}
    syncError={Boolean(data.syncError)}
    refreshing={data.refreshing}
    onSync={canManage ? () => { void data.refreshCredentials(); } : undefined}
    onCheckSync={canManage ? () => { void data.loadSyncStatus(); } : undefined}
  />;
}
