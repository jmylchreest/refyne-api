// Package llm provides LLM provider integrations for refyne-api.
// This file bridges refyne library types with refyne-api's needs.
package llm

import (
	refynellm "github.com/jmylchreest/refyne/pkg/llm"
)

// ConvertModelInfo converts refyne ModelInfo to refyne-api ModelInfo.
func ConvertModelInfo(provider string, rm refynellm.ModelInfo) ModelInfo {
	settings := GetDefaultSettings(provider, rm.ID)
	return ModelInfo{
		ID:            rm.ID,
		Name:          rm.Name,
		Provider:      provider,
		ContextWindow: rm.ContextLength,
		Capabilities: ModelCapabilities{
			SupportsStructuredOutputs: rm.Capabilities.SupportsStructuredOutputs,
			SupportsTools:             rm.Capabilities.SupportsTools,
			SupportsStreaming:         rm.Capabilities.SupportsStreaming,
			SupportsReasoning:         rm.Capabilities.SupportsReasoning,
			SupportsResponseFormat:    rm.Capabilities.SupportsResponseFormat,
		},
		DefaultTemp:      settings.Temperature,
		DefaultMaxTokens: settings.MaxTokens,
	}
}

// ConvertModelInfoList converts a slice of refyne ModelInfo to refyne-api ModelInfo.
func ConvertModelInfoList(provider string, models []refynellm.ModelInfo) []ModelInfo {
	result := make([]ModelInfo, 0, len(models))
	for _, m := range models {
		result = append(result, ConvertModelInfo(provider, m))
	}
	return result
}

// ConvertCapabilities converts refyne capabilities to refyne-api capabilities.
func ConvertCapabilities(rc refynellm.ModelCapabilities) ModelCapabilities {
	return ModelCapabilities{
		SupportsStructuredOutputs: rc.SupportsStructuredOutputs,
		SupportsTools:             rc.SupportsTools,
		SupportsStreaming:         rc.SupportsStreaming,
		SupportsReasoning:         rc.SupportsReasoning,
		SupportsResponseFormat:    rc.SupportsResponseFormat,
	}
}
