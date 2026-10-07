import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Select } from '@/components/ui/Select';
import { Input } from '@/components/ui/Input';
import { Button } from '@/components/ui/Button';
import { ApiError, clearPricingCredentialModel, fetchCredentialPricingSubjects, fetchPricing, fetchPricingCredentialModel, fetchPricingCredentialModels, fetchUsedModels, savePricingCredentialModel } from '@/lib/api';
import type { PricingCredential, PricingCredentialModel } from '@/lib/types';
import styles from './PricingCredentialDefaultsCard.module.scss';

export function PricingCredentialModelsCard({ canManage = true }: { canManage?: boolean }) {
  const { t } = useTranslation();
  const [subjects, setSubjects] = useState<PricingCredential[]>([]);
  const [models, setModels] = useState<string[]>([]);
  const [exceptions, setExceptions] = useState<PricingCredentialModel[]>([]);
  const [selected, setSelected] = useState('');
  const [model, setModel] = useState('');
  const [config, setConfig] = useState<PricingCredentialModel | null>(null);
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
    setBusy(true); setError(''); setFieldError(''); setNotice('');
    return controller;
  };
  const applyConfig = (saved: PricingCredentialModel) => {
    setConfig(saved);
    setMultiplier(saved.multiplier === null ? '' : saved.multiplier.toLocaleString('en-US', { useGrouping: false, maximumSignificantDigits: 21 }));
    setExceptions(previous => [
      ...previous.filter(item => item.model !== saved.model),
      ...(saved.multiplier === null ? [] : [saved]),
    ]);
  };
  const handleError = (cause: unknown, fallback: string) => {
    // Never display upstream bodies: only localized safe errors are allowed.
    if (cause instanceof ApiError && (cause.status === 401 || cause.status === 403)) {
      setDenied(true); setSubjects([]); setModels([]); setExceptions([]);
      setSelected(''); setModel(''); setConfig(null); setMultiplier(''); setNotice(''); setFieldError('');
      setError('pricing_credential_models.permission_denied');
    } else if (fallback === 'pricing_credential_models.save_failed' && cause instanceof ApiError && (cause.status === 400 || cause.status === 422)) {
      setFieldError('pricing_credential_models.invalid_multiplier');
    } else {
      setError(cause instanceof ApiError && cause.status === 409 ? 'pricing_credential_models.conflict' : fallback);
    }
  };
  const loadChoices = async (signal: AbortSignal) => {
    const [directory, used, pricing] = await Promise.all([
      fetchCredentialPricingSubjects(signal), fetchUsedModels(signal), fetchPricing(signal),
    ]);
    if (signal.aborted) return null;
    const registered = directory.credentials.filter(item => item.subject_id);
    setSubjects(registered);
    setModels([...new Set([...used.models, ...pricing.pricing.map(item => item.model)])].sort());
    return registered;
  };

  useEffect(() => {
    const controller = new AbortController();
    requestRef.current = controller;
    setSubjects([]); setModels([]); setExceptions([]); setSelected(''); setModel('');
    setConfig(null); setMultiplier(''); setError(''); setFieldError(''); setNotice(''); setDenied(false); setBusy(canManage);
    if (canManage) {
      void loadChoices(controller.signal).catch(cause => {
        if (!controller.signal.aborted) handleError(cause, 'pricing_credential_models.load_failed');
      }).finally(() => { if (!controller.signal.aborted) setBusy(false); });
    }
    return () => requestRef.current?.abort();
  }, [canManage]);

  const chooseSubject = async (subjectId: string) => {
    if (busy) return;
    const controller = beginRequest();
    setSelected(subjectId); setModel(''); setConfig(null); setMultiplier(''); setExceptions([]);
    try {
      const saved = await fetchPricingCredentialModels(subjectId, controller.signal);
      if (!controller.signal.aborted) setExceptions(saved.models);
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, 'pricing_credential_models.load_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const chooseModel = async (modelName: string) => {
    if (busy || !selected) return;
    const controller = beginRequest();
    setModel(modelName); setConfig(null); setMultiplier('');
    try {
      const saved = await fetchPricingCredentialModel(selected, modelName, controller.signal);
      if (!controller.signal.aborted) applyConfig(saved);
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, 'pricing_credential_models.load_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const refresh = async () => {
    if (busy) return;
    const controller = beginRequest();
    setConfig(null); setMultiplier(''); setExceptions([]);
    try {
      const registered = await loadChoices(controller.signal);
      if (!registered) return;
      if (selected && registered.some(item => item.subject_id === selected)) {
        const saved = await fetchPricingCredentialModels(selected, controller.signal);
        if (controller.signal.aborted) return;
        setExceptions(saved.models);
        if (model) {
          const readback = await fetchPricingCredentialModel(selected, model, controller.signal);
          if (!controller.signal.aborted) applyConfig(readback);
        }
      } else { setSelected(''); setModel(''); }
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, 'pricing_credential_models.load_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const save = async (clear = false) => {
    if (busy || !selected || !model || !config || (clear && config.multiplier === null)) return;
    const text = multiplier.trim();
    const match = /^(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)([xX%])?$/.exec(text);
    const value = match ? Number(match[1] ? text.slice(0, -1) : text) / (match[1] === '%' ? 100 : 1) : NaN;
    if (!clear && (!Number.isFinite(value) || value < 0)) {
      setError(''); setNotice(''); setFieldError('pricing_credential_models.invalid_multiplier'); return;
    }
    const controller = beginRequest();
    try {
      const saved = await (clear ? clearPricingCredentialModel(selected, model, controller.signal) : savePricingCredentialModel(selected, model, text, controller.signal));
      if (controller.signal.aborted) return;
      // Retain the committed canonical result even if readback fails.
      applyConfig(saved);
      setNotice(clear ? 'pricing_credential_models.cleared' : 'pricing_credential_models.saved');
      try {
        const readback = await fetchPricingCredentialModel(selected, model, controller.signal);
        if (!controller.signal.aborted) applyConfig(readback);
      } catch (cause) {
        if (!controller.signal.aborted) handleError(cause, 'pricing_credential_models.load_failed');
      }
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, clear ? 'pricing_credential_models.clear_failed' : 'pricing_credential_models.save_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const label = (item: PricingCredential) => [item.alias, item.name, item.provider_type, item.auth_type, item.endpoint,
    t(`pricing_credentials.status.${item.status}`), t(`pricing_credentials.binding.${item.binding_status}`), item.subject_id].filter(Boolean).join(' · ');
  const choice = subjects.find(item => item.subject_id === selected);
  const modelNames = [...new Set([...models, ...exceptions.map(item => item.model), ...(model ? [model] : [])])].sort();

  if (!canManage) return null;
  return (
    <Card title={t('pricing_credential_models.title')} subtitle={t('pricing_credential_models.scope_help')}>
      <div className={styles.body}>
        <p className={styles.hint}>{t('pricing_credential_models.history_warning')}</p>
        <p className={styles.hint}>{t('pricing_credential_models.replacement_warning')}</p>
        {error && <div className={styles.error} role="alert">{t(error)}</div>}
        {notice && <div role="status">{t(notice)}</div>}
        {!denied && <>
          <div className={styles.actions}>
            <div className={styles.selector}>
              <Select value={selected} onChange={value => void chooseSubject(value)} disabled={busy}
                ariaLabel={t('pricing_credential_models.select')} placeholder={t('pricing_credential_models.select')}
                options={subjects.map(item => ({ value: item.subject_id!, label: label(item) }))} dropdownClassName={styles.options}
                search={{ placeholder: t('pricing_credentials.search'), noResultsText: t('pricing_credential_models.empty') }} />
            </div>
            <Button variant="secondary" onClick={() => void refresh()} disabled={busy}>{t('pricing_credential_models.refresh')}</Button>
          </div>
          {choice && <p className={styles.hint}>{label(choice)}</p>}
          <div className={styles.actions}><div className={styles.selector}>
            <Select value={model} onChange={value => void chooseModel(value)} disabled={busy || !selected}
              ariaLabel={t('pricing_credential_models.model')} placeholder={t('pricing_credential_models.model')}
              options={modelNames.map(item => ({ value: item, label: item }))} dropdownClassName={styles.options}
              search={{ placeholder: t('pricing_credential_models.model_search'), noResultsText: t('pricing_credential_models.no_models') }} />
          </div></div>
          <p className={styles.hint}>{t('pricing_credential_models.model_help')}</p>
          {busy && <div role="status">{t('common.loading')}</div>}
          {!busy && subjects.length === 0 && <p className={styles.hint}>{t('pricing_credential_models.empty')}</p>}
          {config && <div className={styles.readback}>
            <span>{t('pricing_credential_models.current')}</span><span>{config.model}</span>
            {config.multiplier === null ? <span>{t('pricing_credential_models.inherited')}</span> : <>
              <span>{t('pricing_credential_models.active')}</span>
              <output aria-label={t('pricing_credential_models.canonical')}>{String(config.multiplier)}</output>
            </>}
          </div>}
          <Input type="text" label={t('pricing_credential_models.multiplier')} aria-label={t('pricing_credential_models.multiplier')}
            value={multiplier} onChange={event => { setMultiplier(event.target.value); setFieldError(''); }} disabled={busy || !config}
            placeholder="1.2x" hint={t('pricing_credential_models.input_help')} error={fieldError ? t(fieldError) : undefined} />
          <div className={styles.actions}>
            <Button onClick={() => void save()} disabled={busy || !config}>{t('pricing_credential_models.save')}</Button>
            <Button variant="secondary" onClick={() => void save(true)} disabled={busy || !config || config.multiplier === null}>{t('pricing_credential_models.clear')}</Button>
          </div>
        </>}
      </div>
    </Card>
  );
}
