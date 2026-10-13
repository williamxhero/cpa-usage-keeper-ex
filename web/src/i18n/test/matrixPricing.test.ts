import { describe, expect, it } from 'vitest';
import i18n, { SUPPORTED_LANGUAGES } from '../index';

type Messages = { [key: string]: string | Messages };

const entries = (messages: Messages, prefix = ''): [string, string][] => (
  Object.entries(messages).flatMap(([key, value]) => {
    const path = prefix ? `${prefix}.${key}` : key;
    return typeof value === 'string' ? [[path, value]] : entries(value, path);
  })
);
const placeholders = (message: string) => (message.match(/\{\{[^{}]+\}\}/g) ?? []).sort();

const english = Object.fromEntries(entries(i18n.getResourceBundle('en', 'matrix_pricing')));

describe('matrix_pricing translations', () => {
  it.each(SUPPORTED_LANGUAGES)('provides complete copy without fallback in %s', (language) => {
    const translated = Object.fromEntries(entries(i18n.getResourceBundle(language, 'matrix_pricing')));
    expect(Object.keys(translated).sort()).toEqual(Object.keys(english).sort());
    for (const [key, message] of Object.entries(translated)) {
      const path = `${language}:matrix_pricing.${key}`;
      expect(message.trim(), path).not.toBe('');
      expect(placeholders(message), path).toEqual(placeholders(english[key]));
      expect(i18n.exists(key, { lng: language, ns: 'matrix_pricing', fallbackLng: false }), path).toBe(true);
      expect(i18n.t(key, { lng: language, ns: 'matrix_pricing', fallbackLng: false }), path).not.toBe(key);
    }
  });

  it('covers every supported dynamic source, reason, credential status, and sync state', () => {
    const dynamicKeys = [
      'official_source_official', 'official_source_openrouter', 'official_source_legacy_seed',
      'official_source_models_dev', 'official_source_litellm', 'official_source_unknown',
      'official_available', 'official_stale', 'official_missing', 'official_ambiguous',
      'official_unavailable', 'official_no_match', 'official_ambiguous_match', 'official_unknown_reason',
      'status_active', 'status_disabled', 'status_stale', 'status_unknown',
      'sync_state_idle', 'sync_state_queued', 'sync_state_running', 'sync_state_succeeded',
      'sync_state_failed', 'sync_state_unavailable',
    ];
    for (const language of SUPPORTED_LANGUAGES) {
      for (const key of dynamicKeys) {
        expect(i18n.t(key, { lng: language, ns: 'matrix_pricing', fallbackLng: false }), `${language}:${key}`).not.toBe(key);
      }
    }
  });

  it('resolves the title through the separate namespace', () => {
    expect(i18n.t('title', { lng: 'en', ns: 'matrix_pricing', fallbackLng: false })).toBe('Pricing matrix');
    expect(i18n.t('title', { lng: 'zh', ns: 'matrix_pricing', fallbackLng: false })).toBe('价格矩阵');
    expect(i18n.t('title', { lng: 'zh-TW', ns: 'matrix_pricing', fallbackLng: false })).toBe('價格矩陣');
  });

  it('keeps missing segments, unknown metadata, and historical recalculation explicit', () => {
    expect(english.official_missing_price).toBe('Not provided');
    expect(english.official_unknown_reason).toBe('Availability reason not provided');
    expect(english.official_source_unknown).toBe('Unknown source');
    expect(english.effective_summary_unavailable).toBe('Effective prices are not provided in this summary.');
    expect(english.history_warning).toContain('recalculate historical estimates');
    expect(english.official_read_limit).toContain('administrators only');
    expect(english.official_readonly).toContain('administrator');
  });
});
