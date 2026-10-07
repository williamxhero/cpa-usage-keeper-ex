import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Select } from '@/components/ui/Select';
import { Button } from '@/components/ui/Button';
import { Modal } from '@/components/ui/Modal';
import { ApiError, correctPricingIdentity, fetchPricingIdentityState, migratePricingIdentity } from '@/lib/api';
import type { PricingCredential, PricingIdentityBinding, PricingIdentityMutationResult, PricingIdentityState } from '@/lib/types';
import styles from './PricingIdentityMigrationCard.module.scss';

type Confirmation = { action: 'migrate' | 'unbind' | 'rebind'; snapshotId: string; subjectId: string; targetId: string; ref: string; label: string };

export function PricingIdentityMigrationCard({ canManage = true, onChanged }: { canManage?: boolean; onChanged?: () => void }) {
  const { t } = useTranslation();
  const [state, setState] = useState<PricingIdentityState | null>(null);
  const [subject, setSubject] = useState('');
  const [directory, setDirectory] = useState('');
  const [binding, setBinding] = useState('');
  const [target, setTarget] = useState('');
  const [pending, setPending] = useState<Confirmation | null>(null);
  const [saved, setSaved] = useState<PricingIdentityMutationResult | null>(null);
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
  const resetSelection = useCallback(() => {
    setSubject(''); setDirectory(''); setBinding(''); setTarget(''); setPending(null);
  }, []);
  const handleError = useCallback((cause: unknown, fallback: string) => {
    // Display only localized, allowlisted errors, never response bodies.
    if (cause instanceof ApiError && (cause.status === 401 || cause.status === 403)) {
      setDenied(true); setState(null); setSaved(null); setNotice(''); resetSelection();
      setError('pricing_identity_migration.permission_denied');
    } else {
      setError(cause instanceof ApiError && cause.status === 409 ? 'pricing_identity_migration.conflict' : fallback);
    }
  }, [resetSelection]);
  useEffect(() => {
    const controller = new AbortController(); requestRef.current = controller;
    setState(null); setSaved(null); resetSelection(); setDenied(false); setError(''); setNotice(''); setBusy(canManage);
    if (canManage) {
      void fetchPricingIdentityState(controller.signal).then(result => {
        if (!controller.signal.aborted) setState(result);
      }).catch(cause => {
        if (!controller.signal.aborted) handleError(cause, 'pricing_identity_migration.load_failed');
      }).finally(() => { if (!controller.signal.aborted) setBusy(false); });
    }
    return () => requestRef.current?.abort();
  }, [canManage, handleError, resetSelection]);
  const begin = () => {
    requestRef.current?.abort();
    const controller = new AbortController(); requestRef.current = controller;
    setBusy(true); setError(''); setNotice(''); return controller;
  };
  const refresh = async () => {
    if (busy) return;
    const controller = begin(); resetSelection(); setState(null);
    try {
      const result = await fetchPricingIdentityState(controller.signal);
      if (!controller.signal.aborted) setState(result);
    } catch (cause) {
      if (!controller.signal.aborted) handleError(cause, 'pricing_identity_migration.load_failed');
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const label = (item: PricingCredential) => [item.alias, item.name, item.provider_type, item.auth_type, item.endpoint,
    t(`pricing_credentials.status.${item.status}`), t(`pricing_credentials.binding.${item.binding_status}`), item.subject_id].filter(Boolean).join(' · ');
  const bindingLabel = (item: PricingIdentityBinding) => `${label(item.credential)} · ${item.ref}`;
  const selectedDirectory = state?.directory.find(item => item.ref === directory);
  const selectedBinding = state?.bindings.find(item => item.ref === binding);
  const migrateSelectable = !!selectedDirectory && selectedDirectory.credential.status !== 'stale' && selectedDirectory.credential.binding_status === 'unbound';
  const rebindSelectable = !!selectedBinding && !!target && (!selectedBinding.enabled || target !== selectedBinding.subject_id) && !['unknown', 'ambiguous'].includes(selectedBinding.credential.binding_status);
  const confirmMigration = () => {
    if (busy || !state || !subject || !migrateSelectable || !selectedDirectory) return;
    setPending({ action: 'migrate', snapshotId: state.snapshot_id, subjectId: subject, targetId: subject, ref: directory, label: label(selectedDirectory.credential) });
  };
  const confirmCorrection = (action: 'unbind' | 'rebind') => {
    if (busy || !state || !selectedBinding || (action === 'unbind' ? !selectedBinding.enabled : !rebindSelectable)) return;
    setPending({ action, snapshotId: state.snapshot_id, subjectId: selectedBinding.subject_id, targetId: action === 'rebind' ? target : '', ref: binding, label: bindingLabel(selectedBinding) });
  };
  const submit = async () => {
    if (busy || !pending) return;
    const choice = pending;
    const controller = begin();
    try {
      const result = choice.action === 'migrate'
        ? await migratePricingIdentity({ subject_id: choice.subjectId, directory_ref: choice.ref, snapshot_id: choice.snapshotId, confirmed: true }, controller.signal)
        : await correctPricingIdentity(choice.ref, { expected_subject_id: choice.subjectId, target_subject_id: choice.targetId || undefined, action: choice.action, snapshot_id: choice.snapshotId, confirmed: true }, controller.signal);
      if (controller.signal.aborted) return;
      // Preserve the actual committed receipt even if canonical GET fails. Old
      // snapshot-scoped selections must never remain actionable after a write.
      setSaved(result); resetSelection(); setState(null); setNotice('pricing_identity_migration.saved');
      notifyChanged();
      try {
        const readback = await fetchPricingIdentityState(controller.signal);
        if (!controller.signal.aborted) setState(readback);
      } catch (cause) {
        if (!controller.signal.aborted) handleError(cause, 'pricing_identity_migration.readback_failed');
      }
    } catch (cause) {
      if (controller.signal.aborted) return;
      resetSelection();
      handleError(cause, 'pricing_identity_migration.save_failed');
      if (cause instanceof ApiError && cause.status === 409) {
        setState(null);
        try {
          const result = await fetchPricingIdentityState(controller.signal);
          if (!controller.signal.aborted) setState(result);
        } catch (refreshError) {
          if (!controller.signal.aborted) handleError(refreshError, 'pricing_identity_migration.load_failed');
        }
      }
    } finally { if (!controller.signal.aborted) setBusy(false); }
  };
  if (!canManage) return null;
  const subjectOptions = state?.subjects.filter(item => item.subject_id).map(item => ({ value: item.subject_id!, label: label(item) })) ?? [];
  return <Card title={t('pricing_identity_migration.title')} subtitle={t('pricing_identity_migration.scope_help')}>
    <div className={styles.body}>
      {error && <div role="alert" className={styles.error}>{t(error)}</div>}
      {notice && <div role="status">{t(notice)}</div>}
      {!denied && <>
        <div className={styles.actions}><Button variant="secondary" disabled={busy} onClick={() => void refresh()}>{t('pricing_identity_migration.refresh')}</Button></div>
        {busy && <div role="status">{t('common.loading')}</div>}
        {saved && <div className={styles.readback}><span>{t('pricing_identity_migration.receipt')}</span><output aria-label={t('pricing_identity_migration.receipt')}>{saved.binding_ref} · {saved.subject_id} · {t(saved.enabled ? 'pricing_credentials.binding.bound' : 'pricing_credentials.binding.unbound')}</output></div>}
        <section className={styles.body} aria-label={t('pricing_identity_migration.migrate_title')}>
          <h3>{t('pricing_identity_migration.migrate_title')}</h3>
          <p className={styles.hint}>{t('pricing_identity_migration.migrate_help')}</p>
          <div className={styles.selector}><Select value={subject} disabled={busy || !state} onChange={setSubject} ariaLabel={t('pricing_identity_migration.subject')} placeholder={t('pricing_identity_migration.subject')}
            options={subjectOptions} dropdownClassName={styles.options} search={{ placeholder: t('pricing_credentials.search'), noResultsText: t('pricing_identity_migration.no_subjects') }} /></div>
          <div className={styles.selector}><Select value={directory} disabled={busy || !state} onChange={setDirectory} ariaLabel={t('pricing_identity_migration.directory')} placeholder={t('pricing_identity_migration.directory')}
            options={state?.directory.map(item => ({ value: item.ref, label: label(item.credential), disabled: item.credential.status === 'stale' || item.credential.binding_status !== 'unbound' })) ?? []}
            dropdownClassName={styles.options} search={{ placeholder: t('pricing_credentials.search'), noResultsText: t('pricing_identity_migration.no_directory') }} /></div>
          <div className={styles.actions}><Button disabled={busy || !subject || !migrateSelectable} onClick={confirmMigration}>{t('pricing_identity_migration.migrate')}</Button></div>
        </section>
        <section className={styles.body} aria-label={t('pricing_identity_migration.correction_title')}>
          <h3>{t('pricing_identity_migration.correction_title')}</h3>
          <p className={styles.hint}>{t('pricing_identity_migration.correction_help')}</p>
          <div className={styles.selector}><Select value={binding} disabled={busy || !state} onChange={value => { setBinding(value); setTarget(''); }} ariaLabel={t('pricing_identity_migration.binding')} placeholder={t('pricing_identity_migration.binding')}
            options={state?.bindings.map(item => ({ value: item.ref, label: bindingLabel(item) })) ?? []} dropdownClassName={styles.options}
            search={{ placeholder: t('pricing_credentials.search'), noResultsText: t('pricing_identity_migration.no_bindings') }} /></div>
          {selectedBinding && <p className={styles.hint}>{bindingLabel(selectedBinding)}</p>}
          <div className={styles.selector}><Select value={target} disabled={busy || !selectedBinding} onChange={setTarget} ariaLabel={t('pricing_identity_migration.target')} placeholder={t('pricing_identity_migration.target')}
            options={subjectOptions.map(item => ({ ...item, disabled: !!selectedBinding?.enabled && item.value === selectedBinding.subject_id }))} dropdownClassName={styles.options}
            search={{ placeholder: t('pricing_credentials.search'), noResultsText: t('pricing_identity_migration.no_subjects') }} /></div>
          <div className={styles.actions}>
            <Button variant="secondary" disabled={busy || !selectedBinding?.enabled} onClick={() => confirmCorrection('unbind')}>{t('pricing_identity_migration.unbind')}</Button>
            <Button variant="secondary" disabled={busy || !rebindSelectable} onClick={() => confirmCorrection('rebind')}>{t('pricing_identity_migration.rebind')}</Button>
          </div>
        </section>
        <Modal open={!!pending} title={t(pending?.action === 'migrate' ? 'pricing_identity_migration.migrate_confirm' : 'pricing_identity_migration.correction_confirm')} onClose={() => { if (!busy) setPending(null); }} closeDisabled={busy}
          footer={<><Button variant="secondary" disabled={busy} onClick={() => setPending(null)}>{t('common.cancel')}</Button><Button disabled={busy || !pending} onClick={() => void submit()}>{t('pricing_identity_migration.confirm')}</Button></>}>
          {pending && <div className={styles.body}><p>{pending.label}</p><p>{t('pricing_identity_migration.owner')} {pending.subjectId}</p>
            {pending.action === 'rebind' && pending.targetId && <p>{t('pricing_identity_migration.target')} {pending.targetId}</p>}
            <p>{t(pending.action === 'migrate' ? 'pricing_identity_migration.migrate_warning' : 'pricing_identity_migration.correction_warning')}</p>
            <p>{t(`pricing_identity_migration.${pending.action}_detail`)}</p>
          </div>}
        </Modal>
      </>}
    </div>
  </Card>;
}
