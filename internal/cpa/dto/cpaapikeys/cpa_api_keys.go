package cpaapikeys

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ManagementAPIKeysResponse 保存 v8 access/api-keys 的直接数组，供现有同步与模型调用使用。
type ManagementAPIKeysResponse struct {
	APIKeys []string
}

func (r *ManagementAPIKeysResponse) UnmarshalJSON(data []byte) error {
	if len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '[' {
		return fmt.Errorf("client API keys must be an array")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	keys := make([]string, 0, len(items))
	for index, raw := range items {
		var key string
		trimmed := bytes.TrimSpace(raw)
		if bytes.Equal(trimmed, []byte("null")) || len(trimmed) > 0 && trimmed[0] == '"' {
			if err := json.Unmarshal(raw, &key); err != nil {
				return err
			}
		} else {
			// 直接取 CPA 返回的数字文本，不用 float64，也不还原 YAML 原始写法。
			var number json.Number
			if err := json.Unmarshal(raw, &number); err != nil {
				return fmt.Errorf("client API key %d: %w", index, err)
			}
			key = number.String()
		}
		keys = append(keys, key)
	}
	r.APIKeys = keys
	return nil
}
