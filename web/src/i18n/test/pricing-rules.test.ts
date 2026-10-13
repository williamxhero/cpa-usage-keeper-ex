import { describe, expect, it } from 'vitest'
import i18n, { SUPPORTED_LANGUAGES } from '../index'

describe('dual cost estimate translations', () => {
  it('uses exact Chinese labels and complete independent status, explanation, and reason copy without fallback', () => {
    const flatten = (value: Record<string, unknown>, prefix = ''): string[] => Object.entries(value).flatMap(([key, entry]) => typeof entry === 'string' ? [`${prefix}${key}`] : flatten(entry as Record<string, unknown>, `${prefix}${key}.`));
    const keys = flatten(i18n.getResourceBundle('en', 'translation').cost_estimates);
    expect(i18n.t('cost_estimates.configured', { lng: 'zh' })).toBe('配置价估算费用');
    expect(i18n.t('cost_estimates.reference', { lng: 'zh' })).toBe('基准参考费用');
    expect(i18n.t('cost_estimates.configured', { lng: 'zh-TW' })).toBe('配置價估算費用');
    expect(i18n.t('cost_estimates.reference', { lng: 'zh-TW' })).toBe('基準參考費用');
    for (const language of SUPPORTED_LANGUAGES) {
      expect(flatten(i18n.getResourceBundle(language, 'translation').cost_estimates).sort()).toEqual([...keys].sort());
      for (const key of keys) {
        const path = `cost_estimates.${key}`;
        expect(i18n.exists(path, { lng: language, fallbackLng: false })).toBe(true);
        expect(i18n.t(path, { lng: language, fallbackLng: false }).trim()).not.toBe('');
        expect(i18n.t(path, { lng: language, fallbackLng: false })).not.toBe(path);
      }
    }
  });
});
