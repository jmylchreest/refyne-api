package service

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// WebhookService handles webhook delivery.
type WebhookService struct {
	logger *slog.Logger
	client *http.Client
}

// NewWebhookService creates a new webhook service.
func NewWebhookService(logger *slog.Logger) *WebhookService {
	return &WebhookService{
		logger: logger,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Send delivers a webhook payload to the specified URL.
// This is a fire-and-forget operation that doesn't block on delivery.
func (s *WebhookService) Send(ctx context.Context, url string, payload any) {
	go s.deliver(url, payload)
}

// SendSync delivers a webhook and waits for the response.
func (s *WebhookService) SendSync(ctx context.Context, url string, payload any) error {
	return s.deliver(url, payload)
}

func (s *WebhookService) deliver(url string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		s.logger.Error("webhook: failed to marshal payload", "error", err)
		return err
	}

	// Retry up to 3 times with exponential backoff
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*attempt) * time.Second)
		}

		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			s.logger.Error("webhook: failed to create request", "error", err)
			return err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Refyne-Webhook/1.0")

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			s.logger.Warn("webhook: delivery failed", "url", url, "attempt", attempt+1, "error", err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			s.logger.Info("webhook: delivered successfully", "url", url, "status", resp.StatusCode)
			return nil
		}

		lastErr = &WebhookError{StatusCode: resp.StatusCode}
		s.logger.Warn("webhook: non-success status", "url", url, "status", resp.StatusCode, "attempt", attempt+1)
	}

	s.logger.Error("webhook: delivery failed after retries", "url", url, "error", lastErr)
	return lastErr
}

// WebhookError represents a webhook delivery error.
type WebhookError struct {
	StatusCode int
}

func (e *WebhookError) Error() string {
	return "webhook delivery failed with status: " + http.StatusText(e.StatusCode)
}
