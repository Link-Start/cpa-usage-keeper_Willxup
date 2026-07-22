// @vitest-environment happy-dom

import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import '@/i18n';
import { RecentActivityPanel } from '../RecentActivityPanel';
import { buildUsageActivityFixture } from './activityFixtures';

describe('RecentActivityPanel window switcher', () => {
  let container: HTMLDivElement;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement('div');
    document.body.appendChild(container);
  });

  afterEach(() => {
    container.remove();
  });

  it('uses semantic view labels without replacing an active Day request', () => {
    const onWindowChange = vi.fn();
    const root = createRoot(container);
    act(() => root.render(
      <RecentActivityPanel
        activity={buildUsageActivityFixture([1])}
        loading={false}
        error=""
        window="24h"
        requestIdentity="admin::today"
        onWindowChange={onWindowChange}
      />,
    ));

    const buttons = Array.from(container.querySelectorAll('button'));
    expect(buttons.map((button) => button.textContent)).toEqual(['Day', 'Week', 'Month', 'Year']);
    const activeButton = buttons.find((button) => button.textContent === 'Day');
    const sevenDayButton = buttons.find((button) => button.textContent === 'Week');
    act(() => activeButton?.click());
    expect(onWindowChange).not.toHaveBeenCalled();

    act(() => sevenDayButton?.click());
    expect(onWindowChange).toHaveBeenCalledWith('7d');
    act(() => root.unmount());
  });
});
