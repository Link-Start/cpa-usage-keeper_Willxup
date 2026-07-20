package test

import (
	"context"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/timeutil"

	"gorm.io/gorm"
)

func TestAggregateUsageActivityStatsUsesIndependentCheckpointAndCanonicalTokens(t *testing.T) {
	// 准备：固定项目时区和聚合 now，让 retention 与 daily 边界完全可重复。
	previousLocal := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = previousLocal })
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

	// fresh database 同时包含现有 Overview 表和新的 Activity 表。
	db := openTestDatabase(t)
	events := []entities.UsageEvent{
		{EventKey: "activity-1", APIGroupKey: " provider-a ", Model: "model-a", Timestamp: now.Add(-time.Hour), Failed: false, InputTokens: 100, OutputTokens: 20, ReasoningTokens: 5, CachedTokens: 900, CacheReadTokens: 10, CacheCreationTokens: 3, TotalTokens: 138},
		{EventKey: "activity-2", APIGroupKey: "provider-a", Model: "model-a", Timestamp: now.Add(-30 * time.Minute), Failed: true, InputTokens: 200, OutputTokens: 30, ReasoningTokens: 6, CachedTokens: 800, CacheReadTokens: 20, CacheCreationTokens: 4, TotalTokens: 260},
		{EventKey: "activity-3", APIGroupKey: "provider-a", Model: "model-a", Timestamp: now.Add(-4 * 24 * time.Hour), Failed: false, InputTokens: 300, OutputTokens: 40, ReasoningTokens: 7, CachedTokens: 700, CacheReadTokens: 30, CacheCreationTokens: 5, TotalTokens: 382},
		{EventKey: "activity-4", APIGroupKey: "provider-a", Model: "model-a", Timestamp: now.Add(-10 * 24 * time.Hour), Failed: false, InputTokens: 400, OutputTokens: 50, ReasoningTokens: 8, CachedTokens: 600, CacheReadTokens: 40, CacheCreationTokens: 6, TotalTokens: 504},
	}
	if _, _, err := repository.InsertUsageEvents(db, events); err != nil {
		t.Fatalf("insert usage events: %v", err)
	}

	// Overview checkpoint 使用无关 sentinel，证明 Activity 不读取或推进它。
	overviewCheckpoint := entities.UsageOverviewAggregationCheckpoint{Name: "overview", LastAggregatedUsageEventID: 777}
	if err := db.Create(&overviewCheckpoint).Error; err != nil {
		t.Fatalf("seed overview checkpoint: %v", err)
	}

	// 执行：先检查独立 pending 状态，再通过完整入口追平所有 Activity batches。
	pending, err := repository.HasPendingUsageActivityAggregation(context.Background(), db)
	if err != nil {
		t.Fatalf("HasPendingUsageActivityAggregation returned error: %v", err)
	}
	// 断言：已有 raw events 且 Activity cursor 不存在时必须报告 pending。
	if !pending {
		t.Fatal("expected activity aggregation to be pending")
	}

	// 执行：完整追平入口循环处理所有 Activity batches。
	if err := repository.AggregateUsageActivityStats(context.Background(), db, now); err != nil {
		t.Fatalf("AggregateUsageActivityStats returned error: %v", err)
	}

	// 断言：三个短 grain 分别应用 3d/8d/31d gate，daily 始终累计全部 retained raw events。
	assertRepositoryUsageActivityTotals(t, db, entities.UsageActivityGrainShort, repositoryUsageActivityTotals{Success: 1, Failure: 1, Input: 300, Output: 50, Reasoning: 11, CacheRead: 30, CacheCreation: 7, Total: 398})
	assertRepositoryUsageActivityTotals(t, db, entities.UsageActivityGrainMedium, repositoryUsageActivityTotals{Success: 2, Failure: 1, Input: 600, Output: 90, Reasoning: 18, CacheRead: 60, CacheCreation: 12, Total: 780})
	assertRepositoryUsageActivityTotals(t, db, entities.UsageActivityGrainLong, repositoryUsageActivityTotals{Success: 3, Failure: 1, Input: 1000, Output: 140, Reasoning: 26, CacheRead: 100, CacheCreation: 18, Total: 1284})
	assertRepositoryUsageActivityTotals(t, db, entities.UsageActivityGrainDaily, repositoryUsageActivityTotals{Success: 3, Failure: 1, Input: 1000, Output: 140, Reasoning: 26, CacheRead: 100, CacheCreation: 18, Total: 1284})

	// Activity checkpoint 必须独立推进到最后一个 event ID。
	var activityCheckpoint entities.UsageActivityAggregationCheckpoint
	if err := db.Where("name = ?", "activity").Take(&activityCheckpoint).Error; err != nil {
		t.Fatalf("load activity checkpoint: %v", err)
	}
	if activityCheckpoint.LastAggregatedUsageEventID != 4 {
		t.Fatalf("expected activity checkpoint 4, got %+v", activityCheckpoint)
	}
	var unchangedOverview entities.UsageOverviewAggregationCheckpoint
	if err := db.Where("name = ?", "overview").Take(&unchangedOverview).Error; err != nil {
		t.Fatalf("load overview checkpoint: %v", err)
	}
	if unchangedOverview.LastAggregatedUsageEventID != 777 {
		t.Fatalf("activity changed overview checkpoint: %+v", unchangedOverview)
	}

	// 执行：第二次追平并重新读取 pending 状态，验证幂等终态。
	if err := repository.AggregateUsageActivityStats(context.Background(), db, now); err != nil {
		t.Fatalf("rerun AggregateUsageActivityStats: %v", err)
	}
	assertRepositoryUsageActivityTotals(t, db, entities.UsageActivityGrainDaily, repositoryUsageActivityTotals{Success: 3, Failure: 1, Input: 1000, Output: 140, Reasoning: 26, CacheRead: 100, CacheCreation: 18, Total: 1284})

	// 断言：追平后 pending 检查必须变为 false。
	pending, err = repository.HasPendingUsageActivityAggregation(context.Background(), db)
	if err != nil {
		t.Fatalf("second HasPendingUsageActivityAggregation returned error: %v", err)
	}
	if pending {
		t.Fatal("expected activity aggregation to be caught up")
	}
}

func TestCleanupUsageActivityStatsUsesPerGrainBucketEndRetention(t *testing.T) {
	// 准备：固定 cleanup now，分别构造刚过期和仍保留的稀疏行。
	previousLocal := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = previousLocal })
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	db := openTestDatabase(t)

	// daily-old 必须永久保留，其余 old-* 必须按各自 grain 删除。
	rows := []entities.UsageActivityStat{
		usageActivityCleanupRow(entities.UsageActivityGrainShort, "short-old", now.Add(-4*24*time.Hour), now.Add(-4*24*time.Hour+time.Minute)),
		usageActivityCleanupRow(entities.UsageActivityGrainShort, "short-fresh", now.Add(-2*24*time.Hour), now.Add(-2*24*time.Hour+time.Minute)),
		usageActivityCleanupRow(entities.UsageActivityGrainMedium, "medium-old", now.Add(-9*24*time.Hour), now.Add(-9*24*time.Hour+time.Minute)),
		usageActivityCleanupRow(entities.UsageActivityGrainMedium, "medium-fresh", now.Add(-7*24*time.Hour), now.Add(-7*24*time.Hour+time.Minute)),
		usageActivityCleanupRow(entities.UsageActivityGrainLong, "long-old", now.Add(-32*24*time.Hour), now.Add(-32*24*time.Hour+time.Minute)),
		usageActivityCleanupRow(entities.UsageActivityGrainLong, "long-fresh", now.Add(-30*24*time.Hour), now.Add(-30*24*time.Hour+time.Minute)),
		usageActivityCleanupRow(entities.UsageActivityGrainDaily, "daily-old", now.Add(-1000*24*time.Hour), now.Add(-999*24*time.Hour)),
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed activity cleanup rows: %v", err)
	}

	// 执行：运行一次按 grain+bucket_end 的 Activity cleanup。
	if err := repository.CleanupUsageActivityStats(db, now); err != nil {
		t.Fatalf("CleanupUsageActivityStats returned error: %v", err)
	}
	// 断言：只保留三个 fresh 短 grain 行和永久 daily 行。
	var remaining []string
	if err := db.Model(&entities.UsageActivityStat{}).Order("api_group_key asc").Pluck("api_group_key", &remaining).Error; err != nil {
		t.Fatalf("load remaining activity rows: %v", err)
	}
	want := []string{"daily-old", "long-fresh", "medium-fresh", "short-fresh"}
	if len(remaining) != len(want) {
		t.Fatalf("unexpected remaining activity rows: got=%v want=%v", remaining, want)
	}
	for index := range want {
		if remaining[index] != want[index] {
			t.Fatalf("unexpected remaining activity rows: got=%v want=%v", remaining, want)
		}
	}
}

type repositoryUsageActivityTotals struct {
	Success       int64
	Failure       int64
	Input         int64
	Output        int64
	Reasoning     int64
	CacheRead     int64
	CacheCreation int64
	Total         int64
}

func assertRepositoryUsageActivityTotals(t *testing.T, db interface {
	Model(value any) *gorm.DB
}, grain entities.UsageActivityGrain, want repositoryUsageActivityTotals) {
	// 此 helper 汇总稀疏 bucket，只比较每个 grain 的最终累计效果。
	t.Helper()
	var got repositoryUsageActivityTotals
	if err := db.Model(&entities.UsageActivityStat{}).
		Select(`COALESCE(SUM(success_count), 0) AS success,
			COALESCE(SUM(failure_count), 0) AS failure,
			COALESCE(SUM(input_tokens), 0) AS input,
			COALESCE(SUM(output_tokens), 0) AS output,
			COALESCE(SUM(reasoning_tokens), 0) AS reasoning,
			COALESCE(SUM(cache_read_tokens), 0) AS cache_read,
			COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation,
			COALESCE(SUM(total_tokens), 0) AS total`).
		Where("grain = ?", grain).
		Scan(&got).Error; err != nil {
		t.Fatalf("sum %s activity rows: %v", grain, err)
	}
	if got != want {
		t.Fatalf("unexpected %s totals: got=%+v want=%+v", grain, got, want)
	}
}

func usageActivityCleanupRow(grain entities.UsageActivityGrain, apiGroupKey string, start, end time.Time) entities.UsageActivityStat {
	// cleanup fixture 只需要唯一边界和一个非零请求计数。
	return entities.UsageActivityStat{Grain: grain, BucketStart: timeutil.NormalizeStorageTime(start), BucketEnd: timeutil.NormalizeStorageTime(end), APIGroupKey: apiGroupKey, SuccessCount: 1}
}
