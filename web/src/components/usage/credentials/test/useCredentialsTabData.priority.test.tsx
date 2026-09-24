// @vitest-environment happy-dom

import { act, useEffect } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useCredentialsTabData } from '../useCredentialsTabData'

globalThis.IS_REACT_ACT_ENVIRONMENT = true

type TabData = ReturnType<typeof useCredentialsTabData>
let latest: TabData | null = null

function Harness({ onNotice, onPrioritySaved }: { onNotice: (kind: 'success' | 'info' | 'error', text: string) => void; onPrioritySaved: () => void }) {
  const data = useCredentialsTabData({ enabledAuthFiles: true, enabledAiProviders: true, onNotice, onPrioritySaved })
  useEffect(() => { latest = data }, [data])
  return null
}

describe('credential priority save data flow', () => {
  let container: HTMLDivElement
  let root: Root
  let fetchMock: ReturnType<typeof vi.spyOn>
  const onNotice = vi.fn()
  const onPrioritySaved = vi.fn()

  beforeEach(() => {
    window.localStorage.clear()
    onNotice.mockReset()
    onPrioritySaved.mockReset()
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      if (String(input).includes('/priority')) return Response.json({ auth_index: 'idx/one', priority: -2 })
      return Response.json({ identities: [], total_count: 0, total_pages: 1, type_counts: [] })
    })
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
    latest = null
    fetchMock.mockRestore()
  })

  it('refreshes sorted pages after a priority PATCH and notifies the detail drawer', async () => {
    await act(async () => root.render(<Harness onNotice={onNotice} onPrioritySaved={onPrioritySaved} />))
    const beforePages = fetchMock.mock.calls.filter(([input]) => String(input).includes('/usage/identities/page')).length
    await act(async () => { await latest!.saveAiProviderPriority('local-id', 'idx/one', -2) })
    const patches = fetchMock.mock.calls.filter(([input, init]) => String(input).includes('/priority') && init?.method === 'PATCH')
    expect(patches).toHaveLength(1)
    expect(String(patches[0][0])).toContain('/ai-providers/idx%2Fone/priority')
    expect(JSON.parse(String(patches[0][1]?.body))).toEqual({ priority: -2 })
    expect(fetchMock.mock.calls.filter(([input]) => String(input).includes('/usage/identities/page')).length).toBeGreaterThan(beforePages)
    expect(onPrioritySaved).toHaveBeenCalledTimes(1)
    expect(onNotice).toHaveBeenCalledWith('success', expect.any(String))
  })

  it('keeps failure visible and refreshes a stale target', async () => {
    fetchMock.mockImplementation(async (input) => {
      if (String(input).includes('/priority')) return Response.json({ error: 'not found' }, { status: 404 })
      return Response.json({ identities: [], total_count: 0, total_pages: 1, type_counts: [] })
    })
    await act(async () => root.render(<Harness onNotice={onNotice} onPrioritySaved={onPrioritySaved} />))
    const beforePages = fetchMock.mock.calls.filter(([input]) => String(input).includes('/usage/identities/page')).length
    await act(async () => { await expect(latest!.saveAuthFilePriority('local-id', 'stale', 3)).rejects.toThrow() })
    expect(fetchMock.mock.calls.filter(([input]) => String(input).includes('/usage/identities/page')).length).toBeGreaterThan(beforePages)
    expect(onPrioritySaved).not.toHaveBeenCalled()
    expect(onNotice).toHaveBeenCalledWith('error', expect.any(String))
  })
})
