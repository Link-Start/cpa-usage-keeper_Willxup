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
	return json.Unmarshal(data, &r.APIKeys)
}
