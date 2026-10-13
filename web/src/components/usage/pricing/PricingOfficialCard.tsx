import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Select } from '@/components/ui/Select';
import { Modal } from '@/components/ui/Modal';
import { useOfficialPricingData } from '@/components/usage/hooks/useMatrixPricingData';
import type { PricingOfficialEntry, PricingOfficialInput, PricingOfficialRefreshSource, PricingOfficialSource } from '@/lib/types';
import styles from './PricingOfficialCard.module.scss';

const fields = [
  ['prompt_price_per_1m', 'official_input'],
  ['completion_price_per_1m', 'official_output'],
  ['cache_read_price_per_1m', 'official_cache_read'],
  ['cache_write_price_per_1m', 'official_cache_write'],
] as const;
const sourceKeys: Record<string, string> = {
  official: 'official_source_official', openrouter: 'official_source_openrouter',
  'legacy-seed': 'official_source_legacy_seed', legacy_seed: 'official_source_legacy_seed',
  'models-dev': 'official_source_models_dev', litellm: 'official_source_litellm',
};
const writableSources = new Set<PricingOfficialSource>(['official', 'openrouter', 'legacy-seed', 'models-dev', 'litellm']);
const reasonKeys: Record<string, string> = {
  no_match: 'official_no_match', ambiguous_match: 'official_ambiguous_match',
  ambiguous: 'official_ambiguous', missing_segments: 'official_missing',
  stale: 'official_stale', unavailable: 'official_unavailable',
};
const pageSize = 25;
const decimalText = (value: number) => value.toLocaleString('en-US', { useGrouping: false, maximumSignificantDigits: 21 });
const validPrice = (value: unknown): value is number => typeof value === 'number' && Number.isFinite(value) && value >= 0;
const safeHash = (value?: string) => value && /^[a-f0-9]{64}$/i.test(value) ? value : '';
const safeVersion = (value?: string) => value && /^(?:v?\d+(?:\.\d+){0,3}|\d{4}-\d{2}-\d{2}(?:T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)?)$/.test(value) ? value : '';

type Draft = Record<(typeof fields)[number][0], string>;
interface PricingOfficialCardProps {
  canManage?: boolean;
  onAuthRequired?: () => void;
  onChanged?: () => void;
  onNotice?: (kind: 'success' | 'info' | 'error', message: string) => void;
  refreshRevision?: number;
}

export function PricingOfficialCard(props: PricingOfficialCardProps) {
  // Remounting aborts hook requests and discards drafts/previews on access or revision changes.
  return <OfficialCardContent key={`${Boolean(props.canManage)}:${props.refreshRevision ?? 0}`} {...props} />;
}

function OfficialCardContent({ canManage = false, onAuthRequired, onChanged, onNotice, refreshRevision = 0 }: PricingOfficialCardProps) {
  const { t, i18n } = useTranslation('matrix_pricing');
  const data = useOfficialPricingData({ canManage, onAuthRequired, onChanged, onNotice });
  const { loadOfficialPricing, clearPreview, canRead, canEdit } = data;
  const [search, setSearch] = useState('');
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(0);
  const [source, setSource] = useState<PricingOfficialRefreshSource>('models-dev');
  const [editing, setEditing] = useState<PricingOfficialEntry | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [fieldError, setFieldError] = useState(false);
  const [notice, setNotice] = useState('');
  const [previewSearch, setPreviewSearch] = useState('');
  const [previewPage, setPreviewPage] = useState(0);
  const revisionRef = useRef(refreshRevision);
  const lifetimeRef = useRef(0);
  const busy = data.loading || data.saving || data.previewing;

  useEffect(() => {
    const lifetime = lifetimeRef.current + 1;
    lifetimeRef.current = lifetime;
    return () => { lifetimeRef.current = lifetime + 1; };
  }, [canManage, refreshRevision]);
  useEffect(() => {
    if (revisionRef.current === refreshRevision) return;
    revisionRef.current = refreshRevision;
    clearPreview();
    setEditing(null); setDraft(null); setNotice('');
    if (canRead) void loadOfficialPricing();
  }, [refreshRevision, canRead, clearPreview, loadOfficialPricing]);

  const filtered = useMemo(() => {
    const needle = query.toLocaleLowerCase();
    return data.pricing.filter(row => `${row.canonical_model_id} ${row.display_model_id}`.toLocaleLowerCase().includes(needle));
  }, [data.pricing, query]);
  const lastPage = Math.max(0, Math.ceil(filtered.length / pageSize) - 1);
  const currentPage = Math.min(page, lastPage);
  const rows = filtered.slice(currentPage * pageSize, (currentPage + 1) * pageSize);
  const previewRows = (data.preview?.prices ?? []).filter(row =>
    `${row.canonical_model_id} ${row.display_model_id}`.toLocaleLowerCase().includes(previewSearch.toLocaleLowerCase()));
  const lastPreviewPage = Math.max(0, Math.ceil(previewRows.length / pageSize) - 1);
  const currentPreviewPage = Math.min(previewPage, lastPreviewPage);

  const timestamp = (value?: string) => {
    const date = value ? new Date(value) : null;
    return date && Number.isFinite(date.getTime()) ? date.toLocaleString(i18n.language) : t('official_unknown_time');
  };
  const price = (value: unknown) => validPrice(value) ? `$${decimalText(value)}` : t('official_missing_price');
  const status = (row: PricingOfficialEntry) => {
    // Only explicit safe reason codes are rendered, never upstream diagnostic text.
    const reason = 'unavailable_reason' in row ? row.unavailable_reason : undefined;
    const missing = fields.some(([field]) => !validPrice(row[field])) || row.missing_segments.length > 0;
    return <div className={styles.status}>
      {row.stale && <span>{t('official_stale')}</span>}
      {missing && <span>{t('official_missing')}</span>}
      {typeof reason === 'string' && reason && <span>{t(`${reasonKeys[reason] ?? 'official_unknown_reason'}`)}</span>}
      {!row.stale && !missing && !reason && <span>{t('official_available')}</span>}
    </div>;
  };
  const table = (entries: PricingOfficialEntry[]) => <div className={styles.scroll} data-testid="official-pricing-scroll" role="region" aria-label={t('official_table')} tabIndex={0}>
    <table className={styles.table} aria-label={t('official_table')}>
      <thead><tr><th scope="col">{t('official_model')}</th>
        {fields.map(([field, key]) => <th scope="col" key={field}>{t(`${key}`)}</th>)}
        <th scope="col">{t('official_source')}</th><th scope="col">{t('official_status')}</th>
        <th scope="col">{t('official_updated')}</th>{canEdit && !data.preview && <th scope="col">{t('official_edit')}</th>}
      </tr></thead>
      <tbody>{entries.map(row => <tr key={row.id}>
        <th scope="row"><div className={styles.identity}><span>{row.display_model_id}</span>
          <small>{t('official_canonical_id')}: {row.canonical_model_id}</small>
          <small>{t('official_display_id')}: {row.display_model_id}</small></div></th>
        {fields.map(([field]) => <td className={styles.price} key={field}>{price(row[field])}</td>)}
        <td>{t(`${sourceKeys[row.source] ?? 'official_source_unknown'}`)}</td><td>{status(row)}</td>
        <td><div className={styles.metadata}><span>{t('official_updated')}: {timestamp(row.updated_at)}</span>
          <span>{t('official_checked')}: {timestamp(row.fetched_at)}</span>
          {safeVersion(row.source_version) && <span>{t('official_source_version')}: {safeVersion(row.source_version)}</span>}
          {safeHash(row.response_hash) && <span>{t('official_response_hash')}: {safeHash(row.response_hash)}</span>}
        </div></td>
        {canEdit && !data.preview && <td><Button variant="secondary" size="sm" disabled={busy || !writableSources.has(row.source as PricingOfficialSource)} onClick={() => {
          if (!canManage || !canEdit || busy) return;
          clearPreview(); setEditing(row); setDraft(Object.fromEntries(fields.map(([field]) => [field, validPrice(row[field]) ? decimalText(row[field]) : ''])) as Draft);
          setFieldError(false); setNotice('');
        }}>{t('official_edit')}</Button></td>}
      </tr>)}</tbody>
    </table>
  </div>;
  const publish = (key: string) => {
    setNotice(t(`${key}`));
  };
  const save = async () => {
    if (!canManage || !canEdit || busy || !editing || !draft || !writableSources.has(editing.source as PricingOfficialSource)) return;
    if (fields.some(([field]) => !/^(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)$/.test(draft[field].trim()) || !validPrice(Number(draft[field])))) {
      setFieldError(true); return;
    }
    const input: PricingOfficialInput = {
      canonical_model_id: editing.canonical_model_id, display_model_id: editing.display_model_id,
      source: editing.source as PricingOfficialSource, pricing_style: editing.pricing_style,
      prompt_price_per_1m: Number(draft.prompt_price_per_1m), completion_price_per_1m: Number(draft.completion_price_per_1m),
      cache_read_price_per_1m: Number(draft.cache_read_price_per_1m), cache_write_price_per_1m: Number(draft.cache_write_price_per_1m),
      ...(editing.source_url ? { source_url: editing.source_url } : {}),
      ...(editing.aliases ? { aliases: editing.aliases } : {}),
    };
    const lifetime = lifetimeRef.current;
    const saved = await data.saveOfficialPrice(input);
    if (!saved || lifetime !== lifetimeRef.current) return;
    setEditing(null); setDraft(null); publish('official_saved');
  };
  const preview = async () => {
    if (!canManage || !canEdit || busy) return;
    setEditing(null); setDraft(null); setNotice(''); setPreviewSearch(''); setPreviewPage(0);
    await data.previewOfficialPricing(source);
  };
  const commit = async () => {
    if (!canManage || !canEdit || busy || !data.preview) return;
    const lifetime = lifetimeRef.current;
    const result = await data.commitOfficialPricing();
    if (result && lifetime === lifetimeRef.current) publish('official_refreshed');
  };
  const closeEdit = () => { if (!busy) { setEditing(null); setDraft(null); } };

  return <Card title={t('official_title')} subtitle={t('official_help')}>
    <div className={styles.body}>
      {!canRead ? <><p className={styles.hint}>{t('official_readonly')}</p><p role="status">{t('official_read_limit')}</p></> : <>
        {data.error && <div className={styles.error} role="alert">{data.error}</div>}
        {data.previewError && <div className={styles.error} role="alert">{data.previewError}</div>}
        {notice && <div role="status">{notice}</div>}
        <form className={styles.actions} onSubmit={event => { event.preventDefault(); setQuery(search.trim()); setPage(0); }}>
          <div className={styles.search}><Input aria-label={t('official_search')} placeholder={t('official_search')} value={search} maxLength={200} onChange={event => setSearch(event.target.value)} /></div>
          <Button type="submit" variant="secondary">{t('official_search_action')}</Button>
        </form>
        {data.loading && <div role="status">{t('official_loading')}</div>}
        {rows.length > 0 ? table(rows) : !data.loading && <p>{t('official_empty')}</p>}
        <div className={styles.actions} aria-label={t('official_page', { page: currentPage + 1 })}>
          <Button variant="secondary" disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}>{t('official_previous')}</Button>
          <span>{t('official_page', { page: currentPage + 1 })}</span>
          <Button variant="secondary" disabled={currentPage >= lastPage} onClick={() => setPage(currentPage + 1)}>{t('official_next')}</Button>
        </div>
        {canManage && canEdit && <>
          <p className={styles.hint}>{t('official_refresh_help')}</p>
          <div className={styles.actions}><Select value={source} disabled={busy} ariaLabel={t('official_refresh_source')} onChange={value => { clearPreview(); setSource(value as PricingOfficialRefreshSource); }}
            options={(['models-dev', 'litellm', 'openrouter'] as const).map(value => ({ value, label: t(`${sourceKeys[value]}`) }))} />
            <Button disabled={busy} onClick={() => void preview()}>{t('official_refresh_preview')}</Button></div>
          <Modal open={!!editing && !!draft} title={t('official_edit_title')} onClose={closeEdit} closeDisabled={busy}
            footer={<><Button variant="secondary" disabled={busy} onClick={closeEdit}>{t('official_cancel')}</Button><Button disabled={busy} onClick={() => void save()}>{t('official_save')}</Button></>}>
            <p className={styles.hint}>{editing?.canonical_model_id}</p><p className={styles.hint}>{t('official_edit_help')}</p>
            {data.error && <div className={styles.error} role="alert">{data.error}</div>}
            {fieldError && <div className={styles.error} role="alert">{t('official_invalid_prices')}</div>}
            <div className={styles.rates}>{fields.map(([field, key]) => <Input key={field} label={t(`${key}`)} aria-label={t(`${key}`)} inputMode="decimal" value={draft?.[field] ?? ''} disabled={busy} aria-invalid={fieldError}
              onChange={event => { setDraft(previous => previous ? { ...previous, [field]: event.target.value } : null); setFieldError(false); }} />)}</div>
          </Modal>
          <Modal open={!!data.preview} width={900} title={t('official_refresh_preview')} onClose={() => { if (!busy) clearPreview(); }} closeDisabled={busy}
            footer={<><Button variant="secondary" disabled={busy} onClick={clearPreview}>{t('official_cancel')}</Button><Button disabled={busy || !data.preview} onClick={() => void commit()}>{t('official_commit_refresh')}</Button></>}>
            {data.preview && <div className={styles.preview}>
              <p className={styles.hint}>{t('official_refresh_help')}</p>
              <div>{t('official_preview_id')}: {data.preview.preview_id}</div>
              <div>{t('official_preview_source')}: {t(`${sourceKeys[data.preview.source_id] ?? 'official_source_unknown'}`)}</div>
              {safeHash(data.preview.response_hash) && <div>{t('official_response_hash')}: {safeHash(data.preview.response_hash)}</div>}
              <div>{t('official_checked')}: {timestamp(data.preview.fetched_at)}</div>
              <div>{t('official_preview_count')}: {data.preview.prices.length}</div>
              {data.preview.stale && <div>{t('official_stale')}</div>}
              {!!data.preview.ambiguous_models?.length && <div>{t('official_preview_ambiguous')}: {data.preview.ambiguous_models.length}</div>}
              {data.previewError && <div className={styles.error} role="alert">{data.previewError}</div>}
              <Input aria-label={t('official_search')} placeholder={t('official_search')} value={previewSearch} maxLength={200} onChange={event => { setPreviewSearch(event.target.value); setPreviewPage(0); }} />
              {table(previewRows.slice(currentPreviewPage * pageSize, (currentPreviewPage + 1) * pageSize))}
              <div className={styles.actions}><Button variant="secondary" disabled={currentPreviewPage === 0} onClick={() => setPreviewPage(currentPreviewPage - 1)}>{t('official_previous')}</Button>
                <span>{t('official_page', { page: currentPreviewPage + 1 })}</span>
                <Button variant="secondary" disabled={currentPreviewPage >= lastPreviewPage} onClick={() => setPreviewPage(currentPreviewPage + 1)}>{t('official_next')}</Button></div>
            </div>}
          </Modal>
        </>}
      </>}
    </div>
  </Card>;
}
