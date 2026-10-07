import { describe, expect, it } from 'vitest'
import i18n, { SUPPORTED_LANGUAGES } from '../index'

describe('credential model exception translations', () => {
  it('has complete nonempty exact-match, inheritance, baseline and history copy in every language', () => {
    const keys = ['title', 'scope_help', 'history_warning', 'replacement_warning', 'select', 'empty', 'refresh', 'model', 'model_search', 'no_models', 'model_help', 'current', 'inherited', 'active', 'canonical', 'multiplier', 'input_help', 'save', 'clear', 'saved', 'cleared', 'invalid_multiplier', 'load_failed', 'save_failed', 'clear_failed', 'permission_denied', 'conflict', 'mode', 'fixed', 'prompt_price_per_1m', 'completion_price_per_1m', 'cache_read_price_per_1m', 'cache_write_price_per_1m', 'fixed_help', 'style', 'style_inherit', 'style_help', 'invalid_fixed', 'style_required']
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
      expect(bundle.fixed_help).toContain('USD / 1M')
      expect(bundle.fixed_help).toContain(language === 'en' ? 'zero' : '零')
      expect(bundle.style_help).toContain('ModelAlias')
      expect(bundle.style_help).toContain('OpenAI')
      expect(bundle.style_help).toContain('Claude')
    }
  })
})

describe('channel model exception translations', () => {
  it('has complete localized channel, exact-match, precedence, fixed tariff and history copy', () => {
    const keys = ['title', 'scope_help', 'history_warning', 'replacement_warning', 'select', 'channel_search', 'empty', 'members_help', 'refresh', 'model', 'model_search', 'no_models', 'model_help', 'current', 'inherited', 'active', 'canonical', 'multiplier', 'input_help', 'mode', 'fixed', 'prompt_price_per_1m', 'completion_price_per_1m', 'cache_read_price_per_1m', 'cache_write_price_per_1m', 'fixed_help', 'style', 'style_inherit', 'style_help', 'save', 'clear', 'saved', 'cleared', 'invalid_multiplier', 'invalid_fixed', 'style_required', 'load_failed', 'save_failed', 'clear_failed', 'permission_denied', 'conflict']
    const precedence = {
      en: ['credential + model', 'credential default', 'channel + model', 'channel default', 'complete legacy pricing'],
      zh: ['凭证 + 模型', '凭证默认', '渠道 + 模型', '渠道默认', '完整旧定价'],
      'zh-TW': ['憑證 + 模型', '憑證預設', '渠道 + 模型', '渠道預設', '完整舊定價'],
    }
    for (const language of SUPPORTED_LANGUAGES) {
      const bundle = i18n.getResourceBundle(language, 'translation').pricing_channel_models
      expect(Object.keys(bundle).sort()).toEqual([...keys].sort())
      for (const key of keys) {
        const path = `pricing_channel_models.${key}`
        expect(i18n.exists(path, { lng: language, fallbackLng: false }), `${language}: ${path}`).toBe(true)
        expect(i18n.t(path, { lng: language, fallbackLng: false }).trim()).not.toBe('')
        expect(i18n.t(path, { lng: language, fallbackLng: false })).not.toBe(path)
      }
      expect(bundle.members_help).toContain('{{count}}')
      const positions = precedence[language].map(layer => bundle.replacement_warning.indexOf(layer))
      for (const [index, position] of positions.entries()) {
        expect(position).toBeGreaterThanOrEqual(0)
        if (index > 0) expect(position).toBeGreaterThan(positions[index - 1])
      }
      expect(bundle.replacement_warning).toContain('tier'); expect(bundle.replacement_warning).toContain('reasoning')
      expect(bundle.model_help).toContain('ModelAlias')
      expect(bundle.style_help).toContain('ModelAlias'); expect(bundle.style_help).toContain('OpenAI'); expect(bundle.style_help).toContain('Claude')
      expect(bundle.fixed_help).toContain('USD / 1M'); expect(bundle.fixed_help).toContain(language === 'en' ? 'zero' : '零')
      expect(bundle.input_help).toContain('0'); expect(bundle.input_help).toContain('1')
      const memberCopy = i18n.t('pricing_channel_models.members_help', { lng: language, count: 7 })
      expect(memberCopy).toContain('7'); expect(memberCopy).not.toContain('{{count}}')
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

describe('default pricing inheritance copy coherence', () => {
  it('describes every layer, clearing one scope, and restoration only after all new overrides are cleared', () => {
    const layers = {
      en: ['credential + model', 'credential default', 'channel + model', 'channel default', 'complete legacy pricing'],
      zh: ['凭证 + 模型', '凭证默认', '渠道 + 模型', '渠道默认', '完整旧定价'],
      'zh-TW': ['憑證 + 模型', '憑證預設', '渠道 + 模型', '渠道預設', '完整舊定價'],
    }
    for (const language of SUPPORTED_LANGUAGES) {
      const resources = i18n.getResourceBundle(language, 'translation')
      const defaults = resources.pricing_credential_defaults
      const channels = resources.pricing_channels
      const next = language === 'en' ? 'next applicable' : language === 'zh' ? '下一适用' : '下一適用'
      const clearAll = language === 'en' ? 'Only clearing all new overrides' : language === 'zh' ? '只有清除全部新覆盖' : '只有清除全部新覆蓋'
      for (const text of [defaults.scope_help, defaults.inherited, defaults.cleared, channels.scope_help, channels.inherited]) {
        expect(text).toContain(next)
      }
      expect(defaults.cleared).toContain(clearAll)
      for (const text of [defaults.replacement_warning, channels.priority_help]) {
        const positions = layers[language].map(layer => text.indexOf(layer))
        for (const [index, position] of positions.entries()) {
          expect(position).toBeGreaterThanOrEqual(0)
          if (index > 0) expect(position).toBeGreaterThan(positions[index - 1])
        }
        expect(text).toContain(next); expect(text).toContain(clearAll)
        expect(text).toContain('tier'); expect(text).toContain('reasoning')
        expect(text).toContain(language === 'en' ? 'no stacking or multiplication' : language === 'zh' ? '不叠加或连乘' : '不疊加或連乘')
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
      expect(bundle.delete_help).toContain(language === 'en' ? 'every saved channel-model exception' : language === 'zh' ? '全部已保存渠道模型例外' : '全部已儲存渠道模型例外')
      expect(bundle.confirm_delete).toContain(language === 'en' ? 'model exceptions' : '模型例外')
      expect(bundle.priority_help).toContain('tier'); expect(bundle.priority_help).toContain('reasoning')
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
