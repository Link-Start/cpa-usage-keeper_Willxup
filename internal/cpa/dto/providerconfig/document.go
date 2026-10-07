package providerconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Document 单独保留 v8 配置原文；未消费字段和数值通过 RawMessage 无损传递。
type Document struct{ Groups []Group }

type Group struct {
	Fields map[string]json.RawMessage
	Keys   []Key
}

type Key struct{ Fields map[string]json.RawMessage }

// Location 只在一次 GET/PUT 内定位首个匹配成员，不作为长期身份保存。
type Location struct {
	Group int
	Key   int
}

func (d *Document) UnmarshalJSON(data []byte) error {
	var groups []json.RawMessage
	if err := decodeArray(data, &groups); err != nil {
		return err
	}
	d.Groups = make([]Group, 0, len(groups))
	for groupIndex, raw := range groups {
		fields, err := decodeObject(raw)
		if err != nil {
			return fmt.Errorf("group %d: %w", groupIndex, err)
		}
		var rawKeys []json.RawMessage
		if err := decodeArray(fields["keys"], &rawKeys); err != nil {
			return fmt.Errorf("group %d keys: %w", groupIndex, err)
		}
		group := Group{Fields: fields, Keys: make([]Key, 0, len(rawKeys))}
		for keyIndex, rawKey := range rawKeys {
			keyFields, err := decodeObject(rawKey)
			if err != nil {
				return fmt.Errorf("group %d key %d: %w", groupIndex, keyIndex, err)
			}
			group.Keys = append(group.Keys, Key{Fields: keyFields})
		}
		d.Groups = append(d.Groups, group)
	}
	return nil
}

func (d Document) MarshalJSON() ([]byte, error) {
	groups := make([]map[string]json.RawMessage, 0, len(d.Groups))
	for _, group := range d.Groups {
		fields := cloneFields(group.Fields)
		keys := make([]map[string]json.RawMessage, 0, len(group.Keys))
		for _, key := range group.Keys {
			keys = append(keys, key.Fields)
		}
		encoded, err := json.Marshal(keys)
		if err != nil {
			return nil, err
		}
		fields["keys"] = encoded
		groups = append(groups, fields)
	}
	return json.Marshal(groups)
}

func (d *Document) FindFirst(authIndex string) (Location, bool) {
	for groupIndex, group := range d.Groups {
		for keyIndex, key := range group.Keys {
			var index string
			if json.Unmarshal(key.Fields["auth_index"], &index) == nil && strings.TrimSpace(index) == authIndex {
				return Location{Group: groupIndex, Key: keyIndex}, true
			}
		}
	}
	return Location{}, false
}

func decodeArray(data []byte, target any) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '[' {
		return fmt.Errorf("expected an array")
	}
	return json.Unmarshal(data, target)
}

func decodeObject(data []byte) (map[string]json.RawMessage, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '{' {
		return nil, fmt.Errorf("expected an object")
	}
	var fields map[string]json.RawMessage
	err := json.Unmarshal(data, &fields)
	return fields, err
}

func inherits(data json.RawMessage) bool {
	return len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null"))
}

func cloneFields(fields map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(fields))
	for key, value := range fields {
		out[key] = value
	}
	return out
}

func (d *Document) SetKeyPriority(location Location, priority int) {
	d.Groups[location.Group].Keys[location.Key].Fields["priority"] = json.RawMessage(fmt.Sprintf("%d", priority))
}

func (d *Document) SetGroupPriority(location Location, priority int) {
	d.Groups[location.Group].Fields["priority"] = json.RawMessage(fmt.Sprintf("%d", priority))
}

func (d *Document) SetKeyExcludedModels(location Location, models []string) error {
	if models == nil {
		models = []string{}
	}
	encoded, err := json.Marshal(models)
	if err != nil {
		return err
	}
	d.Groups[location.Group].Keys[location.Key].Fields["excluded-models"] = encoded
	return nil
}

// Writable 只剔除凭证层只读标识；请求头、模型和插件里的同名字段保持原样。
func (d *Document) Writable() Document {
	writable := Document{Groups: make([]Group, 0, len(d.Groups))}
	for _, group := range d.Groups {
		copy := Group{Fields: cloneFields(group.Fields), Keys: make([]Key, 0, len(group.Keys))}
		delete(copy.Fields, "auth_index")
		delete(copy.Fields, "auth-index")
		for _, key := range group.Keys {
			fields := cloneFields(key.Fields)
			delete(fields, "auth_index")
			delete(fields, "auth-index")
			copy.Keys = append(copy.Keys, Key{Fields: fields})
		}
		writable.Groups = append(writable.Groups, copy)
	}
	return writable
}
