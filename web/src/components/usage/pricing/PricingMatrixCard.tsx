import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Input } from '@/components/ui/Input';
import { Select } from '@/components/ui/Select';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import type { PricingCredential, PricingCredentialSyncStatus, PricingMatrixCell, PricingMatrixEffectiveCell, PricingMatrixCellInput, PricingMatrixQuery, PricingMatrixResponse, PricingStyle } from '@/lib/types';
import styles from './PricingMatrixCard.module.scss';

const rateFields = ['prompt_price_per_1m', 'completion_price_per_1m', 'cache_read_price_per_1m', 'cache_write_price_per_1m'] as const;
type RateField = typeof rateFields[number];
type Pair = Pick<PricingMatrixCellInput, 'subject_id' | 'canonical_model_id'>;
type Draft = { targets: Pair[]; rates: Record<RateField, string>; style: PricingStyle | '' };
const pairKey = (pair: Pair) => JSON.stringify([pair.subject_id, pair.canonical_model_id]);
const emptyRates = (): Record<RateField, string> => ({ prompt_price_per_1m: '', completion_price_per_1m: '', cache_read_price_per_1m: '', cache_write_price_per_1m: '' });
const EMPTY_CREDENTIALS: PricingCredential[] = [];
const EMPTY_MODELS: string[] = [];
const abbreviatedName = (value: string) => {
  const characters = Array.from(value.trim());
  if (!characters.length) return '—';
  if (characters.length < 16) return characters.slice(0, Math.min(3, characters.length - 1)).join('') + '....';
  return characters.slice(0, 4).join('') + '....' + characters.slice(-4).join('');
};
const credentialName = (credential: PricingCredential) => credential.alias?.trim() || credential.key_hint?.trim() || abbreviatedName(credential.name);
const officialSources = new Set(['official', 'models-dev', 'litellm', 'openrouter', 'legacy-seed']);

export interface PricingMatrixCardProps {
  matrix: PricingMatrixResponse | null;
  query: PricingMatrixQuery;
  onQueryChange: (query: PricingMatrixQuery) => void;
  canManage?: boolean;
  loading?: boolean;
  saving?: boolean;
  error?: boolean;
  onReload?: () => void;
  onSaveCells?: (cells: PricingMatrixCellInput[]) => Promise<boolean>;
  syncStatus?: PricingCredentialSyncStatus | null;
  syncLoading?: boolean;
  syncError?: boolean;
  refreshing?: boolean;
  onSync?: () => void;
  onCheckSync?: () => void;
}

/** Renders only server-provided values; inheritance is resolved in the pinned backend snapshot. */
export function PricingMatrixCard(props: PricingMatrixCardProps) {
  const { query, canManage = false, onSaveCells } = props;
  const credentialContext = JSON.stringify([query.credential ?? '', query.credentialPage ?? 1, query.credentialPageSize ?? 50]);
  const context = JSON.stringify([credentialContext, query.model ?? '', query.modelPage ?? 1, query.modelPageSize ?? 50, canManage, Boolean(onSaveCells)]);
  const [choice, setChoice] = useState({ context: credentialContext, subject: '' });
  // Reset drafts and pending feedback when the query or write permission changes.
  // Keep the mobile credential while searching/paging its models.
  return <MatrixCardContent key={context} {...props} mobileCredential={choice.context === credentialContext ? choice.subject : ''} onChooseCredential={subject => setChoice({ context: credentialContext, subject })} />;
}

function MatrixCardContent({ matrix, query, onQueryChange, canManage = false, loading = false, saving = false, error = false, onReload, onSaveCells, syncStatus, syncLoading = false, syncError = false, refreshing = false, onSync, onCheckSync, mobileCredential, onChooseCredential }: PricingMatrixCardProps & { mobileCredential: string; onChooseCredential: (subject: string) => void }) {
  const { t, i18n } = useTranslation('matrix_pricing');
  const mobile = useMediaQuery('(max-width: 768px)');
  const [selected, setSelected] = useState<Pair[]>([]);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [fieldError, setFieldError] = useState('');
  const [mutationError, setMutationError] = useState('');
  const [notice, setNotice] = useState('');
  const [pending, setPending] = useState(false);
  const operationRef = useRef(0);
  const pendingRef = useRef(false);
  const permissionRef = useRef(canManage);
  useEffect(() => { permissionRef.current = canManage; }, [canManage]);
  useEffect(() => () => { operationRef.current += 1; }, []);
  const busy = loading || saving || pending;
  const writeBlocked = busy || error;
  const maxCells = 5000;
  const credentials = matrix?.credentials ?? EMPTY_CREDENTIALS;
  const models = matrix?.models ?? EMPTY_MODELS;
  const cells = useMemo(() => new Map<string, PricingMatrixCell | PricingMatrixEffectiveCell>([
    ...(matrix?.effective_cells ?? []).map(cell => [pairKey(cell), cell] as const),
    ...(matrix?.cells ?? []).map(cell => [pairKey(cell), cell] as const),
  ]), [matrix]);
  const visiblePairs = useMemo(() => new Set(credentials.flatMap(credential => credential.subject_id ? models.map(model => pairKey({ subject_id: credential.subject_id!, canonical_model_id: model })) : [])), [credentials, models]);
  const selection = selected.filter(pair => visiblePairs.has(pairKey(pair)));
  const chosenCredential = credentials.find(credential => credential.subject_id === mobileCredential);
  const selectedKeys = new Set(selection.map(pairKey));
  const safeCredentialLabel = (credential: PricingCredential) => [credentialName(credential), credential.provider_type, t(`status_${credential.status}`)].filter(Boolean).join(' · ');

  const changeQuery = (next: PricingMatrixQuery) => {
    if (busy) return;
    setSelected([]); setDraft(null); setFieldError(''); setMutationError(''); setNotice('');
    onQueryChange(next);
  };
  const toggle = (pair: Pair) => {
    if (!permissionRef.current || writeBlocked) return;
    if (!selectedKeys.has(pairKey(pair)) && selection.length >= maxCells) { setMutationError('batch_limit'); return; }
    setSelected(previous => selectedKeys.has(pairKey(pair)) ? previous.filter(item => pairKey(item) !== pairKey(pair)) : [...selection, pair]);
  };
  const edit = (targets: Pair[]) => {
    if (!permissionRef.current || writeBlocked || !onSaveCells || targets.length === 0) return;
    if (targets.length > maxCells) { setMutationError('batch_limit'); return; }
    if (targets.some(pair => !visiblePairs.has(pairKey(pair)))) return;
    const targetCells = targets.map(pair => cells.get(pairKey(pair)));
    const first = targetCells[0];
    const rates = emptyRates();
    // Never choose one target's price for a batch of different effective prices.
    for (const field of rateFields) {
      const value = first?.[field];
      if (value !== undefined && value !== null && Number.isFinite(value) && value >= 0 && targetCells.every(cell => cell?.[field] === value)) rates[field] = String(value);
    }
    const style = first?.pricing_style ?? '';
    setDraft({ targets, rates, style: targetCells.every(cell => cell?.pricing_style === style) ? style : '' });
    setFieldError(''); setMutationError(''); setNotice('');
  };
  const mutate = async (targets: Pair[], clear: boolean) => {
    if (!permissionRef.current || pendingRef.current || writeBlocked || !onSaveCells || targets.length === 0) return;
    if (targets.length > maxCells) { setMutationError('batch_limit'); return; }
    // A reload may remove a subject/model while its editor remains open.
    if (targets.some(pair => !visiblePairs.has(pairKey(pair)))) { setMutationError('selection_changed'); return; }
    let inputs: PricingMatrixCellInput[];
    if (clear) inputs = targets.map(pair => ({ ...pair, mode: 'inherit' }));
    else {
      if (!draft) return;
      const rates = Object.fromEntries(rateFields.map(field => [field, Number(draft.rates[field].trim())])) as Record<RateField, number>;
      if (rateFields.some(field => !/^(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/.test(draft.rates[field].trim()) || !Number.isFinite(rates[field]) || rates[field] < 0)) {
        setFieldError('invalid_prices'); return;
      }
      if (draft.style !== 'openai' && draft.style !== 'claude') { setFieldError('style_required'); return; }
      const pricingStyle = draft.style;
      inputs = targets.map(pair => ({ ...pair, mode: 'fixed', pricing_style: pricingStyle, ...rates }));
    }
    pendingRef.current = true; setPending(true); setMutationError(''); setFieldError(''); setNotice('');
    const operation = ++operationRef.current;
    try {
      const committed = await onSaveCells(inputs);
      if (operation !== operationRef.current) return;
      if (!committed) { setMutationError(clear ? 'clear_failed' : 'save_failed'); return; }
      setDraft(null); setSelected([]); setNotice(clear ? 'cleared' : 'saved');
    } catch {
      // Transport bodies and exception messages may contain credentials; never render them.
      if (operation === operationRef.current) setMutationError(clear ? 'clear_failed' : 'save_failed');
    } finally {
      if (operation === operationRef.current) { pendingRef.current = false; setPending(false); }
    }
  };
  const pagination = (axis: 'credential' | 'model') => {
    const page = (axis === 'credential' ? matrix?.credential_page : matrix?.model_page) ?? (axis === 'credential' ? query.credentialPage : query.modelPage) ?? 1;
    const size = (axis === 'credential' ? matrix?.credential_page_size : matrix?.model_page_size) ?? 50;
    const total = (axis === 'credential' ? matrix?.total_credentials : matrix?.total_models) ?? 0;
    const pages = Math.max(1, Math.ceil(total / size));
    return <nav className={styles.pagination} aria-label={t(`${axis}_pages`)}>
      <span>{t(`${axis === 'credential' ? 'credentials' : 'models'}`)}</span>
      <Button type="button" size="sm" variant="secondary" disabled={busy || page <= 1} onClick={() => changeQuery({ ...query, [axis === 'credential' ? 'credentialPage' : 'modelPage']: page - 1 })}>{t('previous')}</Button>
      <span>{t('page', { page, pages })}</span>
      <Button type="button" size="sm" variant="secondary" disabled={busy || page >= pages} onClick={() => changeQuery({ ...query, [axis === 'credential' ? 'credentialPage' : 'modelPage']: page + 1 })}>{t('next')}</Button>
    </nav>;
  };
  const renderCell = (credential: PricingCredential, model: string) => {
    const pair = { subject_id: credential.subject_id ?? '', canonical_model_id: model };
    const cell = cells.get(pairKey(pair));
    const covered = cell?.covered === true && cell.mode === 'fixed';
    const unavailable = cell && 'status' in cell ? cell.status === 'unavailable' : !cell && Boolean(matrix?.effective_cells);
    return <div className={styles.cell}>
      <strong>{t(covered ? 'matrix' : unavailable ? 'official_unavailable' : 'inherited')}</strong>
      {cell && 'status' in cell && cell.status === 'partial' && <span className={styles.hint}>{t('cost_estimates.partial', { ns: 'translation' })}</span>}
      {cell?.source && officialSources.has(cell.source) && <span className={styles.hint}>{t('official_source')}: {t(`official_source_${cell.source.replaceAll('-', '_')}`)}</span>}
      <dl className={styles.prices}>{rateFields.map(field => {
        const value = cell?.[field];
        // A known zero is displayed; a missing segment is never converted to zero.
        return <div key={field}><dt>{t(`${field}`)}</dt><dd>{typeof value === 'number' && Number.isFinite(value) && value >= 0 ? <output>{String(value)}</output> : t('missing')}</dd></div>;
      })}</dl>
      {cell?.pricing_style && <span className={styles.hint}>{t('style')}: {t(`style_${cell.pricing_style}`)}</span>}
      {canManage && onSaveCells && credential.subject_id && <div className={styles.cellActions}>
        <label className={styles.check}><input type="checkbox" aria-label={t('select_cell', { credential: credentialName(credential), model })} checked={selectedKeys.has(pairKey(pair))} disabled={busy} onChange={() => toggle(pair)} />{t('select_cell', { credential: credentialName(credential), model })}</label>
        <Button type="button" size="sm" variant="secondary" aria-label={t('edit_cell', { credential: credentialName(credential), model })} disabled={busy} onClick={() => edit([pair])}>{t('edit')}</Button>
      </div>}
    </div>;
  };

  return <Card title={t('title')} subtitle={t('subtitle')}>
    <div className={styles.body} aria-busy={busy}>
      <p className={styles.hint}>{t('history_warning')}</p>
      {matrix && !matrix.effective_cells && <p className={styles.hint}>{t('effective_summary_unavailable')}</p>}
      <p className={styles.hint}>{t('units')}</p>
      {canManage && <p className={styles.hint}>{t('batch_help')}</p>}
      {!canManage && <p className={styles.hint}>{t('readonly')}</p>}
      {error && <div className={styles.error} role="alert">{t('load_failed')}</div>}
      {loading && <p role="status">{t('loading')}</p>}
      {canManage && <div className={styles.sync}>
        <span>{t('sync_status')}: {syncStatus ? t(`sync_state_${syncStatus.state}`) : t('never_synced')}</span>
        <span>{t('last_sync')}: {syncStatus?.last_success_at && Number.isFinite(Date.parse(syncStatus.last_success_at)) ? new Date(syncStatus.last_success_at).toLocaleString(i18n.language) : t('never_synced')}</span>
        {onSync && <Button type="button" size="sm" variant="secondary" disabled={busy || refreshing || syncLoading || syncStatus?.in_flight} onClick={onSync}>{t(`${refreshing ? 'syncing' : 'sync'}`)}</Button>}
        {onCheckSync && <Button type="button" size="sm" variant="secondary" disabled={busy || syncLoading} onClick={onCheckSync}>{t('sync_check')}</Button>}
        {syncError && <div className={styles.error} role="alert">{t('sync_failed')}</div>}
      </div>}
      <div className={styles.filters}>
        <Input type="search" label={t('credential_search')} aria-label={t('credential_search')} value={query.credential ?? ''} disabled={busy} onChange={event => changeQuery({ ...query, credential: event.target.value, credentialPage: 1 })} />
        {!mobile && <Input type="search" label={t('model_search')} aria-label={t('model_search')} value={query.model ?? ''} disabled={busy} onChange={event => changeQuery({ ...query, model: event.target.value, modelPage: 1 })} />}
        {onReload && <Button type="button" variant="secondary" disabled={busy} onClick={onReload}>{t('refresh')}</Button>}
      </div>
      {pagination('credential')}
      {!loading && credentials.length === 0 && <p role="status">{t('empty_credentials')}</p>}
      {mobile ? <div className={styles.mobile}>
        <Select value={chosenCredential?.subject_id ?? ''} ariaLabel={t('select_credential')} placeholder={t('select_credential')} disabled={busy} dropdownClassName={styles.options} options={credentials.filter(item => item.subject_id).map(item => ({ value: item.subject_id!, label: safeCredentialLabel(item) }))} onChange={value => { if (busy) return; onChooseCredential(value); setSelected([]); setDraft(null); setFieldError(''); setMutationError(''); setNotice(''); }} />
        {chosenCredential ? <>
          <p className={styles.hint}>{safeCredentialLabel(chosenCredential)}</p>
          <Input type="search" label={t('model_search')} aria-label={t('model_search')} value={query.model ?? ''} disabled={busy} onChange={event => {
            if (busy) return;
            setSelected([]); setDraft(null); setMutationError(''); onQueryChange({ ...query, model: event.target.value, modelPage: 1 });
          }} />
          <div className={styles.modelCards}>{models.map(model => <article className={styles.modelCard} key={model}><h4>{model}</h4>{renderCell(chosenCredential, model)}</article>)}</div>
        </> : <p className={styles.hint}>{t('choose_credential')}</p>}
      </div> : credentials.length > 0 && models.length > 0 && <div className={styles.tableScroll} tabIndex={0} role="region" aria-label={t('title')}>
        <table className={styles.table}><thead><tr><th className={styles.credential} scope="col">{t('credential')}</th>{models.map(model => <th scope="col" key={model}>{model}</th>)}</tr></thead>
          <tbody>{credentials.map(credential => <tr key={credential.subject_id ?? credential.directory_id}><th className={styles.credential} scope="row">{safeCredentialLabel(credential)}</th>{models.map(model => <td key={model}>{renderCell(credential, model)}</td>)}</tr>)}</tbody>
        </table>
      </div>}
      {!loading && models.length === 0 && <p role="status">{t('empty_models')}</p>}
      {pagination('model')}
      {canManage && onSaveCells && selection.length > 0 && <div className={styles.actions}>
        <span>{t('selected', { count: selection.length })}</span>
        <Button type="button" disabled={busy} onClick={() => edit(selection)}>{t('batch_edit')}</Button>
        <Button type="button" variant="secondary" disabled={busy} onClick={() => void mutate(selection, true)}>{t('batch_clear')}</Button>
        <Button type="button" variant="ghost" disabled={busy} onClick={() => setSelected([])}>{t('clear_selection')}</Button>
      </div>}
      {canManage && draft && <form className={styles.editor} onSubmit={event => { event.preventDefault(); void mutate(draft.targets, false); }}>
        <h4>{t(`${draft.targets.length > 1 ? 'batch_title' : 'editor_title'}`, { count: draft.targets.length })}</h4>
        <p className={styles.hint}>{draft.targets.map(pair => `${credentialName(credentials.find(credential => credential.subject_id === pair.subject_id) ?? { name: '', alias: '' } as PricingCredential)} / ${pair.canonical_model_id}`).join(' · ')}</p>
        <div className={styles.rates}>{rateFields.map(field => <Input key={field} type="text" inputMode="decimal" label={t(`${field}`)} aria-label={t(`${field}`)} value={draft.rates[field]} disabled={busy} aria-invalid={fieldError === 'invalid_prices'} onChange={event => { setDraft({ ...draft, rates: { ...draft.rates, [field]: event.target.value } }); setFieldError(''); }} />)}</div>
        <Select value={draft.style} ariaLabel={t('style')} placeholder={t('style')} disabled={busy} options={(['openai', 'claude'] as const).map(value => ({ value, label: t(`style_${value}`) }))} onChange={value => { if (value === 'openai' || value === 'claude') { setDraft({ ...draft, style: value }); setFieldError(''); } }} />
        {fieldError && <div className={styles.error} role="alert">{t(`${fieldError}`)}</div>}
        <div className={styles.actions}>
          <Button type="submit" disabled={busy}>{t(`${pending || saving ? 'saving' : 'save'}`)}</Button>
          <Button type="button" variant="secondary" disabled={busy} onClick={() => void mutate(draft.targets, true)}>{t('clear')}</Button>
          <Button type="button" variant="ghost" disabled={busy} onClick={() => { setDraft(null); setFieldError(''); setMutationError(''); }}>{t('cancel')}</Button>
        </div>
      </form>}
      {mutationError && <div className={styles.error} role="alert">{t(`${mutationError}`)}</div>}
      {notice && <p role="status">{t(`${notice}`)}</p>}
    </div>
  </Card>;
}
