import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import type { DualCosts, PricingSelection } from '@/lib/types';
import { DualCostsDisplay, estimateAmount, estimateStatus, PricingExplanation } from './DualCosts';
import styles from './RequestCost.module.scss';

/** List cells stay compact; authoritative estimates and provenance remain on demand. */
export function RequestCost({ costs, configuredFallback, selection }: { costs?: DualCosts; configuredFallback?: number | null; selection?: PricingSelection }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const detailCosts = costs ?? selection?.dual_costs;
  const configured = detailCosts?.configured;
  const amount = estimateAmount(configured, configuredFallback);
  const knownZero = configured ? configured.has_known && configured.total_cost_usd === 0 : configuredFallback === 0;
  const shown = knownZero ? '$0' : amount;
  const title = configured ? estimateStatus(configured, t) : typeof configuredFallback === 'number' && Number.isFinite(configuredFallback) ? t('cost_estimates.complete') : t('cost_estimates.unavailable');
  return <>
    {detailCosts || selection ? <button type="button" className={styles.amount} data-request-cost title={title} aria-label={`${t('cost_estimates.explanation')}: ${shown}`} onClick={event => { event.stopPropagation(); setOpen(true); }}>{shown}</button> : <span data-request-cost title={title}>{shown}</span>}
    {open && <Modal open title={t('cost_estimates.explanation')} onClose={() => setOpen(false)}>
      <DualCostsDisplay costs={detailCosts} configuredFallback={configuredFallback} />
      <PricingExplanation selection={selection} hideDualCosts />
    </Modal>}
  </>;
}
