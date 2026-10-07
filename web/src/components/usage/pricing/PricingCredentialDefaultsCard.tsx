import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Select } from '@/components/ui/Select';
import { Input } from '@/components/ui/Input';
import { Button } from '@/components/ui/Button';
import { ApiError, clearPricingCredentialDefault, fetchCredentialPricingSubjects, fetchPricingCredentialDefault, savePricingCredentialDefault } from '@/lib/api';
import type { PricingCredential, PricingCredentialDefault } from '@/lib/types';
import styles from './PricingCredentialDefaultsCard.module.scss';

export function PricingCredentialDefaultsCard({ canManage = true }: { canManage?: boolean }) {
  const { t } = useTranslation();
  const [subjects, setSubjects] = useState<PricingCredential[]>([]);
  const [selected, setSelected] = useState('');
  const [config, setConfig] = useState<PricingCredentialDefault | null>(null);
  const [multiplier, setMultiplier] = useState('');
  const [busy, setBusy] = useState(false);
  const [denied, setDenied] = useState(false);
  const [error, setError] = useState('');
  const [fieldError, setFieldError] = useState('');
  const [notice, setNotice] = useState('');
  const requestRef = useRef<AbortController | null>(null);

  const beginRequest = () => {
    requestRef.current?.abort();
    const controller = new AbortController();
    requestRef.current = controller;
    setBusy(true);
    setError('');
    setFieldError('');
    setNotice('');
    return controller;
  };
  const applyConfig = (saved: PricingCredentialDefault) => {
    setConfig(saved);
    // Keep server numbers editable in the plain-decimal grammar, even when String uses an exponent.
    setMultiplier(saved.multiplier === null ? '' : saved.multiplier.toLocaleString('en-US', { useGrouping: false, maximumSignificantDigits: 21 }));
  };
  const handleError = (cause: unknown, fallback: string) => {
    // Upstream error bodies may contain secrets; only display our localized allowlist.
    if (cause instanceof ApiError && (cause.status === 401 || cause.status === 403)) {
      setDenied(true);
      setSubjects([]);
      setSelected('');
      setConfig(null);
      setMultiplier('');
      setNotice('');
      setFieldError('');
      setError('pricing_credential_defaults.permission_denied');
    } else if (fallback === 'pricing_credential_defaults.save_failed' && cause instanceof ApiError && (cause.status === 400 || cause.status === 422)) {
      setFieldError('pricing_credential_defaults.invalid_multiplier');
    } else {
      setError(cause instanceof ApiError && cause.status === 409 ? 'pricing_credential_defaults.conflict' : fallback);
    }
  };

  useEffect(() => {
    const controller = new AbortController();
    requestRef.current = controller;
    setSubjects([]);
    setSelected('');
    setConfig(null);
    setMultiplier('');
    setError('');
    setFieldError('');
    setNotice('');
    setDenied(false);
    setBusy(canManage);
    if (canManage) {
      void fetchCredentialPricingSubjects(controller.signal).then(result => {
        if (!controller.signal.aborted) setSubjects(result.credentials.filter(item => item.subject_id));
      }).catch(cause => {
        if (!controller.signal.aborted) handleError(cause, 'pricing_credential_defaults.load_failed');
      }).finally(() => { if (!controller.signal.aborted) setBusy(false); });
    }
    return () => requestRef.current?.abort();
  }, [canManage]);

  const choose = async (subjectId: string) => {
    if (busy) return;
    const controller = beginRequest();
    setSelected(subjectId);
    setConfig(null);
    setMultiplier('');
    try {
      const saved = await fetchPricingCredentialDefault(subjectId, controller.signal);
      if (!controller.signal.aborted) applyConfig(saved);
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, 'pricing_credential_defaults.load_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const refresh = async () => {
    if (busy) return;
    const controller = beginRequest();
    setConfig(null);
    setMultiplier('');
    try {
      const rows = await fetchCredentialPricingSubjects(controller.signal);
      if (controller.signal.aborted) return;
      setSubjects(rows.credentials.filter(item => item.subject_id));
      if (selected && rows.credentials.some(item => item.subject_id === selected)) {
        const saved = await fetchPricingCredentialDefault(selected, controller.signal);
        if (!controller.signal.aborted) applyConfig(saved);
      } else {
        setSelected('');
      }
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, 'pricing_credential_defaults.load_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const save = async (clear = false) => {
    if (busy || !selected || !config || (clear && config.multiplier === null)) return;
    const text = multiplier.trim();
    const match = /^(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)([xX%])?$/.exec(text);
    const value = match ? Number(match[1] ? text.slice(0, -1) : text) / (match[1] === '%' ? 100 : 1) : NaN;
    if (!clear && (!Number.isFinite(value) || value < 0)) {
      setError('');
      setNotice('');
      setFieldError('pricing_credential_defaults.invalid_multiplier');
      return;
    }
    const controller = beginRequest();
    try {
      const saved = await (clear ? clearPricingCredentialDefault(selected, controller.signal) : savePricingCredentialDefault(selected, text, controller.signal));
      if (controller.signal.aborted) return;
      // Retain the committed canonical value even if the subsequent GET fails.
      applyConfig(saved);
      setNotice(clear ? 'pricing_credential_defaults.cleared' : 'pricing_credential_defaults.saved');
      try {
        const readback = await fetchPricingCredentialDefault(selected, controller.signal);
        if (!controller.signal.aborted) applyConfig(readback);
      } catch (cause) {
        if (!controller.signal.aborted) handleError(cause, 'pricing_credential_defaults.load_failed');
      }
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, clear ? 'pricing_credential_defaults.clear_failed' : 'pricing_credential_defaults.save_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const label = (item: PricingCredential) => [item.alias, item.name, item.provider_type, item.auth_type, item.endpoint,
    t(`pricing_credentials.status.${item.status}`), t(`pricing_credentials.binding.${item.binding_status}`), item.subject_id].filter(Boolean).join(' · ');
  const choice = subjects.find(item => item.subject_id === selected);

  if (!canManage) return null;
  return (
    <Card title={t('pricing_credential_defaults.title')} subtitle={t('pricing_credential_defaults.scope_help')}>
      <div className={styles.body}>
        <p className={styles.hint}>{t('pricing_credential_defaults.history_warning')}</p>
        <p className={styles.hint}>{t('pricing_credential_defaults.replacement_warning')}</p>
        {error && <div className={styles.error} role="alert">{t(error)}</div>}
        {notice && <div role="status">{t(notice)}</div>}
        {!denied && <>
          <div className={styles.actions}>
            <div className={styles.selector}>
              <Select value={selected} onChange={value => void choose(value)} disabled={busy}
                ariaLabel={t('pricing_credential_defaults.select')} placeholder={t('pricing_credential_defaults.select')}
                options={subjects.map(item => ({ value: item.subject_id!, label: label(item) }))}
                dropdownClassName={styles.options}
                search={{ placeholder: t('pricing_credentials.search'), noResultsText: t('pricing_credential_defaults.empty') }} />
            </div>
            <Button variant="secondary" onClick={() => void refresh()} disabled={busy}>{t('pricing_credential_defaults.refresh')}</Button>
          </div>
          {choice && <p className={styles.hint}>{label(choice)}</p>}
          {busy && <div role="status">{t('common.loading')}</div>}
          {!busy && subjects.length === 0 && <p className={styles.hint}>{t('pricing_credential_defaults.empty')}</p>}
          {config && <div className={styles.readback}>
            <span>{t('pricing_credential_defaults.current')}</span>
            {config.multiplier === null ? <span>{t('pricing_credential_defaults.inherited')}</span> : <>
              <span>{t('pricing_credential_defaults.active')}</span>
              <output aria-label={t('pricing_credential_defaults.canonical')}>{String(config.multiplier)}</output>
            </>}
          </div>}
          <Input type="text" label={t('pricing_credential_defaults.multiplier')} aria-label={t('pricing_credential_defaults.multiplier')}
            value={multiplier} onChange={event => { setMultiplier(event.target.value); setFieldError(''); }} disabled={busy || !config}
            placeholder="0.2x" hint={t('pricing_credential_defaults.input_help')} error={fieldError ? t(fieldError) : undefined} />
          <div className={styles.actions}>
            <Button onClick={() => void save()} disabled={busy || !config}>{t('pricing_credential_defaults.save')}</Button>
            <Button variant="secondary" onClick={() => void save(true)} disabled={busy || !config || config.multiplier === null}>{t('pricing_credential_defaults.clear')}</Button>
          </div>
        </>}
      </div>
    </Card>
  );
}
