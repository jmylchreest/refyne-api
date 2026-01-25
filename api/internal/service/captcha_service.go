// Package service provides business logic services.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jmylchreest/refyne-api/internal/captcha"
)

// CaptchaService provides internal captcha/browser solving for dynamic content.
// This is an internal service used by the extraction pipeline, not exposed to users.
// Users with the "content_dynamic" feature get JavaScript/real browser support.
type CaptchaService struct {
	client   *captcha.Client
	sessions sync.Map // sessionID -> SessionInfo
	logger   *slog.Logger

	// External captcha service API key (for 2captcha, etc. when native solving fails)
	externalAPIKey string
}

// SessionInfo tracks session ownership and instance routing.
type SessionInfo struct {
	SessionID  string
	InstanceID string
	UserID     string
	CreatedAt  time.Time
}

// CaptchaServiceConfig holds configuration for the captcha service.
type CaptchaServiceConfig struct {
	ServiceURL     string // Internal captcha service URL (use .internal for Fly private networking)
	Secret         string // HMAC secret for signing requests
	ExternalAPIKey string // API key for external captcha services (2captcha, etc.)
	Logger         *slog.Logger
}

// NewCaptchaService creates a new captcha service for internal use.
func NewCaptchaService(cfg CaptchaServiceConfig) *CaptchaService {
	client := captcha.NewClient(captcha.ClientConfig{
		BaseURL: cfg.ServiceURL,
		Secret:  cfg.Secret,
		Timeout: 120 * time.Second,
		Logger:  cfg.Logger,
	})

	return &CaptchaService{
		client:         client,
		logger:         cfg.Logger,
		externalAPIKey: cfg.ExternalAPIKey,
	}
}

// CheckHealth checks the captcha service health and logs the version.
// This is useful for verifying connectivity and version compatibility on startup.
// Returns nil if the service is unavailable (scale-to-zero) - this is not an error.
func (s *CaptchaService) CheckHealth(ctx context.Context) error {
	health, err := s.client.Health(ctx)
	if err != nil {
		s.logger.Debug("captcha service health check failed (may be scaled to zero)", "error", err)
		return nil // Not an error - service may be scaled to zero
	}

	s.logger.Info("captcha service connected",
		"status", health.Status,
		"captcha_version", health.Version,
	)

	return nil
}

// CaptchaSolveInput is the input for solving a captcha/fetching dynamic content.
type CaptchaSolveInput struct {
	URL        string
	Session    string
	MaxTimeout int
	Cookies    []captcha.Cookie
	Proxy      *captcha.ProxyConfig
	JobID      string // Optional job ID for tracking/logging
}

// CaptchaSolveOutput is the output from solving a captcha.
type CaptchaSolveOutput struct {
	Status        string
	Message       string
	Solution      *captcha.Solution
	SessionID     string
	ChallengeType string
	Solved        bool
	// ExternalServiceUsed indicates if an external service (2captcha, etc.) was used
	ExternalServiceUsed bool
}

// FetchDynamicContent fetches content using a real browser, solving any challenges encountered.
// This is called internally by the extraction service for users with "content_dynamic" feature.
// Returns ErrSessionNotFound if specified session doesn't exist, ErrSessionNotOwned if user doesn't own it.
func (s *CaptchaService) FetchDynamicContent(ctx context.Context, userID, tier string, input CaptchaSolveInput) (*CaptchaSolveOutput, error) {
	// Get instance ID for session affinity if session is specified
	instanceID := ""
	if input.Session != "" {
		info, ok := s.sessions.Load(input.Session)
		if !ok {
			return nil, ErrSessionNotFound
		}
		sessionInfo := info.(SessionInfo)
		// Validate session ownership
		if sessionInfo.UserID != userID {
			s.logger.Warn("session ownership validation failed in fetch",
				"session_id", input.Session,
				"owner_id", sessionInfo.UserID,
				"requester_id", userID,
			)
			return nil, ErrSessionNotOwned
		}
		instanceID = sessionInfo.InstanceID
	}

	// Create user context for the captcha service
	userCtx := captcha.UserContext{
		UserID:   userID,
		Tier:     tier,
		Features: []string{"content_dynamic"},
		JobID:    input.JobID,
	}

	// Build request
	req := captcha.SolveRequest{
		Cmd:        "request.get",
		URL:        input.URL,
		Session:    input.Session,
		MaxTimeout: input.MaxTimeout,
		Cookies:    input.Cookies,
		Proxy:      input.Proxy,
	}

	// If we have an external API key, include it for fallback to external services
	if s.externalAPIKey != "" {
		req.ExternalAPIKey = s.externalAPIKey
	}

	s.logger.Info("fetching dynamic content",
		"user_id", userID,
		"job_id", input.JobID,
		"url", input.URL,
		"session", input.Session,
	)

	// Send request to captcha service
	resp, err := s.client.Solve(ctx, userCtx, req, instanceID)
	if err != nil {
		s.logger.Error("captcha service error",
			"user_id", userID,
			"job_id", input.JobID,
			"url", input.URL,
			"error", err,
		)
		return nil, fmt.Errorf("captcha service error: %w", err)
	}

	if resp.Status != "ok" {
		s.logger.Warn("dynamic content fetch failed",
			"user_id", userID,
			"job_id", input.JobID,
			"url", input.URL,
			"status", resp.Status,
			"message", resp.Message,
		)
		return nil, fmt.Errorf("dynamic content fetch failed: %s", resp.Message)
	}

	// Update session instance ID for future affinity routing
	if input.Session != "" && resp.InstanceID != "" {
		if info, ok := s.sessions.Load(input.Session); ok {
			sessionInfo := info.(SessionInfo)
			if sessionInfo.InstanceID != resp.InstanceID {
				sessionInfo.InstanceID = resp.InstanceID
				s.sessions.Store(input.Session, sessionInfo)
				s.logger.Debug("updated session instance ID",
					"session", input.Session,
					"instance_id", resp.InstanceID,
				)
			}
		}
	}

	s.logger.Info("dynamic content fetched",
		"user_id", userID,
		"job_id", input.JobID,
		"url", input.URL,
		"challenge_type", resp.ChallengeType,
		"challenged", resp.Challenged,
		"solved", resp.Solved,
		"external_service_used", resp.SolverUsed != "" && resp.SolverUsed != "native",
	)

	return &CaptchaSolveOutput{
		Status:              resp.Status,
		Message:             resp.Message,
		Solution:            resp.Solution,
		SessionID:           resp.Session,
		ChallengeType:       resp.ChallengeType,
		Solved:              resp.Solved,
		ExternalServiceUsed: resp.SolverUsed != "" && resp.SolverUsed != "native",
	}, nil
}

// CreateSession creates a new browser session for maintaining state across requests.
func (s *CaptchaService) CreateSession(ctx context.Context, userID, tier string) (*SessionInfo, error) {
	userCtx := captcha.UserContext{
		UserID:   userID,
		Tier:     tier,
		Features: []string{"content_dynamic"},
	}

	resp, err := s.client.CreateSession(ctx, userCtx, "")
	if err != nil {
		return nil, err
	}

	if resp.Status != "ok" {
		return nil, fmt.Errorf("failed to create session: %s", resp.Message)
	}

	// Store session info for routing
	info := SessionInfo{
		SessionID:  resp.Session,
		InstanceID: "", // Will be populated on first request
		UserID:     userID,
		CreatedAt:  time.Now(),
	}
	s.sessions.Store(resp.Session, info)

	return &info, nil
}

// ErrSessionNotFound is returned when a session does not exist.
var ErrSessionNotFound = fmt.Errorf("session not found")

// ErrSessionNotOwned is returned when a user tries to access a session they don't own.
var ErrSessionNotOwned = fmt.Errorf("session not owned by user")

// DestroySession destroys a browser session.
// Returns ErrSessionNotFound if session doesn't exist, ErrSessionNotOwned if user doesn't own it.
func (s *CaptchaService) DestroySession(ctx context.Context, userID, tier, sessionID string) error {
	// Get instance ID for routing and validate ownership
	info, ok := s.sessions.Load(sessionID)
	if !ok {
		return ErrSessionNotFound
	}

	sessionInfo := info.(SessionInfo)

	// Validate session ownership - users can only destroy their own sessions
	if sessionInfo.UserID != userID {
		s.logger.Warn("session ownership validation failed",
			"session_id", sessionID,
			"owner_id", sessionInfo.UserID,
			"requester_id", userID,
		)
		return ErrSessionNotOwned
	}

	userCtx := captcha.UserContext{
		UserID:   userID,
		Tier:     tier,
		Features: []string{"content_dynamic"},
	}

	resp, err := s.client.DestroySession(ctx, userCtx, sessionID, sessionInfo.InstanceID)
	if err != nil {
		return err
	}

	if resp.Status != "ok" {
		return fmt.Errorf("failed to destroy session: %s", resp.Message)
	}

	// Remove from local tracking
	s.sessions.Delete(sessionID)

	return nil
}

// CleanupUserSessions destroys all sessions for a user.
func (s *CaptchaService) CleanupUserSessions(ctx context.Context, userID, tier string) error {
	var sessionsToCleanup []string

	s.sessions.Range(func(key, value any) bool {
		info := value.(SessionInfo)
		if info.UserID == userID {
			sessionsToCleanup = append(sessionsToCleanup, info.SessionID)
		}
		return true
	})

	var lastErr error
	for _, sessionID := range sessionsToCleanup {
		if err := s.DestroySession(ctx, userID, tier, sessionID); err != nil {
			s.logger.Warn("failed to cleanup session", "session_id", sessionID, "error", err)
			lastErr = err
		}
	}

	return lastErr
}
