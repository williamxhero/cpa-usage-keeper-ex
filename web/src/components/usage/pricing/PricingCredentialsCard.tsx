import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Select } from '@/components/ui/Select';
import { Button } from '@/components/ui/Button';
import { ApiError, bindPricingCredential, fetchCredentialPricingSubjects, fetchPricingCredentials } from '@/lib/api';
import type { PricingCredential } from '@/lib/types';
import styles from './PricingCredentialsCard.module.scss';

export function PricingCredentialsCard({ canManage = true, onChanged }: { canManage?: boolean; onChanged?: () => void }) {
  const { t } = useTranslation();
  const [directory, setDirectory] = useState<PricingCredential[]>([]);
  const [subjects, setSubjects] = useState<PricingCredential[]>([]);
  const [selected, setSelected] = useState('');
  const [busy, setBusy] = useState(false);
  const [denied, setDenied] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const requestRef = useRef<AbortController | null>(null);
  const onChangedRef = useRef(onChanged);
  useEffect(() => { onChangedRef.current = onChanged; }, [onChanged]);
  const notifyChanged = () => {
    // A parent refresh is fire-and-forget, not part of the committed mutation.
    try { void Promise.resolve(onChangedRef.current?.()).catch(() => {}); }
    catch { /* Refresh errors must not become mutation errors. */ }
  };

  const load = async (signal: AbortSignal) => {
    const [rows, saved] = await Promise.all([fetchPricingCredentials(signal), fetchCredentialPricingSubjects(signal)]);
    if (!signal.aborted) {
      setDirectory(rows.credentials);
      setSubjects(saved.credentials);
    }
  };
  const handleError = (cause: unknown, fallback: string) => {
    // Do not display arbitrary upstream error bodies in the selector.
    if (cause instanceof ApiError && (cause.status === 401 || cause.status === 403)) {
      setDenied(true);
      setDirectory([]);
      setSubjects([]);
      setSelected('');
      setError('pricing_credentials.permission_denied');
    } else {
      setError(cause instanceof ApiError && cause.status === 409 ? 'pricing_credentials.conflict' : fallback);
    }
  };

  useEffect(() => {
    const controller = new AbortController();
    requestRef.current = controller;
    if (canManage) {
      setBusy(true);
      void load(controller.signal).catch(cause => {
        if (!controller.signal.aborted) handleError(cause, 'pricing_credentials.load_failed');
      }).finally(() => { if (!controller.signal.aborted) setBusy(false); });
    }
    return () => requestRef.current?.abort();
  }, [canManage]);

  const refresh = async () => {
    if (busy) return;
    const controller = new AbortController();
    requestRef.current?.abort();
    requestRef.current = controller;
    setBusy(true);
    setError('');
    setNotice('');
    setSelected('');
    try { await load(controller.signal); }
    catch (cause) { if (!controller.signal.aborted) handleError(cause, 'pricing_credentials.load_failed'); }
    finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const choice = directory.find(item => String(item.directory_id) === selected);
  const save = async () => {
    if (busy || !choice || choice.binding_status !== 'unbound') return;
    const controller = new AbortController();
    requestRef.current?.abort();
    requestRef.current = controller;
    setBusy(true);
    setError('');
    setNotice('');
    try {
      const saved = await bindPricingCredential(choice.directory_id, controller.signal);
      if (controller.signal.aborted) return;
      // A successful commit remains visible even if the subsequent refresh fails.
      setSubjects(current => [...current.filter(item => item.subject_id !== saved.subject_id), saved]);
      setDirectory(current => current.map(item => item.directory_id === saved.directory_id ? saved : item));
      setSelected('');
      setNotice('pricing_credentials.saved');
      notifyChanged();
      try { await load(controller.signal); }
      catch (cause) { if (!controller.signal.aborted) handleError(cause, 'pricing_credentials.load_failed'); }
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, 'pricing_credentials.save_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const label = (item: PricingCredential) => [
    item.auth_type === 'apikey' ? item.alias || item.key_hint : item.name,
    item.provider_type,
    item.auth_type === 'oauth' ? 'Oauth' : item.auth_type,
    item.auth_type === 'apikey' ? item.endpoint : undefined,
    t(`pricing_credentials.status.${item.status}`),
    t(`pricing_credentials.binding.${item.binding_status}`),
  ].filter(Boolean).join(' · ');

  if (!canManage) return null;
  return (
    <Card title={t('pricing_credentials.title')} subtitle={t('pricing_credentials.no_pricing_change')}>
      <div className={styles.body}>
        <p className={styles.hint}>{t('pricing_credentials.identity_help')}</p>
        {error && <div className={styles.error} role="alert">{t(error)}</div>}
        {notice && <div role="status">{t(notice)}</div>}
        {!denied && <>
          <div className={styles.actions}>
            <div className={styles.selector}>
              <Select value={selected} onChange={setSelected} disabled={busy}
                ariaLabel={t('pricing_credentials.select')}
                placeholder={t('pricing_credentials.select')}
                options={directory.map(item => ({ value: String(item.directory_id), label: label(item), disabled: item.binding_status !== 'unbound' }))}
                search={{ placeholder: t('pricing_credentials.search'), noResultsText: t('pricing_credentials.empty') }} />
            </div>
            <Button onClick={() => void save()} disabled={busy || choice?.binding_status !== 'unbound'}>{t('pricing_credentials.register')}</Button>
            <Button variant="secondary" onClick={() => void refresh()} disabled={busy}>{t('pricing_credentials.refresh')}</Button>
          </div>
          {busy && <div role="status">{t('common.loading')}</div>}
          {!busy && directory.length === 0 && <p className={styles.hint}>{t('pricing_credentials.empty')}</p>}
          <h3 className={styles.heading}>{t('pricing_credentials.subjects')}</h3>
          {subjects.length === 0 ? <p className={styles.hint}>{t('pricing_credentials.no_subjects')}</p> :
            <ul className={styles.subjects}>{subjects.map(item => <li key={item.subject_id}>
              <span>{label(item)}</span>
              <code>{item.subject_id}</code>
            </li>)}</ul>}
        </>}
      </div>
    </Card>
  );
}
