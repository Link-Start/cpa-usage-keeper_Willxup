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
		if err := decodeFields(group.Fields, &shared, "prefix", "base-url", "excluded-models"); err != nil {
			return nil, fmt.Errorf("group %d: %w", groupIndex, err)
		}
		baseURL := strings.TrimSpace(shared.BaseURL)
		if baseURL == "" {
			baseURL = defaultBaseURL
		}
		for keyIndex, key := range group.Keys {
			var member ProviderKeyConfig
			if err := decodeFields(key.Fields, &member, "api-key", "prefix", "name", "base-url", "note", "excluded-models"); err != nil {
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
		// 成员另行投影，避免组结构中的 keys 先触发严格字符串解码。
		groupFields := cloneFields(group.Fields)
		delete(groupFields, "keys")
		if err := decodeFields(groupFields, &provider, "name", "prefix", "base-url", "note"); err != nil {
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
			if err := decodeFields(key.Fields, &entry, "api-key"); err != nil {
				return nil, fmt.Errorf("group %d key: %w", index, err)
			}
			provider.APIKeyEntries = append(provider.APIKeyEntries, entry)
		}
		providers = append(providers, provider)
	}
	return providers, nil
}

func decodeFields(fields map[string]json.RawMessage, target any, textFields ...string) error {
	// 只在消费文字字段的投影副本中保留 CPA 返回数字的文本，不改变 GET 原文和写回值。
	projection := cloneFields(fields)
	for _, field := range textFields {
		raw := projection[field]
		if inherits(raw) {
			continue
		}
		var converted json.RawMessage
		var err error
		if field == "excluded-models" {
			converted, err = projectTextList(raw)
		} else {
			converted, err = projectTextValue(raw)
		}
		if err != nil {
			return fmt.Errorf("field %s: %w", field, err)
		}
		projection[field] = converted
	}
	data, err := json.Marshal(projection)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// JSON 字符串/null 保持原语义；数字直接取 json.Number 文本，避免经过 float64。
func projectTextValue(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "null" || strings.HasPrefix(trimmed, "\"") {
		return raw, nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return nil, err
	}
	return json.Marshal(number.String())
}

func projectTextList(raw json.RawMessage) (json.RawMessage, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	for index, item := range items {
		converted, err := projectTextValue(item)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", index, err)
		}
		items[index] = converted
	}
	return json.Marshal(items)
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
