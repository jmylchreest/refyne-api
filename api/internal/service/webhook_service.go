package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// WebhookService handles webhook delivery with tracking and signatures.
type WebhookService struct {
	logger       *slog.Logger
	client       *http.Client
	webhookRepo  repository.WebhookRepository
	deliveryRepo repository.WebhookDeliveryRepository
	encryptor    *crypto.Encryptor
}

// NewWebhookService creates a new webhook service.
func NewWebhookService(
	logger *slog.Logger,
	webhookRepo repository.WebhookRepository,
	deliveryRepo repository.WebhookDeliveryRepository,
	encryptor *crypto.Encryptor,
) *WebhookService {
	return &WebhookService{
		logger: logger,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		webhookRepo:  webhookRepo,
		deliveryRepo: deliveryRepo,
		encryptor:    encryptor,
	}
}

// WebhookPayload represents the payload sent to webhooks.
type WebhookPayload struct {
	Event     string    `json:"event"`
	Timestamp time.Time `json:"timestamp"`
	JobID     string    `json:"job_id"`
	Data      any       `json:"data"`
}

// WebhookConfig represents configuration for a single webhook delivery.
// Used for both persistent webhooks and ephemeral webhooks.
type WebhookConfig struct {
	WebhookID *string          // Reference to persistent webhook (nil for ephemeral)
	URL       string           // Webhook URL
	Secret    string           // Plaintext secret for HMAC signing
	Headers   []models.Header  // Custom headers
	Events    []string         // Event types to subscribe to (["*"] for all)
}

// DeliveryResult contains the result of a webhook delivery attempt.
type DeliveryResult struct {
	DeliveryID     string
	StatusCode     int
	ResponseBody   string
	ResponseTimeMs int
	Error          error
}

// Send delivers a webhook payload asynchronously.
// This is a fire-and-forget operation that logs delivery status.
func (s *WebhookService) Send(ctx context.Context, config *WebhookConfig, eventType string, jobID string, data any) {
	go func() {
		_, _ = s.DeliverWithTracking(context.Background(), config, eventType, jobID, data)
	}()
}

// DeliverWithTracking delivers a webhook and tracks the delivery in the database.
func (s *WebhookService) DeliverWithTracking(ctx context.Context, config *WebhookConfig, eventType string, jobID string, data any) (*DeliveryResult, error) {
	// Check if this event type is subscribed
	if !s.isEventSubscribed(config.Events, eventType) {
		return nil, nil // Not subscribed to this event
	}

	payload := WebhookPayload{
		Event:     eventType,
		Timestamp: time.Now().UTC(),
		JobID:     jobID,
		Data:      data,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		s.logger.Error("webhook: failed to marshal payload", "error", err)
		return nil, err
	}

	// Create delivery record
	delivery := &models.WebhookDelivery{
		WebhookID:      config.WebhookID,
		JobID:          jobID,
		EventType:      eventType,
		URL:            config.URL,
		PayloadJSON:    string(payloadBytes),
		RequestHeaders: config.Headers,
		Status:         models.WebhookDeliveryStatusPending,
		AttemptNumber:  1,
		MaxAttempts:    3,
	}

	if s.deliveryRepo != nil {
		if err := s.deliveryRepo.Create(ctx, delivery); err != nil {
			s.logger.Error("webhook: failed to create delivery record", "error", err)
			// Continue with delivery even if tracking fails
		}
	}

	// Attempt delivery
	result := s.deliverWithRetries(ctx, config, payloadBytes, delivery)

	// Update delivery record with result
	if s.deliveryRepo != nil && delivery.ID != "" {
		if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
			s.logger.Error("webhook: failed to update delivery record", "error", err)
		}
	}

	return result, result.Error
}

// deliverWithRetries attempts to deliver a webhook with retries.
func (s *WebhookService) deliverWithRetries(ctx context.Context, config *WebhookConfig, payloadBytes []byte, delivery *models.WebhookDelivery) *DeliveryResult {
	result := &DeliveryResult{
		DeliveryID: delivery.ID,
	}

	for attempt := 1; attempt <= delivery.MaxAttempts; attempt++ {
		delivery.AttemptNumber = attempt

		if attempt > 1 {
			// Exponential backoff: 1s, 4s, 9s
			backoff := time.Duration(attempt*attempt) * time.Second
			time.Sleep(backoff)
		}

		statusCode, responseBody, responseTime, err := s.deliver(ctx, config, payloadBytes)
		result.StatusCode = statusCode
		result.ResponseBody = responseBody
		result.ResponseTimeMs = responseTime

		// Update delivery record
		delivery.StatusCode = &statusCode
		delivery.ResponseBody = responseBody
		delivery.ResponseTimeMs = &responseTime

		if err == nil && statusCode >= 200 && statusCode < 300 {
			// Success
			now := time.Now()
			delivery.Status = models.WebhookDeliveryStatusSuccess
			delivery.DeliveredAt = &now
			s.logger.Info("webhook: delivered successfully",
				"url", config.URL,
				"status", statusCode,
				"attempt", attempt,
				"response_time_ms", responseTime,
			)
			return result
		}

		// Record error
		if err != nil {
			delivery.ErrorMessage = err.Error()
			result.Error = err
		} else {
			errMsg := fmt.Sprintf("HTTP %d: %s", statusCode, http.StatusText(statusCode))
			delivery.ErrorMessage = errMsg
			result.Error = &WebhookError{StatusCode: statusCode}
		}

		if attempt < delivery.MaxAttempts {
			delivery.Status = models.WebhookDeliveryStatusRetrying
			nextRetry := time.Now().Add(time.Duration((attempt+1)*(attempt+1)) * time.Second)
			delivery.NextRetryAt = &nextRetry
			s.logger.Warn("webhook: delivery failed, will retry",
				"url", config.URL,
				"attempt", attempt,
				"next_retry", nextRetry,
				"error", delivery.ErrorMessage,
			)
		} else {
			delivery.Status = models.WebhookDeliveryStatusFailed
			s.logger.Error("webhook: delivery failed after all retries",
				"url", config.URL,
				"attempts", attempt,
				"error", delivery.ErrorMessage,
			)
		}
	}

	return result
}

// deliver performs a single delivery attempt.
func (s *WebhookService) deliver(ctx context.Context, config *WebhookConfig, payloadBytes []byte) (int, string, int, error) {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		return 0, "", 0, err
	}

	// Set standard headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Refyne-Webhook/1.0")

	// Set HMAC signature if secret is provided
	if config.Secret != "" {
		signature := s.computeSignature(payloadBytes, config.Secret)
		req.Header.Set("X-Refyne-Signature", signature)
		req.Header.Set("X-Refyne-Signature-256", "sha256="+signature)
	}

	// Set custom headers
	for _, header := range config.Headers {
		req.Header.Set(header.Name, header.Value)
	}

	resp, err := s.client.Do(req)
	responseTime := int(time.Since(start).Milliseconds())

	if err != nil {
		return 0, "", responseTime, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body (limit to 64KB)
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	responseBody := string(bodyBytes)

	return resp.StatusCode, responseBody, responseTime, nil
}

// computeSignature computes HMAC-SHA256 signature for the payload.
func (s *WebhookService) computeSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// isEventSubscribed checks if an event type matches the subscription filter.
func (s *WebhookService) isEventSubscribed(events []string, eventType string) bool {
	if len(events) == 0 {
		return true // Default to all events
	}

	for _, event := range events {
		if event == "*" || event == eventType {
			return true
		}
	}

	return false
}

// SendForJob delivers webhooks for a job event.
// It handles both persistent webhooks (by user ID) and ephemeral webhooks (via config).
func (s *WebhookService) SendForJob(ctx context.Context, userID string, eventType string, jobID string, data any, ephemeralConfig *WebhookConfig) {
	// Send to ephemeral webhook if provided
	if ephemeralConfig != nil && ephemeralConfig.URL != "" {
		s.Send(ctx, ephemeralConfig, eventType, jobID, data)
	}

	// Send to persistent webhooks
	if s.webhookRepo != nil {
		webhooks, err := s.webhookRepo.GetActiveByUserID(ctx, userID)
		if err != nil {
			s.logger.Error("webhook: failed to get active webhooks", "user_id", userID, "error", err)
			return
		}

		for _, webhook := range webhooks {
			// Decrypt secret if present
			var secret string
			if webhook.SecretEncrypted != "" && s.encryptor != nil {
				decrypted, err := s.encryptor.Decrypt(webhook.SecretEncrypted)
				if err != nil {
					s.logger.Warn("webhook: failed to decrypt secret", "webhook_id", webhook.ID, "error", err)
				} else {
					secret = decrypted
				}
			}

			config := &WebhookConfig{
				WebhookID: &webhook.ID,
				URL:       webhook.URL,
				Secret:    secret,
				Headers:   webhook.Headers,
				Events:    webhook.Events,
			}

			s.Send(ctx, config, eventType, jobID, data)
		}
	}
}

// GetDeliveriesForJob returns all webhook deliveries for a job.
func (s *WebhookService) GetDeliveriesForJob(ctx context.Context, jobID string) ([]*models.WebhookDelivery, error) {
	if s.deliveryRepo == nil {
		return nil, nil
	}
	return s.deliveryRepo.GetByJobID(ctx, jobID)
}

// ProcessPendingRetries processes webhook deliveries that are due for retry.
func (s *WebhookService) ProcessPendingRetries(ctx context.Context, limit int) (int, error) {
	if s.deliveryRepo == nil {
		return 0, nil
	}

	deliveries, err := s.deliveryRepo.GetPendingRetries(ctx, limit)
	if err != nil {
		return 0, err
	}

	processed := 0
	for _, delivery := range deliveries {
		// Get webhook config
		var config *WebhookConfig

		if delivery.WebhookID != nil && s.webhookRepo != nil {
			webhook, err := s.webhookRepo.GetByID(ctx, *delivery.WebhookID)
			if err != nil {
				s.logger.Error("webhook: failed to get webhook for retry", "webhook_id", *delivery.WebhookID, "error", err)
				continue
			}
			if webhook == nil || !webhook.IsActive {
				// Webhook was deleted or deactivated, mark delivery as failed
				delivery.Status = models.WebhookDeliveryStatusFailed
				delivery.ErrorMessage = "webhook deleted or deactivated"
				_ = s.deliveryRepo.Update(ctx, delivery)
				continue
			}

			var secret string
			if webhook.SecretEncrypted != "" && s.encryptor != nil {
				decrypted, err := s.encryptor.Decrypt(webhook.SecretEncrypted)
				if err == nil {
					secret = decrypted
				}
			}

			config = &WebhookConfig{
				WebhookID: &webhook.ID,
				URL:       webhook.URL,
				Secret:    secret,
				Headers:   webhook.Headers,
				Events:    webhook.Events,
			}
		} else {
			// Ephemeral webhook - use stored URL and headers
			config = &WebhookConfig{
				URL:     delivery.URL,
				Headers: delivery.RequestHeaders,
				Events:  []string{"*"},
			}
		}

		// Attempt delivery
		statusCode, responseBody, responseTime, err := s.deliver(ctx, config, []byte(delivery.PayloadJSON))

		delivery.StatusCode = &statusCode
		delivery.ResponseBody = responseBody
		delivery.ResponseTimeMs = &responseTime
		delivery.AttemptNumber++

		if err == nil && statusCode >= 200 && statusCode < 300 {
			now := time.Now()
			delivery.Status = models.WebhookDeliveryStatusSuccess
			delivery.DeliveredAt = &now
		} else {
			if err != nil {
				delivery.ErrorMessage = err.Error()
			} else {
				delivery.ErrorMessage = fmt.Sprintf("HTTP %d: %s", statusCode, http.StatusText(statusCode))
			}

			if delivery.AttemptNumber >= delivery.MaxAttempts {
				delivery.Status = models.WebhookDeliveryStatusFailed
			} else {
				delivery.Status = models.WebhookDeliveryStatusRetrying
				nextRetry := time.Now().Add(time.Duration(delivery.AttemptNumber*delivery.AttemptNumber) * time.Second)
				delivery.NextRetryAt = &nextRetry
			}
		}

		if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
			s.logger.Error("webhook: failed to update retry delivery", "delivery_id", delivery.ID, "error", err)
		}

		processed++
	}

	return processed, nil
}

// WebhookError represents a webhook delivery error.
type WebhookError struct {
	StatusCode int
}

func (e *WebhookError) Error() string {
	return "webhook delivery failed with status: " + http.StatusText(e.StatusCode)
}

// Legacy methods for backward compatibility

// Send delivers a webhook payload to the specified URL (legacy).
// Deprecated: Use SendForJob or DeliverWithTracking instead.
func (s *WebhookService) SendLegacy(ctx context.Context, url string, payload any) {
	config := &WebhookConfig{
		URL:    url,
		Events: []string{"*"},
	}
	go func() {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			s.logger.Error("webhook: failed to marshal payload", "error", err)
			return
		}
		_, _, _, _ = s.deliver(context.Background(), config, payloadBytes)
	}()
}
