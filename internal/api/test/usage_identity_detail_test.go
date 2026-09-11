package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "cpa-usage-keeper/internal/api"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/service"
)

func TestUsageIdentityDetailReadsOffPageAndDeletedIdentity(t *testing.T) {
	for _, authType := range []entities.UsageIdentityAuthType{1, 2} {
		t.Run(fmt.Sprint(authType), func(t *testing.T) {
			db := openUsageIdentityAliasAPIDatabase(t)
			resetAt := time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)
			row := entities.UsageIdentity{ID: 1, AuthType: authType, Identity: "detail-fixture", Name: "Fixture", TotalRequests: 10, ResetTotalRequests: 8, StatsResetAt: &resetAt, IsDeleted: true}
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			for id := int64(2); id <= 12; id++ {
				if err := db.Create(&entities.UsageIdentity{ID: id, AuthType: authType, Identity: fmt.Sprint(id), TotalRequests: 100}).Error; err != nil {
					t.Fatal(err)
				}
			}
			router := NewRouter(nil, nil, nil, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentity: service.NewUsageIdentityService(db)})
			page := httptest.NewRecorder()
			router.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/api/v1/usage/identities/page?sort=total_requests&page_size=10", nil))
			var listed struct {
				Identities []struct{ ID string } `json:"identities"`
			}
			if err := json.Unmarshal(page.Body.Bytes(), &listed); err != nil {
				t.Fatal(err)
			}
			if len(listed.Identities) != 10 {
				t.Fatalf("page: %s", page.Body.String())
			}
			for _, item := range listed.Identities {
				if item.ID == "1" {
					t.Fatal("fixture should be outside the current page")
				}
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/usage/identities/1", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("detail status %d: %s", response.Code, response.Body.String())
			}
			var detail struct {
				ID            string           `json:"id"`
				TotalRequests int64            `json:"total_requests"`
				Period        map[string]int64 `json:"period_stats"`
				StatsResetAt  time.Time        `json:"stats_reset_at"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
				t.Fatal(err)
			}
			if detail.ID != "1" || detail.TotalRequests != 10 || detail.Period["total_requests"] != 2 || !detail.StatsResetAt.Equal(resetAt) {
				t.Fatalf("detail: %s", response.Body.String())
			}
		})
	}
}

func TestUsageIdentityDetailValidatesIDAndAuthorization(t *testing.T) {
	db := openUsageIdentityAliasAPIDatabase(t)
	seedUsageIdentityAliasAPIIdentity(t, db)
	for _, tc := range []struct {
		id   string
		auth bool
		want int
	}{
		{"no", false, http.StatusBadRequest},
		{"0", false, http.StatusBadRequest},
		{"999", false, http.StatusNotFound},
		{"1", true, http.StatusUnauthorized},
	} {
		t.Run(tc.id, func(t *testing.T) {
			router := NewRouter(nil, nil, nil, nil, AuthConfig{Enabled: tc.auth, LoginPassword: "test-password", SessionTTL: time.Hour}, nil, "", OptionalProviders{UsageIdentity: service.NewUsageIdentityService(db)})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/usage/identities/"+tc.id, nil))
			if response.Code != tc.want {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
		})
	}
}
