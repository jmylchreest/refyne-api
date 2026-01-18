package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	svix "github.com/svix/svix-webhooks/go"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// ClerkWebhookHandler handles Clerk webhook events.
type ClerkWebhookHandler struct {
	cfg        *config.Config
	balanceSvc *service.BalanceService
	cleanupSvc *service.UserCleanupService
	tierSyncSvc *service.TierSyncService
	logger     *slog.Logger
}

// NewClerkWebhookHandler creates a new Clerk webhook handler.
func NewClerkWebhookHandler(cfg *config.Config, balanceSvc *service.BalanceService, cleanupSvc *service.UserCleanupService, tierSyncSvc *service.TierSyncService, logger *slog.Logger) *ClerkWebhookHandler {
	return &ClerkWebhookHandler{
		cfg:         cfg,
		balanceSvc:  balanceSvc,
		cleanupSvc:  cleanupSvc,
		tierSyncSvc: tierSyncSvc,
		logger:      logger,
	}
}

// ClerkWebhookEvent represents a Clerk webhook event.
type ClerkWebhookEvent struct {
	Type   string          `json:"type"`
	Object string          `json:"object"`
	Data   json.RawMessage `json:"data"`
}

// SubscriptionData represents subscription data from Clerk.
type SubscriptionData struct {
	ID             string `json:"id"`
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id,omitempty"`
	Status         string `json:"status"`
	PlanID         string `json:"plan_id"`
	PlanName       string `json:"plan_name,omitempty"`
	PeriodStart    int64  `json:"period_start,omitempty"` // Unix timestamp
	PeriodEnd      int64  `json:"period_end,omitempty"`   // Unix timestamp
}

// SubscriptionItemData represents subscription item data from Clerk.
type SubscriptionItemData struct {
	ID             string `json:"id"`
	SubscriptionID string `json:"subscription_id"`
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id,omitempty"`
	Status         string `json:"status"`
	PlanID         string `json:"plan_id"`
	PlanName       string `json:"plan_name,omitempty"`
	PeriodStart    int64  `json:"period_start,omitempty"` // Unix timestamp
	PeriodEnd      int64  `json:"period_end,omitempty"`   // Unix timestamp
}

// PaymentAttemptData represents payment attempt data from Clerk.
type PaymentAttemptData struct {
	ID                 string  `json:"id"`
	SubscriptionItemID string  `json:"subscription_item_id"`
	UserID             string  `json:"user_id"`
	OrganizationID     string  `json:"organization_id,omitempty"`
	Status             string  `json:"status"` // pending, paid, failed
	AmountDue          int64   `json:"amount_due"`
	AmountPaid         int64   `json:"amount_paid"`
	Currency           string  `json:"currency"`
	PlanID             string  `json:"plan_id"`
	PlanName           string  `json:"plan_name,omitempty"`
}

// HandleWebhook processes incoming Clerk webhooks.
func (h *ClerkWebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	const maxBodySize = 65536 // 64KB

	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read webhook body", "error", err)
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Verify webhook signature using Svix
	headers := http.Header{}
	headers.Set("svix-id", r.Header.Get("svix-id"))
	headers.Set("svix-timestamp", r.Header.Get("svix-timestamp"))
	headers.Set("svix-signature", r.Header.Get("svix-signature"))

	wh, err := svix.NewWebhook(h.cfg.ClerkWebhookSecret)
	if err != nil {
		h.logger.Error("failed to create webhook verifier", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	err = wh.Verify(payload, headers)
	if err != nil {
		h.logger.Error("failed to verify webhook signature", "error", err)
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	// Parse the event
	var event ClerkWebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		h.logger.Error("failed to parse webhook event", "error", err)
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	// Handle the event
	ctx := r.Context()
	if err := h.handleEvent(ctx, event); err != nil {
		h.logger.Error("failed to handle webhook event", "type", event.Type, "error", err)
		// Return 200 to prevent retries for business logic errors
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleEvent routes events to appropriate handlers.
func (h *ClerkWebhookHandler) handleEvent(ctx context.Context, event ClerkWebhookEvent) error {
	h.logger.Info("received Clerk webhook", "type", event.Type)

	switch event.Type {
	case "subscription.active":
		return h.handleSubscriptionActive(ctx, event.Data)

	case "subscriptionItem.active":
		return h.handleSubscriptionItemActive(ctx, event.Data)

	case "subscriptionItem.updated":
		// Handle period updates on renewal (status stays active but period changes)
		return h.handleSubscriptionItemUpdated(ctx, event.Data)

	case "paymentAttempt.updated":
		return h.handlePaymentAttemptUpdated(ctx, event.Data)

	case "subscription.canceled", "subscriptionItem.canceled", "subscriptionItem.ended":
		return h.handleSubscriptionCanceled(ctx, event.Data)

	case "user.deleted":
		return h.handleUserDeleted(ctx, event.Data)

	case "plan.created", "plan.updated":
		// Sync tier visibility/display names when plans change in Clerk
		return h.handlePlanChanged(ctx)

	default:
		h.logger.Debug("unhandled webhook event type", "type", event.Type)
		return nil
	}
}

// handleSubscriptionActive handles new active subscriptions.
func (h *ClerkWebhookHandler) handleSubscriptionActive(ctx context.Context, data json.RawMessage) error {
	var sub SubscriptionData
	if err := json.Unmarshal(data, &sub); err != nil {
		return err
	}

	if sub.UserID == "" {
		h.logger.Warn("subscription missing user_id", "subscription_id", sub.ID)
		return nil
	}

	// Update subscription period if provided (Clerk uses milliseconds)
	if sub.PeriodStart > 0 && sub.PeriodEnd > 0 {
		periodStart := time.UnixMilli(sub.PeriodStart)
		periodEnd := time.UnixMilli(sub.PeriodEnd)
		if err := h.balanceSvc.UpdateSubscriptionPeriod(ctx, sub.UserID, periodStart, periodEnd); err != nil {
			h.logger.Error("failed to update subscription period", "user_id", sub.UserID, "error", err)
			// Continue processing - don't fail the webhook for this
		}
	}

	// Determine tier from plan
	tier := h.planToTier(sub.PlanID, sub.PlanName)

	// Use subscription ID as idempotency key
	if err := h.balanceSvc.AddSubscriptionCredits(ctx, sub.UserID, sub.ID, tier); err != nil {
		if errors.Is(err, service.ErrDuplicatePayment) {
			h.logger.Info("duplicate subscription activation ignored", "subscription_id", sub.ID)
			return nil
		}
		return err
	}

	h.logger.Info("added subscription credits",
		"user_id", sub.UserID,
		"subscription_id", sub.ID,
		"tier", tier,
	)

	return nil
}

// handleSubscriptionItemActive handles subscription item activation (after successful payment).
// This is also called when subscriptions renew, updating period_start and period_end.
func (h *ClerkWebhookHandler) handleSubscriptionItemActive(ctx context.Context, data json.RawMessage) error {
	var item SubscriptionItemData
	if err := json.Unmarshal(data, &item); err != nil {
		return err
	}

	if item.UserID == "" {
		h.logger.Warn("subscription item missing user_id", "item_id", item.ID)
		return nil
	}

	// Update subscription period if provided - this is where period dates get updated on renewal (Clerk uses milliseconds)
	if item.PeriodStart > 0 && item.PeriodEnd > 0 {
		periodStart := time.UnixMilli(item.PeriodStart)
		periodEnd := time.UnixMilli(item.PeriodEnd)
		if err := h.balanceSvc.UpdateSubscriptionPeriod(ctx, item.UserID, periodStart, periodEnd); err != nil {
			h.logger.Error("failed to update subscription period", "user_id", item.UserID, "error", err)
			// Continue processing - don't fail the webhook for this
		}
	}

	tier := h.planToTier(item.PlanID, item.PlanName)

	// Use subscription item ID as idempotency key
	if err := h.balanceSvc.AddSubscriptionCredits(ctx, item.UserID, item.ID, tier); err != nil {
		if errors.Is(err, service.ErrDuplicatePayment) {
			h.logger.Info("duplicate subscription item activation ignored", "item_id", item.ID)
			return nil
		}
		return err
	}

	h.logger.Info("added subscription credits from item activation",
		"user_id", item.UserID,
		"item_id", item.ID,
		"tier", tier,
	)

	return nil
}

// handleSubscriptionItemUpdated handles subscription item updates.
// This is fired when a subscription renews and the period dates change (status stays active).
func (h *ClerkWebhookHandler) handleSubscriptionItemUpdated(ctx context.Context, data json.RawMessage) error {
	var item SubscriptionItemData
	if err := json.Unmarshal(data, &item); err != nil {
		return err
	}

	if item.UserID == "" {
		h.logger.Warn("subscription item update missing user_id", "item_id", item.ID)
		return nil
	}

	// Update subscription period if provided (Clerk uses milliseconds)
	if item.PeriodStart > 0 && item.PeriodEnd > 0 {
		periodStart := time.UnixMilli(item.PeriodStart)
		periodEnd := time.UnixMilli(item.PeriodEnd)
		if err := h.balanceSvc.UpdateSubscriptionPeriod(ctx, item.UserID, periodStart, periodEnd); err != nil {
			h.logger.Error("failed to update subscription period on item update", "user_id", item.UserID, "error", err)
			return err
		}
		h.logger.Info("subscription period updated on renewal",
			"user_id", item.UserID,
			"item_id", item.ID,
			"period_start", periodStart.Format(time.RFC3339),
			"period_end", periodEnd.Format(time.RFC3339),
		)
	}

	return nil
}

// handlePaymentAttemptUpdated handles payment attempt status changes.
func (h *ClerkWebhookHandler) handlePaymentAttemptUpdated(ctx context.Context, data json.RawMessage) error {
	var payment PaymentAttemptData
	if err := json.Unmarshal(data, &payment); err != nil {
		return err
	}

	// Only process successful payments
	if payment.Status != "paid" {
		h.logger.Debug("ignoring non-paid payment attempt", "id", payment.ID, "status", payment.Status)
		return nil
	}

	if payment.UserID == "" {
		h.logger.Warn("payment attempt missing user_id", "payment_id", payment.ID)
		return nil
	}

	tier := h.planToTier(payment.PlanID, payment.PlanName)

	// Use payment attempt ID as idempotency key
	if err := h.balanceSvc.AddSubscriptionCredits(ctx, payment.UserID, payment.ID, tier); err != nil {
		if errors.Is(err, service.ErrDuplicatePayment) {
			h.logger.Info("duplicate payment attempt ignored", "payment_id", payment.ID)
			return nil
		}
		return err
	}

	h.logger.Info("added subscription credits from payment",
		"user_id", payment.UserID,
		"payment_id", payment.ID,
		"tier", tier,
		"amount_paid", payment.AmountPaid,
	)

	return nil
}

// handleSubscriptionCanceled handles subscription cancellation.
func (h *ClerkWebhookHandler) handleSubscriptionCanceled(ctx context.Context, data json.RawMessage) error {
	var sub SubscriptionData
	if err := json.Unmarshal(data, &sub); err != nil {
		// Try as subscription item
		var item SubscriptionItemData
		if err := json.Unmarshal(data, &item); err != nil {
			return err
		}
		h.logger.Info("subscription item canceled",
			"user_id", item.UserID,
			"item_id", item.ID,
		)
		return nil
	}

	h.logger.Info("subscription canceled",
		"user_id", sub.UserID,
		"subscription_id", sub.ID,
	)

	// Note: Credits are not clawed back on cancellation
	// User keeps remaining credits until they expire

	return nil
}

// UserDeletedData represents user deletion data from Clerk.
type UserDeletedData struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// handleUserDeleted handles user account deletion.
// This cleans up all user data when a user deletes their account via Clerk.
func (h *ClerkWebhookHandler) handleUserDeleted(ctx context.Context, data json.RawMessage) error {
	var userData UserDeletedData
	if err := json.Unmarshal(data, &userData); err != nil {
		return err
	}

	if userData.ID == "" {
		h.logger.Warn("user.deleted event missing user id")
		return nil
	}

	h.logger.Info("processing user deletion",
		"user_id", userData.ID,
	)

	// Delete all user data
	if h.cleanupSvc != nil {
		if err := h.cleanupSvc.DeleteAllUserData(ctx, userData.ID); err != nil {
			h.logger.Error("failed to delete user data",
				"user_id", userData.ID,
				"error", err,
			)
			return err
		}
	} else {
		h.logger.Warn("user cleanup service not configured, skipping data deletion",
			"user_id", userData.ID,
		)
	}

	h.logger.Info("user data deleted successfully",
		"user_id", userData.ID,
	)

	return nil
}

// planToTier maps Clerk plan IDs/names to our internal tier names.
func (h *ClerkWebhookHandler) planToTier(planID, planName string) string {
	// Map based on plan name or ID patterns
	// Adjust these based on your actual Clerk plan configuration
	switch {
	case contains(planName, "pro") || contains(planID, "pro"):
		return "pro"
	case contains(planName, "starter") || contains(planID, "starter"):
		return "starter"
	case contains(planName, "free") || contains(planID, "free"):
		return "free"
	default:
		// Default to starter if unrecognized
		return "starter"
	}
}

// contains checks if s contains substr (case-insensitive would be better but keeping simple).
func contains(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// handlePlanChanged triggers a tier sync when plans are created or updated in Clerk.
func (h *ClerkWebhookHandler) handlePlanChanged(ctx context.Context) error {
	if h.tierSyncSvc == nil {
		h.logger.Debug("tier sync service not configured, skipping plan sync")
		return nil
	}

	if err := h.tierSyncSvc.SyncFromClerk(ctx); err != nil {
		h.logger.Error("failed to sync tiers from Clerk", "error", err)
		return err
	}

	h.logger.Info("tier metadata synced from Clerk plan webhook")
	return nil
}
