// Package auth provides Clerk Backend API client and subscription caching.
package auth

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	// DefaultSubscriptionCacheTTL is the default TTL for cached subscriptions.
	DefaultSubscriptionCacheTTL = 5 * time.Minute
)

// CachedSubscription wraps a UserSubscription with expiration time.
type CachedSubscription struct {
	Subscription *UserSubscription
	ExpiresAt    time.Time
}

// IsExpired returns true if the cached entry has expired.
func (c *CachedSubscription) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// SubscriptionCache provides an in-memory cache for user subscriptions.
// It's safe for concurrent access.
type SubscriptionCache struct {
	mu       sync.RWMutex
	cache    map[string]*CachedSubscription
	ttl      time.Duration
	client   *ClerkBackendClient
	logger   *slog.Logger
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewSubscriptionCache creates a new subscription cache.
func NewSubscriptionCache(client *ClerkBackendClient, ttl time.Duration, logger *slog.Logger) *SubscriptionCache {
	if ttl <= 0 {
		ttl = DefaultSubscriptionCacheTTL
	}
	if logger == nil {
		logger = slog.Default()
	}

	c := &SubscriptionCache{
		cache:  make(map[string]*CachedSubscription),
		ttl:    ttl,
		client: client,
		logger: logger,
		stopCh: make(chan struct{}),
	}

	// Start background cleanup goroutine
	go c.cleanupLoop()

	return c
}

// GetSubscription retrieves a user's subscription, using cache if available.
// Returns nil if the user has no subscription (also cached as a "no subscription" entry).
func (c *SubscriptionCache) GetSubscription(ctx context.Context, userID string) (*UserSubscription, error) {
	// Check cache first
	c.mu.RLock()
	cached, ok := c.cache[userID]
	c.mu.RUnlock()

	if ok && !cached.IsExpired() {
		return cached.Subscription, nil
	}

	// Cache miss or expired - fetch from Clerk
	sub, err := c.client.GetUserSubscription(ctx, userID)
	if err != nil {
		c.logger.Warn("failed to fetch subscription from Clerk",
			"user_id", userID,
			"error", err,
		)
		// On error, return cached value if we have one (stale is better than error)
		if ok {
			return cached.Subscription, nil
		}
		return nil, err
	}

	// Cache the result (including nil for "no subscription")
	c.mu.Lock()
	c.cache[userID] = &CachedSubscription{
		Subscription: sub,
		ExpiresAt:    time.Now().Add(c.ttl),
	}
	c.mu.Unlock()

	c.logger.Debug("cached subscription",
		"user_id", userID,
		"has_subscription", sub != nil,
		"ttl", c.ttl,
	)

	return sub, nil
}

// Invalidate removes a user's subscription from the cache.
// Call this when receiving a webhook about subscription changes.
func (c *SubscriptionCache) Invalidate(userID string) {
	c.mu.Lock()
	delete(c.cache, userID)
	c.mu.Unlock()

	c.logger.Debug("invalidated subscription cache",
		"user_id", userID,
	)
}

// InvalidateAll clears the entire cache.
func (c *SubscriptionCache) InvalidateAll() {
	c.mu.Lock()
	c.cache = make(map[string]*CachedSubscription)
	c.mu.Unlock()

	c.logger.Debug("invalidated all subscription caches")
}

// Stop gracefully shuts down the cache cleanup goroutine.
func (c *SubscriptionCache) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
}

// cleanupLoop periodically removes expired entries from the cache.
func (c *SubscriptionCache) cleanupLoop() {
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

// cleanup removes all expired entries from the cache.
func (c *SubscriptionCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	expired := 0
	for userID, cached := range c.cache {
		if now.After(cached.ExpiresAt) {
			delete(c.cache, userID)
			expired++
		}
	}

	if expired > 0 {
		c.logger.Debug("cleaned up expired subscription cache entries",
			"expired_count", expired,
			"remaining_count", len(c.cache),
		)
	}
}

// Size returns the current number of cached entries.
func (c *SubscriptionCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}
