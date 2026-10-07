package test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"cpa-usage-keeper/internal/cpa"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
)

func TestV8MetadataSyncFlattensFirstMatchAndSeparatesConfigAbsenceFromFailure(t *testing.T) {
	db := openMetadataTestDatabase(t, "v8-metadata.db")
	var phase atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v8/management/credentials":
			fmt.Fprint(w, `{"files":[]}`)
		case "/v8/management/config/access/api-keys":
			switch phase.Load() {
			case 0:
				fmt.Fprint(w, `["client-key"]`)
			case 1:
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"error":"not_found"}`)
			default:
				fmt.Fprint(w, `[]`)
			}
		case "/v8/management/config/api-keys/codex":
			switch phase.Load() {
			case 0:
				fmt.Fprint(w, `[{"name":"unused group name","priority":7,"excluded-models":["*"],"keys":[{"api-key":"first","auth_index":"shared"},{"api-key":"second","auth_index":"second","priority":0,"excluded-models":[]}]},{"priority":99,"keys":[{"api-key":"duplicate","auth_index":"shared"}]}]`)
			case 1:
				w.WriteHeader(http.StatusBadGateway)
				fmt.Fprint(w, `{"error":"unavailable"}`)
			default:
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"error":"not_found"}`)
			}
		case "/v8/management/config/api-keys/meta":
			fmt.Fprint(w, `[{"base-url":"  ","keys":[{"api-key":"meta","auth_index":"meta-auth"}]}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not_found"}`)
		}
	}))
	defer server.Close()
	syncer := service.NewSyncServiceWithClient(db, server.URL, cpa.NewClient(server.URL, "management-secret", time.Second, false))
	if err := syncer.SyncMetadata(context.Background()); err != nil {
		t.Fatal(err)
	}
	identities := loadMetadataIdentityMap(t, db)
	first := identities[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "shared")]
	second := identities[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "second")]
	meta := identities[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "meta-auth")]
	if first.LookupKey != "first" || *first.Priority != 7 || !*first.Disabled || first.Name != "codex" {
		t.Fatalf("first metadata = %+v", first)
	}
	if *second.Priority != 0 || *second.Disabled {
		t.Fatalf("override metadata = %+v", second)
	}
	if meta.BaseURL != "https://api.meta.ai/v1" || *meta.Priority != 0 {
		t.Fatalf("meta metadata = %+v", meta)
	}
	phase.Store(1)
	if err := syncer.SyncMetadata(context.Background()); err == nil {
		t.Fatal("upstream failures returned success")
	}
	first = loadMetadataIdentityMap(t, db)[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "shared")]
	if first.IsDeleted || first.LookupKey != "first" {
		t.Fatalf("failed provider sync changed row = %+v", first)
	}
	keys, err := repository.ListActiveCPAAPIKeys(db)
	if err != nil || len(keys) != 1 || keys[0].APIKey != "client-key" {
		t.Fatalf("client 404 changed keys = %+v, err = %v", keys, err)
	}
	phase.Store(2)
	if err := syncer.SyncMetadata(context.Background()); err != nil {
		t.Fatal(err)
	}
	first = loadMetadataIdentityMap(t, db)[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "shared")]
	if !first.IsDeleted {
		t.Fatalf("absent config left active row = %+v", first)
	}
	keys, err = repository.ListActiveCPAAPIKeys(db)
	if err != nil || len(keys) != 0 {
		t.Fatalf("empty access array left keys = %+v, err = %v", keys, err)
	}
}

func TestV8MetadataSyncPersistsReturnedNumericTextAndClientKeys(t *testing.T) {
	db := openMetadataTestDatabase(t, "v8-numeric-text.db")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v8/management/credentials":
			fmt.Fprint(w, `{"files":[]}`)
		case "/v8/management/config/access/api-keys":
			fmt.Fprint(w, `[9007199254740993,"00123"]`)
		case "/v8/management/config/api-keys/codex":
			fmt.Fprint(w, `[{"name":123,"prefix":123,"excluded-models":[42],"keys":[{"api-key":9007199254740993,"auth_index":"numeric-native","prefix":null,"excluded-models":null}]}]`)
		case "/v8/management/config/api-keys/openai-compatibility":
			fmt.Fprint(w, `[{"name":9007199254740993,"prefix":789,"keys":[{"api-key":9007199254740993,"auth_index":"numeric-openai"}]}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not_found"}`)
		}
	}))
	defer server.Close()
	syncer := service.NewSyncServiceWithClient(db, server.URL, cpa.NewClient(server.URL, "management-secret", time.Second, false))
	if err := syncer.SyncMetadata(context.Background()); err != nil {
		t.Fatal(err)
	}
	identities := loadMetadataIdentityMap(t, db)
	native := identities[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "numeric-native")]
	openAI := identities[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "numeric-openai")]
	if native.LookupKey != "9007199254740993" || native.Prefix != "123" || native.Name != "codex" {
		t.Fatalf("native row = %+v", native)
	}
	if openAI.LookupKey != "9007199254740993" || openAI.Prefix != "789" || openAI.Name != "9007199254740993" {
		t.Fatalf("OpenAI row = %+v", openAI)
	}
	keys, err := repository.ListActiveCPAAPIKeys(db)
	if err != nil || len(keys) != 2 {
		t.Fatalf("client keys = %#v, err = %v", keys, err)
	}
	seen := map[string]bool{}
	for _, key := range keys {
		seen[key.APIKey] = true
	}
	if !seen["9007199254740993"] || !seen["00123"] {
		t.Fatalf("client key text changed: %#v", keys)
	}
}
