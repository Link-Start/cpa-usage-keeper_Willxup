package providerconfig

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// ProviderKeyConfig 是分组展开后的有效成员视图，不用于回写 CPA 原始配置。
type ProviderKeyConfig struct {
	APIKey         string   `json:"api-key"`
	Prefix         string   `json:"prefix"`
	Name           string   `json:"name"`
	BaseURL        string   `json:"base-url"`
	AuthIndex      string   `json:"auth_index"`
	Priority       *int     `json:"priority"`
	Disabled       *bool    `json:"disabled"`
	ExcludedModels []string `json:"excluded-models"`
	Note           *string  `json:"note"`
}

// OpenAICompatibilityConfig 保留组级配置，成员只提供当前 Key 与运行时标识。
type OpenAICompatibilityConfig struct {
	Name          string              `json:"name"`
	Prefix        string              `json:"prefix"`
	BaseURL       string              `json:"base-url"`
	Priority      *int                `json:"priority"`
	Disabled      *bool               `json:"disabled"`
	Note          *string             `json:"note"`
	APIKeyEntries []OpenAIApiKeyEntry `json:"keys"`
}

type OpenAIApiKeyEntry struct {
	APIKey    string `json:"api-key"`
	AuthIndex string `json:"auth_index"`
}

// ProviderKeys 按组、成员原序展开，只继承 CPA 标准供应商允许共享的展示字段。
func (d *Document) ProviderKeys(defaultBaseURL string) ([]ProviderKeyConfig, error) {
	keys := make([]ProviderKeyConfig, 0)
	for groupIndex, group := range d.Groups {
		var shared struct {
			Prefix         string   `json:"prefix"`
			BaseURL        string   `json:"base-url"`
			Priority       *int     `json:"priority"`
			ExcludedModels []string `json:"excluded-models"`
		}
		if err := decodeFields(group.Fields, &shared); err != nil {
			return nil, fmt.Errorf("group %d: %w", groupIndex, err)
		}
		baseURL := strings.TrimSpace(shared.BaseURL)
		if baseURL == "" {
			baseURL = defaultBaseURL
		}
		for keyIndex, key := range group.Keys {
			var member ProviderKeyConfig
			if err := decodeFields(key.Fields, &member); err != nil {
				return nil, fmt.Errorf("group %d key %d: %w", groupIndex, keyIndex, err)
			}
			// 缺省与 null 继承；显式 0、空字符串和空数组保持成员覆盖语义。
			if inherits(key.Fields["prefix"]) {
				member.Prefix = shared.Prefix
			}
			if inherits(key.Fields["priority"]) {
				member.Priority = shared.Priority
			}
			if inherits(key.Fields["excluded-models"]) {
				member.ExcludedModels = shared.ExcludedModels
			}
			if member.Priority == nil {
				priority := 0
				member.Priority = &priority
			}
			member.Prefix = normalizedPrefix(member.Prefix)
			member.ExcludedModels = normalizedExcludedModels(member.ExcludedModels)
			member.BaseURL = baseURL
			disabled := slices.Contains(member.ExcludedModels, "*")
			member.Disabled = &disabled
			keys = append(keys, member)
		}
	}
	return keys, nil
}

func (d *Document) OpenAIProviders() ([]OpenAICompatibilityConfig, error) {
	providers := make([]OpenAICompatibilityConfig, 0, len(d.Groups))
	for index, group := range d.Groups {
		var provider OpenAICompatibilityConfig
		if err := decodeFields(group.Fields, &provider); err != nil {
			return nil, fmt.Errorf("group %d: %w", index, err)
		}
		if provider.Priority == nil {
			priority := 0
			provider.Priority = &priority
		}
		if provider.Disabled == nil {
			disabled := false
			provider.Disabled = &disabled
		}
		provider.Prefix = normalizedPrefix(provider.Prefix)
		provider.BaseURL = strings.TrimSpace(provider.BaseURL)
		provider.APIKeyEntries = make([]OpenAIApiKeyEntry, 0, len(group.Keys))
		for _, key := range group.Keys {
			var entry OpenAIApiKeyEntry
			if err := decodeFields(key.Fields, &entry); err != nil {
				return nil, fmt.Errorf("group %d key: %w", index, err)
			}
			provider.APIKeyEntries = append(provider.APIKeyEntries, entry)
		}
		providers = append(providers, provider)
	}
	return providers, nil
}

func decodeFields(fields map[string]json.RawMessage, target any) error {
	data, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// v8 GET 返回持久化原文，投影在这里重现 Keeper 消费的少量 CPA 有效值归一化。
func normalizedPrefix(prefix string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if strings.Contains(prefix, "/") {
		return ""
	}
	return prefix
}

func normalizedExcludedModels(models []string) []string {
	if models == nil {
		return nil
	}
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, raw := range models {
		model := strings.ToLower(strings.TrimSpace(raw))
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}
