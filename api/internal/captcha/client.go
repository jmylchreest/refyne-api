// Package captcha provides a client for the captcha service.
package captcha

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Client communicates with the captcha service.
type Client struct {
	baseURL    string
	httpClient *http.Client
	signer     *Signer
	logger     *slog.Logger
}

// ClientConfig holds configuration for the captcha client.
type ClientConfig struct {
	BaseURL    string
	Secret     string
	Timeout    time.Duration
	Logger     *slog.Logger
}

// NewClient creates a new captcha service client.
func NewClient(cfg ClientConfig) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	return &Client{
		baseURL: cfg.BaseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		signer: NewSigner(cfg.Secret),
		logger: cfg.Logger,
	}
}

// Cookie represents an HTTP cookie.
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
	HTTPOnly bool   `json:"httpOnly,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	SameSite string `json:"sameSite,omitempty"`
}

// ProxyConfig represents proxy configuration.
type ProxyConfig struct {
	URL      string `json:"url"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// SolveRequest is the request to solve a captcha challenge.
type SolveRequest struct {
	Cmd            string       `json:"cmd"`
	URL            string       `json:"url,omitempty"`
	Session        string       `json:"session,omitempty"`
	MaxTimeout     int          `json:"maxTimeout,omitempty"`
	Cookies        []Cookie     `json:"cookies,omitempty"`
	Proxy          *ProxyConfig `json:"proxy,omitempty"`
	ExternalAPIKey string       `json:"externalApiKey,omitempty"` // API key for external captcha services (2captcha, etc.)
}

// Solution contains the solved page data.
type Solution struct {
	URL        string   `json:"url"`
	Status     int      `json:"status"`
	Headers    map[string]string `json:"headers,omitempty"`
	Cookies    []Cookie `json:"cookies"`
	UserAgent  string   `json:"userAgent"`
	Response   string   `json:"response"` // HTML content
	Title      string   `json:"title,omitempty"`
	Screenshot string   `json:"screenshot,omitempty"`
}

// SolveResponse is the response from the captcha service.
type SolveResponse struct {
	Status         string    `json:"status"`
	Message        string    `json:"message"`
	Solution       *Solution `json:"solution,omitempty"`
	Session        string    `json:"session,omitempty"`
	Sessions       []string  `json:"sessions,omitempty"`
	StartTimestamp int64     `json:"startTimestamp"`
	EndTimestamp   int64     `json:"endTimestamp"`
	Version        string    `json:"version"`
	ChallengeType  string    `json:"challengeType,omitempty"`
	SolverUsed     string    `json:"solverUsed,omitempty"`
	Challenged     bool      `json:"challenged,omitempty"`
	Solved         bool      `json:"solved,omitempty"`
	Method         string    `json:"method,omitempty"`
	// InstanceID is populated from fly-replay-src header for session affinity routing
	InstanceID string `json:"-"`
}

// UserContext contains user information for the request.
type UserContext struct {
	UserID   string
	Tier     string
	Features []string
	JobID    string // Optional job ID for tracking
}

// HealthResponse is the response from the captcha service health endpoint.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// Health checks the captcha service health and returns version info.
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("health check failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	var health HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("failed to decode health response: %w", err)
	}

	return &health, nil
}

// Solve sends a solve request to the captcha service.
func (c *Client) Solve(ctx context.Context, user UserContext, req SolveRequest, instanceID string) (*SolveResponse, error) {
	return c.request(ctx, user, req, instanceID)
}

// CreateSession creates a new browser session.
func (c *Client) CreateSession(ctx context.Context, user UserContext, sessionID string) (*SolveResponse, error) {
	req := SolveRequest{
		Cmd:     "sessions.create",
		Session: sessionID,
	}
	return c.request(ctx, user, req, "")
}

// DestroySession destroys a browser session.
func (c *Client) DestroySession(ctx context.Context, user UserContext, sessionID, instanceID string) (*SolveResponse, error) {
	req := SolveRequest{
		Cmd:     "sessions.destroy",
		Session: sessionID,
	}
	return c.request(ctx, user, req, instanceID)
}

// ListSessions lists all active sessions.
func (c *Client) ListSessions(ctx context.Context, user UserContext) (*SolveResponse, error) {
	req := SolveRequest{
		Cmd: "sessions.list",
	}
	return c.request(ctx, user, req, "")
}

// request sends a request to the captcha service.
func (c *Client) request(ctx context.Context, user UserContext, req SolveRequest, instanceID string) (*SolveResponse, error) {
	startTime := time.Now()

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	httpReq.Header.Set("Content-Type", "application/json")

	// Sign the request (includes JobID in signature for integrity)
	sig := c.signer.Sign(user.UserID, user.Tier, user.Features, user.JobID, body)
	httpReq.Header.Set("X-Refyne-Signature", sig.Signature)
	httpReq.Header.Set("X-Refyne-Timestamp", sig.Timestamp)
	httpReq.Header.Set("X-Refyne-User-ID", sig.UserID)
	httpReq.Header.Set("X-Refyne-Tier", sig.Tier)
	httpReq.Header.Set("X-Refyne-Features", sig.Features)
	httpReq.Header.Set("X-Refyne-Job-ID", sig.JobID)

	// Add instance routing header if specified (for session affinity)
	if instanceID != "" {
		httpReq.Header.Set("fly-force-instance-id", instanceID)
	}

	c.logger.Info("captcha request",
		"user_id", user.UserID,
		"job_id", user.JobID,
		"cmd", req.Cmd,
		"url", req.URL,
		"session", req.Session,
		"instance_id", instanceID,
	)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.logger.Error("captcha request failed",
			"user_id", user.UserID,
			"job_id", user.JobID,
			"cmd", req.Cmd,
			"url", req.URL,
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	durationMs := time.Since(startTime).Milliseconds()

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("captcha service error",
			"user_id", user.UserID,
			"job_id", user.JobID,
			"cmd", req.Cmd,
			"url", req.URL,
			"status_code", resp.StatusCode,
			"duration_ms", durationMs,
		)
		return nil, fmt.Errorf("captcha service returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var solveResp SolveResponse
	if err := json.Unmarshal(respBody, &solveResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Get instance ID from response header for session affinity tracking
	// fly-replay-src contains the instance ID that handled the request
	respInstanceID := resp.Header.Get("fly-replay-src")
	solveResp.InstanceID = respInstanceID

	c.logger.Info("captcha response",
		"user_id", user.UserID,
		"job_id", user.JobID,
		"cmd", req.Cmd,
		"url", req.URL,
		"status", solveResp.Status,
		"challenged", solveResp.Challenged,
		"solved", solveResp.Solved,
		"challenge_type", solveResp.ChallengeType,
		"solver_used", solveResp.SolverUsed,
		"instance_id", respInstanceID,
		"captcha_version", solveResp.Version,
		"duration_ms", durationMs,
	)

	return &solveResp, nil
}
