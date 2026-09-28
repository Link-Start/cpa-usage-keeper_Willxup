package app

import (
	"time"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/cpa"
	"cpa-usage-keeper/internal/poller"
	"gorm.io/gorm"
)

// NewPricingBootstrapIngestRunner 只构造 CPA 原始消息接收链路，不打开业务 worker 或访问业务表。
// 升级期间仍过滤 metadata 控制消息，但没有 observer，因而不会触发业务 metadata 同步。
func NewPricingBootstrapIngestRunner(cfg config.Config, db *gorm.DB) *poller.RedisIngestRunner {
	return newUsageIngestRunner(cfg, poller.NewPricingBootstrapInboxWriter(db), nil)
}

// newUsageIngestRunner 复用订阅、Redis pull 和 HTTP pull 的既有优先级及降级合同。
// writer 决定启动引导或正常 schema 的 inbox 写列；observer 只在业务运行阶段注入。
func newUsageIngestRunner(cfg config.Config, writer poller.RedisInboxWriter, observer poller.RedisControlMessageObserver) *poller.RedisIngestRunner {
	redisPullSource := poller.NewRedisPullSource(cpa.RedisQueueOptions{
		BaseURL: cfg.CPABaseURL, RedisAddr: cfg.RedisQueueAddr, ManagementKey: cfg.CPAManagementKey,
		Timeout: cfg.RequestTimeout, BatchSize: cfg.RedisQueueBatchSize,
		TLS: cfg.RedisQueueTLS, TLSSkipVerify: cfg.TLSSkipVerify,
	})
	httpPullSource := poller.NewHTTPPullSource(cfg.CPABaseURL, cfg.CPAManagementKey, cfg.RequestTimeout, cfg.TLSSkipVerify, cfg.RedisQueueBatchSize)
	redisSubscribeSource := poller.NewRedisSubscribeSource(poller.RedisSubscribeOptions{
		BaseURL: cfg.CPABaseURL, RedisAddr: cfg.RedisQueueAddr, ManagementKey: cfg.CPAManagementKey,
		Timeout: cfg.RequestTimeout, TLS: cfg.RedisQueueTLS, TLSSkipVerify: cfg.TLSSkipVerify,
	})
	filteredWriter := poller.NewControlAwareRedisInboxWriter(writer, observer)
	runner := poller.NewRedisIngestRunner(redisSubscribeSource, redisPullSource, httpPullSource, filteredWriter, poller.RedisIngestRunnerConfig{
		IdleInterval: cfg.RedisQueueIdleInterval, BatchSize: cfg.RedisQueueBatchSize,
		HTTPBackoffInitial: time.Second, HTTPBackoffMax: 30 * time.Second,
	})
	if observer != nil {
		runner.SetControlMessageObserver(observer)
	}
	return runner
}

// newNormalUsageIngestRunner 在完整业务 App 中保留原标准 source 写入与 metadata 通知。
func newNormalUsageIngestRunner(cfg config.Config, db *gorm.DB, observer poller.RedisControlMessageObserver) *poller.RedisIngestRunner {
	return newUsageIngestRunner(cfg, poller.NewRedisInboxWriter(db), observer)
}
