package models

// Solution contains the result of a successful challenge solve.
type Solution struct {
	URL        string            `json:"url"`
	Status     int               `json:"status"`
	Headers    map[string]string `json:"headers,omitempty"`
	Cookies    []Cookie          `json:"cookies"`
	UserAgent  string            `json:"userAgent"`
	Response   string            `json:"response"`             // HTML content
	Title      string            `json:"title,omitempty"`      // Page title
	Screenshot string            `json:"screenshot,omitempty"` // Base64-encoded screenshot
}

// UsageInfo contains usage tracking data (Refyne extension).
type UsageInfo struct {
	BrowserTimeMS  int64   `json:"browserTimeMs,omitempty"`
	SolverUsed     string  `json:"solverUsed,omitempty"`
	ChallengeType  string  `json:"challengeType,omitempty"`
	BrowserCostUSD float64 `json:"browserCostUsd,omitempty"`
	SolverCostUSD  float64 `json:"solverCostUsd,omitempty"`
}

// SolveResponse is a FlareSolverr-compatible response.
type SolveResponse struct {
	Status         string     `json:"status"`                   // "ok" | "error"
	Message        string     `json:"message"`                  // Human-readable message
	Solution       *Solution  `json:"solution,omitempty"`       // Solution data (on success)
	Session        string     `json:"session,omitempty"`        // Session ID (for session commands)
	Sessions       []string   `json:"sessions,omitempty"`       // List of session IDs (for sessions.list)
	StartTimestamp int64      `json:"startTimestamp"`           // Unix timestamp ms
	EndTimestamp   int64      `json:"endTimestamp"`             // Unix timestamp ms
	Version        string     `json:"version"`                  // Service version
	ChallengeType  string     `json:"challengeType,omitempty"`  // Type of challenge detected
	SolverUsed     string     `json:"solverUsed,omitempty"`     // Solver used (if any) - deprecated, use Method

	// Challenge tracking (Refyne extensions)
	Challenged bool   `json:"challenged"`          // Whether a challenge was detected
	Solved     bool   `json:"solved"`              // Whether the challenge was solved
	Method     string `json:"method,omitempty"`    // How resolved: "cached", "resolved", "2captcha", etc.

	// Refyne extensions
	RequestID string     `json:"requestId,omitempty"` // Unique request ID
	Usage     *UsageInfo `json:"usage,omitempty"`     // Usage tracking
}

// SessionInfo contains information about a browser session.
type SessionInfo struct {
	ID           string `json:"id"`
	CreatedAt    int64  `json:"createdAt"`          // Unix timestamp ms
	LastUsedAt   int64  `json:"lastUsedAt"`         // Unix timestamp ms
	RequestCount int    `json:"requestCount"`       // Number of requests made
	UserAgent    string `json:"userAgent,omitempty"` // User agent if set
	Proxy        string `json:"proxy,omitempty"`     // Proxy URL if set (masked)
}

// SessionsResponse is returned for session management commands.
type SessionsResponse struct {
	Status   string        `json:"status"`
	Message  string        `json:"message"`
	Sessions []SessionInfo `json:"sessions,omitempty"`
	Session  string        `json:"session,omitempty"` // For sessions.create
	Version  string        `json:"version"`
}

// HumaSolveResponse wraps SolveResponse for Huma API.
type HumaSolveResponse struct {
	Body SolveResponse
}

// HumaSessionsResponse wraps SessionsResponse for Huma API.
type HumaSessionsResponse struct {
	Body SessionsResponse
}

// HealthResponse is returned by the health endpoint.
type HealthResponse struct {
	Status          string `json:"status"`
	Version         string `json:"version"`
	BrowserPoolSize int    `json:"browserPoolSize"`
	ActiveSessions  int    `json:"activeSessions"`
	Uptime          int64  `json:"uptimeSeconds"`
}

// HumaHealthResponse wraps HealthResponse for Huma API.
type HumaHealthResponse struct {
	Body HealthResponse
}

// NewErrorResponse creates an error response.
func NewErrorResponse(message string, startTime, endTime int64, version, requestID string) *SolveResponse {
	return &SolveResponse{
		Status:         "error",
		Message:        message,
		StartTimestamp: startTime,
		EndTimestamp:   endTime,
		Version:        version,
		RequestID:      requestID,
	}
}

// NewSuccessResponse creates a success response with a solution.
func NewSuccessResponse(solution *Solution, startTime, endTime int64, version, requestID string) *SolveResponse {
	return &SolveResponse{
		Status:         "ok",
		Message:        "Challenge solved successfully",
		Solution:       solution,
		StartTimestamp: startTime,
		EndTimestamp:   endTime,
		Version:        version,
		RequestID:      requestID,
	}
}
