import { useTranslation } from 'react-i18next';
import type { DualCosts, PriceEstimate, PricingSelection } from '@/lib/types';
import { formatUsd } from '@/utils/usage';
import styles from './DualCosts.module.scss';

type Translate = (key: string) => string;
const REASONS = new Set(['missing_baseline', 'reference_overflow', 'retained_evidence_incomplete', 'missing_price', 'unknown_identity', 'unresolved_identity', 'unbound_channel', 'no_match', 'ambiguous_match', 'missing_segments']);
const SOURCES = new Set(['credential_matrix', 'official', 'models-dev', 'litellm', 'openrouter', 'legacy-seed', 'unavailable', 'mixed', 'none']);
export const pricingSourceLabel = (source: string | undefined, t: Translate) => source && SOURCES.has(source) ? t(`cost_estimates.sources.${source}`) : '';
export const pricingReasonLabel = (reason: string | undefined, t: Translate) => {
  const normalized = reason === 'retained_pricing_evidence_incomplete' ? 'retained_evidence_incomplete' : reason;
  return normalized ? t(REASONS.has(normalized) ? `cost_estimates.reasons.${normalized}` : 'cost_estimates.unavailable') : '';
};
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
  return (['configured', 'reference'] as const).map(kind => {
    const estimate = costs?.[kind];
    const details = [estimateStatus(estimate, t)];
    const source = pricingSourceLabel(estimate?.source, t);
    if (source) details.push(`${t('cost_estimates.source')}: ${source}`);
    if (estimate?.matched_by === 'model' || estimate?.matched_by === 'model_alias') details.push(`${t('cost_estimates.matched_by')}: ${t(`cost_estimates.${estimate.matched_by}`)}`);
    if (estimate?.matched_model) details.push(`${t('cost_estimates.matched_model')}: ${estimate.matched_model}`);
    if (estimate?.snapshot_id) details.push(`${t('cost_estimates.pricing_snapshot')}: ${estimate.snapshot_id}`);
    const amount = estimate && !knownAmount(estimate) && estimate.status === 'unavailable'
      ? t('cost_estimates.unavailable')
      : estimateAmount(estimate, kind === 'configured' ? configuredFallback : undefined);
    return `${t(`cost_estimates.${kind}`)}: ${amount} · ${details.join(' · ')}`;
  });
}
export function DualCostsDisplay({ costs, configuredFallback, valueClassName }: { costs?: DualCosts; configuredFallback?: number | null; valueClassName?: string }) {
  const { t } = useTranslation();
  return <div className={styles.costs}>{(['configured', 'reference'] as const).map(kind => {
    const estimate = costs?.[kind];
    const amount = estimateAmount(estimate, kind === 'configured' ? configuredFallback : undefined);
    const shownAmount = estimate && !knownAmount(estimate) && estimate.status === 'unavailable' ? t('cost_estimates.unavailable') : amount;
    const source = pricingSourceLabel(estimate?.source, t);
    const matchedBy = estimate?.matched_by === 'model' || estimate?.matched_by === 'model_alias' ? t(`cost_estimates.${estimate.matched_by}`) : '';
    return <div key={kind} data-cost-estimate={kind} className={styles.estimate}>
      <span>{t(`cost_estimates.${kind}`)}</span>
      <strong className={valueClassName}>{shownAmount}</strong>
      <small>{estimateStatus(estimate, t)}</small>
      {source && <small>{t('cost_estimates.source')}: {source}</small>}
      {matchedBy && <small>{t('cost_estimates.matched_by')}: {matchedBy}</small>}
      {estimate?.matched_model && <small>{t('cost_estimates.matched_model')}: {estimate.matched_model}</small>}
      {estimate?.snapshot_id && <small>{t('cost_estimates.pricing_snapshot')}: {estimate.snapshot_id}</small>}
    </div>;
  })}</div>;
}
const SCOPES = new Set(['credential_matrix', 'official', 'openrouter', 'unavailable', 'credential_model', 'credential_default', 'channel_model', 'channel_default', 'legacy']);
const MODES = new Set(['multiplier', 'fixed', 'official', 'unavailable', 'legacy']);
const byLabel = (by: string | undefined, t: Translate) => by === 'model' || by === 'model_alias' ? t(`cost_estimates.${by}`) : '—';
const numeric = (value: number | undefined) => typeof value === 'number' && Number.isFinite(value) ? String(value) : '—';
const COMPONENTS = ['uncached_input_cost_usd', 'output_cost_usd', 'cache_read_cost_usd', 'cache_write_cost_usd'] as const;
const RATES = ['prompt_price_per_1m', 'completion_price_per_1m', 'cache_read_price_per_1m', 'cache_write_price_per_1m'] as const;
export function PricingExplanation({ selection, hideDualCosts = false }: { selection?: PricingSelection; hideDualCosts?: boolean }) {
  const { t } = useTranslation();
  if (!selection) return null;
  const fields: Array<[string, string, boolean?]> = [
    ['scope', SCOPES.has(selection.scope) ? t(`cost_estimates.scopes.${selection.scope}`) : '—'],
    ['source', pricingSourceLabel(selection.pricing_source, t)],
    ['pricing_status', ['complete', 'partial', 'unavailable'].includes(selection.pricing_status ?? '') ? t(`cost_estimates.${selection.pricing_status}`) : '—'],
    ['pricing_snapshot', selection.pricing_snapshot_id ?? '—'],
    ['match_method', selection.baseline_by === 'model' || selection.baseline_by === 'model_alias' ? t(`cost_estimates.${selection.baseline_by}`) : '—'],
    ['matched_model', selection.baseline_model ?? '—'],
    ['subject', [selection.subject_name, selection.subject_id].filter(Boolean).join(' · ') || '—', Boolean(selection.subject_name || selection.subject_id)],
    ['channel', [selection.channel_name, selection.channel_id].filter(Boolean).join(' · ') || '—', Boolean(selection.channel_name || selection.channel_id)],
    ['selected_model', `${selection.selected_model || '—'} · ${byLabel(selection.selected_by, t)}`, Boolean(selection.selected_model || selection.selected_by)],
    ['baseline_model', `${selection.baseline_model || '—'} · ${byLabel(selection.baseline_by, t)}`, Boolean(selection.baseline_model || selection.baseline_by)],
    ['mode', MODES.has(selection.mode) ? t(`cost_estimates.modes.${selection.mode}`) : '—'],
    ['multiplier', numeric(selection.multiplier), typeof selection.multiplier === 'number'],
    ['style', selection.pricing_style === 'openai' || selection.pricing_style === 'claude' ? selection.pricing_style : '—', Boolean(selection.pricing_style)],
    ['snapshot', selection.snapshot_id],
  ];
  return <details className={styles.explanation} onClick={event => event.stopPropagation()}>
    <summary>{t('cost_estimates.explanation')}</summary>
    <p>{t('cost_estimates.history_warning')}</p>
    {selection.legacy_adjustments_replaced && <p>{t('cost_estimates.replacement_warning')}</p>}
    <dl>{fields.filter(([, , show = true]) => show).map(([key, value]) => <div key={key}><dt>{t(`cost_estimates.${key}`)}</dt><dd>{value}</dd></div>)}</dl>
    {selection.fixed && <dl>{RATES.map(key => <div key={key}><dt>{t(`cost_estimates.${key}`)}</dt><dd>{numeric(selection.fixed?.[key])}</dd></div>)}</dl>}
    {!hideDualCosts && <DualCostsDisplay costs={selection.dual_costs} />}
    {selection.dual_costs && <dl>{(['configured', 'reference'] as const).flatMap(kind => COMPONENTS.map(key => <div key={`${kind}-${key}`}><dt>{t(`cost_estimates.${kind}`)} · {t(`cost_estimates.${key}`)}</dt><dd>{selection.dual_costs![kind].has_known ? formatUsd(selection.dual_costs![kind][key]) : '—'}</dd></div>))}</dl>}
    {(['legacy_model_multiplier', 'legacy_rule_multiplier', 'final_multiplier'] as const).filter(key => typeof selection[key] === 'number' && Number.isFinite(selection[key])).length > 0 && <dl>{(['legacy_model_multiplier', 'legacy_rule_multiplier', 'final_multiplier'] as const).filter(key => typeof selection[key] === 'number' && Number.isFinite(selection[key])).map(key => <div key={key}><dt>{t(`cost_estimates.${key}`)}</dt><dd>{numeric(selection[key])}</dd></div>)}</dl>}
    {!!selection.matched_rules?.length && <div><strong>{t('cost_estimates.matched_rules')}</strong><ul>{selection.matched_rules.map((rule, index) => <li key={index}>{rule.key} · {numeric(rule.multiplier)}</li>)}</ul></div>}
    {([selection.unavailable_reason, selection.baseline_unavailable_reason, selection.attribution_warning].filter(Boolean) as string[]).map((reason, index) => <p key={index}>{pricingReasonLabel(reason, t)}</p>)}
  </details>;
}
