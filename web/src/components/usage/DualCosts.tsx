import { useTranslation } from 'react-i18next';
import type { DualCosts, PriceEstimate, PricingSelection } from '@/lib/types';
import { formatUsd } from '@/utils/usage';
import styles from './DualCosts.module.scss';

type Translate = (key: string) => string;
const REASONS = new Set(['missing_baseline', 'reference_overflow', 'retained_evidence_incomplete', 'missing_price', 'unknown_identity', 'unresolved_identity', 'unbound_channel']);
export const pricingReasonLabel = (reason: string | undefined, t: Translate) => reason ? t(REASONS.has(reason) ? `cost_estimates.reasons.${reason}` : 'cost_estimates.unavailable') : '';
const knownAmount = (estimate?: PriceEstimate) => estimate?.has_known === true && typeof estimate.total_cost_usd === 'number' && Number.isFinite(estimate.total_cost_usd);
export function estimateAmount(estimate?: PriceEstimate, fallback?: number | null): string {
  if (estimate) return knownAmount(estimate) ? formatUsd(estimate.total_cost_usd!) : '—';
  return typeof fallback === 'number' && Number.isFinite(fallback) ? formatUsd(fallback) : '—';
}
export function estimateStatus(estimate: PriceEstimate | undefined, t: Translate): string {
  if (!estimate) return t('cost_estimates.not_provided');
  const status = ['complete', 'partial', 'unavailable'].includes(estimate.status) ? estimate.status : 'unavailable';
  const zero = knownAmount(estimate) && estimate.total_cost_usd === 0 ? ` · ${t('cost_estimates.known_zero')}` : '';
  const reason = pricingReasonLabel(estimate.unavailable_reason, t);
  return `${t(`cost_estimates.${status}`)}${zero}${reason ? ` · ${reason}` : ''}`;
}
export function dualCostLines(costs: DualCosts | undefined, t: Translate, configuredFallback?: number | null): string[] {
  return (['configured', 'reference'] as const).map(kind => `${t(`cost_estimates.${kind}`)}: ${estimateAmount(costs?.[kind], kind === 'configured' ? configuredFallback : undefined)} · ${estimateStatus(costs?.[kind], t)}`);
}
export function DualCostsDisplay({ costs, configuredFallback, valueClassName }: { costs?: DualCosts; configuredFallback?: number | null; valueClassName?: string }) {
  const { t } = useTranslation();
  return <div className={styles.costs}>{(['configured', 'reference'] as const).map(kind => <div key={kind} data-cost-estimate={kind} className={styles.estimate}>
    <span>{t(`cost_estimates.${kind}`)}</span>
    <strong className={valueClassName}>{estimateAmount(costs?.[kind], kind === 'configured' ? configuredFallback : undefined)}</strong>
    <small>{estimateStatus(costs?.[kind], t)}</small>
  </div>)}</div>;
}
const SCOPES = new Set(['credential_model', 'credential_default', 'channel_model', 'channel_default', 'legacy']);
const MODES = new Set(['multiplier', 'fixed', 'legacy']);
const byLabel = (by: string | undefined, t: Translate) => by === 'model' || by === 'model_alias' ? t(`cost_estimates.${by}`) : '—';
const numeric = (value: number | undefined) => typeof value === 'number' && Number.isFinite(value) ? String(value) : '—';
const COMPONENTS = ['uncached_input_cost_usd', 'output_cost_usd', 'cache_read_cost_usd', 'cache_write_cost_usd'] as const;
const RATES = ['prompt_price_per_1m', 'completion_price_per_1m', 'cache_read_price_per_1m', 'cache_write_price_per_1m'] as const;
export function PricingExplanation({ selection }: { selection?: PricingSelection }) {
  const { t } = useTranslation();
  if (!selection) return null;
  const fields: Array<[string, string]> = [
    ['scope', SCOPES.has(selection.scope) ? t(`cost_estimates.scopes.${selection.scope}`) : '—'],
    ['subject', [selection.subject_name, selection.subject_id].filter(Boolean).join(' · ') || '—'],
    ['channel', [selection.channel_name, selection.channel_id].filter(Boolean).join(' · ') || '—'],
    ['selected_model', `${selection.selected_model || '—'} · ${byLabel(selection.selected_by, t)}`],
    ['baseline_model', `${selection.baseline_model || '—'} · ${byLabel(selection.baseline_by, t)}`],
    ['mode', MODES.has(selection.mode) ? t(`cost_estimates.modes.${selection.mode}`) : '—'],
    ['multiplier', numeric(selection.multiplier)],
    ['style', selection.pricing_style === 'openai' || selection.pricing_style === 'claude' ? selection.pricing_style : '—'],
    ['snapshot', selection.snapshot_id],
  ];
  return <details className={styles.explanation} onClick={event => event.stopPropagation()}>
    <summary>{t('cost_estimates.explanation')}</summary>
    <p>{t('cost_estimates.history_warning')}</p>
    {selection.legacy_adjustments_replaced && <p>{t('cost_estimates.replacement_warning')}</p>}
    <dl>{fields.map(([key, value]) => <div key={key}><dt>{t(`cost_estimates.${key}`)}</dt><dd>{value}</dd></div>)}</dl>
    {selection.fixed && <dl>{RATES.map(key => <div key={key}><dt>{t(`cost_estimates.${key}`)}</dt><dd>{numeric(selection.fixed?.[key])}</dd></div>)}</dl>}
    <DualCostsDisplay costs={selection.dual_costs} />
    {selection.dual_costs && <dl>{(['configured', 'reference'] as const).flatMap(kind => COMPONENTS.map(key => <div key={`${kind}-${key}`}><dt>{t(`cost_estimates.${kind}`)} · {t(`cost_estimates.${key}`)}</dt><dd>{selection.dual_costs![kind].has_known ? formatUsd(selection.dual_costs![kind][key]) : '—'}</dd></div>))}</dl>}
    <dl>{(['legacy_model_multiplier', 'legacy_rule_multiplier', 'final_multiplier'] as const).map(key => <div key={key}><dt>{t(`cost_estimates.${key}`)}</dt><dd>{numeric(selection[key])}</dd></div>)}</dl>
    {!!selection.matched_rules?.length && <div><strong>{t('cost_estimates.matched_rules')}</strong><ul>{selection.matched_rules.map((rule, index) => <li key={index}>{rule.key} · {numeric(rule.multiplier)}</li>)}</ul></div>}
    {([selection.unavailable_reason, selection.baseline_unavailable_reason, selection.attribution_warning].filter(Boolean) as string[]).map((reason, index) => <p key={index}>{pricingReasonLabel(reason, t)}</p>)}
  </details>;
}
