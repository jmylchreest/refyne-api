package models

import (
	"testing"
)

func TestNewErrorResponse(t *testing.T) {
	resp := NewErrorResponse(
		"test error message",
		1000,
		2000,
		"1.0.0",
		"req-123",
	)

	if resp.Status != "error" {
		t.Errorf("Status = %q, want %q", resp.Status, "error")
	}
	if resp.Message != "test error message" {
		t.Errorf("Message = %q, want %q", resp.Message, "test error message")
	}
	if resp.StartTimestamp != 1000 {
		t.Errorf("StartTimestamp = %d, want %d", resp.StartTimestamp, 1000)
	}
	if resp.EndTimestamp != 2000 {
		t.Errorf("EndTimestamp = %d, want %d", resp.EndTimestamp, 2000)
	}
	if resp.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", resp.Version, "1.0.0")
	}
	if resp.RequestID != "req-123" {
		t.Errorf("RequestID = %q, want %q", resp.RequestID, "req-123")
	}
	if resp.Solution != nil {
		t.Errorf("Solution = %+v, want nil", resp.Solution)
	}
}

func TestNewSuccessResponse(t *testing.T) {
	solution := &Solution{
		URL:       "https://example.com",
		Status:    200,
		UserAgent: "Test Agent",
		Response:  "<html></html>",
		Cookies: []Cookie{
			{Name: "session", Value: "abc123"},
		},
	}

	resp := NewSuccessResponse(
		solution,
		1000,
		2000,
		"1.0.0",
		"req-456",
	)

	if resp.Status != "ok" {
		t.Errorf("Status = %q, want %q", resp.Status, "ok")
	}
	if resp.Message != "Challenge solved successfully" {
		t.Errorf("Message = %q, want %q", resp.Message, "Challenge solved successfully")
	}
	if resp.StartTimestamp != 1000 {
		t.Errorf("StartTimestamp = %d, want %d", resp.StartTimestamp, 1000)
	}
	if resp.EndTimestamp != 2000 {
		t.Errorf("EndTimestamp = %d, want %d", resp.EndTimestamp, 2000)
	}
	if resp.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", resp.Version, "1.0.0")
	}
	if resp.RequestID != "req-456" {
		t.Errorf("RequestID = %q, want %q", resp.RequestID, "req-456")
	}
	if resp.Solution == nil {
		t.Error("Solution = nil, want non-nil")
	} else {
		if resp.Solution.URL != "https://example.com" {
			t.Errorf("Solution.URL = %q, want %q", resp.Solution.URL, "https://example.com")
		}
		if resp.Solution.Status != 200 {
			t.Errorf("Solution.Status = %d, want %d", resp.Solution.Status, 200)
		}
		if len(resp.Solution.Cookies) != 1 {
			t.Errorf("len(Solution.Cookies) = %d, want 1", len(resp.Solution.Cookies))
		}
	}
}

func TestSolveResponse_Fields(t *testing.T) {
	// Test that all fields can be set
	resp := &SolveResponse{
		Status:         "ok",
		Message:        "Success",
		Session:        "session-123",
		Sessions:       []string{"s1", "s2"},
		StartTimestamp: 1000,
		EndTimestamp:   2000,
		Version:        "1.0.0",
		ChallengeType:  "cloudflare",
		SolverUsed:     "2captcha",
		RequestID:      "req-789",
		Usage: &UsageInfo{
			BrowserTimeMS:  5000,
			SolverUsed:     "2captcha",
			ChallengeType:  "turnstile",
			BrowserCostUSD: 0.001,
			SolverCostUSD:  0.002,
		},
	}

	if resp.Session != "session-123" {
		t.Errorf("Session = %q, want %q", resp.Session, "session-123")
	}
	if len(resp.Sessions) != 2 {
		t.Errorf("len(Sessions) = %d, want 2", len(resp.Sessions))
	}
	if resp.ChallengeType != "cloudflare" {
		t.Errorf("ChallengeType = %q, want %q", resp.ChallengeType, "cloudflare")
	}
	if resp.SolverUsed != "2captcha" {
		t.Errorf("SolverUsed = %q, want %q", resp.SolverUsed, "2captcha")
	}
	if resp.Usage == nil {
		t.Error("Usage = nil, want non-nil")
	} else {
		if resp.Usage.BrowserTimeMS != 5000 {
			t.Errorf("Usage.BrowserTimeMS = %d, want 5000", resp.Usage.BrowserTimeMS)
		}
	}
}

func TestSessionInfo_Fields(t *testing.T) {
	info := &SessionInfo{
		ID:           "sess-123",
		CreatedAt:    1000,
		LastUsedAt:   2000,
		RequestCount: 5,
		UserAgent:    "Test Agent",
		Proxy:        "http://***@proxy:8080",
	}

	if info.ID != "sess-123" {
		t.Errorf("ID = %q, want %q", info.ID, "sess-123")
	}
	if info.CreatedAt != 1000 {
		t.Errorf("CreatedAt = %d, want 1000", info.CreatedAt)
	}
	if info.LastUsedAt != 2000 {
		t.Errorf("LastUsedAt = %d, want 2000", info.LastUsedAt)
	}
	if info.RequestCount != 5 {
		t.Errorf("RequestCount = %d, want 5", info.RequestCount)
	}
	if info.UserAgent != "Test Agent" {
		t.Errorf("UserAgent = %q, want %q", info.UserAgent, "Test Agent")
	}
	if info.Proxy != "http://***@proxy:8080" {
		t.Errorf("Proxy = %q, want %q", info.Proxy, "http://***@proxy:8080")
	}
}
