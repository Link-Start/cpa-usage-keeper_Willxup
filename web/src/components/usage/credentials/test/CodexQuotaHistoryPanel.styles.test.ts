import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const quotaHistoryStyles = readFileSync(new URL('../CodexQuotaHistoryPanel.module.scss', import.meta.url), 'utf8')

describe('Codex quota history styles', () => {
  it('uses the Keeper card contract for the two top-level sections', () => {
    expect(quotaHistoryStyles).toMatch(/\.card,\s*\.historySection\s*\{[\s\S]*?border-radius:\s*var\(--keeper-card-radius\);/)
    expect(quotaHistoryStyles).toMatch(/\.card,\s*\.historySection\s*\{[\s\S]*?box-shadow:\s*var\(--shadow-lg\);/)
    expect(quotaHistoryStyles).toMatch(/\.cycleCard\s*\{[\s\S]*?border-radius:\s*9px;/)
  })

  it('keeps the window selector in the existing pill-shaped segmented-control language', () => {
    expect(quotaHistoryStyles).toMatch(/\.windowSwitcher\s*\{[\s\S]*?border-radius:\s*999px;/)
    expect(quotaHistoryStyles).toMatch(/\.segmentButton\s*\{[\s\S]*?border-radius:\s*999px;/)
    expect(quotaHistoryStyles).toMatch(/&\[aria-pressed='true'\]\s*\{[\s\S]*?background:\s*var\(--bg-primary\);[\s\S]*?box-shadow:\s*0 6px 14px rgba\(0, 0, 0, 0\.08\);/)
    expect(quotaHistoryStyles).toMatch(/&:hover:not\(:disabled\)\s*\{[\s\S]*?color:\s*var\(--text-primary\);/)
    expect(quotaHistoryStyles).toMatch(/&:focus-visible\s*\{[\s\S]*?outline:\s*2px solid var\(--primary-color\);[\s\S]*?outline-offset:\s*2px;/)
  })

  it('centers the combined chart legend below the graph', () => {
    expect(quotaHistoryStyles).toMatch(/\.chartLegend\s*\{[\s\S]*?justify-content:\s*center;/)
    expect(quotaHistoryStyles).toMatch(/\.costLine\s*\{[\s\S]*?border-top:\s*2px dashed var\(--quota-cost-line-color, #ff5a40\);/)
  })

  it('matches the Analysis header hint and keeps chart facts available to screen readers', () => {
    expect(quotaHistoryStyles).toMatch(/\.costHeaderHint\s*\{[\s\S]*?text-align:\s*right;/)
    expect(quotaHistoryStyles).toMatch(/\.screenReaderOnly\s*\{[\s\S]*?clip-path:\s*inset\(50%\);/)
  })

  it('wraps current-cycle ranges as complete content blocks at every width', () => {
    expect(quotaHistoryStyles).toMatch(/\.currentCycleMeta\s*\{[\s\S]*?display:\s*flex;[\s\S]*?flex-wrap:\s*wrap;/)
    expect(quotaHistoryStyles).toMatch(/\.currentCycleRange,\s*\.currentObservedRange\s*\{[\s\S]*?min-width:\s*0;[\s\S]*?flex:\s*0 1 auto;/)
    expect(quotaHistoryStyles).not.toMatch(/@include mobile\s*\{[\s\S]*?\.currentCycleRange,\s*\.currentObservedRange/)
  })

  it('shows the three quota summaries in one row and stacks all three from the card width', () => {
    const summaryStyles = quotaHistoryStyles.slice(
      quotaHistoryStyles.indexOf('.chartSummary {'),
      quotaHistoryStyles.indexOf('.chartSummaryRow'),
    )
    const summaryRowStyles = quotaHistoryStyles.slice(
      quotaHistoryStyles.indexOf('.chartSummaryRow'),
      quotaHistoryStyles.indexOf('.chartSummaryMetric'),
    )
    const narrowSummaryStyles = quotaHistoryStyles.slice(
      quotaHistoryStyles.indexOf('@container quota-history-card'),
      quotaHistoryStyles.indexOf('.screenReaderOnly'),
    )
    const mobileSummaryStyles = narrowSummaryStyles.slice(
      narrowSummaryStyles.indexOf('@container quota-history-card (max-width: 560px)'),
    )
    expect(quotaHistoryStyles).toMatch(/\.card\s*\{[\s\S]*?container:\s*quota-history-card \/ inline-size;/)
    expect(summaryStyles).toContain('display: grid')
    expect(summaryStyles).toContain('grid-template-columns: repeat(3, minmax(0, 1fr))')
    expect(summaryStyles).toContain('border-top: 1px solid var(--border-color)')
    expect(summaryStyles).not.toContain('620px')
    expect(summaryRowStyles).toMatch(/\.chartSummaryRow\s*\{[\s\S]*?display:\s*grid;[\s\S]*?grid-template-rows:\s*max-content max-content;/)
    expect(summaryRowStyles).toMatch(/& \+ &\s*\{[\s\S]*?border-left:\s*1px solid var\(--border-color\);/)
    expect(summaryRowStyles).toMatch(/dd\s*\{[\s\S]*?display:\s*grid;[\s\S]*?grid-template-columns:\s*repeat\(3, max-content\);[\s\S]*?justify-content:\s*center;/)
    expect(quotaHistoryStyles).toMatch(/\.chartSummaryMetric\s*\{[\s\S]*?display:\s*grid;[\s\S]*?grid-template-columns:\s*14px max-content;/)
    expect(narrowSummaryStyles).toMatch(/@container quota-history-card \(max-width:\s*640px\)/)
    expect(narrowSummaryStyles).toMatch(/\.chartSummary\s*\{[\s\S]*?grid-template-columns:\s*minmax\(0, 1fr\);/)
    expect(narrowSummaryStyles).toMatch(/\.chartSummaryRow\s*\{[\s\S]*?grid-template-columns:\s*108px minmax\(0, 1fr\);/)
    expect(narrowSummaryStyles).toMatch(/dd\s*\{[\s\S]*?grid-template-columns:\s*repeat\(3, max-content\);[\s\S]*?justify-content:\s*start;/)
    expect(narrowSummaryStyles).toMatch(/\.chartSummaryMetric\s*\{[\s\S]*?grid-template-columns:\s*14px max-content;[\s\S]*?white-space:\s*nowrap;/)
    expect(narrowSummaryStyles).toMatch(/@container quota-history-card \(max-width:\s*560px\)/)
    expect(mobileSummaryStyles).toMatch(/\.chartSummaryRow\s*\{[\s\S]*?grid-template-columns:\s*minmax\(0, 1fr\);/)
    expect(mobileSummaryStyles).toMatch(/dd\s*\{[\s\S]*?justify-content:\s*start;/)
  })

  it('pairs completed cycle summaries in two rows and keeps each metric set horizontal when the card narrows', () => {
    const cycleSummaryStyles = quotaHistoryStyles.slice(
      quotaHistoryStyles.indexOf('.cycleSummary {'),
      quotaHistoryStyles.indexOf('@container quota-cycle-card'),
    )
    const narrowCycleSummaryStyles = quotaHistoryStyles.slice(
      quotaHistoryStyles.indexOf('@container quota-cycle-card'),
      quotaHistoryStyles.indexOf('.currentStatus,'),
    )
    const mobileCycleSummaryStyles = narrowCycleSummaryStyles.slice(
      narrowCycleSummaryStyles.indexOf('@container quota-cycle-card (max-width: 560px)'),
    )
    expect(quotaHistoryStyles).toMatch(/\.cycleCard\s*\{[\s\S]*?container:\s*quota-cycle-card \/ inline-size;/)
    expect(cycleSummaryStyles).toMatch(/>\s*\.chartSummaryRow\s*\{[\s\S]*?border-left:\s*0;/)
    expect(cycleSummaryStyles).toMatch(/\.completedCycleSummary\s*\{[\s\S]*?grid-template-columns:\s*repeat\(2, minmax\(0, 1fr\)\);/)
    expect(cycleSummaryStyles).toMatch(/\.completedCycleSummary\s*\{[\s\S]*?\.chartSummaryRow:nth-child\(n \+ 3\)\s*\{[\s\S]*?border-top:\s*1px solid var\(--border-color\);/)
    expect(cycleSummaryStyles).toMatch(/\.currentCycleSummary\s*\{[\s\S]*?grid-template-columns:\s*repeat\(3, minmax\(0, 1fr\)\);/)
    expect(cycleSummaryStyles).toMatch(/dd\s*\{[\s\S]*?grid-template-columns:\s*repeat\(3, max-content\);[\s\S]*?justify-content:\s*center;/)
    expect(cycleSummaryStyles).toMatch(/\.chartSummaryMetric\s*\{[\s\S]*?grid-template-columns:\s*14px max-content;/)
    expect(narrowCycleSummaryStyles).toMatch(/@container quota-cycle-card \(max-width:\s*640px\)/)
    expect(narrowCycleSummaryStyles).toMatch(/\.cycleSummary\s*\{[\s\S]*?grid-template-columns:\s*minmax\(0, 1fr\);/)
    expect(narrowCycleSummaryStyles).toMatch(/\.chartSummaryRow\s*\{[\s\S]*?grid-template-columns:\s*108px minmax\(0, 1fr\);/)
    expect(narrowCycleSummaryStyles).toMatch(/dd\s*\{[\s\S]*?grid-template-columns:\s*repeat\(3, max-content\);[\s\S]*?justify-content:\s*start;/)
    expect(narrowCycleSummaryStyles).toMatch(/@container quota-cycle-card \(max-width:\s*560px\)/)
    expect(mobileCycleSummaryStyles).toMatch(/\.chartSummaryRow\s*\{[\s\S]*?grid-template-columns:\s*minmax\(0, 1fr\);/)
    expect(mobileCycleSummaryStyles).toMatch(/dd\s*\{[\s\S]*?justify-content:\s*start;/)
  })
})
