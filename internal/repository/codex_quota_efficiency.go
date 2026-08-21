package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	repositorydto "cpa-usage-keeper/internal/repository/dto"
	"cpa-usage-keeper/internal/timeutil"

	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

const (
	// codexQuotaEfficiencyBatchCycleLimit 控制单条 CASE 中周期分支数量，短窗口一个月也不会生成无限 SQL。
	codexQuotaEfficiencyBatchCycleLimit = 24
	// codexQuotaEfficiencyBatchTransitionLimit 控制单条 CASE 中百分比变化分支数量，同时保证一个周期绝不拆开查询。
	codexQuotaEfficiencyBatchTransitionLimit = 400
)

// codexQuotaEfficiencyCycleWork 把公开周期 DTO 与仅供本次 SQL 分类使用的查询边界放在一起。
type codexQuotaEfficiencyCycleWork struct {
	// record 是后续同时累加周期总量和区间量的唯一对象。
	record *repositorydto.CodexQuotaEfficiencyCycle
	// queryEnd 对已结束周期等于 reset_at，对当前周期固定截到 GeneratedAt。
	queryEnd time.Time
}

// codexQuotaEfficiencyAggregateRow 承接 SQL 按周期、可选区间和 pricing 维度聚合后的少量行。
type codexQuotaEfficiencyAggregateRow struct {
	CycleID             int64  `gorm:"column:cycle_id"`
	TransitionID        *int64 `gorm:"column:transition_id"`
	APIGroupKey         string `gorm:"column:api_group_key"`
	Model               string `gorm:"column:model"`
	AuthIndex           string `gorm:"column:auth_index"`
	ModelAlias          string `gorm:"column:model_alias"`
	ServiceTier         string `gorm:"column:service_tier"`
	ResponseServiceTier string `gorm:"column:response_service_tier"`
	ReasoningEffort     string `gorm:"column:reasoning_effort"`
	Endpoint            string `gorm:"column:endpoint"`
	ExecutorType        string `gorm:"column:executor_type"`
	Requests            int64  `gorm:"column:requests"`
	SuccessfulRequests  int64  `gorm:"column:successful_requests"`
	FailedRequests      int64  `gorm:"column:failed_requests"`
	InputTokens         int64  `gorm:"column:input_tokens"`
	OutputTokens        int64  `gorm:"column:output_tokens"`
	ReasoningTokens     int64  `gorm:"column:reasoning_tokens"`
	CacheReadTokens     int64  `gorm:"column:cache_read_tokens"`
	CacheCreationTokens int64  `gorm:"column:cache_creation_tokens"`
	TotalTokens         int64  `gorm:"column:total_tokens"`
}

// BuildCodexQuotaEfficiencyHistory 动态连接额度历史与 UsageEvent；它只读数据，绝不把 Token 或 Cost 写入历史表。
func BuildCodexQuotaEfficiencyHistory(ctx context.Context, db *gorm.DB, query repositorydto.CodexQuotaEfficiencyQuery, costResolver pricing.Resolver) (repositorydto.CodexQuotaEfficiencyHistory, error) {
	// 先构造空响应，使“账号暂时没有历史”仍返回稳定的时间口径。
	result := repositorydto.CodexQuotaEfficiencyHistory{GeneratedAt: query.Now, RangeStart: query.RangeStart}
	if db == nil {
		return result, fmt.Errorf("build codex quota efficiency history: database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// auth_index 必须精确限定 UsageEvent；空值会退化成跨账号全表扫描，因此直接拒绝。
	query.AuthIndex = strings.TrimSpace(query.AuthIndex)
	if query.AuthIndex == "" {
		return result, fmt.Errorf("build codex quota efficiency history: auth_index is required")
	}
	if query.Now.IsZero() || query.RangeStart.IsZero() || !query.RangeStart.Before(query.Now) {
		return result, fmt.Errorf("build codex quota efficiency history: invalid time range")
	}
	// 整个响应固定同一 instant；后续当前周期判断、查询截点和 JSON 时间都复用它。
	query.Now = timeutil.NormalizeStorageTime(query.Now)
	query.RangeStart = timeutil.NormalizeStorageTime(query.RangeStart)
	result.GeneratedAt = query.Now
	result.RangeStart = query.RangeStart

	// 父表时间使用 sortableTime，SQL 参数也必须用固定宽度 UTC 文本才能保持 instant 顺序。
	var cycles []entities.CodexQuotaCycle
	err := db.WithContext(ctx).Clauses(dbresolver.Read).
		Where("auth_index = ? AND reset_at >= ? AND window_started_at < ?", query.AuthIndex, timeutil.FormatSortableStorageTime(query.RangeStart), timeutil.FormatSortableStorageTime(query.Now)).
		Order("reset_at DESC, id DESC").
		Find(&cycles).Error
	if err != nil {
		return result, fmt.Errorf("list codex quota efficiency cycles: %w", err)
	}
	if len(cycles) == 0 {
		return result, nil
	}

	// 一次父表结果同时生成窗口选项，避免切换器为同一批数据再执行一条 distinct 查询。
	result.Windows = buildCodexQuotaEfficiencyWindows(cycles, query.Now)
	selected := selectCodexQuotaEfficiencyWindow(result.Windows, query.WindowRole, query.WindowSeconds)
	if selected == nil {
		return result, nil
	}
	selectedCopy := *selected
	result.SelectedWindow = &selectedCopy

	// 只把选中窗口系列的当前/已结束周期送入子段和 UsageEvent 聚合，数据库不处理前端未展示的系列。
	selectedCycles := make([]entities.CodexQuotaCycle, 0, len(cycles))
	cycleIDs := make([]int64, 0, len(cycles))
	for _, cycle := range cycles {
		if string(cycle.WindowRole) != selected.WindowRole || cycle.WindowSeconds != selected.WindowSeconds {
			continue
		}
		selectedCycles = append(selectedCycles, cycle)
		cycleIDs = append(cycleIDs, cycle.ID)
	}
	if len(selectedCycles) == 0 {
		return result, nil
	}

	// 所有子段用一次 IN 查询读出；每周期最多 101 个整数桶，不允许对父周期逐条 Preload。
	var segments []entities.CodexQuotaPercentSegment
	if err := db.WithContext(ctx).Clauses(dbresolver.Read).
		Where("cycle_id IN ?", cycleIDs).
		Order("cycle_id ASC, first_observed_at ASC, id ASC").
		Find(&segments).Error; err != nil {
		return result, fmt.Errorf("list codex quota efficiency segments: %w", err)
	}
	segmentsByCycle := make(map[int64][]entities.CodexQuotaPercentSegment, len(selectedCycles))
	for _, segment := range segments {
		segmentsByCycle[segment.CycleID] = append(segmentsByCycle[segment.CycleID], segment)
	}

	// 先建立全部指针对象再聚合，避免 completed slice 扩容后让 map 中的元素地址失效。
	works := make([]codexQuotaEfficiencyCycleWork, 0, len(selectedCycles))
	completed := make([]*repositorydto.CodexQuotaEfficiencyCycle, 0, len(selectedCycles))
	var current *repositorydto.CodexQuotaEfficiencyCycle
	var nextTransitionID int64 = 1
	for _, cycle := range selectedCycles {
		active := !query.Now.Before(cycle.WindowStartedAt) && query.Now.Before(cycle.ResetAt)
		ended := !cycle.ResetAt.After(query.Now)
		if !active && !ended {
			continue
		}
		record := &repositorydto.CodexQuotaEfficiencyCycle{
			ID:              cycle.ID,
			WindowStartedAt: cycle.WindowStartedAt,
			ResetAt:         cycle.ResetAt,
			FirstObservedAt: cycle.FirstObservedAt,
			LastObservedAt:  cycle.LastObservedAt,
			Usage:           repositorydto.CodexQuotaEfficiencyUsage{CostAvailable: true},
			Transitions:     buildCodexQuotaEfficiencyTransitions(segmentsByCycle[cycle.ID], &nextTransitionID),
		}
		queryEnd := cycle.ResetAt
		if active {
			queryEnd = query.Now
			// 同一系列理论上只有一个活跃周期；若旧数据重叠，reset 较晚的父表排序结果优先。
			if current != nil {
				continue
			}
			current = record
		} else {
			completed = append(completed, record)
		}
		works = append(works, codexQuotaEfficiencyCycleWork{record: record, queryEnd: queryEnd})
	}

	// 完整周期有界分批；一个 Weekly 的当前+历史通常仍在同一条 UsageEvent 聚合 SQL 中完成。
	for start := 0; start < len(works); {
		end := codexQuotaEfficiencyBatchEnd(works, start)
		if err := aggregateCodexQuotaEfficiencyBatch(ctx, db, query.AuthIndex, works[start:end], costResolver); err != nil {
			return result, err
		}
		start = end
	}
	// SQL 聚合结束后再计算每百分点值，保证 CostAvailable 已吸收所有 pricing 分组。
	for _, work := range works {
		finalizeCodexQuotaEfficiencyTransitions(work.record)
	}
	result.CurrentCycle = current
	result.CompletedCycles = make([]repositorydto.CodexQuotaEfficiencyCycle, 0, len(completed))
	for _, record := range completed {
		result.CompletedCycles = append(result.CompletedCycles, *record)
	}
	return result, nil
}

func buildCodexQuotaEfficiencyWindows(cycles []entities.CodexQuotaCycle, now time.Time) []repositorydto.CodexQuotaEfficiencyWindow {
	// role+seconds 是真实系列键；kind 只作为展示文案，不能把变化后的未知窗口合并到固定枚举。
	type windowKey struct {
		role    string
		seconds int64
	}
	windowsByKey := make(map[windowKey]repositorydto.CodexQuotaEfficiencyWindow)
	for _, cycle := range cycles {
		key := windowKey{role: string(cycle.WindowRole), seconds: cycle.WindowSeconds}
		window := windowsByKey[key]
		window.WindowRole = key.role
		window.WindowSeconds = key.seconds
		if window.WindowKind == nil && cycle.WindowKind != nil {
			kind := *cycle.WindowKind
			window.WindowKind = &kind
		}
		if cycle.LastObservedAt.After(window.LastObservedAt) {
			window.LastObservedAt = cycle.LastObservedAt
		}
		if !now.Before(cycle.WindowStartedAt) && now.Before(cycle.ResetAt) {
			window.HasCurrentCycle = true
		}
		windowsByKey[key] = window
	}
	windows := make([]repositorydto.CodexQuotaEfficiencyWindow, 0, len(windowsByKey))
	for _, window := range windowsByKey {
		windows = append(windows, window)
	}
	// 确定性顺序让前端键盘切换稳定：Primary 优先，同角色按较短真实窗口优先。
	sort.Slice(windows, func(left, right int) bool {
		if windows[left].WindowRole != windows[right].WindowRole {
			return windows[left].WindowRole == string(entities.CodexQuotaWindowRolePrimary)
		}
		return windows[left].WindowSeconds < windows[right].WindowSeconds
	})
	return windows
}

func selectCodexQuotaEfficiencyWindow(windows []repositorydto.CodexQuotaEfficiencyWindow, role *string, seconds *int64) *repositorydto.CodexQuotaEfficiencyWindow {
	// 显式筛选只接受完全匹配；调用层负责校验 role/seconds 的值域。
	if role != nil || seconds != nil {
		for index := range windows {
			if role != nil && windows[index].WindowRole != strings.ToLower(strings.TrimSpace(*role)) {
				continue
			}
			if seconds != nil && windows[index].WindowSeconds != *seconds {
				continue
			}
			return &windows[index]
		}
		return nil
	}
	// 默认先选当前活跃系列；多个系列同时活跃时沿用 windows 的 Primary/短窗口确定性顺序。
	for index := range windows {
		if windows[index].HasCurrentCycle {
			return &windows[index]
		}
	}
	// 没有当前周期时选择最近观察系列，而不是假定固定 Weekly 或 FiveHour。
	var selected *repositorydto.CodexQuotaEfficiencyWindow
	for index := range windows {
		if selected == nil || windows[index].LastObservedAt.After(selected.LastObservedAt) {
			selected = &windows[index]
		}
	}
	return selected
}

func buildCodexQuotaEfficiencyTransitions(segments []entities.CodexQuotaPercentSegment, nextID *int64) []repositorydto.CodexQuotaEfficiencyTransition {
	transitions := make([]repositorydto.CodexQuotaEfficiencyTransition, 0, max(0, len(segments)-1))
	for index := 1; index < len(segments); index++ {
		previous := segments[index-1]
		current := segments[index]
		points := previous.RemainingPercent - current.RemainingPercent
		// 历史写入保证单调不升；查询层仍跳过异常非下降行，避免制造负效率样本。
		if points <= 0 {
			continue
		}
		transition := repositorydto.CodexQuotaEfficiencyTransition{
			ID:                    *nextID,
			FromRemainingPercent:  previous.RemainingPercent,
			ToRemainingPercent:    current.RemainingPercent,
			PercentagePoints:      points,
			IsDirect:              points == 1,
			IntervalStartedAt:     previous.LastObservedAt,
			IntervalEndedAt:       current.FirstObservedAt,
			Usage:                 repositorydto.CodexQuotaEfficiencyUsage{CostAvailable: true},
			CostPerPointAvailable: true,
		}
		(*nextID)++
		transitions = append(transitions, transition)
	}
	return transitions
}

func codexQuotaEfficiencyBatchEnd(works []codexQuotaEfficiencyCycleWork, start int) int {
	end := start
	transitionCount := 0
	for end < len(works) {
		nextTransitions := len(works[end].record.Transitions)
		if end > start && (end-start >= codexQuotaEfficiencyBatchCycleLimit || transitionCount+nextTransitions > codexQuotaEfficiencyBatchTransitionLimit) {
			break
		}
		transitionCount += nextTransitions
		end++
	}
	return end
}

func aggregateCodexQuotaEfficiencyBatch(ctx context.Context, db *gorm.DB, authIndex string, works []codexQuotaEfficiencyCycleWork, costResolver pricing.Resolver) error {
	if len(works) == 0 {
		return nil
	}
	// CASE 同时给每条事件分类周期与可选变化区间；稳定段事件 transition_id 为 NULL 但仍贡献周期总量。
	cycleCase, cycleArgs := codexQuotaEfficiencyCycleCase(works)
	transitionCase, transitionArgs := codexQuotaEfficiencyTransitionCase(works)
	dimensions := UsagePricingDimensionColumns(costResolver.ActiveFields())
	selectDimensions := make([]string, 0, len(dimensions))
	for _, dimension := range dimensions {
		if dimension == "model_alias" {
			selectDimensions = append(selectDimensions, "COALESCE(model_alias, '') AS model_alias")
			continue
		}
		selectDimensions = append(selectDimensions, dimension)
	}
	globalStart := works[0].record.WindowStartedAt
	globalEnd := works[0].queryEnd
	for _, work := range works[1:] {
		if work.record.WindowStartedAt.Before(globalStart) {
			globalStart = work.record.WindowStartedAt
		}
		if work.queryEnd.After(globalEnd) {
			globalEnd = work.queryEnd
		}
	}

	// CTE 先完成分类，外层才能用 cycle_id IS NOT NULL 排除同一总时间跨度里的周期空洞。
	innerColumns := strings.Join(selectDimensions, ", ")
	outerDimensions := strings.Join(dimensions, ", ")
	groupColumns := "cycle_id, transition_id"
	if outerDimensions != "" {
		groupColumns += ", " + outerDimensions
	}
	sql := fmt.Sprintf(`WITH classified AS (
	SELECT %s AS cycle_id, %s AS transition_id, %s,
		failed, input_tokens, output_tokens, reasoning_tokens, cache_read_tokens, cache_creation_tokens, total_tokens
	FROM usage_events INDEXED BY idx_usage_events_auth_index_timestamp_id
	WHERE auth_type = ? AND auth_index = ? AND timestamp >= ? AND timestamp < ?
)
SELECT cycle_id, transition_id, %s,
	COUNT(*) AS requests,
	COALESCE(SUM(CASE WHEN failed = 0 THEN 1 ELSE 0 END), 0) AS successful_requests,
	COALESCE(SUM(CASE WHEN failed != 0 THEN 1 ELSE 0 END), 0) AS failed_requests,
	COALESCE(SUM(input_tokens), 0) AS input_tokens,
	COALESCE(SUM(output_tokens), 0) AS output_tokens,
	COALESCE(SUM(reasoning_tokens), 0) AS reasoning_tokens,
	COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
	COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
	COALESCE(SUM(total_tokens), 0) AS total_tokens
FROM classified
WHERE cycle_id IS NOT NULL
GROUP BY %s`, cycleCase, transitionCase, innerColumns, outerDimensions, groupColumns)
	args := make([]any, 0, len(cycleArgs)+len(transitionArgs)+4)
	args = append(args, cycleArgs...)
	args = append(args, transitionArgs...)
	args = append(args, "oauth", authIndex, timeutil.FormatStorageTime(globalStart), timeutil.FormatStorageTime(globalEnd))
	var rows []codexQuotaEfficiencyAggregateRow
	if err := db.WithContext(ctx).Clauses(dbresolver.Read).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return fmt.Errorf("aggregate codex quota efficiency usage: %w", err)
	}

	cyclesByID := make(map[int64]*repositorydto.CodexQuotaEfficiencyCycle, len(works))
	transitionsByID := make(map[int64]*repositorydto.CodexQuotaEfficiencyTransition)
	for _, work := range works {
		cyclesByID[work.record.ID] = work.record
		for index := range work.record.Transitions {
			transition := &work.record.Transitions[index]
			transitionsByID[transition.ID] = transition
		}
	}
	for _, row := range rows {
		cycle := cyclesByID[row.CycleID]
		if cycle == nil {
			continue
		}
		cost := costResolver.Calculate(newUsagePricingCostSubject(
			row.APIGroupKey, row.Model, authIndex, row.ModelAlias, row.ServiceTier, row.ResponseServiceTier,
			row.ReasoningEffort, row.Endpoint, row.ExecutorType,
			row.InputTokens, row.OutputTokens, row.CacheReadTokens, row.CacheCreationTokens,
		))
		// 极少数旧事件可能只有 total_tokens 而没有计价分项；只要存在 Token 且模型未匹配，就仍应标记价格缺失。
		if row.TotalTokens > 0 && cost.MatchedModel == "" {
			cost.Available = false
		}
		addCodexQuotaEfficiencyUsage(&cycle.Usage, row, cost)
		if row.TransitionID != nil {
			if transition := transitionsByID[*row.TransitionID]; transition != nil {
				addCodexQuotaEfficiencyUsage(&transition.Usage, row, cost)
			}
		}
	}
	return nil
}

func codexQuotaEfficiencyCycleCase(works []codexQuotaEfficiencyCycleWork) (string, []any) {
	parts := make([]string, 0, len(works))
	args := make([]any, 0, len(works)*3)
	for _, work := range works {
		parts = append(parts, "WHEN timestamp >= ? AND timestamp < ? THEN ?")
		args = append(args, timeutil.FormatStorageTime(work.record.WindowStartedAt), timeutil.FormatStorageTime(work.queryEnd), work.record.ID)
	}
	return "CASE " + strings.Join(parts, " ") + " END", args
}

func codexQuotaEfficiencyTransitionCase(works []codexQuotaEfficiencyCycleWork) (string, []any) {
	parts := make([]string, 0)
	args := make([]any, 0)
	for _, work := range works {
		for _, transition := range work.record.Transitions {
			// 同时刻变化没有可归属的 UsageEvent，但真实百分比变化仍保留在响应列表中。
			if !transition.IntervalStartedAt.Before(transition.IntervalEndedAt) {
				continue
			}
			parts = append(parts, "WHEN timestamp >= ? AND timestamp < ? THEN ?")
			args = append(args, timeutil.FormatStorageTime(transition.IntervalStartedAt), timeutil.FormatStorageTime(transition.IntervalEndedAt), transition.ID)
		}
	}
	if len(parts) == 0 {
		return "NULL", nil
	}
	return "CASE " + strings.Join(parts, " ") + " END", args
}

func addCodexQuotaEfficiencyUsage(target *repositorydto.CodexQuotaEfficiencyUsage, row codexQuotaEfficiencyAggregateRow, cost pricing.CostResult) {
	target.Requests += row.Requests
	target.SuccessfulRequests += row.SuccessfulRequests
	target.FailedRequests += row.FailedRequests
	target.InputTokens += row.InputTokens
	target.OutputTokens += row.OutputTokens
	target.ReasoningTokens += row.ReasoningTokens
	target.CacheReadTokens += row.CacheReadTokens
	target.CacheCreationTokens += row.CacheCreationTokens
	target.TotalTokens += row.TotalTokens
	target.TotalCostUSD += cost.Cost.TotalCostUSD
	if !cost.Available {
		target.CostAvailable = false
	}
}

func finalizeCodexQuotaEfficiencyTransitions(cycle *repositorydto.CodexQuotaEfficiencyCycle) {
	for index := range cycle.Transitions {
		transition := &cycle.Transitions[index]
		points := float64(transition.PercentagePoints)
		transition.TokensPerPoint = float64(transition.Usage.TotalTokens) / points
		transition.CostPerPoint = transition.Usage.TotalCostUSD / points
		transition.CostPerPointAvailable = transition.Usage.CostAvailable
	}
}
