import { describe, expect, it } from 'vitest';
import i18n from '../index';

describe('range filter labels', () => {
  it('uses the compact Range label in every supported language', () => {
    expect(i18n.getResource('en', 'translation', 'usage_stats.range_filter')).toBe('Range');
    expect(i18n.getResource('zh', 'translation', 'usage_stats.range_filter')).toBe('范围');
    expect(i18n.getResource('zh-TW', 'translation', 'usage_stats.range_filter')).toBe('範圍');
  });
});
