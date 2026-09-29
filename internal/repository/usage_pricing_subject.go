package repository

import (
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/pricing"
)

// UsageEventCostSubject 将待入库或已入库的归一化事件的九维和四类 Token 映射到统一计价输入，
// 并保留 CPA 事件时间供时段分支使用；此处不再执行 Token 归一化。
func UsageEventCostSubject(event entities.UsageEvent) pricing.CostSubject {
	modelAlias := ""
	if event.ModelAlias != nil {
		modelAlias = *event.ModelAlias
	}
	subject := newUsagePricingCostSubject(
		event.APIGroupKey,
		event.Model,
		event.AuthIndex,
		modelAlias,
		event.ServiceTier,
		event.ResponseServiceTier,
		event.ReasoningEffort,
		event.Endpoint,
		event.ExecutorType,
		event.InputTokens,
		event.OutputTokens,
		event.CacheReadTokens,
		event.CacheCreationTokens,
	)
	// 分支时段使用 CPA 已存事件时间，不能使用处理时钟或本地接收时间。
	subject.Timestamp = event.Timestamp
	return subject
}

func UsageOverviewHourlyCostSubject(row entities.UsageOverviewHourlyStat) pricing.CostSubject {
	return newUsagePricingCostSubject(
		row.APIGroupKey,
		row.Model,
		row.AuthIndex,
		row.ModelAlias,
		row.ServiceTier,
		row.ResponseServiceTier,
		row.ReasoningEffort,
		row.Endpoint,
		row.ExecutorType,
		row.InputTokens,
		row.OutputTokens,
		row.CacheReadTokens,
		row.CacheCreationTokens,
	)
}

func UsageOverviewDailyCostSubject(row entities.UsageOverviewDailyStat) pricing.CostSubject {
	return newUsagePricingCostSubject(
		row.APIGroupKey,
		row.Model,
		row.AuthIndex,
		row.ModelAlias,
		row.ServiceTier,
		row.ResponseServiceTier,
		row.ReasoningEffort,
		row.Endpoint,
		row.ExecutorType,
		row.InputTokens,
		row.OutputTokens,
		row.CacheReadTokens,
		row.CacheCreationTokens,
	)
}

func newUsagePricingCostSubject(
	apiGroupKey, model, authIndex, modelAlias, serviceTier, responseServiceTier, reasoningEffort, endpoint, executorType string,
	inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int64,
) pricing.CostSubject {
	return pricing.NewCostSubject(pricing.UsageDimensions{
		APIGroupKey:         apiGroupKey,
		Model:               model,
		AuthIndex:           authIndex,
		ModelAlias:          modelAlias,
		ServiceTier:         serviceTier,
		ResponseServiceTier: responseServiceTier,
		ReasoningEffort:     reasoningEffort,
		Endpoint:            endpoint,
		ExecutorType:        executorType,
	}, helper.UsageTokenCostInput{
		InputTokens:         inputTokens,
		OutputTokens:        outputTokens,
		CacheReadTokens:     cacheReadTokens,
		CacheCreationTokens: cacheCreationTokens,
	})
}
