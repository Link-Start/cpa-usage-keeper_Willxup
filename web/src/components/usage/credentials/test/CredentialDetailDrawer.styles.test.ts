import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const drawerStyles = readFileSync(new URL('../CredentialDetailDrawer.module.scss', import.meta.url), 'utf8')

const scssRule = (selector: string) => {
  const start = drawerStyles.indexOf(selector)
  expect(start).toBeGreaterThanOrEqual(0)

  const openingBrace = drawerStyles.indexOf('{', start + selector.length)
  expect(openingBrace).toBeGreaterThan(start)
  let depth = 1
  for (let index = openingBrace + 1; index < drawerStyles.length; index += 1) {
    if (drawerStyles[index] === '{') {
      depth += 1
    } else if (drawerStyles[index] === '}' && --depth === 0) {
      return drawerStyles.slice(start, index + 1)
    }
  }

  throw new Error(`Unclosed SCSS rule: ${selector}`)
}

describe('CredentialDetailDrawer styles', () => {
  it('uses the shared Keeper radius for every Overview card surface', () => {
    expect(scssRule('.summaryMetric')).toContain('border-radius: var(--keeper-card-radius);')
    expect(scssRule('.overviewSection')).toContain('border-radius: var(--keeper-card-radius);')
  })
})
