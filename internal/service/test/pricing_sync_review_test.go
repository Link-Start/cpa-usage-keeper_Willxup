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
	servicedto "cpa-usage-keeper/internal/service/dto"
)

type pricingReviewTransport func(*http.Request) (*http.Response, error)

func (f pricingReviewTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func previewReviewCatalog(t *testing.T, source, catalog string, names ...string) servicedto.PricingSyncPreview {
	t.Helper()
	original := http.DefaultTransport
	http.DefaultTransport = pricingReviewTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(catalog))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	modelList := make([]models.ModelInfo, 0, len(names))
	for _, name := range names {
		modelList = append(modelList, models.ModelInfo{ID: name})
	}
	provider := service.NewPricingService(openPricingServiceTestDatabase(t), emptyPricingCatalogForTest(),
		stubModelsFetcher{result: &response.ModelsResult{Payload: models.ModelsResponse{Data: modelList}}})
	preview, err := provider.PreviewPricingSync(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	return preview
}

func TestPricingSyncSeparatesFineTunedModelIdentity(t *testing.T) {
	catalog := `{
		"gpt-3.5-turbo":{"litellm_provider":"openai","mode":"chat","input_cost_per_token":0.0000005,"output_cost_per_token":0.0000015},
		"ft:gpt-3.5-turbo":{"litellm_provider":"openai","mode":"chat","input_cost_per_token":0.000003,"output_cost_per_token":0.000006}
	}`
	want := map[string]string{
		"gpt-3.5-turbo":           "gpt-3.5-turbo",
		"custom/gpt-3.5-turbo":    "gpt-3.5-turbo",
		"ft:gpt-3.5-turbo":        "ft:gpt-3.5-turbo",
		"custom/ft:gpt-3.5-turbo": "ft:gpt-3.5-turbo",
		"custom:ft:gpt-3.5-turbo": "ft:gpt-3.5-turbo",
	}
	names := make([]string, 0, len(want))
	for name := range want {
		names = append(names, name)
	}
	preview := previewReviewCatalog(t, "litellm", catalog, names...)
	if len(preview.Matches) != len(want) {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	for _, match := range preview.Matches {
		if match.MatchedModel != want[match.Model] {
			t.Errorf("%s matched %s, want %s", match.Model, match.MatchedModel, want[match.Model])
		}
		price := 0.5
		if strings.HasPrefix(want[match.Model], "ft:") {
			price = 3
		}
		if math.Abs(match.PromptPricePer1M-price) > 1e-10 {
			t.Errorf("%s input price=%v, want %v", match.Model, match.PromptPricePer1M, price)
		}
	}
}

func TestPricingSyncDoesNotFallbackAcrossFineTuningIdentity(t *testing.T) {
	for _, tc := range []struct{ name, catalog string }{
		{"gpt-3.5-turbo", `{"ft:gpt-3.5-turbo":{"litellm_provider":"openai","mode":"chat","input_cost_per_token":0.000003,"output_cost_per_token":0.000006}}`},
		{"ft:gpt-3.5-turbo", `{"gpt-3.5-turbo":{"litellm_provider":"openai","mode":"chat","input_cost_per_token":0.0000005,"output_cost_per_token":0.0000015}}`},
		{"ft:gpt-3.5-turbo:org:suffix:id", `{"gpt-3.5-turbo":{"litellm_provider":"openai","mode":"chat","input_cost_per_token":0.0000005,"output_cost_per_token":0.0000015},"id":{"litellm_provider":"openai","mode":"chat","input_cost_per_token":1,"output_cost_per_token":1}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preview := previewReviewCatalog(t, "litellm", tc.catalog, tc.name)
			if len(preview.Matches) != 0 || len(preview.UnmatchedModels) != 1 {
				t.Fatalf("must not cross billing identities: %+v", preview)
			}
		})
	}
}
