package models

import (
	"time"

	"github.com/go-rod/rod"
)

// BrowserSession represents a persistent browser session.
type BrowserSession struct {
	ID           string
	Browser      *rod.Browser
	Page         *rod.Page
	UserAgent    string
	Cookies      []Cookie
	CreatedAt    time.Time
	LastUsedAt   time.Time
	TTL          time.Duration
	RequestCount int
	ProxyURL     string
}

// IsExpired returns true if the session has exceeded its TTL.
func (s *BrowserSession) IsExpired() bool {
	return time.Since(s.CreatedAt) > s.TTL
}

// IsStale returns true if the session hasn't been used recently.
func (s *BrowserSession) IsStale(idleTimeout time.Duration) bool {
	return time.Since(s.LastUsedAt) > idleTimeout
}

// Touch updates the last used timestamp.
func (s *BrowserSession) Touch() {
	s.LastUsedAt = time.Now()
	s.RequestCount++
}
