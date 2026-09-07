package pricingmetadata

import (
	"encoding/json"
	"strings"
)

type liteLLMModel struct {
	Provider   string   `json:"litellm_provider"`
	Mode       string   `json:"mode"`
	Input      *float64 `json:"input_cost_per_token"`
	Output     *float64 `json:"output_cost_per_token"`
	CacheRead  *float64 `json:"cache_read_input_token_cost"`
	CacheWrite *float64 `json:"cache_creation_input_token_cost"`
}

func decodeLiteLLM(decoder *json.Decoder) ([]Entry, error) {
	var models map[string]liteLLMModel
	if err := decoder.Decode(&models); err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(models))
	for _, id := range sortedKeys(models) {
		model := models[id]
		// 只映射基础文本 token 价格；图片、音频、请求数和上下文阶梯有独立计价语义。
		if model.Mode != "chat" && model.Mode != "completion" && model.Mode != "responses" {
			continue
		}
		if strings.TrimSpace(id) == "" || model.Input == nil || model.Output == nil {
			continue
		}
		provider := liteLLMProviderID(model.Provider)
		if provider == "" {
			continue
		}
		entries = append(entries, Entry{ProviderID: provider, ProviderName: provider, Model: Model{
			ID: id, Name: id,
			Cost: Cost{Input: perMillion(model.Input), Output: perMillion(model.Output), CacheRead: perMillion(model.CacheRead), CacheWrite: perMillion(model.CacheWrite)},
		}})
	}
	return entries, nil
}

func perMillion(price *float64) *float64 {
	if price == nil {
		return nil
	}
	value := *price * 1_000_000
	return &value
}

func liteLLMProviderID(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	// 归一化供应商 ID 后复用既有优先级，不从 CPA 可自定义的模型前缀猜供应商。
	switch provider {
	case "gemini":
		return "google"
	case "vertex_ai", "vertex_ai-language-models":
		return "google-vertex"
	case "vertex_ai-anthropic_models":
		return "google-vertex-anthropic"
	case "bedrock":
		return "amazon-bedrock"
	case "x_ai":
		return "xai"
	case "moonshot":
		return "moonshotai"
	case "dashscope", "qwen_ai_platform":
		return "alibaba-cn"
	case "qwencloud":
		return "alibaba"
	default:
		return provider
	}
}
