package pricing

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
)

const maxSafeContextToken = int64(9_007_199_254_740_991)

type tokenInterval struct{ min, max int64 }
type minuteInterval struct{ start, end int }

type compiledBranch struct {
	prices  BasePrices
	context tokenInterval
	periods []minuteInterval
	allDay  bool
}

// BranchConflictError 让保存接口能指出同时在上下文和时段上重叠的两条分支。
// FieldPaths 指向双方的两个条件，接口层可映射为结构化字段错误。
type BranchConflictError struct {
	BranchIDs  [2]string
	FieldPaths [4]string
}

func (e *BranchConflictError) Error() string {
	return fmt.Sprintf("pricing branches %q and %q overlap", e.BranchIDs[0], e.BranchIDs[1])
}

// CompilePricingSnapshot 编译完整配置为一个只读快照。调用方传入部署 IANA 时区，
// 在完整候选集合校验成功且持久化事务提交后才发布返回值；运行时不重新校验规则。
func CompilePricingSnapshot(configs []ModelPricingConfig, location *time.Location) (*Snapshot, error) {
	if location == nil {
		return nil, fmt.Errorf("pricing timezone is required")
	}
	// 旧快照编译器仍是唯一模型、条件倍率和最坏费用校验入口；完整合同只扩展分支。
	legacy := make([]ModelConfig, len(configs))
	for index, config := range configs {
		if config.PricingStyle != entities.ModelPricingStyleOpenAI && config.PricingStyle != entities.ModelPricingStyleClaude {
			return nil, fmt.Errorf("model %q: pricing_style must be openai or claude", config.Model)
		}
		if config.ConditionalMultipliers == nil || config.Branches == nil {
			return nil, fmt.Errorf("model %q: conditional_multipliers and branches are required arrays", config.Model)
		}
		multiplier := config.ModelMultiplier
		legacy[index] = ModelConfig{
			Pricing: entities.ModelPriceSetting{
				Model: config.Model, PricingStyle: config.PricingStyle,
				PromptPricePer1M: config.BasePrices.Input, CompletionPricePer1M: config.BasePrices.Output,
				CacheReadPricePer1M: config.BasePrices.CacheRead, CacheWritePricePer1M: config.BasePrices.CacheWrite,
				PriceMultiplier: &multiplier,
			},
			Rules: config.ConditionalMultipliers,
		}
	}
	snapshot, err := CompileSnapshot(legacy)
	if err != nil {
		return nil, err
	}
	snapshot.location = location
	snapshot.pricingConfigs = make([]ModelPricingConfig, 0, len(configs))
	normalizedModels := make(map[string]ModelConfig, len(snapshot.modelConfigs))
	for _, model := range snapshot.modelConfigs {
		normalizedModels[model.Pricing.Model] = model
	}
	for _, config := range configs {
		normalizedModel := strings.TrimSpace(config.Model)
		compiled := snapshot.modelsByName[normalizedModel]
		normalizedBranches := make([]PriceBranch, 0, len(config.Branches))
		seen := make(map[string]struct{}, len(config.Branches))
		for branchIndex, input := range config.Branches {
			candidate, normalized, err := compilePriceBranch(input, compiled.pricing, compiled.rules)
			if err != nil {
				return nil, fmt.Errorf("model %q branches[%d]: %w", normalizedModel, branchIndex, err)
			}
			if _, exists := seen[normalized.ID]; exists {
				return nil, fmt.Errorf("model %q: duplicate branch id %q", normalizedModel, normalized.ID)
			}
			seen[normalized.ID] = struct{}{}
			for earlierIndex, earlier := range compiled.branches {
				if intervalsOverlap(earlier.context, candidate.context) && periodsOverlap(earlier.periods, candidate.periods) {
					return nil, &BranchConflictError{
						BranchIDs: [2]string{normalizedBranches[earlierIndex].ID, normalized.ID},
						FieldPaths: [4]string{
							fmt.Sprintf("branches[%d].context", earlierIndex), fmt.Sprintf("branches[%d].period", earlierIndex),
							fmt.Sprintf("branches[%d].context", branchIndex), fmt.Sprintf("branches[%d].period", branchIndex),
						},
					}
				}
			}
			compiled.branches = append(compiled.branches, candidate)
			normalizedBranches = append(normalizedBranches, normalized)
		}
		snapshot.modelsByName[normalizedModel] = compiled
		model := normalizedModels[normalizedModel]
		snapshot.pricingConfigs = append(snapshot.pricingConfigs, ModelPricingConfig{
			Model: normalizedModel, PricingStyle: model.Pricing.PricingStyle,
			BasePrices: config.BasePrices, ModelMultiplier: *model.Pricing.PriceMultiplier,
			ConditionalMultipliers: cloneRules(model.Rules), Branches: normalizedBranches,
		})
	}
	sort.Slice(snapshot.pricingConfigs, func(i, j int) bool { return snapshot.pricingConfigs[i].Model < snapshot.pricingConfigs[j].Model })
	return snapshot, nil
}

// compilePriceBranch 校验分支单价、上下文与时段，并用同一倍率最坏费用规则检查整请求价格。
func compilePriceBranch(input PriceBranch, modelPricing entities.ModelPriceSetting, rules []compiledRule) (compiledBranch, PriceBranch, error) {
	normalized := clonePriceBranch(input)
	normalized.ID = strings.TrimSpace(normalized.ID)
	normalized.Name = strings.TrimSpace(normalized.Name)
	if normalized.ID == "" || normalized.Name == "" {
		return compiledBranch{}, PriceBranch{}, fmt.Errorf("branch id and name are required")
	}
	context, err := compileContext(normalized.Context)
	if err != nil {
		return compiledBranch{}, PriceBranch{}, err
	}
	periods, err := compilePeriod(normalized.Period)
	if err != nil {
		return compiledBranch{}, PriceBranch{}, err
	}
	pricing := modelPricing
	pricing.PromptPricePer1M = normalized.Prices.Input
	pricing.CompletionPricePer1M = normalized.Prices.Output
	pricing.CacheReadPricePer1M = normalized.Prices.CacheRead
	pricing.CacheWritePricePer1M = normalized.Prices.CacheWrite
	if err := validatePricingNumbers(pricing); err != nil {
		return compiledBranch{}, PriceBranch{}, err
	}
	if err := validateWorstCaseCost(pricing, rules); err != nil {
		return compiledBranch{}, PriceBranch{}, err
	}
	return compiledBranch{prices: normalized.Prices, context: context, periods: periods, allDay: normalized.Period.Type == PeriodAll}, normalized, nil
}

// compileContext 把安全整数条件化为闭区间，便于完整交集校验和 O(1) 事件匹配。
func compileContext(condition ContextCondition) (tokenInterval, error) {
	switch condition.Type {
	case ContextAll:
		if condition.Threshold != nil || condition.Min != nil || condition.Max != nil {
			return tokenInterval{}, fmt.Errorf("all context does not accept thresholds")
		}
		return tokenInterval{0, math.MaxInt64}, nil
	case ContextGT, ContextLTE:
		if condition.Threshold == nil || condition.Min != nil || condition.Max != nil || *condition.Threshold < 0 || *condition.Threshold > maxSafeContextToken {
			return tokenInterval{}, fmt.Errorf("%s context requires a non-negative safe threshold", condition.Type)
		}
		if condition.Type == ContextGT {
			if *condition.Threshold == maxSafeContextToken {
				return tokenInterval{}, fmt.Errorf("gt threshold plus one must be a safe integer")
			}
			return tokenInterval{*condition.Threshold + 1, math.MaxInt64}, nil
		}
		return tokenInterval{0, *condition.Threshold}, nil
	case ContextRange:
		if condition.Threshold != nil || condition.Min == nil || condition.Max == nil || *condition.Min < 0 || *condition.Max > maxSafeContextToken || *condition.Min > *condition.Max {
			return tokenInterval{}, fmt.Errorf("range context requires ordered non-negative safe min and max")
		}
		return tokenInterval{*condition.Min, *condition.Max}, nil
	default:
		return tokenInterval{}, fmt.Errorf("unknown context type %q", condition.Type)
	}
}

// compilePeriod 把每日窗口化为半开分钟段；跨午夜拆成两段且不改变时区。
func compilePeriod(condition PeriodCondition) ([]minuteInterval, error) {
	switch condition.Type {
	case PeriodAll:
		if condition.Start != nil || condition.End != nil {
			return nil, fmt.Errorf("all period does not accept start or end")
		}
		return []minuteInterval{{0, 1440}}, nil
	case PeriodWindow:
		if condition.Start == nil || condition.End == nil {
			return nil, fmt.Errorf("window period requires start and end")
		}
		start, err := parseClockMinute(*condition.Start)
		if err != nil {
			return nil, err
		}
		end, err := parseClockMinute(*condition.End)
		if err != nil {
			return nil, err
		}
		if start == end {
			return nil, fmt.Errorf("window start and end must differ")
		}
		if start < end {
			return []minuteInterval{{start, end}}, nil
		}
		if end == 0 {
			return []minuteInterval{{start, 1440}}, nil
		}
		return []minuteInterval{{start, 1440}, {0, end}}, nil
	default:
		return nil, fmt.Errorf("unknown period type %q", condition.Type)
	}
}

func parseClockMinute(value string) (int, error) {
	if len(value) != 5 || value[2] != ':' || value[0] < '0' || value[0] > '9' || value[1] < '0' || value[1] > '9' || value[3] < '0' || value[3] > '9' || value[4] < '0' || value[4] > '9' {
		return 0, fmt.Errorf("period time %q must be HH:mm", value)
	}
	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	minute := int(value[3]-'0')*10 + int(value[4]-'0')
	if hour > 23 || minute > 59 {
		return 0, fmt.Errorf("period time %q is outside the day", value)
	}
	return hour*60 + minute, nil
}

func intervalsOverlap(left, right tokenInterval) bool {
	return left.min <= right.max && right.min <= left.max
}

func periodsOverlap(left, right []minuteInterval) bool {
	for _, a := range left {
		for _, b := range right {
			if a.start < b.end && b.start < a.end {
				return true
			}
		}
	}
	return false
}

// matches 按已归一化输入量和 CPA 事件时间判断分支；全天分支不需要时间字段。
func (b compiledBranch) matches(subject CostSubject, location *time.Location) bool {
	inputTokens := max(subject.Tokens.InputTokens, 0)
	if inputTokens < b.context.min || inputTokens > b.context.max {
		return false
	}
	if b.allDay {
		return true
	}
	local := subject.Timestamp.In(location)
	minute := local.Hour()*60 + local.Minute()
	for _, period := range b.periods {
		if minute >= period.start && minute < period.end {
			return true
		}
	}
	return false
}

func clonePriceBranch(input PriceBranch) PriceBranch {
	result := input
	if input.Context.Threshold != nil {
		value := *input.Context.Threshold
		result.Context.Threshold = &value
	}
	if input.Context.Min != nil {
		value := *input.Context.Min
		result.Context.Min = &value
	}
	if input.Context.Max != nil {
		value := *input.Context.Max
		result.Context.Max = &value
	}
	if input.Period.Start != nil {
		value := *input.Period.Start
		result.Period.Start = &value
	}
	if input.Period.End != nil {
		value := *input.Period.End
		result.Period.End = &value
	}
	return result
}

func clonePricingConfig(input ModelPricingConfig) ModelPricingConfig {
	result := input
	result.ConditionalMultipliers = cloneRules(input.ConditionalMultipliers)
	result.Branches = make([]PriceBranch, len(input.Branches))
	for index := range input.Branches {
		result.Branches[index] = clonePriceBranch(input.Branches[index])
	}
	return result
}

// PricingModelConfigs 返回稳定排序的完整配置深拷贝，不能修改已发布快照。
func (s *Snapshot) PricingModelConfigs() []ModelPricingConfig {
	if s == nil {
		return []ModelPricingConfig{}
	}
	result := make([]ModelPricingConfig, len(s.pricingConfigs))
	for index := range s.pricingConfigs {
		result[index] = clonePricingConfig(s.pricingConfigs[index])
	}
	return result
}

// PricingModelConfig 返回指定模型的完整配置深拷贝。
func (s *Snapshot) PricingModelConfig(model string) (ModelPricingConfig, bool) {
	if s != nil {
		for _, config := range s.pricingConfigs {
			if config.Model == strings.TrimSpace(model) {
				return clonePricingConfig(config), true
			}
		}
	}
	return ModelPricingConfig{}, false
}
