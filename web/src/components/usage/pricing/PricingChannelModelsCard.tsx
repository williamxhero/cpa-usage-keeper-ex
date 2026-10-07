import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Select } from '@/components/ui/Select';
import { Input } from '@/components/ui/Input';
import { Button } from '@/components/ui/Button';
import { ApiError, clearPricingChannelModel, fetchPricingChannels, fetchPricing, fetchPricingChannelModel, fetchPricingChannelModels, fetchUsedModels, savePricingChannelFixed, savePricingChannelModel } from '@/lib/api';
import type { PricingChannel, PricingCredentialFixedInput, PricingChannelModel, PricingStyle } from '@/lib/types';
import styles from './PricingCredentialDefaultsCard.module.scss';

const rateFields = ['prompt_price_per_1m', 'completion_price_per_1m', 'cache_read_price_per_1m', 'cache_write_price_per_1m'] as const;
const emptyRates = () => ({ prompt_price_per_1m: '', completion_price_per_1m: '', cache_read_price_per_1m: '', cache_write_price_per_1m: '' });
const decimalText = (value: number) => value.toLocaleString('en-US', { useGrouping: false, maximumSignificantDigits: 21 });

export function PricingChannelModelsCard({ canManage = true }: { canManage?: boolean }) {
  const { t } = useTranslation();
  const [channels, setChannels] = useState<PricingChannel[]>([]);
  const [models, setModels] = useState<string[]>([]);
  const [exceptions, setExceptions] = useState<PricingChannelModel[]>([]);
  const [selected, setSelected] = useState('');
  const [model, setModel] = useState('');
  const [config, setConfig] = useState<PricingChannelModel | null>(null);
  const [multiplier, setMultiplier] = useState('');
  const [mode, setMode] = useState<'multiplier' | 'fixed'>('multiplier');
  const [rates, setRates] = useState(emptyRates);
  const [pricingStyle, setPricingStyle] = useState<PricingStyle | ''>('');
  const [baselineModels, setBaselineModels] = useState<string[]>([]);
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
  const resetDraft = useCallback(() => {
    setMultiplier(''); setMode('multiplier'); setRates(emptyRates()); setPricingStyle('');
  }, []);
  const applyConfig = (saved: PricingChannelModel) => {
    setConfig(saved);
    setMode(saved.mode === 'fixed' ? 'fixed' : 'multiplier');
    setMultiplier(saved.multiplier === null ? '' : decimalText(saved.multiplier));
    setRates(saved.fixed ? {
      prompt_price_per_1m: decimalText(saved.fixed.prompt_price_per_1m),
      completion_price_per_1m: decimalText(saved.fixed.completion_price_per_1m),
      cache_read_price_per_1m: decimalText(saved.fixed.cache_read_price_per_1m),
      cache_write_price_per_1m: decimalText(saved.fixed.cache_write_price_per_1m),
    } : emptyRates());
    setPricingStyle(saved.fixed?.pricing_style ?? '');
    setExceptions(previous => [
      ...previous.filter(item => item.model !== saved.model),
      ...(saved.mode === 'inherit' ? [] : [saved]),
    ]);
  };
  const handleError = useCallback((cause: unknown, fallback: string, inputMode: 'multiplier' | 'fixed' = 'multiplier') => {
    // Never display upstream bodies: only localized safe errors are allowed.
    if (cause instanceof ApiError && (cause.status === 401 || cause.status === 403)) {
      setDenied(true); setChannels([]); setModels([]); setBaselineModels([]); setExceptions([]);
      setSelected(''); setModel(''); setConfig(null); resetDraft(); setNotice(''); setFieldError('');
      setError('pricing_channel_models.permission_denied');
    } else if (fallback === 'pricing_channel_models.save_failed' && cause instanceof ApiError && (cause.status === 400 || cause.status === 422)) {
      setFieldError(inputMode === 'fixed' ? 'pricing_channel_models.invalid_fixed' : 'pricing_channel_models.invalid_multiplier');
    } else {
      setError(cause instanceof ApiError && cause.status === 409 ? 'pricing_channel_models.conflict' : fallback);
    }
  }, [resetDraft]);
  const loadChoices = async (signal: AbortSignal) => {
    const [directory, used, pricing] = await Promise.all([
      fetchPricingChannels(signal), fetchUsedModels(signal), fetchPricing(signal),
    ]);
    if (signal.aborted) return null;
    const saved = directory.channels.filter(item => item.id);
    setChannels(saved);
    setModels([...new Set([...used.models, ...pricing.pricing.map(item => item.model)])].sort());
    setBaselineModels(pricing.pricing.map(item => item.model));
    return saved;
  };

  useEffect(() => {
    const controller = new AbortController();
    requestRef.current = controller;
    setChannels([]); setModels([]); setBaselineModels([]); setExceptions([]); setSelected(''); setModel('');
    setConfig(null); resetDraft(); setError(''); setFieldError(''); setNotice(''); setDenied(false); setBusy(canManage);
    if (canManage) {
      void loadChoices(controller.signal).catch(cause => {
        if (!controller.signal.aborted) handleError(cause, 'pricing_channel_models.load_failed');
      }).finally(() => { if (!controller.signal.aborted) setBusy(false); });
    }
    return () => requestRef.current?.abort();
  }, [canManage, handleError, resetDraft]);

  const chooseChannel = async (channelId: string) => {
    if (busy) return;
    const controller = beginRequest();
    setSelected(channelId); setModel(''); setConfig(null); resetDraft(); setExceptions([]);
    try {
      const saved = await fetchPricingChannelModels(channelId, controller.signal);
      if (!controller.signal.aborted) setExceptions(saved.models);
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, 'pricing_channel_models.load_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const chooseModel = async (modelName: string) => {
    if (busy || !selected) return;
    const controller = beginRequest();
    setModel(modelName); setConfig(null); resetDraft();
    try {
      const saved = await fetchPricingChannelModel(selected, modelName, controller.signal);
      if (!controller.signal.aborted) applyConfig(saved);
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, 'pricing_channel_models.load_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const refresh = async () => {
    if (busy) return;
    const controller = beginRequest();
    try {
      const savedChannels = await loadChoices(controller.signal);
      if (!savedChannels) return;
      if (selected && savedChannels.some(item => item.id === selected)) {
        const saved = await fetchPricingChannelModels(selected, controller.signal);
        if (controller.signal.aborted) return;
        setExceptions(saved.models);
        if (model) {
          const readback = await fetchPricingChannelModel(selected, model, controller.signal);
          if (!controller.signal.aborted) applyConfig(readback);
        }
      } else {
        setSelected(''); setModel(''); setConfig(null); resetDraft(); setExceptions([]);
      }
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, 'pricing_channel_models.load_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const save = async (clear = false) => {
    if (busy || !selected || !model || !config || (clear && config.mode === 'inherit')) return;
    const text = multiplier.trim();
    const target: PricingCredentialFixedInput = {
      prompt_price_per_1m: rates.prompt_price_per_1m.trim(),
      completion_price_per_1m: rates.completion_price_per_1m.trim(),
      cache_read_price_per_1m: rates.cache_read_price_per_1m.trim(),
      cache_write_price_per_1m: rates.cache_write_price_per_1m.trim(),
      ...(pricingStyle ? { pricing_style: pricingStyle } : {}),
    };
    if (!clear) {
      let invalid = '';
      if (mode === 'fixed') {
        if (rateFields.some(field => !/^(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)$/.test(target[field]) || !Number.isFinite(Number(target[field])))) {
          invalid = 'pricing_channel_models.invalid_fixed';
        } else if (!pricingStyle && !baselineModels.includes(model)) {
          invalid = 'pricing_channel_models.style_required';
        }
      } else {
        const match = /^(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)([xX%])?$/.exec(text);
        const value = match ? Number(match[1] ? text.slice(0, -1) : text) / (match[1] === '%' ? 100 : 1) : NaN;
        if (!Number.isFinite(value) || value < 0) invalid = 'pricing_channel_models.invalid_multiplier';
      }
      if (invalid) { setError(''); setNotice(''); setFieldError(invalid); return; }
    }
    const controller = beginRequest();
    try {
      // Send only the complete target mode; hidden drafts never affect the save.
      const saved = await (clear ? clearPricingChannelModel(selected, model, controller.signal)
        : mode === 'fixed' ? savePricingChannelFixed(selected, model, target, controller.signal)
          : savePricingChannelModel(selected, model, text, controller.signal));
      if (controller.signal.aborted) return;
      // Retain the committed canonical result even if readback fails.
      applyConfig(saved);
      setNotice(clear ? 'pricing_channel_models.cleared' : 'pricing_channel_models.saved');
      try {
        const readback = await fetchPricingChannelModel(selected, model, controller.signal);
        if (!controller.signal.aborted) applyConfig(readback);
      } catch (cause) {
        if (!controller.signal.aborted) handleError(cause, 'pricing_channel_models.load_failed');
      }
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, clear ? 'pricing_channel_models.clear_failed' : 'pricing_channel_models.save_failed', mode);
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const label = (item: PricingChannel) => `${item.name} · ${item.id}`;
  const choice = channels.find(item => item.id === selected);
  const modelNames = [...new Set([...models, ...exceptions.map(item => item.model), ...(model ? [model] : [])])].sort();

  if (!canManage) return null;
  return (
    <Card title={t('pricing_channel_models.title')} subtitle={t('pricing_channel_models.scope_help')}>
      <div className={styles.body}>
        <p className={styles.hint}>{t('pricing_channel_models.history_warning')}</p>
        <p className={styles.hint}>{t('pricing_channel_models.replacement_warning')}</p>
        {error && <div className={styles.error} role="alert">{t(error)}</div>}
        {notice && <div role="status">{t(notice)}</div>}
        {!denied && <>
          <div className={styles.actions}>
            <div className={styles.selector}>
              <Select value={selected} onChange={value => void chooseChannel(value)} disabled={busy}
                ariaLabel={t('pricing_channel_models.select')} placeholder={t('pricing_channel_models.select')}
                options={channels.map(item => ({ value: item.id, label: label(item) }))} dropdownClassName={styles.options}
                search={{ placeholder: t('pricing_channel_models.channel_search'), noResultsText: t('pricing_channel_models.empty') }} />
            </div>
            <Button variant="secondary" onClick={() => void refresh()} disabled={busy}>{t('pricing_channel_models.refresh')}</Button>
          </div>
          {choice && <>
            <p className={styles.hint}>{label(choice)}</p>
            <p className={styles.hint}>{t('pricing_channel_models.members_help', { count: choice.member_subject_ids.length })}</p>
          </>}
          <div className={styles.actions}><div className={styles.selector}>
            <Select value={model} onChange={value => void chooseModel(value)} disabled={busy || !selected}
              ariaLabel={t('pricing_channel_models.model')} placeholder={t('pricing_channel_models.model')}
              options={modelNames.map(item => ({ value: item, label: item }))} dropdownClassName={styles.options}
              search={{ placeholder: t('pricing_channel_models.model_search'), noResultsText: t('pricing_channel_models.no_models') }} />
          </div></div>
          <p className={styles.hint}>{t('pricing_channel_models.model_help')}</p>
          {busy && <div role="status">{t('common.loading')}</div>}
          {!busy && channels.length === 0 && <p className={styles.hint}>{t('pricing_channel_models.empty')}</p>}
          {config && <div className={styles.readback}>
            <span>{t('pricing_channel_models.current')}</span><span>{config.model}</span>
            {config.mode === 'inherit' ? <span>{t('pricing_channel_models.inherited')}</span> : <>
              <span>{t('pricing_channel_models.active')}</span>
              {config.mode === 'fixed' && config.fixed ? <>
                <span>{t('pricing_channel_models.fixed')}</span>
                {rateFields.map(field => <span key={field}>
                  {t(`pricing_channel_models.${field}`)}: <output aria-label={t(`pricing_channel_models.${field}`)}>{String(config.fixed![field])}</output>
                </span>)}
                <span>{t('pricing_channel_models.style')}: {config.fixed.pricing_style || t('pricing_channel_models.style_inherit')}</span>
              </> : <output aria-label={t('pricing_channel_models.canonical')}>{String(config.multiplier)}</output>}
            </>}
          </div>}
          <div className={styles.actions}><div className={styles.selector}>
            <Select value={mode} onChange={value => { if (value === 'multiplier' || value === 'fixed') { setMode(value); setFieldError(''); setNotice(''); } }}
              disabled={busy || !config} ariaLabel={t('pricing_channel_models.mode')}
              options={['multiplier', 'fixed'].map(value => ({ value, label: t(`pricing_channel_models.${value}`) }))} />
          </div></div>
          {mode === 'multiplier' ? <Input type="text" label={t('pricing_channel_models.multiplier')} aria-label={t('pricing_channel_models.multiplier')}
            value={multiplier} onChange={event => { setMultiplier(event.target.value); setFieldError(''); }} disabled={busy || !config}
            placeholder="1.2x" hint={t('pricing_channel_models.input_help')} error={fieldError ? t(fieldError) : undefined} /> : <>
            <p className={styles.hint}>{t('pricing_channel_models.fixed_help')}</p>
            {fieldError && <div className={styles.error} role="alert">{t(fieldError)}</div>}
            <div className={styles.fixedRates}>
              {rateFields.map(field => <Input key={field} type="text" inputMode="decimal"
                label={t(`pricing_channel_models.${field}`)} aria-label={t(`pricing_channel_models.${field}`)}
                value={rates[field]} disabled={busy || !config}
                onChange={event => { setRates(previous => ({ ...previous, [field]: event.target.value })); setFieldError(''); }} />)}
            </div>
            <div className={styles.actions}><div className={styles.selector}>
              <Select value={pricingStyle} onChange={value => { if (value === '' || value === 'openai' || value === 'claude') { setPricingStyle(value); setFieldError(''); } }}
                disabled={busy || !config} ariaLabel={t('pricing_channel_models.style')} placeholder={t('pricing_channel_models.style_inherit')}
                options={[{ value: '', label: t('pricing_channel_models.style_inherit') }, { value: 'openai', label: 'OpenAI' }, { value: 'claude', label: 'Claude' }]} />
            </div></div>
            <p className={styles.hint}>{t('pricing_channel_models.style_help')}</p>
          </>}
          <div className={styles.actions}>
            <Button onClick={() => void save()} disabled={busy || !config}>{t('pricing_channel_models.save')}</Button>
            <Button variant="secondary" onClick={() => void save(true)} disabled={busy || !config || config.mode === 'inherit'}>{t('pricing_channel_models.clear')}</Button>
          </div>
        </>}
      </div>
    </Card>
  );
}
