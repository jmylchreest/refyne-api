package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/webhook"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/service"
)

// StripeWebhookHandler handles Stripe webhook events.
type StripeWebhookHandler struct {
	cfg        *config.Config
	balanceSvc *service.BalanceService
	logger     *slog.Logger
}

// NewStripeWebhookHandler creates a new Stripe webhook handler.
func NewStripeWebhookHandler(cfg *config.Config, balanceSvc *service.BalanceService, logger *slog.Logger) *StripeWebhookHandler {
	// Set Stripe API key
	stripe.Key = cfg.StripeSecretKey

	return &StripeWebhookHandler{
		cfg:        cfg,
		balanceSvc: balanceSvc,
		logger:     logger,
	}
}

// HandleWebhook processes incoming Stripe webhooks.
// This is a raw HTTP handler since huma doesn't handle raw body verification well.
func (h *StripeWebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	const maxBodySize = 65536 // 64KB

	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read webhook body", "error", err)
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Verify webhook signature
	sigHeader := r.Header.Get("Stripe-Signature")
	event, err := webhook.ConstructEvent(payload, sigHeader, h.cfg.StripeWebhookSecret)
	if err != nil {
		h.logger.Error("failed to verify webhook signature", "error", err)
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	// Handle the event
	ctx := r.Context()
	if err := h.handleEvent(ctx, event); err != nil {
		h.logger.Error("failed to handle webhook event", "type", event.Type, "error", err)
		// Return 200 to prevent Stripe from retrying (we'll handle the error internally)
		// For actual failures that should be retried, we could return 500
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleEvent routes events to appropriate handlers.
func (h *StripeWebhookHandler) handleEvent(ctx context.Context, event stripe.Event) error {
	h.logger.Info("received Stripe webhook", "type", event.Type, "id", event.ID)

	switch event.Type {
	case "checkout.session.completed":
		return h.handleCheckoutComplete(ctx, event)

	case "invoice.paid":
		return h.handleInvoicePaid(ctx, event)

	case "customer.subscription.deleted":
		return h.handleSubscriptionCanceled(ctx, event)

	case "charge.refunded":
		return h.handleChargeRefunded(ctx, event)

	default:
		h.logger.Debug("unhandled webhook event type", "type", event.Type)
		return nil
	}
}

// handleCheckoutComplete handles new subscription checkouts.
func (h *StripeWebhookHandler) handleCheckoutComplete(ctx context.Context, event stripe.Event) error {
	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		return fmt.Errorf("failed to unmarshal checkout session: %w", err)
	}

	// Get user ID from metadata
	userID, ok := session.Metadata["clerk_user_id"]
	if !ok || userID == "" {
		h.logger.Warn("checkout session missing clerk_user_id", "session_id", session.ID)
		return nil // Don't error - might be a non-user checkout
	}

	// Get tier from metadata
	tier, ok := session.Metadata["tier"]
	if !ok {
		tier = "starter" // Default tier
	}

	// Add subscription credits
	if err := h.balanceSvc.AddSubscriptionCredits(ctx, userID, session.PaymentIntent.ID, tier); err != nil {
		if errors.Is(err, service.ErrDuplicatePayment) {
			h.logger.Info("duplicate checkout payment ignored", "payment_id", session.PaymentIntent.ID)
			return nil
		}
		return fmt.Errorf("failed to add subscription credits: %w", err)
	}

	h.logger.Info("added subscription credits",
		"user_id", userID,
		"tier", tier,
		"payment_id", session.PaymentIntent.ID,
	)

	return nil
}

// handleInvoicePaid handles recurring subscription payments.
func (h *StripeWebhookHandler) handleInvoicePaid(ctx context.Context, event stripe.Event) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &invoice); err != nil {
		return fmt.Errorf("failed to unmarshal invoice: %w", err)
	}

	// Skip if not a subscription invoice
	if invoice.Subscription == nil {
		return nil
	}

	// Get user ID from subscription metadata or customer metadata
	userID := ""
	tier := "starter"

	if invoice.Subscription != nil && invoice.Subscription.Metadata != nil {
		userID = invoice.Subscription.Metadata["clerk_user_id"]
		if t, ok := invoice.Subscription.Metadata["tier"]; ok {
			tier = t
		}
	}

	if userID == "" && invoice.Customer != nil && invoice.Customer.Metadata != nil {
		userID = invoice.Customer.Metadata["clerk_user_id"]
	}

	if userID == "" {
		h.logger.Warn("invoice missing user ID",
			"invoice_id", invoice.ID,
			"subscription_id", invoice.Subscription.ID,
		)
		return nil
	}

	// Use invoice ID as idempotency key for recurring payments
	paymentID := invoice.ID

	// Add subscription credits
	if err := h.balanceSvc.AddSubscriptionCredits(ctx, userID, paymentID, tier); err != nil {
		if errors.Is(err, service.ErrDuplicatePayment) {
			h.logger.Info("duplicate invoice payment ignored", "invoice_id", invoice.ID)
			return nil
		}
		return fmt.Errorf("failed to add subscription credits: %w", err)
	}

	h.logger.Info("added subscription credits from invoice",
		"user_id", userID,
		"tier", tier,
		"invoice_id", invoice.ID,
	)

	return nil
}

// handleSubscriptionCanceled handles subscription cancellation.
func (h *StripeWebhookHandler) handleSubscriptionCanceled(ctx context.Context, event stripe.Event) error {
	var subscription stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &subscription); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %w", err)
	}

	userID := ""
	if subscription.Metadata != nil {
		userID = subscription.Metadata["clerk_user_id"]
	}

	if userID == "" {
		h.logger.Warn("canceled subscription missing user ID", "subscription_id", subscription.ID)
		return nil
	}

	// Log cancellation (credits are not clawed back per business rules)
	h.logger.Info("subscription canceled",
		"user_id", userID,
		"subscription_id", subscription.ID,
	)

	// Note: We don't remove credits on cancellation
	// User keeps remaining credits until they expire based on rollover policy

	return nil
}

// handleChargeRefunded handles refunds.
func (h *StripeWebhookHandler) handleChargeRefunded(ctx context.Context, event stripe.Event) error {
	var charge stripe.Charge
	if err := json.Unmarshal(event.Data.Raw, &charge); err != nil {
		return fmt.Errorf("failed to unmarshal charge: %w", err)
	}

	userID := ""
	if charge.Metadata != nil {
		userID = charge.Metadata["clerk_user_id"]
	}

	if userID == "" && charge.Customer != nil && charge.Customer.Metadata != nil {
		userID = charge.Customer.Metadata["clerk_user_id"]
	}

	if userID == "" {
		h.logger.Warn("refunded charge missing user ID", "charge_id", charge.ID)
		return nil
	}

	// Calculate refund amount (in dollars)
	refundAmountUSD := float64(charge.AmountRefunded) / 100.0

	// Process refund (doesn't claw back spent credits)
	if err := h.balanceSvc.ProcessRefund(ctx, userID, charge.PaymentIntent.ID, refundAmountUSD); err != nil {
		if errors.Is(err, service.ErrDuplicatePayment) {
			h.logger.Info("duplicate refund ignored", "charge_id", charge.ID)
			return nil
		}
		return fmt.Errorf("failed to process refund: %w", err)
	}

	h.logger.Info("processed refund",
		"user_id", userID,
		"charge_id", charge.ID,
		"refund_amount_usd", refundAmountUSD,
	)

	return nil
}
