import { useCallback, useState } from 'react';
import { PricingMatrixSettings } from '@/components/usage/pricing/PricingMatrixSettings';
import { PricingOfficialCard } from '@/components/usage/pricing/PricingOfficialCard';
import { KeyViewerShell } from '@/features/key-viewer/KeyViewerShell';
import type { KeyViewerPath } from '@/features/key-viewer/navigation';
import type { AuthSessionAPIKeySummary } from '@/lib/types';

interface KeyPricingPageProps {
  apiKey?: AuthSessionAPIKeySummary;
  onNavigate: (path: KeyViewerPath) => void;
  onAuthRequired?: () => void;
}

export function KeyPricingPage({ apiKey, onNavigate, onAuthRequired }: KeyPricingPageProps) {
  const [refreshRevision, setRefreshRevision] = useState(0);
  const handleRefresh = useCallback(() => {
    setRefreshRevision((revision) => revision + 1);
  }, []);

  return (
    <KeyViewerShell
      activePage="pricing"
      apiKey={apiKey}
      onRefresh={handleRefresh}
      onNavigate={onNavigate}
      onAuthRequired={onAuthRequired}
    >
      <PricingMatrixSettings
        canManage={false}
        onAuthRequired={onAuthRequired}
        refreshRevision={refreshRevision}
      />
      <PricingOfficialCard
        canManage={false}
        onAuthRequired={onAuthRequired}
        refreshRevision={refreshRevision}
      />
    </KeyViewerShell>
  );
}
