import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import {
  REQUEST_EVENT_COLUMN_IDS,
  RequestEventsDetailsCard,
} from '../RequestEventsDetailsCard';
import type { UsageEvent } from '@/lib/types';

const baseEvent: UsageEvent = {
  id: '101',
  timestamp: '2026-04-23T02:00:00.000Z',
  api_key: 'Production Key',
  model: 'claude-sonnet',
  source: 'Provider A',
  source_raw: 'source-a',
  source_type: 'openai',
  auth_index: '1',
  failed: false,
  latency_ms: 120,
  ttft_ms: 45,
  speed_tps: 30,
  tokens: {
    input_tokens: 100,
    output_tokens: 60,
    reasoning_tokens: 20,
    cache_read_tokens: 20,
    cache_creation_tokens: 0,
    total_tokens: 200,
  },
  cost_usd: 0.1234,
  cost_available: true,
  pricing_style: 'claude',
};

const renderCard = (events: UsageEvent[]) => renderToStaticMarkup(
  <RequestEventsDetailsCard
    events={events}
    loading={false}
    page={1}
    pageSize={20}
    pageSizeOptions={[20, 50, 100, 500, 1000]}
    totalCount={events.length}
    totalPages={1}
    modelOptions={['claude-sonnet']}
    sourceOptions={[{ value: 'source-a', label: 'Provider A' }]}
    modelFilter="__all__"
    sourceFilter="__all__"
    resultFilter="__all__"
    visibleColumnIds={['service_tier']}
    onPageChange={() => undefined}
    onPageSizeChange={() => undefined}
    onModelFilterChange={() => undefined}
    onSourceFilterChange={() => undefined}
    onResultFilterChange={() => undefined}
  />,
);

const extractSpeedModeCells = (html: string) => (
  Array.from(html.matchAll(/<tr><td\b[^>]*>(.*?)<\/td><\/tr>/gs), (match) => match[1])
);

describe('RequestEventsDetailsCard Speed Mode column', () => {
  it('shows mapped request and response modes in one column with a labeled tooltip', () => {
    const html = renderCard([
      { ...baseEvent, service_tier: 'auto', response_service_tier: 'default' },
      { ...baseEvent, id: '102', service_tier: 'standard', response_service_tier: 'standard' },
    ]);

    expect(REQUEST_EVENT_COLUMN_IDS).not.toContain('response_service_tier');
    expect(html).toContain('>Speed Mode</th>');
    expect(html).not.toContain('>Response Speed Mode</th>');
    expect(extractSpeedModeCells(html)).toEqual(['Auto / Standard', 'Standard / Standard']);
    expect(html).toContain('title="Speed Mode: Auto\nResponse Speed Mode: Standard"');
    expect(html).toContain('aria-label="Speed Mode: Auto; Response Speed Mode: Standard"');
  });

  it('keeps a dash for each missing request or response mode in the cell and tooltip', () => {
    const html = renderCard([
      { ...baseEvent, service_tier: 'priority', response_service_tier: '' },
      { ...baseEvent, id: '102', service_tier: '', response_service_tier: 'default' },
      { ...baseEvent, id: '103', service_tier: '', response_service_tier: '' },
    ]);

    expect(extractSpeedModeCells(html)).toEqual(['Fast / -', '- / Standard', '- / -']);
    expect(html).toContain('title="Speed Mode: Fast\nResponse Speed Mode: -"');
    expect(html).toContain('title="Speed Mode: -\nResponse Speed Mode: Standard"');
    expect(html).toContain('title="Speed Mode: -\nResponse Speed Mode: -"');
  });
});
