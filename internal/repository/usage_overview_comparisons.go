package repository

import (
	"fmt"
	"math"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository/dto"
	"gorm.io/gorm"
)

// applyUsageEventToComparisonOnly 把窄边界事件的已存费用同时归到模型、Key 与已知身份。
func applyUsageEventToComparisonOnly(comparisons *dto.UsageOverviewComparisonsRecord, event entities.UsageEvent, identityLookup analysisIdentityLookup) error {
	cost, available, err := usageOverviewStoredEventCost(event)
	if err != nil {
		return err
	}
	failed := int64(0)
	if event.Failed {
		failed = 1
	}
	row := dto.UsageComparisonItemRecord{Requests: 1, Failures: failed, InputTokens: event.InputTokens, OutputTokens: event.OutputTokens, CacheReadTokens: event.CacheReadTokens, CacheCreationTokens: event.CacheCreationTokens, ReasoningTokens: event.ReasoningTokens, TotalTokens: event.TotalTokens, CostUSD: cost, CostAvailable: available}
	applyUsageOverviewComparison(comparisons, event.Model, event.APIGroupKey, row)
	applyUsageOverviewIdentityComparison(comparisons, identityLookup, event.AuthIndex, row)
	return nil
}

// 同一比较行的已存费用同时归入模型和 API Key 两个维度。
func applyUsageOverviewComparison(comparisons *dto.UsageOverviewComparisonsRecord, model, apiKey string, row dto.UsageComparisonItemRecord) {
	addUsageOverviewComparison(comparisons.Models, normalizeUsageOverviewDimension(model), row)
	addUsageOverviewComparison(comparisons.APIKeys, normalizeUsageOverviewDimension(apiKey), row)
}

func applyUsageOverviewIdentityComparison(comparisons *dto.UsageOverviewComparisonsRecord, identityLookup analysisIdentityLookup, authIndex string, row dto.UsageComparisonItemRecord) {
	if identity, ok := identityLookup.find(entities.UsageIdentityAuthTypeAuthFile, strings.TrimSpace(authIndex)); ok {
		row.Label = identity.label
		addUsageOverviewComparison(comparisons.AuthFiles, identity.identity, row)
	}
	if identity, ok := identityLookup.find(entities.UsageIdentityAuthTypeAIProvider, strings.TrimSpace(authIndex)); ok {
		row.Label = identity.label
		addUsageOverviewComparison(comparisons.AIProviders, identity.identity, row)
	}
}

func addUsageOverviewComparison(items map[string]*dto.UsageComparisonItemRecord, key string, row dto.UsageComparisonItemRecord) {
	item := items[key]
	if item == nil {
		item = &dto.UsageComparisonItemRecord{Key: key, Label: row.Label, CostAvailable: true}
		items[key] = item
	}
	if item.Label == "" && row.Label != "" {
		item.Label = row.Label
	}
	item.Requests += row.Requests
	item.Failures += row.Failures
	item.InputTokens += row.InputTokens
	item.OutputTokens += row.OutputTokens
	item.CacheReadTokens += row.CacheReadTokens
	item.CacheCreationTokens += row.CacheCreationTokens
	item.ReasoningTokens += row.ReasoningTokens
	item.TotalTokens += row.TotalTokens
	item.CostUSD += row.CostUSD
	item.CostAvailable = item.CostAvailable && row.CostAvailable
}

// comparison-only 使用无时间桶的独立 rollup projection。
// 比较查询复用范围规划，边界事件由调用方读取一次并补入比较结果。
func loadAndApplyUsageOverviewStats(overview *dto.UsageOverviewRecord, db *gorm.DB, filter dto.UsageQueryFilter, start, end time.Time, grain string, bucketByDay bool) error {
	if filter.ComparisonOnly {
		rows, err := loadUsageOverviewComparisonProjection(db, filter, start, end, grain)
		if err != nil {
			return err
		}
		var identityLookup analysisIdentityLookup
		authIndexes := make([]string, 0, len(rows))
		seenAuthIndexes := make(map[string]struct{}, len(rows))
		for _, row := range rows {
			if authIndex := strings.TrimSpace(row.AuthIndex); authIndex != "" {
				if _, seen := seenAuthIndexes[authIndex]; !seen {
					authIndexes = append(authIndexes, authIndex)
					seenAuthIndexes[authIndex] = struct{}{}
				}
			}
		}
		identityLookup, err = loadAnalysisIdentityLookup(db, authIndexes)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.MissingCostCount != 0 || row.CostUSD == nil || row.UnavailableCostCount == nil {
				return fmt.Errorf("usage overview %s comparison has unbackfilled cost", grain)
			}
			if math.IsNaN(*row.CostUSD) || math.IsInf(*row.CostUSD, 0) {
				return fmt.Errorf("usage overview %s comparison has non-finite cost", grain)
			}
			comparison := dto.UsageComparisonItemRecord{Requests: row.RequestCount, Failures: row.FailureCount, InputTokens: row.InputTokens, OutputTokens: row.OutputTokens, CacheReadTokens: row.CacheReadTokens, CacheCreationTokens: row.CacheCreationTokens, ReasoningTokens: row.ReasoningTokens, TotalTokens: row.TotalTokens, CostUSD: *row.CostUSD, CostAvailable: *row.UnavailableCostCount == 0}
			applyUsageOverviewComparison(overview.Comparisons, row.Model, row.APIGroupKey, comparison)
			applyUsageOverviewIdentityComparison(overview.Comparisons, identityLookup, row.AuthIndex, comparison)
		}
		return nil
	}
	var model any = &entities.UsageOverviewHourlyStat{}
	if grain == "daily" {
		model = &entities.UsageOverviewDailyStat{}
	}
	rows, err := loadUsageOverviewStatProjection(db.Model(model), filter, start, end, grain)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := applyUsageOverviewStatToOverview(overview, row, bucketByDay); err != nil {
			return err
		}
	}
	return nil
}
