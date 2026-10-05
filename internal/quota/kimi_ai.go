package quota

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strings"

	"cpa-usage-keeper/internal/cpa/dto/apicall"
	"cpa-usage-keeper/internal/cpa/dto/authfiles"
)

type kimiCredentialReader interface {
	FetchKimiCredentialMetadata(context.Context, string) (*authfiles.KimiCredentialMetadata, error)
}

type kimiAIProvider struct {
	caller             ManagementAPICaller
	config, kimiConfig APICallConfig
}

func NewKimiAIProvider(caller ManagementAPICaller, config, kimiConfig APICallConfig) ProviderHandler {
	return kimiAIProvider{caller: caller, config: config, kimiConfig: kimiConfig}
}

func (p kimiAIProvider) Check(ctx context.Context, input ProviderInput) (ProviderOutput, error) {
	identity := input.Identity
	// CPA runtime-only 凭证没有磁盘 path；不能借同名文件猜测这类账号的站点。
	if identity.FileName == nil || strings.TrimSpace(*identity.FileName) == "" || identity.FilePath == nil || strings.TrimSpace(*identity.FilePath) == "" {
		return ProviderOutput{}, fmt.Errorf("Kimi credential is not downloadable; quota domain is unavailable")
	}
	reader, ok := p.caller.(kimiCredentialReader)
	if !ok {
		return ProviderOutput{}, fmt.Errorf("Kimi credential metadata reader is unavailable")
	}
	metadata, err := reader.FetchKimiCredentialMetadata(ctx, *identity.FileName)
	if err != nil {
		return ProviderOutput{}, err
	}
	if metadata == nil {
		return ProviderOutput{}, fmt.Errorf("Kimi credential metadata is unavailable")
	}
	config := p.config
	if kimiCredentialDomain(*metadata, input) == "com" {
		config = p.kimiConfig
	}
	response, err := p.caller.CallManagementAPI(ctx, apicall.Request{
		AuthIndex: identity.Identity, Method: config.Method, URL: config.URL, Header: copyHeaders(config.Headers),
	})
	if err != nil {
		return ProviderOutput{}, err
	}
	usage, err := parseKimiUsagePayload(response)
	if err != nil {
		return ProviderOutput{}, err
	}
	// 国际账号也可能只有月度汇总；保留 shared Kimi 解析，国内查询仍采用原有合同。
	object, err := parseResponseObject(response)
	if err != nil {
		return ProviderOutput{}, err
	}
	monthly := objectField(objectField(object, "usages"), "limit_month_total")
	if ratio := floatPtrField(monthly, "used_ratio"); ratio != nil && !math.IsNaN(*ratio) && !math.IsInf(*ratio, 0) && *ratio >= 0 {
		usage.Usages = &KimiAggregateUsage{MonthTotal: &KimiUsageRatio{UsedRatio: *ratio, ResetTime: stringField(monthly, "reset_time")}}
	}
	return ProviderOutput{Provider: "kimi-ai", Result: KimiResult{Usage: usage}}, nil
}

func kimiCredentialDomain(metadata authfiles.KimiCredentialMetadata, input ProviderInput) string {
	// 对齐 CPA/CPAMC：显式 domain > base_url > 文件 type > provider；URL 只用于识别两种固定站点。
	if metadata.Domain != "" {
		if kimiDomain(metadata.Domain) == "ai" {
			return "ai"
		}
		return "com"
	}
	if parsed, err := url.Parse(metadata.BaseURL); err == nil {
		host := strings.ToLower(parsed.Hostname())
		switch {
		case host == "kimi.ai", strings.HasSuffix(host, ".kimi.ai"):
			return "ai"
		case host == "kimi.com", strings.HasSuffix(host, ".kimi.com"):
			return "com"
		}
	}
	for _, value := range []string{metadata.Type, input.Identity.Provider, input.Identity.Type} {
		if domain := kimiDomain(value); domain != "" {
			return domain
		}
	}
	return "ai"
}

func kimiDomain(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch {
	case value == "ai", value == "kimi-ai", value == "kimi.ai", strings.HasSuffix(value, ".kimi.ai"):
		return "ai"
	case value == "com", value == "kimi", value == "kimi.com", strings.HasSuffix(value, ".kimi.com"):
		return "com"
	default:
		return ""
	}
}
