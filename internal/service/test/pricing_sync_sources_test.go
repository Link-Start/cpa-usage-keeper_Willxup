package test

import (
	"context"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"

	"cpa-usage-keeper/internal/cpa/dto/models"
	"cpa-usage-keeper/internal/cpa/dto/response"
	"cpa-usage-keeper/internal/service"
)

type liteLLMCatalogTransport struct{ t *testing.T }

func (transport liteLLMCatalogTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.String() != "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json" {
		transport.t.Fatalf("unexpected pricing source: %s", request.URL)
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{
		"gpt-test":{"litellm_provider":"openai","mode":"chat","input_cost_per_token":0.0000025,"output_cost_per_token":0.000015,"cache_read_input_token_cost":0.00000025},
		"azure/gpt-test":{"litellm_provider":"azure","mode":"chat","input_cost_per_token":0.000099,"output_cost_per_token":0.000099},
		"claude-test":{"litellm_provider":"anthropic","mode":"chat","input_cost_per_token":0.000003,"output_cost_per_token":0.000015,"cache_creation_input_token_cost":0.00000375},
		"overflow":{"litellm_provider":"openai","mode":"chat","input_cost_per_token":1e308,"output_cost_per_token":1}
	}`))}, nil
}

func TestPricingSyncLiteLLMUsesSharedMatchingAndZeroCacheDefaults(t *testing.T) {
	transport := http.DefaultTransport
	http.DefaultTransport = liteLLMCatalogTransport{t}
	t.Cleanup(func() { http.DefaultTransport = transport })
	provider := service.NewPricingService(openPricingServiceTestDatabase(t), emptyPricingCatalogForTest(), stubModelsFetcher{result: &response.ModelsResult{Payload: models.ModelsResponse{Data: []models.ModelInfo{
		{ID: "custom/gpt-test"}, {ID: "claude-test"}, {ID: "overflow"},
	}}}})
	preview, err := provider.PreviewPricingSync(context.Background(), "litellm")
	if err != nil {
		t.Fatal(err)
	}
	if preview.SourceID != "litellm" || preview.Source != "LiteLLM" || len(preview.Matches) != 2 || len(preview.UnmatchedModels) != 1 {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	for _, match := range preview.Matches {
		if match.Model == "custom/gpt-test" {
			if match.SourceProviderID != "openai" || math.Abs(match.PromptPricePer1M-2.5) > 1e-10 || match.CacheWritePricePer1M != 0 {
				t.Errorf("unexpected OpenAI match: %+v", match)
			}
		} else if match.PricingStyle != "claude" || math.Abs(match.CacheWritePricePer1M-3.75) > 1e-10 || match.CacheReadPricePer1M != 0 {
			t.Errorf("unexpected Claude match: %+v", match)
		}
	}
	prices, err := provider.ListPricing(context.Background())
	if err != nil || len(prices) != 0 {
		t.Fatalf("preview must not save prices: %+v, %v", prices, err)
	}
}
