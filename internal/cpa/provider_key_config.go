package cpa

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"cpa-usage-keeper/internal/cpa/dto/providerconfig"
	"cpa-usage-keeper/internal/cpa/dto/response"
)

// ProviderKeyDisabledExcludedModel 是普通 API Key 有效 excluded-models 中的整条停用标记。
const ProviderKeyDisabledExcludedModel = "*"

func providerKeyEndpoint(providerType string) (path string, kind string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "codex":
		return cpaManagementCodexAPIKeyEndpoint, "codex api keys", true
	case "xai":
		return cpaManagementXAIAPIKeyEndpoint, "xai api keys", true
	case "gemini":
		return cpaManagementGeminiAPIKeyEndpoint, "gemini api keys", true
	case "gemini-interactions":
		return cpaManagementInteractionsAPIKeyEndpoint, "interactions api keys", true
	case "claude":
		return cpaManagementClaudeAPIKeyEndpoint, "claude api keys", true
	case "vertex":
		return cpaManagementVertexAPIKeyEndpoint, "vertex api keys", true
	default:
		return "", "", false
	}
}

func priorityProviderKeyEndpoint(providerType string) (path string, kind string, ok bool) {
	if strings.EqualFold(strings.TrimSpace(providerType), "meta") {
		return cpaManagementMetaAPIKeyEndpoint, "meta api keys", true
	}
	return providerKeyEndpoint(providerType)
}

// ProviderConfigPath 为读取、整列表写入及跨服务互斥提供同一供应商路径。
func ProviderConfigPath(providerType string) (string, bool) {
	if strings.EqualFold(strings.TrimSpace(providerType), "openai") {
		return cpaManagementOpenAICompatibilityEndpoint, true
	}
	path, _, ok := priorityProviderKeyEndpoint(providerType)
	return path, ok
}

func (c *Client) FetchProviderKeyConfig(ctx context.Context, providerType string) (*response.ProviderKeyConfigResult, error) {
	path, kind, ok := providerKeyEndpoint(providerType)
	if !ok {
		return nil, fmt.Errorf("unsupported provider type %q", strings.TrimSpace(providerType))
	}
	return c.fetchProviderKeyConfig(ctx, path, kind)
}

func (c *Client) FetchPriorityProviderConfig(ctx context.Context, providerType string) (*response.ProviderKeyConfigResult, error) {
	path, kind, ok := priorityProviderKeyEndpoint(providerType)
	if !ok {
		return nil, fmt.Errorf("unsupported priority provider type %q", providerType)
	}
	return c.fetchProviderKeyConfig(ctx, path, kind)
}

// UpdateProviderConfig 写入本次 GET 修改后的完整供应商列表，PUT 成功即表示保存成功。
func (c *Client) UpdateProviderConfig(ctx context.Context, providerType string, document *providerconfig.Document) (int, error) {
	path, ok := ProviderConfigPath(providerType)
	if !ok {
		return 0, fmt.Errorf("unsupported provider type %q", providerType)
	}
	if document == nil {
		return 0, fmt.Errorf("provider configuration is unavailable")
	}
	statusCode, _, err := c.doManagementJSONRequestWithBody(ctx, http.MethodPut, path, document.Writable(), nil, "provider configuration")
	return statusCode, err
}

func ProviderKeyStatusSupported(providerType string) bool {
	_, _, ok := providerKeyEndpoint(providerType)
	return ok
}

func ProviderPrioritySupported(providerType string) bool {
	_, ok := ProviderConfigPath(providerType)
	return ok
}
