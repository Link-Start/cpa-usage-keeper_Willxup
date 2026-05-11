package providerconfig

import (
	"encoding/json"
	"testing"
)

func TestProviderKeyConfigUnmarshalsBaseURLVariants(t *testing.T) {
	for _, field := range []string{"base-url", "base_url", "baseURL"} {
		t.Run(field, func(t *testing.T) {
			data := []byte(`{"apiKey":"provider-key","auth-index":"provider-auth","` + field + `":"https://api.openai.com/v1"}`)

			var cfg ProviderKeyConfig
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatalf("unmarshal provider key config: %v", err)
			}
			if cfg.BaseURL != "https://api.openai.com/v1" {
				t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, "https://api.openai.com/v1")
			}
		})
	}
}
