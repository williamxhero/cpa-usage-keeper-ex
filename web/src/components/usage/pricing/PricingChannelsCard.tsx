import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Select } from '@/components/ui/Select';
import { Input } from '@/components/ui/Input';
import { Button } from '@/components/ui/Button';
import { Modal } from '@/components/ui/Modal';
import { ApiError, clearPricingChannelDefault, deletePricingChannel, fetchCredentialPricingSubjects, fetchPricingChannel, fetchPricingChannels, savePricingChannel, savePricingChannelDefault } from '@/lib/api';
import type { PricingChannel, PricingCredential } from '@/lib/types';
import styles from './PricingChannelsCard.module.scss';

export function PricingChannelsCard({ canManage = true }: { canManage?: boolean }) {
  const { t } = useTranslation();
  const [channels, setChannels] = useState<PricingChannel[]>([]);
  const [subjects, setSubjects] = useState<PricingCredential[]>([]);
  const [selected, setSelected] = useState('');
  const [config, setConfig] = useState<PricingChannel | null>(null);
  const [name, setName] = useState('');
  const [members, setMembers] = useState<string[]>([]);
  const [candidate, setCandidate] = useState('');
  const [multiplier, setMultiplier] = useState('');
  const [busy, setBusy] = useState(false);
  const [denied, setDenied] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [error, setError] = useState('');
  const [fieldError, setFieldError] = useState('');
  const [notice, setNotice] = useState('');
  const requestRef = useRef<AbortController | null>(null);
  const reset = useCallback(() => {
    setSelected(''); setConfig(null); setName(''); setMembers([]); setCandidate(''); setMultiplier(''); setConfirmDelete(false);
  }, []);
  const beginRequest = () => {
    requestRef.current?.abort();
    const controller = new AbortController();
    requestRef.current = controller;
    setBusy(true); setError(''); setFieldError(''); setNotice('');
    return controller;
  };
  const applyConfig = (saved: PricingChannel) => {
    setSelected(saved.id); setConfig(saved); setName(saved.name); setMembers(saved.member_subject_ids); setCandidate('');
    setMultiplier(saved.multiplier === null ? '' : saved.multiplier.toLocaleString('en-US', { useGrouping: false, maximumSignificantDigits: 21 }));
    setChannels(rows => [...rows.filter(item => item.id !== saved.id), saved]);
  };
  const handleError = useCallback((cause: unknown, fallback: string) => {
    // Never expose upstream error text or identity data outside the safe selector.
    if (cause instanceof ApiError && (cause.status === 401 || cause.status === 403)) {
      setDenied(true); setChannels([]); setSubjects([]); reset(); setNotice(''); setFieldError('');
      setError('pricing_channels.permission_denied');
    } else if (fallback === 'pricing_channels.save_default_failed' && cause instanceof ApiError && (cause.status === 400 || cause.status === 422)) {
      setFieldError('pricing_channels.invalid_multiplier');
    } else {
      setError(cause instanceof ApiError && cause.status === 409 ? 'pricing_channels.conflict' : fallback);
    }
  }, [reset]);
  useEffect(() => {
    const controller = new AbortController();
    requestRef.current = controller;
    reset(); setChannels([]); setSubjects([]); setDenied(false); setError(''); setNotice(''); setFieldError(''); setBusy(canManage);
    if (canManage) {
      void Promise.all([fetchPricingChannels(controller.signal), fetchCredentialPricingSubjects(controller.signal)]).then(([rows, directory]) => {
        if (!controller.signal.aborted) { setChannels(rows.channels); setSubjects(directory.credentials.filter(item => item.subject_id)); }
      }).catch(cause => {
        if (!controller.signal.aborted) handleError(cause, 'pricing_channels.load_failed');
      }).finally(() => { if (!controller.signal.aborted) setBusy(false); });
    }
    return () => requestRef.current?.abort();
  }, [canManage, reset, handleError]);

  const choose = async (id: string) => {
    if (busy) return;
    reset(); setError(''); setFieldError(''); setNotice('');
    if (!id) return;
    const controller = beginRequest();
    setSelected(id);
    try {
      const saved = await fetchPricingChannel(id, controller.signal);
      if (!controller.signal.aborted) applyConfig(saved);
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, 'pricing_channels.load_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const refresh = async () => {
    if (busy) return;
    const controller = beginRequest();
    try {
      const [rows, directory] = await Promise.all([fetchPricingChannels(controller.signal), fetchCredentialPricingSubjects(controller.signal)]);
      if (controller.signal.aborted) return;
      setChannels(rows.channels); setSubjects(directory.credentials.filter(item => item.subject_id));
      if (selected && rows.channels.some(item => item.id === selected)) {
        const saved = await fetchPricingChannel(selected, controller.signal);
        if (!controller.signal.aborted) applyConfig(saved);
      } else { reset(); }
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, 'pricing_channels.load_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const readback = async (saved: PricingChannel, controller: AbortController) => {
    // Preserve the committed canonical response if the subsequent GET fails.
    applyConfig(saved);
    setNotice('pricing_channels.saved');
    try {
      const result = await fetchPricingChannel(saved.id, controller.signal);
      if (!controller.signal.aborted) applyConfig(result);
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, 'pricing_channels.load_failed');
    }
  };
  const save = async () => {
    if (busy || (selected && !config)) return;
    if (!name.trim()) { setError('pricing_channels.invalid_name'); setNotice(''); return; }
    const controller = beginRequest();
    try {
      const saved = await savePricingChannel(selected || null, { name: name.trim(), member_subject_ids: members }, controller.signal);
      if (!controller.signal.aborted) await readback(saved, controller);
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, 'pricing_channels.save_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const saveDefault = async (clear = false) => {
    if (busy || !config || (clear && config.multiplier === null)) return;
    const text = multiplier.trim();
    const match = /^(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)([xX%])?$/.exec(text);
    const value = match ? Number(match[1] ? text.slice(0, -1) : text) / (match[1] === '%' ? 100 : 1) : NaN;
    if (!clear && (!Number.isFinite(value) || value < 0)) {
      setError(''); setNotice(''); setFieldError('pricing_channels.invalid_multiplier'); return;
    }
    const controller = beginRequest();
    try {
      const saved = await (clear ? clearPricingChannelDefault(config.id, controller.signal) : savePricingChannelDefault(config.id, text, controller.signal));
      if (!controller.signal.aborted) await readback(saved, controller);
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, clear ? 'pricing_channels.clear_failed' : 'pricing_channels.save_default_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const remove = async () => {
    if (busy || !config || !confirmDelete) return;
    const controller = beginRequest();
    try {
      await deletePricingChannel(config.id, true, controller.signal);
      if (controller.signal.aborted) return;
      setChannels(rows => rows.filter(item => item.id !== config.id)); reset(); setNotice('pricing_channels.deleted');
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, 'pricing_channels.delete_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const label = (item: PricingCredential) => [item.alias, item.name, item.provider_type, item.auth_type, item.endpoint,
    t(`pricing_credentials.status.${item.status}`), t(`pricing_credentials.binding.${item.binding_status}`), item.subject_id].filter(Boolean).join(' · ');
  const occupied = (id: string) => channels.some(channel => channel.id !== selected && channel.member_subject_ids.includes(id));
  const selectable = (item: PricingCredential) => !!item.subject_id && ['bound', 'stale'].includes(item.binding_status) && !occupied(item.subject_id) && !members.includes(item.subject_id);
  const candidateSubject = subjects.find(item => item.subject_id === candidate);

  if (!canManage) return null;
  return <Card title={t('pricing_channels.title')} subtitle={t('pricing_channels.scope_help')}>
    <div className={styles.body}>
      <p className={styles.hint}>{t('pricing_channels.history_warning')}</p>
      <p className={styles.hint}>{t('pricing_channels.priority_help')}</p>
      {error && <div className={styles.error} role="alert">{t(error)}</div>}
      {notice && <div role="status">{t(notice)}</div>}
      {!denied && <>
        <div className={styles.actions}>
          <div className={styles.selector}><Select value={selected || '__new__'} disabled={busy} onChange={id => void choose(id === '__new__' ? '' : id)}
            ariaLabel={t('pricing_channels.select')} dropdownClassName={styles.options}
            options={[{ value: '__new__', label: t('pricing_channels.new') }, ...channels.map(item => ({ value: item.id, label: `${item.name} · ${item.id}` }))]} /></div>
          <Button variant="secondary" onClick={() => void refresh()} disabled={busy}>{t('pricing_channels.refresh')}</Button>
        </div>
        {busy && <div role="status">{t('common.loading')}</div>}
        {config && <p className={styles.hint}>{config.id}</p>}
        <Input label={t('pricing_channels.name')} aria-label={t('pricing_channels.name')} value={name} maxLength={128} disabled={busy || (!!selected && !config)} onChange={event => setName(event.target.value)} />
        <p className={styles.hint}>{t('pricing_channels.members_help')}</p>
        <div className={styles.actions}>
          <div className={styles.selector}><Select value={candidate} disabled={busy || (!!selected && !config)} onChange={setCandidate}
            ariaLabel={t('pricing_channels.member_select')} placeholder={t('pricing_channels.member_select')} dropdownClassName={styles.options}
            options={subjects.map(item => ({ value: item.subject_id!, label: label(item), disabled: !selectable(item) }))}
            search={{ placeholder: t('pricing_credentials.search'), noResultsText: t('pricing_channels.no_subjects') }} /></div>
          <Button variant="secondary" disabled={busy || !candidateSubject || !selectable(candidateSubject)} onClick={() => { setMembers(ids => [...ids, candidate]); setCandidate(''); }}>{t('pricing_channels.add_member')}</Button>
        </div>
        <ul className={styles.members} aria-label={t('pricing_channels.members')}>
          {members.map(id => <li key={id}><span>{subjects.find(item => item.subject_id === id) ? label(subjects.find(item => item.subject_id === id)!) : id}</span>
            <Button variant="secondary" disabled={busy} aria-label={`${t('pricing_channels.remove_member')} ${id}`} onClick={() => setMembers(ids => ids.filter(member => member !== id))}>{t('pricing_channels.remove_member')}</Button></li>)}
        </ul>
        {members.length === 0 && <p className={styles.hint}>{t('pricing_channels.no_members')}</p>}
        <div className={styles.actions}>
          <Button onClick={() => void save()} disabled={busy || (!!selected && !config)}>{t(selected ? 'pricing_channels.save_channel' : 'pricing_channels.create')}</Button>
          {config && <Button variant="secondary" disabled={busy} onClick={() => setConfirmDelete(true)}>{t('pricing_channels.delete')}</Button>}
        </div>
        {config && <>
          <div className={styles.readback}><span>{t('pricing_channels.current')}</span>
            {config.multiplier === null ? <span>{t('pricing_channels.inherited')}</span> : <><span>{t('pricing_channels.active')}</span><output aria-label={t('pricing_channels.canonical')}>{String(config.multiplier)}</output></>}
          </div>
          <Input label={t('pricing_channels.multiplier')} aria-label={t('pricing_channels.multiplier')} value={multiplier} disabled={busy}
            onChange={event => { setMultiplier(event.target.value); setFieldError(''); }} hint={t('pricing_channels.input_help')} error={fieldError ? t(fieldError) : undefined} placeholder="0.2x" />
          <div className={styles.actions}><Button disabled={busy} onClick={() => void saveDefault()}>{t('pricing_channels.save_default')}</Button>
            <Button variant="secondary" disabled={busy || config.multiplier === null} onClick={() => void saveDefault(true)}>{t('pricing_channels.clear')}</Button></div>
        </>}
        <Modal open={confirmDelete && !!config} title={t('pricing_channels.delete_title')} onClose={() => setConfirmDelete(false)} closeDisabled={busy}
          footer={<><Button variant="secondary" disabled={busy} onClick={() => setConfirmDelete(false)}>{t('common.cancel')}</Button><Button disabled={busy} onClick={() => void remove()}>{t('pricing_channels.confirm_delete')}</Button></>}>
          <p className={styles.hint}>{t('pricing_channels.delete_help', { name: config?.name, count: config?.member_subject_ids.length ?? 0, multiplier: config?.multiplier === null ? t('pricing_channels.inherited') : String(config?.multiplier) })}</p>
          <p className={styles.hint}>{t('pricing_channels.history_warning')}</p>
        </Modal>
      </>}
    </div>
  </Card>;
}
