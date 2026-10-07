package apicall

import (
	"encoding/json"
	"fmt"
)

type Request struct {
	AuthIndex string            `json:"authIndex"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Header    map[string]string `json:"header,omitempty"`
	Data      any               `json:"data,omitempty"`
}

func (r Request) MarshalJSON() ([]byte, error) {
	type alias Request
	encoded := alias(r)
	if r.Data != nil {
		if _, ok := r.Data.(string); !ok {
			dataBytes, err := json.Marshal(r.Data)
			if err != nil {
				return nil, err
			}
			encoded.Data = string(dataBytes)
		}
	}
	return json.Marshal(encoded)
}

// Response 保存 v8 api-call 的原始字符串 body 与供额度解析使用的文本视图。
type Response struct {
	StatusCode int                 `json:"status_code"`
	Header     map[string][]string `json:"header"`
	BodyText   string              `json:"-"`
	Body       json.RawMessage     `json:"body"`
}

func (r *Response) UnmarshalJSON(data []byte) error {
	var decoded struct {
		StatusCode int                 `json:"status_code"`
		Header     map[string][]string `json:"header"`
		Body       json.RawMessage     `json:"body"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var bodyText string
	if err := json.Unmarshal(decoded.Body, &bodyText); err != nil {
		return fmt.Errorf("decode api-call body: %w", err)
	}
	*r = Response{StatusCode: decoded.StatusCode, Header: decoded.Header, BodyText: bodyText, Body: decoded.Body}
	return nil
}
