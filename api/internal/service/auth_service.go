package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrTokenExpired = errors.New("token expired")
)

// AuthService handles authentication logic.
// Note: User authentication (login, register, OAuth) is handled by Clerk.
// This service only handles API key validation.
type AuthService struct {
	cfg    *config.Config
	repos  *repository.Repositories
	logger *slog.Logger
}

// NewAuthService creates a new auth service.
func NewAuthService(cfg *config.Config, repos *repository.Repositories, logger *slog.Logger) *AuthService {
	return &AuthService{
		cfg:    cfg,
		repos:  repos,
		logger: logger,
	}
}

// TokenClaims represents claims extracted from API key validation.
type TokenClaims struct {
	UserID           string   `json:"uid"`
	Email            string   `json:"email"`
	Tier             string   `json:"tier"`
	GlobalSuperadmin bool     `json:"global_superadmin,omitempty"`
	Scopes           []string `json:"scopes,omitempty"`
}

// ValidateAPIKey validates an API key and returns claims.
// Note: The tier and admin status come from Clerk metadata via the middleware
// when using JWT auth. For API keys, we store minimal info and rely on
// the Clerk user ID for authorization decisions.
func (s *AuthService) ValidateAPIKey(ctx context.Context, apiKey string) (*TokenClaims, error) {
	// Hash the API key
	hash := sha256.Sum256([]byte(apiKey))
	hashStr := hex.EncodeToString(hash[:])

	// Find the key
	key, err := s.repos.APIKey.GetByKeyHash(ctx, hashStr)
	if err != nil {
		return nil, errors.Join(ErrInvalidToken, err)
	}
	if key == nil {
		return nil, ErrInvalidToken
	}

	// Check if revoked
	if key.RevokedAt != nil {
		return nil, ErrInvalidToken
	}

	// Check expiration
	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	// Update last used (fire and forget)
	go func() {
		_ = s.repos.APIKey.UpdateLastUsed(context.Background(), key.ID, time.Now())
	}()

	// Return claims with user ID - tier/admin comes from Clerk metadata
	// For API keys in self-hosted mode, use selfhosted tier for unlimited access.
	// In hosted mode, this returns free tier; Clerk metadata provides actual tier.
	tier := "free"
	if s.cfg.IsSelfHosted() {
		tier = "selfhosted"
	}

	return &TokenClaims{
		UserID:           key.UserID, // Clerk user ID
		Email:            "",         // Not stored with API key
		Tier:             tier,
		GlobalSuperadmin: false, // API keys don't get superadmin access
		Scopes:           key.Scopes,
	}, nil
}
