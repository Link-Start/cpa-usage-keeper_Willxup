import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/Button'
import { Modal } from '@/components/ui/Modal'
import { ApiError } from '@/lib/api'
import type { ModelPricingConfig, PricingBasePrices, PricingConditionalMultiplier, PricingStyle } from '@/lib/types'
import styles from './PricingSettings.module.scss'

type PriceKey = keyof PricingBasePrices
type DraftRule = { id: number; key: string; value: string; multiplier: string }
type FieldErrors = Record<string, string>

interface EditorDraft {
  model: string
  pricingStyle: PricingStyle
  prices: Record<PriceKey, string>
  modelMultiplier: string
  rules: DraftRule[]
}

export interface ModelPricingEditorProps {
  open: boolean
  initialConfig: ModelPricingConfig | null
  modelOptions: string[]
  onClose: () => void
  onSave: (config: ModelPricingConfig) => Promise<unknown>
}

const priceKeys: PriceKey[] = ['input', 'output', 'cache_read', 'cache_write']

// 将已保存数值转为编辑字符串，空白与合法零价分开；已有配置不被草稿修改。
const makeDraft = (config: ModelPricingConfig | null, options: string[]): EditorDraft => ({
  model: config?.model ?? options[0] ?? '',
  pricingStyle: config?.pricing_style ?? 'openai',
  prices: {
    input: config?.base_prices.input.toString() ?? '',
    output: config?.base_prices.output.toString() ?? '',
    cache_read: config?.base_prices.cache_read.toString() ?? '',
    cache_write: config?.base_prices.cache_write.toString() ?? '',
  },
  modelMultiplier: config?.model_multiplier.toString() ?? '1',
  rules: config?.conditional_multipliers.map((rule, id) => ({
    id, key: rule.key, value: rule.value, multiplier: rule.multiplier.toString(),
  })) ?? [],
})

// 表单接受有限非负数，空串不按 Number 的隐式规则转成免费。
const finiteNonNegative = (raw: string): number | null => {
  if (!raw.trim()) return null
  const parsed = Number(raw)
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : null
}

// 组错误通常来自价格或倍率组合校验，标出组内数值输入并聚焦第一个。
const editableErrorPaths = (path: string, ruleCount: number): string[] => {
  if (path === 'base_prices') return priceKeys.map((key) => `base_prices.${key}`)
  if (path === 'conditional_multipliers') return Array.from({ length: ruleCount }, (_, index) => `conditional_multipliers[${index}].multiplier`)
  const rule = /^conditional_multipliers\[(\d+)\](?:\.(key|value|multiplier))?$/.exec(path)
  if (rule) return Number(rule[1]) < ruleCount ? [`conditional_multipliers[${rule[1]}].${rule[2] ?? 'multiplier'}`] : []
  if (path === 'model' || path === 'pricing_style' || path === 'model_multiplier' ||
    /^base_prices\.(input|output|cache_read|cache_write)$/.test(path)) return [path]
  return []
}

// 校验本地必填与数值后生成完整保存体；权威字段支持及组合费用校验由后端执行。
function validatedConfig(draft: EditorDraft, existing: ModelPricingConfig | null): { config: ModelPricingConfig | null; errors: FieldErrors } {
  const errors: FieldErrors = {}
  if (!draft.model.trim()) errors.model = 'required'
  const prices = {} as PricingBasePrices
  for (const key of priceKeys) {
    const value = finiteNonNegative(draft.prices[key])
    if (value === null) errors[`base_prices.${key}`] = 'invalid'
    else prices[key] = value
  }
  const multiplier = finiteNonNegative(draft.modelMultiplier)
  if (multiplier === null) errors.model_multiplier = 'invalid'
  const rules: PricingConditionalMultiplier[] = []
  const seen = new Set<string>()
  draft.rules.forEach((rule, index) => {
    const key = rule.key.trim().toLowerCase()
    const value = rule.value.trim()
    const factor = finiteNonNegative(rule.multiplier)
    if (!key) errors[`conditional_multipliers[${index}].key`] = 'required'
    if (!value) errors[`conditional_multipliers[${index}].value`] = 'required'
    if (factor === null) errors[`conditional_multipliers[${index}].multiplier`] = 'invalid'
    const pair = `${key}\u0000${value}`
    if (key && value && seen.has(pair)) errors[`conditional_multipliers[${index}].key`] = 'duplicate'
    seen.add(pair)
    if (key && value && factor !== null) rules.push({ key, value, multiplier: factor })
  })
  if (Object.keys(errors).length || multiplier === null) return { config: null, errors }
  return {
    config: {
      model: draft.model.trim(), pricing_style: draft.pricingStyle, base_prices: prices,
      model_multiplier: multiplier, conditional_multipliers: rules,
      // 完整配置写入时保留已有分支，避免编辑基础价覆盖它们。
      branches: existing?.branches ?? [],
    },
    errors,
  }
}

// 单窗编辑默认价格、模型倍率与条件倍率，提交失败才展示字段错误；取消不保存。
export function ModelPricingEditor({ open, initialConfig, modelOptions, onClose, onSave }: ModelPricingEditorProps) {
  const { t } = useTranslation()
  const formId = useId().replaceAll(':', '')
  const formRef = useRef<HTMLFormElement | null>(null)
  const conditionsRef = useRef<HTMLDetailsElement | null>(null)
  const errorRef = useRef<HTMLParagraphElement | null>(null)
  const nextRuleId = useRef(initialConfig?.conditional_multipliers.length ?? 0)
  const focusPath = useRef<string | null>(null)
  const [draft, setDraft] = useState(() => makeDraft(initialConfig, modelOptions))
  const [errors, setErrors] = useState<FieldErrors>({})
  const [requestError, setRequestError] = useState('')
  const [saving, setSaving] = useState(false)
  const [shakeAttempt, setShakeAttempt] = useState(0)

  useEffect(() => {
    if (!focusPath.current) return
    const path = focusPath.current
    focusPath.current = null
    if (path.startsWith('conditional_multipliers')) conditionsRef.current!.open = true
    const field = [...(formRef.current?.querySelectorAll<HTMLElement>('[data-pricing-field]') ?? [])]
      .find((element) => element.dataset.pricingField === path)
    const target = field ?? errorRef.current
    target?.focus()
  }, [errors, requestError, shakeAttempt])

  const clearError = (path: string) => {
    setErrors((current) => {
      if (!current[path]) return current
      const next = { ...current }
      delete next[path]
      return next
    })
    setRequestError('')
  }

  const editPrice = (key: PriceKey, value: string) => {
    setDraft((current) => ({ ...current, prices: { ...current.prices, [key]: value } }))
    clearError(`base_prices.${key}`)
  }

  const editRule = (id: number, field: 'key' | 'value' | 'multiplier', value: string) => {
    const index = draft.rules.findIndex((rule) => rule.id === id)
    setDraft((current) => ({
      ...current,
      rules: current.rules.map((rule) => rule.id === id ? { ...rule, [field]: value } : rule),
    }))
    if (index >= 0) clearError(`conditional_multipliers[${index}].${field}`)
  }

  // 先定位草稿错误，再提交一次完整配置；服务端字段错误映射回控件，网络错误保留草稿。
  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (saving) return
    const validated = validatedConfig(draft, initialConfig)
    if (!validated.config) {
      setRequestError('')
      setErrors(validated.errors)
      setShakeAttempt((attempt) => attempt + 1)
      focusPath.current = Object.keys(validated.errors)[0] ?? null
      return
    }
    // 保存只提交完整配置；写入成功后的列表刷新失败由 hook 单独报告。
    setSaving(true)
    setRequestError('')
    try {
      await onSave(validated.config)
      onClose()
    } catch (failure) {
      const fieldErrors: FieldErrors = {}
      let unmapped = false
      if (failure instanceof ApiError) {
        failure.fields?.forEach((field) => {
          const paths = editableErrorPaths(field.path, draft.rules.length)
          if (paths.length) paths.forEach((path) => { fieldErrors[path] = field.code })
          else unmapped = true
        })
      }
      setErrors(fieldErrors)
      setShakeAttempt((attempt) => attempt + 1)
      focusPath.current = Object.keys(fieldErrors)[0] ?? null
      const conflict = failure instanceof ApiError ? failure.fields?.find((field) => field.branch_ids?.length) : null
      const names = conflict?.branch_ids?.map((id) => initialConfig?.branches.find((branch) => branch.id === id)?.name ?? id)
      const showSummary = !Object.keys(fieldErrors).length || unmapped || Boolean(names?.length)
      setRequestError(showSummary
        ? `${failure instanceof Error ? failure.message : t('usage_stats.pricing_settings_save_failed')}${names?.length ? ` · ${names.join(' / ')}` : ''}`
        : '')
      if (!Object.keys(fieldErrors).length) focusPath.current = 'request-error'
    } finally {
      setSaving(false)
    }
  }

  const field = (path: string, label: string, value: string, onChange: (value: string) => void, type = 'text') => {
    const error = errors[path]
    const descriptionId = `${formId}-${path.replace(/[^a-z0-9]/gi, '-')}-error`
    return <label key={path} className={`${styles.field} ${error ? styles.invalid : ''}`}
      data-shake={error && shakeAttempt ? shakeAttempt % 2 ? 'odd' : 'even' : undefined}>
      <span>{label}</span>
      <input type={type} min={type === 'number' ? 0 : undefined} step={type === 'number' ? 'any' : undefined}
        value={value} onChange={(event) => onChange(event.target.value)} disabled={saving}
        data-pricing-field={path} aria-invalid={Boolean(error)}
        aria-describedby={error ? descriptionId : undefined} />
      {error ? <span id={descriptionId} className={styles.screenReaderOnly}>
        {t(`usage_stats.pricing_settings_error_${['required', 'invalid', 'duplicate', 'conflict'].includes(error) ? error : 'invalid'}`)}
      </span> : null}
    </label>
  }

  return <Modal open={open} title={initialConfig?.model ?? t('usage_stats.pricing_settings_editor_title')}
    width={1120} className={styles.editorModal} closeDisabled={saving} onClose={onClose}>
    <form ref={formRef} className={styles.editorBody} noValidate onSubmit={(event) => void submit(event)}>
      {!initialConfig ? <label className={`${styles.modelField} ${errors.model ? styles.invalid : ''}`}
        data-shake={errors.model && shakeAttempt ? shakeAttempt % 2 ? 'odd' : 'even' : undefined}>
        <span>{t('usage_stats.pricing_settings_model')}</span>
        <select value={draft.model} disabled={saving} data-pricing-field="model" aria-invalid={Boolean(errors.model)}
          aria-describedby={errors.model ? `${formId}-model-error` : undefined}
          onChange={(event) => {
            // 候选模型切换保留价格草稿，但清除上一候选的全部错误反馈。
            setDraft((current) => ({ ...current, model: event.target.value }))
            setErrors({})
            setRequestError('')
            setShakeAttempt(0)
            focusPath.current = null
          }}>
          {modelOptions.map((model) => <option key={model} value={model}>{model}</option>)}
        </select>
        {errors.model ? <span id={`${formId}-model-error`} className={styles.screenReaderOnly}>
          {t(`usage_stats.pricing_settings_error_${errors.model === 'required' ? 'required' : 'invalid'}`)}
        </span> : null}
      </label> : null}

      <div className={styles.sectionHeading}>
        <h3>{t('usage_stats.pricing_settings_default_prices')}</h3>
        <label className={`${styles.styleField} ${errors.pricing_style ? styles.invalid : ''}`}>{t('usage_stats.model_price_style')}
          <select value={draft.pricingStyle} disabled={saving} data-pricing-field="pricing_style" aria-invalid={Boolean(errors.pricing_style)}
            aria-describedby={errors.pricing_style ? `${formId}-pricing-style-error` : undefined}
            onChange={(event) => { setDraft((current) => ({ ...current, pricingStyle: event.target.value as PricingStyle })); clearError('pricing_style') }}>
            <option value="openai">{t('usage_stats.model_price_style_openai')}</option>
            <option value="claude">{t('usage_stats.model_price_style_claude')}</option>
          </select>
          {errors.pricing_style ? <span id={`${formId}-pricing-style-error`} className={styles.screenReaderOnly}>
            {t('usage_stats.pricing_settings_error_invalid')}
          </span> : null}
        </label>
      </div>
      <div className={styles.priceGrid}>
        {priceKeys.map((key) => field(`base_prices.${key}`, t(`usage_stats.pricing_settings_${key}`), draft.prices[key], (value) => editPrice(key, value), 'number'))}
      </div>
      <div className={styles.multiplierRow}>
        {field('model_multiplier', t('usage_stats.pricing_settings_model_multiplier'), draft.modelMultiplier,
          (value) => { setDraft((current) => ({ ...current, modelMultiplier: value })); clearError('model_multiplier') }, 'number')}
        <span>{t('usage_stats.pricing_settings_multiplier_hint')}</span>
      </div>

      <details ref={conditionsRef} className={styles.conditionSection} open>
        <summary className={styles.conditionSummary}>
          <strong>{t('usage_stats.pricing_settings_conditions')}</strong>
          <span>{draft.rules.length}</span>
        </summary>
        <div className={styles.conditionBody}>
        {draft.rules.map((rule, index) => <div className={styles.conditionRow} key={rule.id}>
          {field(`conditional_multipliers[${index}].key`, t('usage_stats.model_price_rules_key'), rule.key,
            (value) => editRule(rule.id, 'key', value))}
          {field(`conditional_multipliers[${index}].value`, t('usage_stats.model_price_rules_value'), rule.value,
            (value) => editRule(rule.id, 'value', value))}
          {field(`conditional_multipliers[${index}].multiplier`, t('usage_stats.model_price_rules_multiplier'), rule.multiplier,
            (value) => editRule(rule.id, 'multiplier', value), 'number')}
          <Button type="button" variant="ghost" appearance="action" disabled={saving} aria-label={t('usage_stats.pricing_settings_delete_condition')}
            onClick={() => { setDraft((current) => ({ ...current, rules: current.rules.filter((item) => item.id !== rule.id) })); setErrors({}) }}>
            {t('common.delete')}
          </Button>
        </div>)}
        <Button type="button" variant="secondary" appearance="action" disabled={saving}
          onClick={() => setDraft((current) => ({ ...current, rules: [...current.rules, { id: nextRuleId.current++, key: '', value: '', multiplier: '1' }] }))}>
          {t('usage_stats.pricing_settings_add_condition')}
        </Button>
        <p className={styles.hint}>{t('usage_stats.pricing_settings_conditions_hint')}</p>
        </div>
      </details>

      {initialConfig?.branches.length ? <p className={styles.branchSummary}>
        {t('usage_stats.pricing_settings_existing_branches', { count: initialConfig.branches.length })}
      </p> : null}
      <p className={styles.historyNotice}>{t('usage_stats.pricing_settings_history_notice')}</p>
      {requestError ? <p ref={errorRef} tabIndex={-1} className={styles.requestError} role="alert" data-pricing-field="request-error">{requestError}</p> : null}
      <div className={styles.editorFooter}>
        <Button type="button" variant="secondary" appearance="action" disabled={saving} onClick={onClose}>{t('common.cancel')}</Button>
        <Button type="submit" appearance="action" loading={saving}>{t('common.save')}</Button>
      </div>
    </form>
  </Modal>
}
