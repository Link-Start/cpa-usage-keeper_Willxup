// @vitest-environment happy-dom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ModelPricingConfig } from '@/lib/types'
import { PricingSettings } from '../PricingSettings'

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => undefined },
  useTranslation: () => ({ t: (key: string, params?: Record<string, unknown>) => (
    params ? `${key}:${JSON.stringify(params)}` : key
  ) }),
}))

const existing: ModelPricingConfig = {
  model: 'provider/claude-sonnet', pricing_style: 'claude',
  base_prices: { input: 3, output: 15, cache_read: 0.3, cache_write: 3.75 },
  model_multiplier: 1.4,
  conditional_multipliers: [
    { key: 'reasoning_effort', value: 'xhigh', multiplier: 1.2 },
    { key: 'endpoint', value: '/v1/responses', multiplier: 1.1 },
  ],
  branches: [{
    id: 'saved-branch', name: 'Long context', context: { type: 'gt', threshold: 200_000 },
    period: { type: 'all' },
    prices: { input: 2.5, output: 12, cache_read: 0.25, cache_write: 3 },
  }],
}

const setInput = async (input: HTMLInputElement, value: string) => {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

const button = (label: string) => {
  const found = [...document.body.querySelectorAll<HTMLButtonElement>('button')]
    .find((candidate) => candidate.textContent?.trim() === label)
  expect(found, label).toBeDefined()
  return found!
}

describe('PricingSettings', () => {
  let container: HTMLDivElement
  let root: Root
  let configs: ModelPricingConfig[]
  let revision: number
  let writes: Array<{ method: string; url: URL; body?: ModelPricingConfig }>

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true
    configs = [structuredClone(existing)]
    revision = 7
    writes = []
    vi.stubGlobal('window', window)
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const url = new URL(String(input), 'http://localhost')
      if (url.pathname.endsWith('/pricing/model-options')) {
        return Response.json({ models: ['provider/claude-sonnet', 'openai/gpt-5', 'gemini/3'] })
      }
      if (!url.pathname.endsWith('/pricing/models')) throw new Error(`unexpected pricing request ${url}`)
      if (init?.method === 'PUT') {
        const config = JSON.parse(String(init.body)) as ModelPricingConfig
        writes.push({ method: 'PUT', url, body: config })
        configs = [...configs.filter((item) => item.model !== config.model), config]
        revision++
        return Response.json({ model: config.model, config_revision: revision })
      }
      if (init?.method === 'DELETE') {
        writes.push({ method: 'DELETE', url })
        configs = configs.filter((item) => item.model !== url.searchParams.get('model'))
        revision++
        return Response.json({ config_revision: revision })
      }
      return Response.json({ models: configs, config_revision: revision })
    })
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  const renderSettings = async () => {
    await act(async () => root.render(<PricingSettings />))
  }

  it('loads complete saved prices and adds a model with free-text conditions in one save', async () => {
    await renderSettings()
    expect(document.body.textContent).toContain('provider/claude-sonnet')
    await act(async () => button('usage_stats.pricing_settings_add').click())
    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    const modelSelect = dialog.querySelector<HTMLSelectElement>('[data-pricing-field="model"]')!
    const inputPrice = dialog.querySelector<HTMLInputElement>('[data-pricing-field="base_prices.input"]')!
    expect(modelSelect.compareDocumentPosition(inputPrice) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(modelSelect.value).toBe('openai/gpt-5')
    expect(inputPrice.getAttribute('aria-invalid')).not.toBe('true')
    for (const [field, value] of Object.entries({ input: '1.25', output: '10', cache_read: '0.125', cache_write: '0' })) {
      await setInput(dialog.querySelector<HTMLInputElement>(`[data-pricing-field="base_prices.${field}"]`)!, value)
    }
    await act(async () => button('usage_stats.pricing_settings_add_condition').click())
    await setInput(dialog.querySelector<HTMLInputElement>('[data-pricing-field="conditional_multipliers[0].key"]')!, 'reasoning_effort')
    await setInput(dialog.querySelector<HTMLInputElement>('[data-pricing-field="conditional_multipliers[0].value"]')!, 'xhigh')
    await setInput(dialog.querySelector<HTMLInputElement>('[data-pricing-field="conditional_multipliers[0].multiplier"]')!, '1.5')
    await act(async () => button('common.save').click())

    expect(writes).toHaveLength(1)
    expect(writes[0].body).toEqual({
      model: 'openai/gpt-5', pricing_style: 'openai',
      base_prices: { input: 1.25, output: 10, cache_read: 0.125, cache_write: 0 },
      model_multiplier: 1,
      conditional_multipliers: [{ key: 'reasoning_effort', value: 'xhigh', multiplier: 1.5 }],
      branches: [],
    })
    expect(document.body.textContent).toContain('openai/gpt-5')
  })

  it('discards an edit draft and later saves the original branches and legal condition values', async () => {
    await renderSettings()
    await act(async () => button('common.edit').click())
    let dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    expect(dialog.querySelector<HTMLSelectElement>('[data-pricing-field="model"]')).toBeNull()
    expect(dialog.querySelector<HTMLInputElement>('[data-pricing-field="conditional_multipliers[0].value"]')?.value).toBe('xhigh')
    expect(dialog.querySelector<HTMLInputElement>('[data-pricing-field="conditional_multipliers[1].key"]')?.value).toBe('endpoint')
    expect(dialog.querySelector<HTMLInputElement>('[data-pricing-field="conditional_multipliers[1].value"]')?.value).toBe('/v1/responses')
    await setInput(dialog.querySelector<HTMLInputElement>('[data-pricing-field="base_prices.input"]')!, '99')
    await act(async () => button('common.cancel').click())
    expect(writes).toHaveLength(0)

    await act(async () => button('common.edit').click())
    dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    expect(dialog.querySelector<HTMLInputElement>('[data-pricing-field="base_prices.input"]')?.value).toBe('3')
    await setInput(dialog.querySelector<HTMLInputElement>('[data-pricing-field="base_prices.input"]')!, '4.5')
    await act(async () => button('common.save').click())
    expect(writes).toHaveLength(1)
    expect(writes[0].body?.base_prices.input).toBe(4.5)
    expect(writes[0].body?.conditional_multipliers).toEqual(existing.conditional_multipliers)
    expect(writes[0].body?.branches).toEqual(existing.branches)
  })

  it('keeps an unsaved price while switching new-model candidates and discards it on cancel', async () => {
    await renderSettings()
    await act(async () => button('usage_stats.pricing_settings_add').click())
    let dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    await setInput(dialog.querySelector<HTMLInputElement>('[data-pricing-field="base_prices.input"]')!, '1.75')
    await act(async () => button('common.save').click())
    expect(dialog.querySelector<HTMLInputElement>('[data-pricing-field="base_prices.output"]')?.getAttribute('aria-invalid')).toBe('true')
    const select = dialog.querySelector<HTMLSelectElement>('[data-pricing-field="model"]')!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')!.set!.call(select, 'gemini/3')
      select.dispatchEvent(new Event('change', { bubbles: true }))
    })
    expect(select.value).toBe('gemini/3')
    expect(dialog.querySelector<HTMLInputElement>('[data-pricing-field="base_prices.input"]')?.value).toBe('1.75')
    expect(dialog.querySelector('[aria-invalid="true"]')).toBeNull()
    expect(dialog.querySelector('[data-shake]')).toBeNull()
    await act(async () => button('common.cancel').click())
    expect(writes).toHaveLength(0)

    await act(async () => button('usage_stats.pricing_settings_add').click())
    dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    expect(dialog.querySelector<HTMLInputElement>('[data-pricing-field="base_prices.input"]')?.value).toBe('')
    expect(dialog.querySelector<HTMLSelectElement>('[data-pricing-field="model"]')?.value).toBe('openai/gpt-5')
  })

  it('shows submitted validation only after save, focuses the field, and accepts explicit zero', async () => {
    await renderSettings()
    await act(async () => button('usage_stats.pricing_settings_add').click())
    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    const input = dialog.querySelector<HTMLInputElement>('[data-pricing-field="base_prices.input"]')!
    const form = dialog.querySelector('form')!
    expect(form.noValidate).toBe(true)
    expect(input.getAttribute('aria-invalid')).toBe('false')

    await act(async () => button('common.save').click())
    expect(writes).toHaveLength(0)
    expect(input.getAttribute('aria-invalid')).toBe('true')
    expect(document.activeElement).toBe(input)
    expect(input.closest('[data-shake]')?.getAttribute('data-shake')).toBe('odd')

    for (const key of ['input', 'output', 'cache_read', 'cache_write']) {
      await setInput(dialog.querySelector<HTMLInputElement>(`[data-pricing-field="base_prices.${key}"]`)!, '0')
    }
    expect(input.getAttribute('aria-invalid')).toBe('false')
    await act(async () => button('common.save').click())
    expect(writes).toHaveLength(1)
    expect(writes[0].body?.base_prices).toEqual({ input: 0, output: 0, cache_read: 0, cache_write: 0 })
  })

  it.each([
    ['conditional_multipliers', 'conditional_multipliers[0].multiplier', 'conditional_multipliers[1].multiplier'],
    ['base_prices', 'base_prices.input', 'base_prices.output'],
  ])('maps a backend %s group error to relevant numeric fields instead of only a banner', async (groupPath, expectedField, anotherField) => {
    await renderSettings()
    await act(async () => button('common.edit').click())
    const details = document.querySelector<HTMLDetailsElement>('details')!
    await act(async () => details.querySelector('summary')!.click())
    expect(details.open).toBe(false)
    vi.mocked(globalThis.fetch).mockImplementationOnce(async () => Response.json({
      code: 'invalid_pricing', message: 'Invalid field group',
      fields: [{ path: groupPath, code: 'invalid' }],
    }, { status: 400 }))
    await act(async () => button('common.save').click())
    const field = [...document.querySelectorAll<HTMLInputElement>('[data-pricing-field]')]
      .find((input) => input.dataset.pricingField === expectedField)!
    expect(field.getAttribute('aria-invalid')).toBe('true')
    expect(document.querySelector<HTMLInputElement>(`[data-pricing-field="${anotherField}"]`)?.getAttribute('aria-invalid')).toBe('true')
    if (groupPath === 'conditional_multipliers') expect(details.open).toBe(true)
    expect(document.activeElement).toBe(field)
    expect(document.querySelector('[role="alert"]')).toBeNull()
  })

  it('collapses conditions without discarding their draft values', async () => {
    await renderSettings()
    await act(async () => button('common.edit').click())
    const details = document.querySelector<HTMLDetailsElement>('details')!
    const value = details.querySelector<HTMLInputElement>('[data-pricing-field="conditional_multipliers[0].value"]')!
    await setInput(value, 'new-free-text')
    await act(async () => details.querySelector('summary')!.click())
    expect(details.open).toBe(false)
    await act(async () => details.querySelector('summary')!.click())
    expect(details.open).toBe(true)
    expect(value.value).toBe('new-free-text')
    expect(writes).toHaveLength(0)
  })

  it('requires confirmation before deleting configuration and keeps the model query intact', async () => {
    await renderSettings()
    await act(async () => button('common.delete').click())
    expect(writes).toHaveLength(0)
    const confirmation = document.querySelector<HTMLElement>('[role="dialog"]')!
    expect(confirmation.textContent).toContain('provider/claude-sonnet')
    await act(async () => confirmation.querySelector<HTMLButtonElement>('button')!.click())
    expect(writes).toHaveLength(0)
    await act(async () => button('common.delete').click())
    const confirm = [...document.querySelectorAll<HTMLElement>('[role="dialog"] button')]
      .find((candidate) => candidate.textContent?.trim() === 'common.delete') as HTMLButtonElement
    await act(async () => confirm.click())
    expect(writes).toHaveLength(1)
    expect(writes[0].method).toBe('DELETE')
    expect(writes[0].url.pathname).toBe('/api/v1/pricing/models')
    expect(writes[0].url.searchParams.get('model')).toBe('provider/claude-sonnet')
    expect(document.body.textContent).toContain('usage_stats.model_price_empty')
  })
})
