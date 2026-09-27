package pricing

import (
	"time"

	"cpa-usage-keeper/internal/helper"
)

// CostSubject 是所有 usage 来源进入计价领域的唯一固定输入。
type CostSubject struct {
	Dimensions UsageDimensions
	Tokens     helper.UsageTokenCostInput
	// Timestamp 是已存 CPA 事件时间，供每日时段分支匹配。
	Timestamp time.Time
}

func NewCostSubject(dimensions UsageDimensions, tokens helper.UsageTokenCostInput) CostSubject {
	return CostSubject{
		Dimensions: canonicalizeUsageDimensions(dimensions),
		Tokens:     tokens,
	}
}

type CostResult struct {
	Cost           helper.UsageTokenCostBreakdown
	Available      bool
	PricingStyle   string
	MatchedModel   string
	MatchedBy      string
	RuleMultiplier float64
}

// Resolver 在创建时固定绑定一个 Snapshot，确保单个响应不会混用新旧价格。
type Resolver struct {
	snapshot *Snapshot
}

func (r Resolver) ActiveFields() ActiveFields {
	if r.snapshot == nil {
		return 0
	}
	return r.snapshot.activeFields
}

func (r Resolver) Calculate(subject CostSubject) CostResult {
	model, matchedModel, matchedBy, found := r.matchModel(subject.Dimensions)
	if !found {
		return CostResult{
			Available:      !helper.UsageTokenInputRequiresPricing(subject.Tokens),
			RuleMultiplier: 1,
		}
	}

	// 分支在编译时已校验互斥；这里仅按归一化输入量及已存 CPA 时间挑一组整请求单价。
	selected := model.pricing
	for _, branch := range model.branches {
		if !branch.matches(subject, r.snapshot.location) {
			continue
		}
		selected.PromptPricePer1M = branch.prices.Input
		selected.CompletionPricePer1M = branch.prices.Output
		selected.CacheReadPricePer1M = branch.prices.CacheRead
		selected.CacheWritePricePer1M = branch.prices.CacheWrite
		break
	}
	breakdown := helper.CalculateUsageTokenCostBreakdown(subject.Tokens, selected)
	ruleMultiplier := 1.0
	if model.pricing.PriceMultiplier == nil || *model.pricing.PriceMultiplier != 0 {
		ruleMultiplier = matchingRuleMultiplier(model.rules, subject.Dimensions)
		breakdown = helper.ScaleUsageTokenCostBreakdown(breakdown, ruleMultiplier)
	}
	return CostResult{
		Cost:           breakdown,
		Available:      true,
		PricingStyle:   model.pricing.PricingStyle,
		MatchedModel:   matchedModel,
		MatchedBy:      matchedBy,
		RuleMultiplier: ruleMultiplier,
	}
}

// CalculateFee 供事件入库和显式重算写回使用，只暴露总额及缺价可用性。
// 它与旧读取调用共用 Calculate 的模型、分支、倍率和四类 Token 公式。
func (r Resolver) CalculateFee(subject CostSubject) FeeResult {
	result := r.Calculate(subject)
	return FeeResult{TotalCostUSD: result.Cost.TotalCostUSD, Available: result.Available}
}

func (r Resolver) matchModel(dimensions UsageDimensions) (compiledModel, string, string, bool) {
	if r.snapshot == nil {
		return compiledModel{}, "", "", false
	}
	if model, ok := r.snapshot.modelsByName[dimensions.Model]; ok {
		return model, dimensions.Model, "model", true
	}
	if model, ok := r.snapshot.modelsByName[dimensions.ModelAlias]; ok {
		return model, dimensions.ModelAlias, "model_alias", true
	}
	return compiledModel{}, "", "", false
}

func matchingRuleMultiplier(rules []compiledRule, dimensions UsageDimensions) float64 {
	multiplier := 1.0
	for _, rule := range rules {
		if dimensions.Value(rule.field) != rule.value {
			continue
		}
		if rule.multiplier == 0 {
			return 0
		}
		multiplier *= rule.multiplier
	}
	return multiplier
}
