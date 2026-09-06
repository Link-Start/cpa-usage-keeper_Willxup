// @vitest-environment happy-dom

import React, { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '@/i18n';
import { RequestEventsDetailsCard } from '../RequestEventsDetailsCard';

describe('RequestEventsDetailsCard model search', () => {
  let container: HTMLDivElement;
  let root: Root;
  const onModelFilterChange = vi.fn();
  const modelOptions = ['claude-sonnet-4', 'gpt-5', 'gpt-5-mini', 'gemini-2.5-pro'];

  beforeEach(async () => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    await i18n.changeLanguage('en');
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    await act(async () => root.render(
      <RequestEventsDetailsCard
        events={[]}
        loading={false}
        totalCount={0}
        modelOptions={modelOptions}
        sourceOptions={[]}
        modelFilter="claude-sonnet-4"
        sourceFilter="__all__"
        resultFilter="__all__"
        onModelFilterChange={onModelFilterChange}
        onSourceFilterChange={() => undefined}
        onResultFilterChange={() => undefined}
      />
    ));
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
    vi.clearAllMocks();
  });

  const trigger = () => container.querySelector<HTMLButtonElement>('button[aria-label="Model"]')!;
  const input = () => document.querySelector<HTMLInputElement>('input[role="combobox"]')!;
  const options = () => Array.from(document.querySelectorAll('[role="option"]')).map((node) => node.textContent);
  const typeQuery = async (query: string) => {
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input(), query);
      input().dispatchEvent(new Event('input', { bubbles: true }));
    });
  };

  it('filters names locally without changing the query until a model is selected', async () => {
    await act(async () => trigger().click());
    expect(input()).not.toBeNull();
    expect(document.activeElement).toBe(input());
    await typeQuery(' GPT-5 ');
    expect(options()).toEqual(['gpt-5', 'gpt-5-mini']);
    expect(onModelFilterChange).not.toHaveBeenCalled();
    expect(trigger().textContent).toContain('claude-sonnet-4');

    await act(async () => document.querySelectorAll<HTMLButtonElement>('[role="option"]')[1].click());
    expect(onModelFilterChange).toHaveBeenCalledExactlyOnceWith('gpt-5-mini');
    expect(input()).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });

  it('shows an empty result, preserves the selection on Escape, and resets search when reopened', async () => {
    await act(async () => trigger().click());
    expect(input()).not.toBeNull();
    await typeQuery('missing-model');
    expect(options()).toEqual([]);
    expect(document.body.textContent).toContain('No matching models');
    await act(async () => input().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true })));
    expect(onModelFilterChange).not.toHaveBeenCalled();
    await act(async () => input().dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })));
    expect(document.activeElement).toBe(trigger());
    await act(async () => trigger().click());
    expect(input().value).toBe('');
    expect(options()).toEqual(['All', ...modelOptions]);
    expect(document.querySelector('[role="option"][aria-selected="true"]')?.textContent).toBe('claude-sonnet-4');
    await act(async () => document.querySelector<HTMLButtonElement>('[role="option"]')!.click());
    expect(onModelFilterChange).toHaveBeenCalledExactlyOnceWith('__all__');
  });

  it('keeps the source and result dropdowns without a search input', async () => {
    for (const label of ['Source', 'Result']) {
      const button = container.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)!;
      await act(async () => button.click());
      expect(document.querySelector('[role="listbox"]')).not.toBeNull();
      expect(input()).toBeNull();
      await act(async () => button.click());
    }
  });

  it('supports keyboard selection without intercepting text editing or IME confirmation', async () => {
    await act(async () => trigger().dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true })));
    await typeQuery('gpt');
    const keyDown = async (key: string, isComposing = false) => {
      const event = new KeyboardEvent('keydown', { key, isComposing, bubbles: true, cancelable: true });
      await act(async () => input().dispatchEvent(event));
      return event;
    };
    const highlighted = () => document.getElementById(input().getAttribute('aria-activedescendant')!)?.textContent;

    expect(highlighted()).toBe('gpt-5');
    await keyDown('ArrowDown');
    expect(highlighted()).toBe('gpt-5-mini');
    await keyDown('ArrowUp');
    expect(highlighted()).toBe('gpt-5');
    for (const key of [' ', 'Home', 'End']) {
      expect((await keyDown(key)).defaultPrevented).toBe(false);
    }
    await keyDown('Enter', true);
    expect(onModelFilterChange).not.toHaveBeenCalled();
    expect(input()).not.toBeNull();
    await keyDown('Enter');
    expect(onModelFilterChange).toHaveBeenCalledExactlyOnceWith('gpt-5');

    await act(async () => trigger().click());
    expect((await keyDown('Tab')).defaultPrevented).toBe(false);
    expect(input()).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });
});
