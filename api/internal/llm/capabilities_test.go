package llm

import (
	"testing"
)

// ========================================
// ModelCapabilities Tests
// ========================================

func TestModelCapabilities_Fields(t *testing.T) {
	caps := ModelCapabilities{
		SupportsStructuredOutputs: true,
		SupportsTools:             true,
		SupportsStreaming:         true,
		SupportsReasoning:         false,
		SupportsResponseFormat:    true,
	}

	if !caps.SupportsStructuredOutputs {
		t.Error("SupportsStructuredOutputs should be true")
	}
	if !caps.SupportsTools {
		t.Error("SupportsTools should be true")
	}
	if !caps.SupportsStreaming {
		t.Error("SupportsStreaming should be true")
	}
	if caps.SupportsReasoning {
		t.Error("SupportsReasoning should be false")
	}
	if !caps.SupportsResponseFormat {
		t.Error("SupportsResponseFormat should be true")
	}
}

func TestModelCapabilities_ZeroValue(t *testing.T) {
	var caps ModelCapabilities

	// All fields should be false by default
	if caps.SupportsStructuredOutputs {
		t.Error("SupportsStructuredOutputs should be false by default")
	}
	if caps.SupportsTools {
		t.Error("SupportsTools should be false by default")
	}
	if caps.SupportsStreaming {
		t.Error("SupportsStreaming should be false by default")
	}
	if caps.SupportsReasoning {
		t.Error("SupportsReasoning should be false by default")
	}
	if caps.SupportsResponseFormat {
		t.Error("SupportsResponseFormat should be false by default")
	}
}
