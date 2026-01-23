package handlers

import (
	"context"
	"log/slog"

	"github.com/jmylchreest/refyne-api/internal/http/mw"
)

// UserContext holds user-related context extracted from JWT claims.
// Used across handlers to avoid repeating the same extraction logic.
type UserContext struct {
	UserID              string
	Tier                string
	BYOKAllowed         bool
	ModelsCustomAllowed bool
	LLMProvider         string // For S3 API keys: forced LLM provider
	LLMModel            string // For S3 API keys: forced LLM model
}

// ExtractUserContext extracts user context from JWT claims.
// Returns default values (tier="free", features disabled) if claims are missing.
func ExtractUserContext(ctx context.Context) UserContext {
	uc := UserContext{
		UserID: getUserID(ctx),
		Tier:   "free",
	}

	if claims := mw.GetUserClaims(ctx); claims != nil {
		if claims.Tier != "" {
			uc.Tier = claims.Tier
		}
		uc.BYOKAllowed = claims.HasFeature("provider_byok")
		uc.ModelsCustomAllowed = claims.HasFeature("models_custom")
		uc.LLMProvider = claims.LLMProvider
		uc.LLMModel = claims.LLMModel

		if uc.LLMProvider != "" || uc.LLMModel != "" {
			slog.Debug("extracted injected LLM config from claims",
				"user_id", uc.UserID,
				"llm_provider", uc.LLMProvider,
				"llm_model", uc.LLMModel,
			)
		}
	}

	return uc
}

// IsAuthenticated returns true if the user context has a valid user ID.
func (uc UserContext) IsAuthenticated() bool {
	return uc.UserID != ""
}
