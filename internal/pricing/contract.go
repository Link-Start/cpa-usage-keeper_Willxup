package pricing

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// BasePrices 是四类 Token 的必填单价，单位 USD／1M Token；0 表示明确的免费价格。
type BasePrices struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
}

// UnmarshalJSON 区分缺失／null 与合法的零单价；数值范围由完整配置编译器校验。
func (p *BasePrices) UnmarshalJSON(data []byte) error {
	var wire struct {
		Input      *float64 `json:"input"`
		Output     *float64 `json:"output"`
		CacheRead  *float64 `json:"cache_read"`
		CacheWrite *float64 `json:"cache_write"`
	}
	if err := decodePricingObject(data, &wire); err != nil {
		return err
	}
	if wire.Input == nil || wire.Output == nil || wire.CacheRead == nil || wire.CacheWrite == nil {
		return fmt.Errorf("base_prices requires input, output, cache_read and cache_write")
	}
	*p = BasePrices{Input: *wire.Input, Output: *wire.Output, CacheRead: *wire.CacheRead, CacheWrite: *wire.CacheWrite}
	return nil
}

// ModelPricingConfig 是保存和展示共用的完整模型配置；空数组是清空，缺失或 null 不代表清空。
type ModelPricingConfig struct {
	Model                  string        `json:"model"`
	PricingStyle           string        `json:"pricing_style"`
	BasePrices             BasePrices    `json:"base_prices"`
	ModelMultiplier        float64       `json:"model_multiplier"`
	ConditionalMultipliers []RuleConfig  `json:"conditional_multipliers"`
	Branches               []PriceBranch `json:"branches"`
}

// UnmarshalJSON 保证完整保存不会将省略的字段悄悄转换为免费、倍率零或清空规则。
func (c *ModelPricingConfig) UnmarshalJSON(data []byte) error {
	var wire struct {
		Model                  *string        `json:"model"`
		PricingStyle           *string        `json:"pricing_style"`
		BasePrices             *BasePrices    `json:"base_prices"`
		ModelMultiplier        *float64       `json:"model_multiplier"`
		ConditionalMultipliers *[]RuleConfig  `json:"conditional_multipliers"`
		Branches               *[]PriceBranch `json:"branches"`
	}
	if err := decodePricingObject(data, &wire); err != nil {
		return err
	}
	if wire.Model == nil || wire.PricingStyle == nil || wire.BasePrices == nil || wire.ModelMultiplier == nil || wire.ConditionalMultipliers == nil || wire.Branches == nil {
		return fmt.Errorf("model pricing config requires model, pricing_style, base_prices, model_multiplier, conditional_multipliers and branches")
	}
	*c = ModelPricingConfig{
		Model: *wire.Model, PricingStyle: *wire.PricingStyle, BasePrices: *wire.BasePrices,
		ModelMultiplier: *wire.ModelMultiplier, ConditionalMultipliers: *wire.ConditionalMultipliers, Branches: *wire.Branches,
	}
	return nil
}

// PriceBranch 对一条请求选出整组单价；非默认分支按上下文与每日时段共同匹配。
type PriceBranch struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Context ContextCondition `json:"context"`
	Period  PeriodCondition  `json:"period"`
	Prices  BasePrices       `json:"prices"`
}

// UnmarshalJSON 拒绝缺失的分支条件或价格，具体区间与冲突留给配置编译器检查。
func (b *PriceBranch) UnmarshalJSON(data []byte) error {
	var wire struct {
		ID      *string           `json:"id"`
		Name    *string           `json:"name"`
		Context *ContextCondition `json:"context"`
		Period  *PeriodCondition  `json:"period"`
		Prices  *BasePrices       `json:"prices"`
	}
	if err := decodePricingObject(data, &wire); err != nil {
		return err
	}
	if wire.ID == nil || wire.Name == nil || wire.Context == nil || wire.Period == nil || wire.Prices == nil {
		return fmt.Errorf("price branch requires id, name, context, period and prices")
	}
	*b = PriceBranch{ID: *wire.ID, Name: *wire.Name, Context: *wire.Context, Period: *wire.Period, Prices: *wire.Prices}
	return nil
}

type ContextConditionType string

const (
	ContextAll   ContextConditionType = "all"
	ContextGT    ContextConditionType = "gt"
	ContextLTE   ContextConditionType = "lte"
	ContextRange ContextConditionType = "range"
)

// ContextCondition 用归一化 input_tokens 选整条请求的价格；可选数值用指针保留阈值 0。
type ContextCondition struct {
	Type      ContextConditionType `json:"type"`
	Threshold *int64               `json:"threshold,omitempty"`
	Min       *int64               `json:"min,omitempty"`
	Max       *int64               `json:"max,omitempty"`
}

// UnmarshalJSON 只接受当前 type 对应的字段；安全整数及区间顺序由编译器校验。
func (c *ContextCondition) UnmarshalJSON(data []byte) error {
	var present map[string]json.RawMessage
	if err := json.Unmarshal(data, &present); err != nil {
		return err
	}
	var wire struct {
		Type      *ContextConditionType `json:"type"`
		Threshold *int64                `json:"threshold"`
		Min       *int64                `json:"min"`
		Max       *int64                `json:"max"`
	}
	if err := decodePricingObject(data, &wire); err != nil {
		return err
	}
	if wire.Type == nil {
		return fmt.Errorf("context type is required")
	}
	switch *wire.Type {
	case ContextAll:
		if hasPricingField(present, "threshold", "min", "max") {
			return fmt.Errorf("all context does not accept thresholds")
		}
	case ContextGT, ContextLTE:
		if wire.Threshold == nil || hasPricingField(present, "min", "max") {
			return fmt.Errorf("%s context requires only threshold", *wire.Type)
		}
	case ContextRange:
		if hasPricingField(present, "threshold") || wire.Min == nil || wire.Max == nil {
			return fmt.Errorf("range context requires only min and max")
		}
	default:
		return fmt.Errorf("unknown context type %q", *wire.Type)
	}
	*c = ContextCondition{Type: *wire.Type, Threshold: wire.Threshold, Min: wire.Min, Max: wire.Max}
	return nil
}

type PeriodConditionType string

const (
	PeriodAll    PeriodConditionType = "all"
	PeriodWindow PeriodConditionType = "window"
)

// PeriodCondition 的 window 按部署时区解释已存 CPA 时间；start 包含，end 不包含。
type PeriodCondition struct {
	Type  PeriodConditionType `json:"type"`
	Start *string             `json:"start,omitempty"`
	End   *string             `json:"end,omitempty"`
}

// UnmarshalJSON 只接受当前 type 对应的字段；HH:mm 与跨午夜语义由编译器校验。
func (p *PeriodCondition) UnmarshalJSON(data []byte) error {
	var present map[string]json.RawMessage
	if err := json.Unmarshal(data, &present); err != nil {
		return err
	}
	var wire struct {
		Type  *PeriodConditionType `json:"type"`
		Start *string              `json:"start"`
		End   *string              `json:"end"`
	}
	if err := decodePricingObject(data, &wire); err != nil {
		return err
	}
	if wire.Type == nil {
		return fmt.Errorf("period type is required")
	}
	switch *wire.Type {
	case PeriodAll:
		if hasPricingField(present, "start", "end") {
			return fmt.Errorf("all period does not accept start or end")
		}
	case PeriodWindow:
		if wire.Start == nil || wire.End == nil {
			return fmt.Errorf("window period requires start and end")
		}
	default:
		return fmt.Errorf("unknown period type %q", *wire.Type)
	}
	*p = PeriodCondition{Type: *wire.Type, Start: wire.Start, End: wire.End}
	return nil
}

// FeeResult 是事件持久化的唯一计价结果；缺价保留 0 金额及 false 可用性。
type FeeResult struct {
	TotalCostUSD float64 `json:"total_cost_usd"`
	Available    bool    `json:"cost_available"`
}

// decodePricingObject 拒绝未知字段，防止完整配置写入时静默丢失客户端发送的条件。
func decodePricingObject(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func hasPricingField(fields map[string]json.RawMessage, names ...string) bool {
	for _, name := range names {
		if _, ok := fields[name]; ok {
			return true
		}
	}
	return false
}
