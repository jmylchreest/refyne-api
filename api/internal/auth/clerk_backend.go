// Package auth provides Clerk Backend API client for fetching subscription products.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const clerkAPIBaseURL = "https://api.clerk.com/v1"

// ClerkBackendClient provides access to Clerk's Backend API.
type ClerkBackendClient struct {
	secretKey  string
	httpClient *http.Client
}

// NewClerkBackendClient creates a new Clerk Backend API client.
func NewClerkBackendClient(secretKey string) *ClerkBackendClient {
	return &ClerkBackendClient{
		secretKey: secretKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SubscriptionProduct represents a Clerk billing plan (tier).
type SubscriptionProduct struct {
	ID              string `json:"id"`               // e.g., "cplan_xxx"
	Name            string `json:"name"`             // e.g., "Pro Plan"
	Slug            string `json:"slug"`             // e.g., "pro" - used as tier name
	Description     string `json:"description"`
	IsDefault       bool   `json:"is_default"`       // Default plan for new users
	PubliclyVisible bool   `json:"publicly_visible"` // Whether plan is displayed in pricing components
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

// ListPlansResponse represents the response from Clerk's billing plans API.
type ListPlansResponse struct {
	Data       []SubscriptionProduct `json:"data"`
	TotalCount int                   `json:"total_count"`
}

// ListSubscriptionProducts fetches all billing plans from Clerk.
// These represent the available subscription tiers (e.g., free, pro, enterprise).
func (c *ClerkBackendClient) ListSubscriptionProducts(ctx context.Context) ([]SubscriptionProduct, error) {
	if c.secretKey == "" {
		return nil, fmt.Errorf("clerk secret key not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clerkAPIBaseURL+"/billing/plans", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch plans: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("clerk API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result ListPlansResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Data, nil
}

// GetSubscriptionProduct fetches a single billing plan by ID.
func (c *ClerkBackendClient) GetSubscriptionProduct(ctx context.Context, planID string) (*SubscriptionProduct, error) {
	if c.secretKey == "" {
		return nil, fmt.Errorf("clerk secret key not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clerkAPIBaseURL+"/billing/plans/"+planID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch plan: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Plan not found
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("clerk API returned status %d: %s", resp.StatusCode, string(body))
	}

	var plan SubscriptionProduct
	if err := json.NewDecoder(resp.Body).Decode(&plan); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &plan, nil
}

// ValidateProductExists checks if a product with the given ID exists.
func (c *ClerkBackendClient) ValidateProductExists(ctx context.Context, productID string) (bool, string, error) {
	product, err := c.GetSubscriptionProduct(ctx, productID)
	if err != nil {
		return false, "", err
	}
	if product == nil {
		return false, "", nil
	}
	return true, product.Slug, nil
}

// UserSubscription represents a user's active subscription from Clerk Commerce.
type UserSubscription struct {
	ID                string              `json:"id"`                   // Subscription ID
	PlanID            string              `json:"plan_id"`              // Plan/product ID
	PlanSlug          string              `json:"plan_slug"`            // Plan slug (tier name)
	Status            string              `json:"status"`               // active, canceled, past_due, etc.
	Features          []SubscriptionFeature `json:"features,omitempty"` // Features from the plan
	CurrentPeriodStart int64              `json:"current_period_start"` // Billing period start
	CurrentPeriodEnd   int64              `json:"current_period_end"`   // Billing period end
}

// SubscriptionFeature represents a feature from Clerk Commerce.
type SubscriptionFeature struct {
	ID   string `json:"id"`   // Feature ID
	Slug string `json:"slug"` // Feature slug (e.g., "provider_byok")
}

// clerkSubscriptionResponse is the raw Clerk API response for user subscription.
// The subscription contains subscription_items, each with a plan that has features.
type clerkSubscriptionResponse struct {
	ID                string                       `json:"id"`
	Status            string                       `json:"status"`
	SubscriptionItems []clerkSubscriptionItemResponse `json:"subscription_items"`
}

type clerkSubscriptionItemResponse struct {
	PlanID      string `json:"plan_id"`
	PlanPeriod  string `json:"plan_period"`
	PeriodStart int64  `json:"period_start"`
	PeriodEnd   int64  `json:"period_end"`
	Plan        struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		Features []struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		} `json:"features"`
	} `json:"plan"`
}

// GetUserSubscription fetches a user's current subscription from Clerk Commerce.
// Returns nil if the user has no active subscription.
func (c *ClerkBackendClient) GetUserSubscription(ctx context.Context, userID string) (*UserSubscription, error) {
	if c.secretKey == "" {
		return nil, fmt.Errorf("clerk secret key not configured")
	}

	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clerkAPIBaseURL+"/users/"+userID+"/billing/subscription", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch subscription: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 404 means no subscription
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("clerk API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result clerkSubscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract plan and features from subscription_items (use first item)
	var planID, planSlug string
	var periodStart, periodEnd int64
	var features []SubscriptionFeature

	if len(result.SubscriptionItems) > 0 {
		item := result.SubscriptionItems[0]
		planID = item.Plan.ID
		planSlug = item.Plan.Slug
		periodStart = item.PeriodStart
		periodEnd = item.PeriodEnd

		for _, f := range item.Plan.Features {
			features = append(features, SubscriptionFeature{
				ID:   f.ID,
				Slug: f.Slug,
			})
		}
	}

	// Log parsed result
	slog.Debug("parsed clerk subscription",
		"user_id", userID,
		"subscription_id", result.ID,
		"status", result.Status,
		"plan_id", planID,
		"plan_slug", planSlug,
		"feature_count", len(features),
	)

	// Convert to our type
	sub := &UserSubscription{
		ID:                 result.ID,
		PlanID:             planID,
		PlanSlug:           planSlug,
		Status:             result.Status,
		CurrentPeriodStart: periodStart,
		CurrentPeriodEnd:   periodEnd,
		Features:           features,
	}

	return sub, nil
}
