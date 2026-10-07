import { describe, expect, it } from 'vitest'
import i18n, { SUPPORTED_LANGUAGES } from '../index'

describe('credential model exception translations', () => {
  it('has complete nonempty exact-match, inheritance, baseline and history copy in every language', () => {
    const keys = ['title', 'scope_help', 'history_warning', 'replacement_warning', 'select', 'empty', 'refresh', 'model', 'model_search', 'no_models', 'model_help', 'current', 'inherited', 'active', 'canonical', 'multiplier', 'input_help', 'save', 'clear', 'saved', 'cleared', 'invalid_multiplier', 'load_failed', 'save_failed', 'clear_failed', 'permission_denied', 'conflict']
    for (const language of SUPPORTED_LANGUAGES) {
      const bundle = i18n.getResourceBundle(language, 'translation').pricing_credential_models
      expect(Object.keys(bundle).sort()).toEqual([...keys].sort())
      for (const key of keys) {
        const path = `pricing_credential_models.${key}`
        expect(i18n.exists(path, { lng: language, fallbackLng: false }), `${language}: ${path}`).toBe(true)
        expect(i18n.t(path, { lng: language, fallbackLng: false }).trim()).not.toBe('')
        expect(i18n.t(path, { lng: language, fallbackLng: false })).not.toBe(path)
      }
      expect(bundle.model_help).toContain('ModelAlias')
      expect(bundle.input_help).toContain('0')
      expect(bundle.replacement_warning).toContain('reasoning')
    }
  })
})

describe('credential default multiplier translations', () => {
  it('has complete nonempty copy without fallback in every language', () => {
    const keys = ['title', 'scope_help', 'history_warning', 'replacement_warning', 'select', 'empty', 'refresh', 'current', 'inherited', 'active', 'canonical', 'multiplier', 'input_help', 'save', 'clear', 'saved', 'cleared', 'invalid_multiplier', 'load_failed', 'save_failed', 'clear_failed', 'permission_denied', 'conflict']
    for (const language of SUPPORTED_LANGUAGES) {
      const bundle = i18n.getResourceBundle(language, 'translation').pricing_credential_defaults
      expect(Object.keys(bundle).sort()).toEqual([...keys].sort())
      for (const key of keys) {
        const path = `pricing_credential_defaults.${key}`
        expect(i18n.exists(path, { lng: language, fallbackLng: false }), `${language}: ${path}`).toBe(true)
        expect(i18n.t(path, { lng: language, fallbackLng: false }).trim()).not.toBe('')
        expect(i18n.t(path, { lng: language, fallbackLng: false })).not.toBe(path)
      }
    }
  })
})

describe('pricing credential translations', () => {
  it('has complete selector, save, error, identity and status copy in every language', () => {
    const keys = ['title', 'no_pricing_change', 'identity_help', 'select', 'search', 'empty', 'register', 'refresh', 'reference', 'subjects', 'no_subjects', 'saved', 'load_failed', 'save_failed', 'permission_denied', 'conflict', ...['active', 'disabled', 'stale', 'unknown'].map(key => `status.${key}`), ...['unbound', 'bound', 'stale', 'unknown', 'ambiguous'].map(key => `binding.${key}`)]
    for (const language of SUPPORTED_LANGUAGES) {
      for (const key of keys) {
        const path = `pricing_credentials.${key}`
        expect(i18n.exists(path, { lng: language, fallbackLng: false }), `${language}: ${path}`).toBe(true)
        expect(i18n.t(path, { lng: language })).not.toBe(path)
      }
    }
  })
})

describe('named pricing channel translations', () => {
  it('provides all management, priority, history and dependency copy in every language', () => {
    const keys = ['title', 'scope_help', 'history_warning', 'priority_help', 'select', 'new', 'refresh', 'name', 'members_help', 'member_select', 'no_subjects', 'add_member', 'members', 'remove_member', 'no_members', 'create', 'save_channel', 'delete', 'delete_title', 'delete_help', 'confirm_delete', 'current', 'inherited', 'active', 'canonical', 'multiplier', 'input_help', 'save_default', 'clear', 'saved', 'deleted', 'invalid_name', 'invalid_multiplier', 'load_failed', 'save_failed', 'save_default_failed', 'clear_failed', 'delete_failed', 'permission_denied', 'conflict']
    for (const language of SUPPORTED_LANGUAGES) {
      const bundle = i18n.getResourceBundle(language, 'translation').pricing_channels
      expect(Object.keys(bundle).sort()).toEqual([...keys].sort())
      for (const key of keys) {
        const path = `pricing_channels.${key}`
        expect(i18n.exists(path, { lng: language, fallbackLng: false }), `${language}: ${path}`).toBe(true)
        expect(i18n.t(path, { lng: language, fallbackLng: false }).trim()).not.toBe('')
        expect(i18n.t(path, { lng: language, fallbackLng: false })).not.toBe(path)
      }
      expect(bundle.delete_help).toContain('{{count}}')
      expect(bundle.delete_help).toContain('{{multiplier}}')
    }
  })
})

describe('pricing rule translations', () => {
  it('keeps the help copy limited to the two approved examples', () => {
    for (const language of SUPPORTED_LANGUAGES) {
      const usageStats = i18n.getResourceBundle(language, 'translation').usage_stats
      const help = [
        usageStats.model_price_rules_help,
        usageStats.model_price_rules_help_service_tier,
        usageStats.model_price_rules_help_reasoning_effort,
      ].join(' ')
      expect(help).toContain('service_tier')
      expect(help).toContain('priority')
      expect(help).toContain('reasoning_effort')
      expect(help).toContain('xhigh')
      expect(help).not.toContain('api_group_key')
      expect(help).not.toContain('response_service_tier')
      expect(help).not.toContain('executor_type')
    }
  })

  it('interpolates the selected count in every language', () => {
    expect(i18n.t('usage_stats.model_price_sync_update_selected', { lng: 'en', count: 3 })).toBe('Update Selected (3)')
    expect(i18n.t('usage_stats.model_price_sync_update_selected', { lng: 'zh', count: 3 })).toBe('更新所选（3）')
    expect(i18n.t('usage_stats.model_price_sync_update_selected', { lng: 'zh-TW', count: 3 })).toBe('更新所選（3）')
  })
})
