import { describe, expect, it } from 'vitest';
import i18n, { SUPPORTED_LANGUAGES } from './index';

const flattenKeys = (value: unknown, prefix = ''): string[] => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return [prefix];
  return Object.entries(value).flatMap(([key, child]) => {
    const path = prefix ? `${prefix}.${key}` : key;
    return flattenKeys(child, path);
  });
};

describe('i18n resources', () => {
  it('keeps every supported language aligned with English keys', () => {
    const englishKeys = flattenKeys(i18n.getResourceBundle('en', 'translation')).sort();

    for (const language of SUPPORTED_LANGUAGES) {
      expect(flattenKeys(i18n.getResourceBundle(language, 'translation')).sort()).toEqual(englishKeys);
    }
  });

  it('localizes Analysis tab and composition titles in Chinese', () => {
    expect(i18n.getResource('zh', 'translation', 'usage_stats.tab_analysis')).toBe('分析');
    expect(i18n.getResource('zh', 'translation', 'usage_stats.analysis_auth_files_composition_title')).toBe('认证文件构成');
    expect(i18n.getResource('zh', 'translation', 'usage_stats.analysis_ai_provider_composition_title')).toBe('AI 供应商构成');
    expect(i18n.getResource('zh-TW', 'translation', 'usage_stats.tab_analysis')).toBe('分析');
    expect(i18n.getResource('zh-TW', 'translation', 'usage_stats.analysis_auth_files_composition_title')).toBe('認證檔案組成');
    expect(i18n.getResource('zh-TW', 'translation', 'usage_stats.analysis_ai_provider_composition_title')).toBe('AI 供應商組成');
  });
});
