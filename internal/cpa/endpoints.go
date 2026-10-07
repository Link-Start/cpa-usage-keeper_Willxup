package cpa

const (
	cpaManagementAuthFilesEndpoint           = "/v8/management/credentials"
	cpaManagementAuthFilesDownloadEndpoint   = "/v8/management/credentials/download"
	cpaManagementAuthFilesStatusEndpoint     = "/v8/management/credentials/status"
	cpaManagementAuthFilesFieldsEndpoint     = "/v8/management/credentials/fields"
	cpaManagementAPIKeysEndpoint             = "/v0/management/api-keys"
	cpaManagementVertexAPIKeyEndpoint        = "/v0/management/vertex-api-key"
	cpaManagementGeminiAPIKeyEndpoint        = "/v0/management/gemini-api-key"
	cpaManagementCodexAPIKeyEndpoint         = "/v0/management/codex-api-key"
	cpaManagementClaudeAPIKeyEndpoint        = "/v0/management/claude-api-key"
	cpaManagementMetaAPIKeyEndpoint          = "/v0/management/meta-api-key"
	cpaManagementAmpcodeEndpoint             = "/v0/management/ampcode"
	cpaManagementOpenAICompatibilityEndpoint = "/v0/management/openai-compatibility"
	cpaManagementUsageQueueEndpoint          = "/v8/management/observability/usage/queue"
	cpaManagementAPICallEndpoint             = "/v8/management/requests/api-call"
	cpaManagementResetQuotaEndpoint          = "/v8/management/routing/cooldown/reset"
	cpaManagementRequestLogByIDEndpoint      = "/v8/management/observability/logs/requests"
	cpaModelsEndpoint                        = "/v1/models"

	cpaManagementRedisNetwork       = "tcp"
	ManagementRedisDefaultPort      = "8317"
	ManagementRedisAuthCommand      = "AUTH"
	ManagementRedisPopCommand       = "LPOP"
	ManagementRedisSubscribeCommand = "SUBSCRIBE"
	ManagementUsageQueueKey         = "usage"
	ManagementUsageLegacyQueueKey   = "queue"
	ManagementUsageSubscribeChannel = "usage"
	// ManagementErrorsSubscribeChannel 是 CPA 只广播、不提供 LPOP 补偿的凭证错误 channel。
	ManagementErrorsSubscribeChannel = "errors"
	ManagementUsageQueueMaxBatchSize = 10000
)

const (
	// cpaManagementInteractionsAPIKeyEndpoint 只读取 Gemini Interactions metadata，不参与 usage 拉取。
	cpaManagementInteractionsAPIKeyEndpoint = "/v0/management/interactions-api-key"
	// cpaManagementXAIAPIKeyEndpoint 只读取 xAI API Key metadata，不改变现有 xAI OAuth 或 quota 路径。
	cpaManagementXAIAPIKeyEndpoint = "/v0/management/xai-api-key"
)
