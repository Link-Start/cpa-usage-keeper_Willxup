package service

import (
	"testing"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository/dto"
)

func TestNormalizeUsageEventTokensUsesCodexStyleOutputForGeminiFamily(t *testing.T) {
	for _, usageType := range []string{"gemini", "vertex", "gemini-cli", "gemini-cli-code-assist", "antigravity", "aistudio", "ai-studio"} {
		t.Run(usageType, func(t *testing.T) {
			event := NormalizeUsageEventTokens(entities.UsageEvent{
				InputTokens:     11,
				OutputTokens:    7,
				ReasoningTokens: 3,
				CachedTokens:    5,
				TotalTokens:     21,
			}, usageType)

			if event.InputTokens != 11 || event.OutputTokens != 10 || event.ReasoningTokens != 3 || event.CachedTokens != 5 || event.TotalTokens != 21 {
				t.Fatalf("expected %s to normalize to Codex-style output tokens, got %+v", usageType, event)
			}
		})
	}
}

func TestNormalizeUsageEventTokensBackfillsTotalWithCodexStyleOutput(t *testing.T) {
	event := NormalizeUsageEventTokens(entities.UsageEvent{
		InputTokens:     11,
		OutputTokens:    7,
		ReasoningTokens: 3,
		CachedTokens:    5,
	}, "gemini")

	if event.InputTokens != 11 || event.OutputTokens != 10 || event.ReasoningTokens != 3 || event.TotalTokens != 21 {
		t.Fatalf("expected Gemini missing total to use input plus normalized output, got %+v", event)
	}
}

func TestNormalizeUsageEventTokensDoesNotDoubleCountCodexReasoningWhenTotalMissing(t *testing.T) {
	event := NormalizeUsageEventTokens(entities.UsageEvent{
		InputTokens:     11,
		OutputTokens:    10,
		ReasoningTokens: 3,
		CachedTokens:    5,
	}, "codex")

	if event.InputTokens != 11 || event.OutputTokens != 10 || event.ReasoningTokens != 3 || event.TotalTokens != 21 {
		t.Fatalf("expected Codex missing total to use input plus output, got %+v", event)
	}
}

func TestNormalizeUsageEventTokensKeepsAlreadyIncludedOutputWhenTotalMissing(t *testing.T) {
	for _, usageType := range []string{"codex", "openai", "custom"} {
		t.Run(usageType, func(t *testing.T) {
			event := NormalizeUsageEventTokens(entities.UsageEvent{
				InputTokens:     11,
				OutputTokens:    10,
				ReasoningTokens: 3,
				CachedTokens:    5,
			}, usageType)

			if event.InputTokens != 11 || event.OutputTokens != 10 || event.ReasoningTokens != 3 || event.CachedTokens != 5 || event.TotalTokens != 21 {
				t.Fatalf("expected %s to keep Codex/OpenAI-style output tokens, got %+v", usageType, event)
			}
		})
	}
}

func TestNormalizeUsageEventTokensDoesNotFoldCodexWhenCompatibilityWouldFold(t *testing.T) {
	event := NormalizeUsageEventTokens(entities.UsageEvent{
		InputTokens:     11,
		OutputTokens:    7,
		ReasoningTokens: 3,
		CachedTokens:    5,
		TotalTokens:     21,
	}, "codex")

	if event.InputTokens != 11 || event.OutputTokens != 7 || event.ReasoningTokens != 3 || event.CachedTokens != 5 || event.TotalTokens != 21 {
		t.Fatalf("expected codex normalization to keep output unchanged, got %+v", event)
	}
}

func TestNormalizeXAIStyleTokensKeepsResponsesOutput(t *testing.T) {
	tokens := normalizeXAIStyleTokens(dto.TokenStats{
		InputTokens:     11,
		OutputTokens:    10,
		ReasoningTokens: 3,
		CachedTokens:    5,
	})

	if tokens.InputTokens != 11 || tokens.OutputTokens != 10 || tokens.ReasoningTokens != 3 || tokens.CachedTokens != 5 || tokens.TotalTokens != 21 {
		t.Fatalf("expected xAI Responses tokens to keep Codex-style output tokens, got %+v", tokens)
	}
}

func TestNormalizeUsageEventTokensFoldsGeminiStyleReasoningForOpenAICompatibility(t *testing.T) {
	for _, usageType := range []string{"openai", "openai-compatible", "openai_compatibility"} {
		t.Run(usageType, func(t *testing.T) {
			event := NormalizeUsageEventTokens(entities.UsageEvent{
				InputTokens:     11,
				OutputTokens:    7,
				ReasoningTokens: 3,
				CachedTokens:    5,
				TotalTokens:     21,
			}, usageType)

			if event.InputTokens != 11 || event.OutputTokens != 10 || event.ReasoningTokens != 3 || event.CachedTokens != 5 || event.TotalTokens != 21 {
				t.Fatalf("expected %s to fold Gemini-style reasoning into output, got %+v", usageType, event)
			}
		})
	}
}

func TestNormalizeUsageEventTokensKeepsCodexStyleOutputForOpenAICompatibility(t *testing.T) {
	for _, usageType := range []string{"openai", "openai-compatible", "openai_compatibility"} {
		t.Run(usageType, func(t *testing.T) {
			event := NormalizeUsageEventTokens(entities.UsageEvent{
				InputTokens:     11,
				OutputTokens:    10,
				ReasoningTokens: 3,
				CachedTokens:    5,
				TotalTokens:     21,
			}, usageType)

			if event.InputTokens != 11 || event.OutputTokens != 10 || event.ReasoningTokens != 3 || event.CachedTokens != 5 || event.TotalTokens != 21 {
				t.Fatalf("expected %s to keep Codex-style output when reasoning is already included, got %+v", usageType, event)
			}
		})
	}
}

func TestNormalizeUsageEventTokensDoesNotFoldCodexReasoningWhenTotalPresent(t *testing.T) {
	for _, usageType := range []string{"codex"} {
		t.Run(usageType, func(t *testing.T) {
			event := NormalizeUsageEventTokens(entities.UsageEvent{
				InputTokens:     11,
				OutputTokens:    10,
				ReasoningTokens: 3,
				CachedTokens:    5,
				TotalTokens:     21,
			}, usageType)

			if event.InputTokens != 11 || event.OutputTokens != 10 || event.ReasoningTokens != 3 || event.CachedTokens != 5 || event.TotalTokens != 21 {
				t.Fatalf("expected %s normalization to keep output unchanged, got %+v", usageType, event)
			}
		})
	}
}
