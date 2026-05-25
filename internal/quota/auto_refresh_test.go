package quota

import (
	"context"
	"errors"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
)

func TestRunAutoRefreshQueuesOnlyActiveAuthFiles(t *testing.T) {
	db := openQuotaTestDatabase(t)
	disabled := true
	seedUsageIdentity(t, db, entities.UsageIdentity{Identity: "auth-1", Provider: "claude", Type: "auth-file", AuthType: entities.UsageIdentityAuthTypeAuthFile})
	seedUsageIdentity(t, db, entities.UsageIdentity{Identity: "  ", Provider: "claude", Type: "auth-file", AuthType: entities.UsageIdentityAuthTypeAuthFile})
	seedUsageIdentity(t, db, entities.UsageIdentity{Identity: "disabled-1", Provider: "claude", Type: "auth-file", AuthType: entities.UsageIdentityAuthTypeAuthFile, Disabled: &disabled})
	seedUsageIdentity(t, db, entities.UsageIdentity{Identity: "deleted-1", Provider: "claude", Type: "auth-file", AuthType: entities.UsageIdentityAuthTypeAuthFile, IsDeleted: true})
	seedUsageIdentity(t, db, entities.UsageIdentity{Identity: "provider-1", Provider: "openai", Type: "openai", AuthType: entities.UsageIdentityAuthTypeAIProvider})
	handler := &refreshHandlerStub{output: ProviderOutput{Result: ClaudeResult{Usage: &ClaudeUsagePayload{FiveHour: &ClaudeUsageWindow{Utilization: 25}}}}}
	service := NewServiceWithRegistry(db, NewProviderRegistry(map[string]ProviderHandler{"claude": handler}))
	service.refreshCooldown = func(time.Duration) {}

	if err := service.RunAutoRefresh(context.Background()); err != nil {
		t.Fatalf("RunAutoRefresh returned error: %v", err)
	}
	waitForRefreshTask(t, service, "auth-1", RefreshTaskStatusCompleted)
	if handler.callCount() != 1 {
		t.Fatalf("expected only active auth file to refresh, got %d calls", handler.callCount())
	}
	for _, authIndex := range []string{"disabled-1", "deleted-1", "provider-1"} {
		if _, err := service.GetRefreshTaskByAuthIndex(context.Background(), authIndex); !errors.Is(err, ErrTaskNotFound) {
			t.Fatalf("expected %s to stay out of auto refresh queue, got %v", authIndex, err)
		}
	}
	service.refreshMu.Lock()
	_, hasBlankTask := service.refreshTasks["  "]
	service.refreshMu.Unlock()
	if hasBlankTask {
		t.Fatal("expected blank identity to stay out of auto refresh queue")
	}
}

func TestRunAutoRefreshSkipsCachedHTTPFailures(t *testing.T) {
	db := openQuotaTestDatabase(t)
	seedUsageIdentity(t, db, entities.UsageIdentity{Identity: "auth-1", Provider: "claude", Type: "auth-file", AuthType: entities.UsageIdentityAuthTypeAuthFile})
	handler := &refreshHandlerStub{err: ProviderHTTPError{StatusCode: 401, Message: "expired token"}}
	service := NewServiceWithRegistry(db, NewProviderRegistry(map[string]ProviderHandler{"claude": handler}))
	service.refreshCooldown = func(time.Duration) {}

	first, err := service.Refresh(context.Background(), RefreshRequest{AuthIndexes: []string{"auth-1"}, Source: RefreshSourceManual})
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	waitForRefreshTask(t, service, first.Tasks[0].AuthIndex, RefreshTaskStatusFailed)
	if handler.callCount() != 1 {
		t.Fatalf("expected one manual provider call, got %d", handler.callCount())
	}
	if err := service.RunAutoRefresh(context.Background()); err != nil {
		t.Fatalf("RunAutoRefresh returned error: %v", err)
	}
	if handler.callCount() != 1 {
		t.Fatalf("expected auto refresh to skip cached 401, got %d calls", handler.callCount())
	}
}

func TestRunAutoRefreshLogsRoundStartAndEndOnce(t *testing.T) {
	db := openQuotaTestDatabase(t)
	seedUsageIdentity(t, db, entities.UsageIdentity{Identity: "auth-1", Provider: "claude", Type: "auth-file", AuthType: entities.UsageIdentityAuthTypeAuthFile})
	handler := &refreshHandlerStub{output: ProviderOutput{Result: ClaudeResult{Usage: &ClaudeUsagePayload{FiveHour: &ClaudeUsageWindow{Utilization: 25}}}}}
	service := NewServiceWithRegistry(db, NewProviderRegistry(map[string]ProviderHandler{"claude": handler}))
	service.refreshCooldown = func(time.Duration) {}
	hook := logrustest.NewGlobal()
	t.Cleanup(func() {
		hook.Reset()
	})

	if err := service.RunAutoRefresh(context.Background()); err != nil {
		t.Fatalf("RunAutoRefresh returned error: %v", err)
	}

	assertAutoRefreshRoundLogs(t, hook, 1, 1)
}

func TestRunAutoRefreshLogsRoundEndWhenIdentityScanFails(t *testing.T) {
	db := openQuotaTestDatabase(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB returned error: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close db returned error: %v", err)
	}
	service := NewServiceWithRegistry(db, NewProviderRegistry(nil))
	hook := logrustest.NewGlobal()
	t.Cleanup(func() {
		hook.Reset()
	})

	if err := service.RunAutoRefresh(context.Background()); err == nil {
		t.Fatal("expected RunAutoRefresh to return scan error")
	}

	assertAutoRefreshRoundLogs(t, hook, 1, 1)
}

func assertAutoRefreshRoundLogs(t *testing.T, hook *logrustest.Hook, wantStart int, wantEnd int) {
	t.Helper()
	startLogs := 0
	endLogs := 0
	for _, entry := range hook.AllEntries() {
		if entry.Level != logrus.InfoLevel {
			continue
		}
		switch entry.Message {
		case "quota auto refresh round started":
			startLogs++
		case "quota auto refresh round completed":
			endLogs++
		}
	}
	if startLogs != wantStart || endLogs != wantEnd {
		t.Fatalf("expected start=%d end=%d info logs, got start=%d end=%d entries=%+v", wantStart, wantEnd, startLogs, endLogs, hook.AllEntries())
	}
}

func TestNewServiceWithRegistryAndOptionsUsesConfiguredAutoRefreshInterval(t *testing.T) {
	db := openQuotaTestDatabase(t)
	service := NewServiceWithRegistryAndOptions(db, NewProviderRegistry(nil), ServiceOptions{AutoRefreshInterval: 2 * time.Minute})
	if service.autoRefreshInterval != 2*time.Minute {
		t.Fatalf("expected configured auto refresh interval 2m, got %s", service.autoRefreshInterval)
	}
}

func TestRefreshCacheableHTTPStatusCodesAlsoControlAutoRefreshSkip(t *testing.T) {
	if _, ok := RefreshCacheableHTTPStatusCodes[401]; !ok {
		t.Fatal("expected 401 to be configured as cacheable and auto-refresh-skipped")
	}
}
