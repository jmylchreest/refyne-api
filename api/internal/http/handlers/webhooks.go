package handlers

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/refyne-api/internal/crypto"
	"github.com/jmylchreest/refyne-api/internal/http/mw"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// WebhookHandler handles webhook CRUD endpoints.
type WebhookHandler struct {
	webhookRepo  repository.WebhookRepository
	deliveryRepo repository.WebhookDeliveryRepository
	encryptor    *crypto.Encryptor
}

// NewWebhookHandler creates a new webhook handler.
func NewWebhookHandler(
	webhookRepo repository.WebhookRepository,
	deliveryRepo repository.WebhookDeliveryRepository,
	encryptor *crypto.Encryptor,
) *WebhookHandler {
	return &WebhookHandler{
		webhookRepo:  webhookRepo,
		deliveryRepo: deliveryRepo,
		encryptor:    encryptor,
	}
}

// WebhookInput represents webhook data in API requests.
type WebhookInput struct {
	Name     string               `json:"name" minLength:"1" maxLength:"64" doc:"Unique name for this webhook"`
	URL      string               `json:"url" format:"uri" minLength:"1" doc:"Webhook URL to send events to"`
	Secret   string               `json:"secret,omitempty" maxLength:"256" doc:"Secret for HMAC-SHA256 signature (leave empty to disable signing)"`
	Events   []string             `json:"events,omitempty" doc:"Event types to subscribe to (empty or [\"*\"] for all events)"`
	Headers  []WebhookHeaderInput `json:"headers,omitempty" maxItems:"10" doc:"Custom headers to include in webhook requests"`
	IsActive bool                 `json:"is_active" doc:"Whether this webhook is active"`
}

// WebhookResponse represents a webhook in API responses.
type WebhookResponse struct {
	ID        string                 `json:"id" doc:"Unique webhook ID"`
	Name      string                 `json:"name" doc:"Webhook name"`
	URL       string                 `json:"url" doc:"Webhook URL"`
	HasSecret bool                   `json:"has_secret" doc:"Whether this webhook has a secret configured"`
	Events    []string               `json:"events" doc:"Subscribed event types"`
	Headers   []WebhookHeaderInput   `json:"headers,omitempty" doc:"Custom headers"`
	IsActive  bool                   `json:"is_active" doc:"Whether this webhook is active"`
	CreatedAt string                 `json:"created_at" doc:"Creation timestamp"`
	UpdatedAt string                 `json:"updated_at" doc:"Last update timestamp"`
}

// WebhookDeliveryResponse represents a webhook delivery in API responses.
type WebhookDeliveryResponse struct {
	ID             string   `json:"id" doc:"Delivery ID"`
	WebhookID      *string  `json:"webhook_id,omitempty" doc:"Webhook ID (null for ephemeral webhooks)"`
	JobID          string   `json:"job_id" doc:"Associated job ID"`
	EventType      string   `json:"event_type" doc:"Event type that triggered this delivery"`
	URL            string   `json:"url" doc:"Destination URL"`
	StatusCode     *int     `json:"status_code,omitempty" doc:"HTTP status code received"`
	ResponseTimeMs *int     `json:"response_time_ms,omitempty" doc:"Response time in milliseconds"`
	Status         string   `json:"status" doc:"Delivery status (pending, success, failed, retrying)"`
	ErrorMessage   string   `json:"error_message,omitempty" doc:"Error message if failed"`
	AttemptNumber  int      `json:"attempt_number" doc:"Current attempt number"`
	MaxAttempts    int      `json:"max_attempts" doc:"Maximum retry attempts"`
	NextRetryAt    *string  `json:"next_retry_at,omitempty" doc:"Next retry time if retrying"`
	CreatedAt      string   `json:"created_at" doc:"Creation timestamp"`
	DeliveredAt    *string  `json:"delivered_at,omitempty" doc:"Successful delivery timestamp"`
}

// ListWebhooksOutput represents the list webhooks response.
type ListWebhooksOutput struct {
	Body struct {
		Webhooks []WebhookResponse `json:"webhooks" doc:"List of user's webhooks"`
	}
}

// ListWebhooks returns all webhooks for the authenticated user.
func (h *WebhookHandler) ListWebhooks(ctx context.Context, input *struct{}) (*ListWebhooksOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("authentication required")
	}

	webhooks, err := h.webhookRepo.GetByUserID(ctx, claims.UserID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list webhooks: " + err.Error())
	}

	responses := make([]WebhookResponse, 0, len(webhooks))
	for _, w := range webhooks {
		responses = append(responses, webhookToResponse(w))
	}

	return &ListWebhooksOutput{
		Body: struct {
			Webhooks []WebhookResponse `json:"webhooks" doc:"List of user's webhooks"`
		}{Webhooks: responses},
	}, nil
}

// GetWebhookInput represents the get webhook request.
type GetWebhookInput struct {
	ID string `path:"id" doc:"Webhook ID"`
}

// GetWebhookOutput represents the get webhook response.
type GetWebhookOutput struct {
	Body WebhookResponse
}

// GetWebhook returns a specific webhook.
func (h *WebhookHandler) GetWebhook(ctx context.Context, input *GetWebhookInput) (*GetWebhookOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("authentication required")
	}

	webhook, err := h.webhookRepo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get webhook: " + err.Error())
	}
	if webhook == nil {
		return nil, huma.Error404NotFound("webhook not found")
	}
	if webhook.UserID != claims.UserID {
		return nil, huma.Error403Forbidden("access denied")
	}

	return &GetWebhookOutput{
		Body: webhookToResponse(webhook),
	}, nil
}

// CreateWebhookInput represents the create webhook request.
type CreateWebhookInput struct {
	Body WebhookInput
}

// CreateWebhookOutput represents the create webhook response.
type CreateWebhookOutput struct {
	Body WebhookResponse
}

// CreateWebhook creates a new webhook.
func (h *WebhookHandler) CreateWebhook(ctx context.Context, input *CreateWebhookInput) (*CreateWebhookOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("authentication required")
	}

	// Check for duplicate name
	existing, err := h.webhookRepo.GetByUserAndName(ctx, claims.UserID, input.Body.Name)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to check webhook: " + err.Error())
	}
	if existing != nil {
		return nil, huma.Error409Conflict("a webhook with this name already exists")
	}

	// Encrypt secret if provided
	var secretEncrypted string
	if input.Body.Secret != "" && h.encryptor != nil {
		encrypted, err := h.encryptor.Encrypt(input.Body.Secret)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to encrypt secret")
		}
		secretEncrypted = encrypted
	}

	// Default events to all if not specified
	events := input.Body.Events
	if len(events) == 0 {
		events = []string{"*"}
	}

	// Convert headers
	headers := make([]models.Header, 0, len(input.Body.Headers))
	for _, h := range input.Body.Headers {
		headers = append(headers, models.Header{Name: h.Name, Value: h.Value})
	}

	webhook := &models.Webhook{
		UserID:          claims.UserID,
		Name:            input.Body.Name,
		URL:             input.Body.URL,
		SecretEncrypted: secretEncrypted,
		Events:          events,
		Headers:         headers,
		IsActive:        input.Body.IsActive,
	}

	if err := h.webhookRepo.Create(ctx, webhook); err != nil {
		return nil, huma.Error500InternalServerError("failed to create webhook: " + err.Error())
	}

	return &CreateWebhookOutput{
		Body: webhookToResponse(webhook),
	}, nil
}

// UpdateWebhookInput represents the update webhook request.
type UpdateWebhookInput struct {
	ID   string `path:"id" doc:"Webhook ID"`
	Body WebhookInput
}

// UpdateWebhookOutput represents the update webhook response.
type UpdateWebhookOutput struct {
	Body WebhookResponse
}

// UpdateWebhook updates an existing webhook.
func (h *WebhookHandler) UpdateWebhook(ctx context.Context, input *UpdateWebhookInput) (*UpdateWebhookOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("authentication required")
	}

	webhook, err := h.webhookRepo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get webhook: " + err.Error())
	}
	if webhook == nil {
		return nil, huma.Error404NotFound("webhook not found")
	}
	if webhook.UserID != claims.UserID {
		return nil, huma.Error403Forbidden("access denied")
	}

	// Check for duplicate name (if changed)
	if input.Body.Name != webhook.Name {
		existing, err := h.webhookRepo.GetByUserAndName(ctx, claims.UserID, input.Body.Name)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to check webhook: " + err.Error())
		}
		if existing != nil {
			return nil, huma.Error409Conflict("a webhook with this name already exists")
		}
	}

	// Update fields
	webhook.Name = input.Body.Name
	webhook.URL = input.Body.URL
	webhook.IsActive = input.Body.IsActive

	// Update secret if provided
	if input.Body.Secret != "" && h.encryptor != nil {
		encrypted, err := h.encryptor.Encrypt(input.Body.Secret)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to encrypt secret")
		}
		webhook.SecretEncrypted = encrypted
	}

	// Update events
	events := input.Body.Events
	if len(events) == 0 {
		events = []string{"*"}
	}
	webhook.Events = events

	// Update headers
	headers := make([]models.Header, 0, len(input.Body.Headers))
	for _, h := range input.Body.Headers {
		headers = append(headers, models.Header{Name: h.Name, Value: h.Value})
	}
	webhook.Headers = headers

	if err := h.webhookRepo.Update(ctx, webhook); err != nil {
		return nil, huma.Error500InternalServerError("failed to update webhook: " + err.Error())
	}

	return &UpdateWebhookOutput{
		Body: webhookToResponse(webhook),
	}, nil
}

// DeleteWebhookInput represents the delete webhook request.
type DeleteWebhookInput struct {
	ID string `path:"id" doc:"Webhook ID"`
}

// DeleteWebhookOutput represents the delete webhook response.
type DeleteWebhookOutput struct {
	Body struct {
		Success bool `json:"success" doc:"Whether deletion was successful"`
	}
}

// DeleteWebhook deletes a webhook.
func (h *WebhookHandler) DeleteWebhook(ctx context.Context, input *DeleteWebhookInput) (*DeleteWebhookOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("authentication required")
	}

	webhook, err := h.webhookRepo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get webhook: " + err.Error())
	}
	if webhook == nil {
		return nil, huma.Error404NotFound("webhook not found")
	}
	if webhook.UserID != claims.UserID {
		return nil, huma.Error403Forbidden("access denied")
	}

	if err := h.webhookRepo.Delete(ctx, input.ID); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete webhook: " + err.Error())
	}

	return &DeleteWebhookOutput{
		Body: struct {
			Success bool `json:"success" doc:"Whether deletion was successful"`
		}{Success: true},
	}, nil
}

// ListWebhookDeliveriesInput represents the list deliveries request.
type ListWebhookDeliveriesInput struct {
	ID     string `path:"id" doc:"Webhook ID"`
	Limit  int    `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Maximum number of deliveries to return"`
	Offset int    `query:"offset" default:"0" minimum:"0" doc:"Number of deliveries to skip"`
}

// ListWebhookDeliveriesOutput represents the list deliveries response.
type ListWebhookDeliveriesOutput struct {
	Body struct {
		Deliveries []WebhookDeliveryResponse `json:"deliveries" doc:"List of webhook deliveries"`
	}
}

// ListWebhookDeliveries returns deliveries for a specific webhook.
func (h *WebhookHandler) ListWebhookDeliveries(ctx context.Context, input *ListWebhookDeliveriesInput) (*ListWebhookDeliveriesOutput, error) {
	claims := mw.GetUserClaims(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("authentication required")
	}

	// Verify webhook ownership
	webhook, err := h.webhookRepo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get webhook: " + err.Error())
	}
	if webhook == nil {
		return nil, huma.Error404NotFound("webhook not found")
	}
	if webhook.UserID != claims.UserID {
		return nil, huma.Error403Forbidden("access denied")
	}

	deliveries, err := h.deliveryRepo.GetByWebhookID(ctx, input.ID, input.Limit, input.Offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list deliveries: " + err.Error())
	}

	responses := make([]WebhookDeliveryResponse, 0, len(deliveries))
	for _, d := range deliveries {
		responses = append(responses, deliveryToResponse(d))
	}

	return &ListWebhookDeliveriesOutput{
		Body: struct {
			Deliveries []WebhookDeliveryResponse `json:"deliveries" doc:"List of webhook deliveries"`
		}{Deliveries: responses},
	}, nil
}

// webhookToResponse converts a Webhook model to a response.
func webhookToResponse(w *models.Webhook) WebhookResponse {
	headers := make([]WebhookHeaderInput, 0, len(w.Headers))
	for _, h := range w.Headers {
		headers = append(headers, WebhookHeaderInput{Name: h.Name, Value: h.Value})
	}

	return WebhookResponse{
		ID:        w.ID,
		Name:      w.Name,
		URL:       w.URL,
		HasSecret: w.SecretEncrypted != "",
		Events:    w.Events,
		Headers:   headers,
		IsActive:  w.IsActive,
		CreatedAt: w.CreatedAt.Format(time.RFC3339),
		UpdatedAt: w.UpdatedAt.Format(time.RFC3339),
	}
}

// deliveryToResponse converts a WebhookDelivery model to a response.
func deliveryToResponse(d *models.WebhookDelivery) WebhookDeliveryResponse {
	var nextRetryAt *string
	if d.NextRetryAt != nil {
		s := d.NextRetryAt.Format(time.RFC3339)
		nextRetryAt = &s
	}

	var deliveredAt *string
	if d.DeliveredAt != nil {
		s := d.DeliveredAt.Format(time.RFC3339)
		deliveredAt = &s
	}

	return WebhookDeliveryResponse{
		ID:             d.ID,
		WebhookID:      d.WebhookID,
		JobID:          d.JobID,
		EventType:      d.EventType,
		URL:            d.URL,
		StatusCode:     d.StatusCode,
		ResponseTimeMs: d.ResponseTimeMs,
		Status:         string(d.Status),
		ErrorMessage:   d.ErrorMessage,
		AttemptNumber:  d.AttemptNumber,
		MaxAttempts:    d.MaxAttempts,
		NextRetryAt:    nextRetryAt,
		CreatedAt:      d.CreatedAt.Format(time.RFC3339),
		DeliveredAt:    deliveredAt,
	}
}
