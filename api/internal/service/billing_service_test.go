package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jmylchreest/refyne-api/internal/config"
	"github.com/jmylchreest/refyne-api/internal/llm"
	"github.com/jmylchreest/refyne-api/internal/models"
	"github.com/jmylchreest/refyne-api/internal/repository"
)

// mockBillingUsageRepository extends mockUsageRepository for billing service tests
type mockBillingUsageRepository struct {
	mu           sync.RWMutex
	records      []*models.UsageRecord
	monthlySpend map[string]float64
	createErr    error
}

func newMockBillingUsageRepository() *mockBillingUsageRepository {
	return &mockBillingUsageRepository{
		records:      make([]*models.UsageRecord, 0),
		monthlySpend: make(map[string]float64),
	}
}

func (m *mockBillingUsageRepository) Create(ctx context.Context, record *models.UsageRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return m.createErr
	}
	m.records = append(m.records, record)
	return nil
}

func (m *mockBillingUsageRepository) GetByUserID(ctx context.Context, userID string, startDate, endDate string) ([]*models.UsageRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.UsageRecord
	for _, r := range m.records {
		if r.UserID == userID {
			result = append(result, r)
		}
	}
	return result, nil
}

func (m *mockBillingUsageRepository) GetSummary(ctx context.Context, userID string, period string) (*repository.UsageSummary, error) {
	return &repository.UsageSummary{}, nil
}

func (m *mockBillingUsageRepository) GetSummaryByDateRange(ctx context.Context, userID string, startDate, endDate time.Time) (*repository.UsageSummary, error) {
	return &repository.UsageSummary{}, nil
}

func (m *mockBillingUsageRepository) GetMonthlySpend(ctx context.Context, userID string, month time.Time) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := userID + month.Format("2006-01")
	return m.monthlySpend[key], nil
}

func (m *mockBillingUsageRepository) CountByUserAndDateRange(ctx context.Context, userID string, startDate, endDate string) (int, error) {
	return len(m.records), nil
}

// mockUsageInsightRepository implements repository.UsageInsightRepository for testing
type mockUsageInsightRepository struct {
	mu        sync.RWMutex
	insights  []*models.UsageInsight
	createErr error
}

func newMockUsageInsightRepository() *mockUsageInsightRepository {
	return &mockUsageInsightRepository{
		insights: make([]*models.UsageInsight, 0),
	}
}

func (m *mockUsageInsightRepository) Create(ctx context.Context, insight *models.UsageInsight) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return m.createErr
	}
	m.insights = append(m.insights, insight)
	return nil
}

func (m *mockUsageInsightRepository) GetByUsageID(ctx context.Context, usageID string) (*models.UsageInsight, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, i := range m.insights {
		if i.UsageID == usageID {
			return i, nil
		}
	}
	return nil, nil
}

func (m *mockUsageInsightRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.UsageInsight, error) {
	return m.insights, nil
}

// mockSchemaSnapshotRepository implements repository.SchemaSnapshotRepository for testing
type mockSchemaSnapshotRepository struct {
	mu        sync.RWMutex
	snapshots map[string]*models.SchemaSnapshot
	byHash    map[string]*models.SchemaSnapshot
	version   int
	createErr error
}

func newMockSchemaSnapshotRepository() *mockSchemaSnapshotRepository {
	return &mockSchemaSnapshotRepository{
		snapshots: make(map[string]*models.SchemaSnapshot),
		byHash:    make(map[string]*models.SchemaSnapshot),
		version:   1,
	}
}

func (m *mockSchemaSnapshotRepository) Create(ctx context.Context, snapshot *models.SchemaSnapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return m.createErr
	}
	m.snapshots[snapshot.ID] = snapshot
	m.byHash[snapshot.UserID+":"+snapshot.Hash] = snapshot
	return nil
}

func (m *mockSchemaSnapshotRepository) GetByID(ctx context.Context, id string) (*models.SchemaSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshots[id], nil
}

func (m *mockSchemaSnapshotRepository) GetByUserAndHash(ctx context.Context, userID, hash string) (*models.SchemaSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byHash[userID+":"+hash], nil
}

func (m *mockSchemaSnapshotRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.SchemaSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.SchemaSnapshot
	for _, s := range m.snapshots {
		if s.UserID == userID {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *mockSchemaSnapshotRepository) GetNextVersion(ctx context.Context, userID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := m.version
	m.version++
	return v, nil
}

func (m *mockSchemaSnapshotRepository) IncrementUsageCount(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.snapshots[id]; ok {
		s.UsageCount++
	}
	return nil
}

// newTestBillingService creates a billing service with mocks for testing
func newTestBillingService() (
	*BillingService,
	*mockBalanceRepository,
	*mockCreditTransactionRepository,
	*mockBillingUsageRepository,
	*mockUsageInsightRepository,
	*mockSchemaSnapshotRepository,
) {
	balanceRepo := newMockBalanceRepository()
	creditRepo := newMockCreditTransactionRepository()
	usageRepo := newMockBillingUsageRepository()
	insightRepo := newMockUsageInsightRepository()
	schemaRepo := newMockSchemaSnapshotRepository()

	repos := &repository.Repositories{
		Balance:           balanceRepo,
		CreditTransaction: creditRepo,
		Usage:             usageRepo,
		UsageInsight:      insightRepo,
		SchemaSnapshot:    schemaRepo,
	}

	billingCfg := config.DefaultBillingConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a pricing service for cost estimation
	pricingSvc := NewPricingService(PricingServiceConfig{}, logger)

	svc := NewBillingService(repos, &billingCfg, pricingSvc, logger)
	return svc, balanceRepo, creditRepo, usageRepo, insightRepo, schemaRepo
}

func TestGetAvailableBalance(t *testing.T) {
	svc, _, creditRepo, _, _, _ := newTestBillingService()
	ctx := context.Background()
	userID := "user_123"

	tests := []struct {
		name    string
		balance float64
	}{
		{"positive balance", 100.0},
		{"zero balance", 0.0},
		{"negative balance", -10.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creditRepo.availableBalance = tt.balance
			got, err := svc.GetAvailableBalance(ctx, userID)
			if err != nil {
				t.Fatalf("GetAvailableBalance() error = %v", err)
			}
			if got != tt.balance {
				t.Errorf("GetAvailableBalance() = %v, want %v", got, tt.balance)
			}
		})
	}
}

func TestCheckSufficientBalance(t *testing.T) {
	svc, _, creditRepo, _, _, _ := newTestBillingService()
	ctx := context.Background()
	userID := "user_123"

	tests := []struct {
		name         string
		balance      float64
		estimatedUSD float64
		wantErr      bool
	}{
		{"sufficient balance", 100.0, 50.0, false},
		{"exact balance", 50.0, 50.0, false},
		{"insufficient balance", 10.0, 50.0, true},
		{"zero balance", 0.0, 50.0, true},
		{"zero cost", 100.0, 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creditRepo.availableBalance = tt.balance
			err := svc.CheckSufficientBalance(ctx, userID, tt.estimatedUSD)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckSufficientBalance() error = %v, wantErr %v", err, tt.wantErr)
			}
			// Verify it's an LLMError with ErrInsufficientCredits
			if err != nil {
				if !errors.Is(err, llm.ErrInsufficientCredits) {
					t.Errorf("expected ErrInsufficientCredits, got %v", err)
				}
			}
		})
	}
}

func TestDeductUsage(t *testing.T) {
	ctx := context.Background()
	userID := "user_123"
	jobID := "job_123"

	tests := []struct {
		name      string
		amountUSD float64
		wantTx    bool
	}{
		{"positive amount", 10.0, true},
		{"zero amount", 0.0, false},
		{"negative amount", -5.0, false},
		{"small amount", 0.001, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, balanceRepo, creditRepo, _, _, _ := newTestBillingService()

			// Set up initial balance
			balanceRepo.balances[userID] = &models.UserBalance{
				UserID:     userID,
				BalanceUSD: 100.0,
			}

			err := svc.DeductUsage(ctx, userID, tt.amountUSD, &jobID)
			if err != nil {
				t.Fatalf("DeductUsage() error = %v", err)
			}

			if tt.wantTx {
				if len(creditRepo.transactions) == 0 {
					t.Error("expected transaction to be created")
					return
				}
				tx := creditRepo.transactions[0]
				if tx.AmountUSD != -tt.amountUSD {
					t.Errorf("transaction amount = %v, want %v", tx.AmountUSD, -tt.amountUSD)
				}
				if tx.Type != models.TxTypeUsage {
					t.Errorf("transaction type = %v, want %v", tx.Type, models.TxTypeUsage)
				}
			} else {
				if len(creditRepo.transactions) > 0 {
					t.Error("expected no transaction to be created")
				}
			}
		})
	}
}

func TestEstimateCost(t *testing.T) {
	svc, _, _, _, _, _ := newTestBillingService()

	tests := []struct {
		name     string
		pages    int
		model    string
		provider string
		minCost  float64 // Just verify it's positive and reasonable
	}{
		{"single page", 1, "gpt-4", "openai", 0.0},
		{"multiple pages", 10, "claude-3-opus", "anthropic", 0.0},
		{"zero pages", 0, "gpt-4", "openai", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := svc.EstimateCost(tt.pages, tt.model, tt.provider)
			// EstimateCost should always return a reasonable estimate
			// It doubles the base cost for margin, so it should be >= 0
			if cost < tt.minCost {
				t.Errorf("EstimateCost() = %v, want >= %v", cost, tt.minCost)
			}
		})
	}
}

func TestCalculateTotalCost(t *testing.T) {
	svc, _, _, _, _, _ := newTestBillingService()

	// Actual tier limits: all tiers have 2% markup (0.02), no per-tx cost
	tests := []struct {
		name        string
		llmCostUSD  float64
		tier        string
		isBYOK      bool
		wantTotal   float64
		wantMarkup  float64
	}{
		{
			name:       "BYOK - no markup",
			llmCostUSD: 1.0,
			tier:       "free",
			isBYOK:     true,
			wantTotal:  1.0,
			wantMarkup: 0.0,
		},
		{
			name:       "free tier - 2% markup",
			llmCostUSD: 1.0,
			tier:       "free",
			isBYOK:     false,
			// Free tier: 2% markup, no per-tx cost
			wantTotal:  1.02,
			wantMarkup: 0.02,
		},
		{
			name:       "pro tier - 2% markup",
			llmCostUSD: 1.0,
			tier:       "pro",
			isBYOK:     false,
			// Pro tier: 2% markup, no per-tx cost
			wantTotal:  1.02,
			wantMarkup: 0.02,
		},
		{
			name:       "selfhosted tier - no markup",
			llmCostUSD: 1.0,
			tier:       "selfhosted",
			isBYOK:     false,
			// Selfhosted tier: 0% markup
			wantTotal:  1.0,
			wantMarkup: 0.0,
		},
		{
			name:       "zero cost",
			llmCostUSD: 0.0,
			tier:       "free",
			isBYOK:     false,
			// No per-transaction cost
			wantTotal:  0.0,
			wantMarkup: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, markupRate, markupUSD := svc.CalculateTotalCost(tt.llmCostUSD, tt.tier, tt.isBYOK)

			// For BYOK, all should be zero except the base cost
			if tt.isBYOK {
				if total != tt.llmCostUSD {
					t.Errorf("total = %v, want %v", total, tt.llmCostUSD)
				}
				if markupRate != 0 || markupUSD != 0 {
					t.Errorf("BYOK should have no markup, got rate=%v, usd=%v", markupRate, markupUSD)
				}
				return
			}

			// Allow small floating point tolerance
			tolerance := 0.001
			if total < tt.wantTotal-tolerance || total > tt.wantTotal+tolerance {
				t.Errorf("total = %v, want %v", total, tt.wantTotal)
			}
			if markupUSD < tt.wantMarkup-tolerance || markupUSD > tt.wantMarkup+tolerance {
				t.Errorf("markupUSD = %v, want %v", markupUSD, tt.wantMarkup)
			}
		})
	}
}

func TestGetOrCreateSchemaSnapshot(t *testing.T) {
	svc, _, _, _, _, schemaRepo := newTestBillingService()
	ctx := context.Background()
	userID := "user_123"
	schemaJSON := `{"type": "object", "properties": {"name": {"type": "string"}}}`

	// First call should create new snapshot
	snapshot1, err := svc.GetOrCreateSchemaSnapshot(ctx, userID, schemaJSON)
	if err != nil {
		t.Fatalf("GetOrCreateSchemaSnapshot() error = %v", err)
	}
	if snapshot1 == nil {
		t.Fatal("expected snapshot to be created")
	}
	if snapshot1.Version != 1 {
		t.Errorf("version = %d, want 1", snapshot1.Version)
	}
	if snapshot1.UsageCount != 1 {
		t.Errorf("usage count = %d, want 1", snapshot1.UsageCount)
	}
	if snapshot1.SchemaJSON != schemaJSON {
		t.Error("schema JSON does not match")
	}

	// Store the snapshot in byHash for the second lookup
	schemaRepo.mu.Lock()
	schemaRepo.byHash[userID+":"+snapshot1.Hash] = schemaRepo.snapshots[snapshot1.ID]
	schemaRepo.mu.Unlock()

	// Second call with same schema should return existing
	snapshot2, err := svc.GetOrCreateSchemaSnapshot(ctx, userID, schemaJSON)
	if err != nil {
		t.Fatalf("GetOrCreateSchemaSnapshot() second call error = %v", err)
	}
	if snapshot2.ID != snapshot1.ID {
		t.Error("expected same snapshot to be returned")
	}
	// Usage count should be incremented
	if snapshot2.UsageCount != 2 {
		t.Errorf("usage count = %d, want 2", snapshot2.UsageCount)
	}

	// Different schema should create new snapshot
	differentSchema := `{"type": "array"}`
	snapshot3, err := svc.GetOrCreateSchemaSnapshot(ctx, userID, differentSchema)
	if err != nil {
		t.Fatalf("GetOrCreateSchemaSnapshot() different schema error = %v", err)
	}
	if snapshot3.ID == snapshot1.ID {
		t.Error("expected different snapshot for different schema")
	}
	if snapshot3.Version != 2 {
		t.Errorf("version = %d, want 2", snapshot3.Version)
	}
}

func TestRecordUsage(t *testing.T) {
	svc, _, _, usageRepo, insightRepo, _ := newTestBillingService()
	ctx := context.Background()

	record := &UsageRecord{
		UserID:            "user_123",
		JobID:             "job_123",
		JobType:           models.JobTypeExtract,
		Status:            "success",
		TotalChargedUSD:   0.05,
		IsBYOK:            false,
		TargetURL:         "https://example.com",
		SchemaID:          "schema_123",
		TokensInput:       1000,
		TokensOutput:      500,
		LLMCostUSD:        0.03,
		MarkupRate:        0.5,
		MarkupUSD:         0.015,
		LLMProvider:       "openrouter",
		LLMModel:          "gpt-4",
		GenerationID:      "gen_123",
		PagesAttempted:    1,
		PagesSuccessful:   1,
		FetchDurationMs:   100,
		ExtractDurationMs: 500,
		TotalDurationMs:   600,
		RequestID:         "req_123",
	}

	err := svc.RecordUsage(ctx, record)
	if err != nil {
		t.Fatalf("RecordUsage() error = %v", err)
	}

	// Verify usage record was created
	if len(usageRepo.records) != 1 {
		t.Fatalf("expected 1 usage record, got %d", len(usageRepo.records))
	}

	usage := usageRepo.records[0]
	if usage.UserID != record.UserID {
		t.Errorf("user_id = %v, want %v", usage.UserID, record.UserID)
	}
	if usage.JobID != record.JobID {
		t.Errorf("job_id = %v, want %v", usage.JobID, record.JobID)
	}
	if usage.TotalChargedUSD != record.TotalChargedUSD {
		t.Errorf("total_charged = %v, want %v", usage.TotalChargedUSD, record.TotalChargedUSD)
	}
	if usage.IsBYOK != record.IsBYOK {
		t.Errorf("is_byok = %v, want %v", usage.IsBYOK, record.IsBYOK)
	}

	// Verify usage insight was created
	if len(insightRepo.insights) != 1 {
		t.Fatalf("expected 1 usage insight, got %d", len(insightRepo.insights))
	}

	insight := insightRepo.insights[0]
	if insight.TargetURL != record.TargetURL {
		t.Errorf("target_url = %v, want %v", insight.TargetURL, record.TargetURL)
	}
	if insight.TokensInput != record.TokensInput {
		t.Errorf("tokens_input = %v, want %v", insight.TokensInput, record.TokensInput)
	}
	if insight.LLMModel != record.LLMModel {
		t.Errorf("llm_model = %v, want %v", insight.LLMModel, record.LLMModel)
	}
}

func TestRecordUsageInsightFailure(t *testing.T) {
	svc, _, _, usageRepo, insightRepo, _ := newTestBillingService()
	ctx := context.Background()

	// Make insight creation fail
	insightRepo.createErr = errors.New("insight creation failed")

	record := &UsageRecord{
		UserID:          "user_123",
		JobID:           "job_123",
		JobType:         models.JobTypeExtract,
		Status:          "success",
		TotalChargedUSD: 0.05,
	}

	// RecordUsage should NOT fail if insight creation fails (it's secondary)
	err := svc.RecordUsage(ctx, record)
	if err != nil {
		t.Fatalf("RecordUsage() should not fail when insight fails, error = %v", err)
	}

	// Usage record should still be created
	if len(usageRepo.records) != 1 {
		t.Error("usage record should still be created")
	}
}

func TestIsBYOK(t *testing.T) {
	svc, _, _, _, _, _ := newTestBillingService()

	serviceOR := "sk-or-service"
	serviceAnthropic := "sk-ant-service"
	serviceOpenAI := "sk-openai-service"

	tests := []struct {
		name                  string
		provider              string
		apiKey                string
		serviceOpenRouterKey  string
		serviceAnthropicKey   string
		serviceOpenAIKey      string
		want                  bool
	}{
		{
			name:     "ollama - always BYOK",
			provider: llm.ProviderOllama,
			apiKey:   "",
			want:     true,
		},
		{
			name:                 "matching OpenRouter service key",
			provider:             llm.ProviderOpenRouter,
			apiKey:               serviceOR,
			serviceOpenRouterKey: serviceOR,
			want:                 false,
		},
		{
			name:                 "different OpenRouter key - BYOK",
			provider:             llm.ProviderOpenRouter,
			apiKey:               "user-key-123",
			serviceOpenRouterKey: serviceOR,
			want:                 true,
		},
		{
			name:                "matching Anthropic service key",
			provider:            llm.ProviderAnthropic,
			apiKey:              serviceAnthropic,
			serviceAnthropicKey: serviceAnthropic,
			want:                false,
		},
		{
			name:                "different Anthropic key - BYOK",
			provider:            llm.ProviderAnthropic,
			apiKey:              "user-ant-key",
			serviceAnthropicKey: serviceAnthropic,
			want:                true,
		},
		{
			name:             "matching OpenAI service key",
			provider:         llm.ProviderOpenAI,
			apiKey:           serviceOpenAI,
			serviceOpenAIKey: serviceOpenAI,
			want:             false,
		},
		{
			name:             "different OpenAI key - BYOK",
			provider:         llm.ProviderOpenAI,
			apiKey:           "user-openai-key",
			serviceOpenAIKey: serviceOpenAI,
			want:             true,
		},
		{
			name:     "empty API key - not BYOK",
			provider: llm.ProviderOpenAI,
			apiKey:   "",
			want:     false,
		},
		{
			name:                 "API key with no service keys configured",
			provider:             llm.ProviderOpenRouter,
			apiKey:               "user-key",
			serviceOpenRouterKey: "",
			want:                 true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.IsBYOK(tt.provider, tt.apiKey, tt.serviceOpenRouterKey, tt.serviceAnthropicKey, tt.serviceOpenAIKey)
			if got != tt.want {
				t.Errorf("IsBYOK() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChargeForUsage(t *testing.T) {
	ctx := context.Background()
	userID := "user_123"

	tests := []struct {
		name           string
		input          *ChargeForUsageInput
		wantStatus     string
		wantDeduction  bool
		wantBYOKProv   string
	}{
		{
			name: "successful extraction - non-BYOK",
			input: &ChargeForUsageInput{
				UserID:          userID,
				Tier:            "pro",
				JobID:           "job_123",
				JobType:         models.JobTypeExtract,
				IsBYOK:          false,
				TokensInput:     1000,
				TokensOutput:    500,
				Model:           "gpt-4",
				Provider:        "openrouter",
				TargetURL:       "https://example.com",
				PagesAttempted:  1,
				PagesSuccessful: 1,
			},
			wantStatus:    "success",
			wantDeduction: true,
		},
		{
			name: "BYOK - no deduction",
			input: &ChargeForUsageInput{
				UserID:          userID,
				Tier:            "pro",
				JobID:           "job_byok",
				JobType:         models.JobTypeExtract,
				IsBYOK:          true,
				TokensInput:     1000,
				TokensOutput:    500,
				Model:           "gpt-4",
				Provider:        "openrouter",
				PagesAttempted:  1,
				PagesSuccessful: 1,
			},
			wantStatus:    "success",
			wantDeduction: false,
			wantBYOKProv:  "openrouter",
		},
		{
			name: "failed extraction",
			input: &ChargeForUsageInput{
				UserID:       userID,
				Tier:         "free",
				JobID:        "job_failed",
				JobType:      models.JobTypeExtract,
				IsBYOK:       false,
				TokensInput:  500,
				TokensOutput: 0,
				Model:        "gpt-4",
				Provider:     "openrouter",
				ErrorMessage: "extraction failed",
				ErrorCode:    "ERR_PARSE",
			},
			wantStatus:    "failed",
			wantDeduction: true,
		},
		{
			name: "partial success",
			input: &ChargeForUsageInput{
				UserID:          userID,
				Tier:            "pro",
				JobID:           "job_partial",
				JobType:         models.JobTypeCrawl,
				IsBYOK:          false,
				TokensInput:     2000,
				TokensOutput:    1000,
				Model:           "gpt-4",
				Provider:        "openrouter",
				PagesAttempted:  5,
				PagesSuccessful: 3,
			},
			wantStatus:    "partial",
			wantDeduction: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, balanceRepo, creditRepo, usageRepo, insightRepo, _ := newTestBillingService()

			// Set up initial balance
			balanceRepo.balances[userID] = &models.UserBalance{
				UserID:     userID,
				BalanceUSD: 100.0,
			}

			result, err := svc.ChargeForUsage(ctx, tt.input)
			if err != nil {
				t.Fatalf("ChargeForUsage() error = %v", err)
			}

			// Verify result
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if result.LLMCostUSD < 0 {
				t.Error("LLM cost should be non-negative")
			}

			// Check if deduction was made
			hasDeduction := len(creditRepo.transactions) > 0
			if hasDeduction != tt.wantDeduction {
				t.Errorf("deduction made = %v, want %v", hasDeduction, tt.wantDeduction)
			}

			// Verify BYOK markup
			if tt.input.IsBYOK {
				if result.MarkupRate != 0 || result.MarkupUSD != 0 {
					t.Error("BYOK should have no markup")
				}
			}

			// Verify usage record
			if len(usageRepo.records) != 1 {
				t.Fatal("expected usage record to be created")
			}
			usage := usageRepo.records[0]
			if usage.Status != tt.wantStatus {
				t.Errorf("status = %v, want %v", usage.Status, tt.wantStatus)
			}

			// Verify insight record for BYOK provider
			if len(insightRepo.insights) != 1 {
				t.Fatal("expected insight record to be created")
			}
			insight := insightRepo.insights[0]
			if tt.wantBYOKProv != "" && insight.BYOKProvider != tt.wantBYOKProv {
				t.Errorf("byok_provider = %v, want %v", insight.BYOKProvider, tt.wantBYOKProv)
			}
		})
	}
}

func TestChargeForUsageZeroTokens(t *testing.T) {
	svc, balanceRepo, _, usageRepo, _, _ := newTestBillingService()
	ctx := context.Background()
	userID := "user_123"

	balanceRepo.balances[userID] = &models.UserBalance{
		UserID:     userID,
		BalanceUSD: 100.0,
	}

	input := &ChargeForUsageInput{
		UserID:          userID,
		Tier:            "free",
		JobID:           "job_zero",
		JobType:         models.JobTypeExtract,
		IsBYOK:          false,
		TokensInput:     0,
		TokensOutput:    0,
		Model:           "gpt-4",
		Provider:        "openrouter",
		PagesAttempted:  1,
		PagesSuccessful: 1,
	}

	result, err := svc.ChargeForUsage(ctx, input)
	if err != nil {
		t.Fatalf("ChargeForUsage() error = %v", err)
	}

	// With zero tokens, cost should be zero or minimal
	if result.LLMCostUSD < 0 {
		t.Error("LLM cost should be non-negative")
	}

	// Usage should still be recorded
	if len(usageRepo.records) != 1 {
		t.Error("usage record should be created even for zero tokens")
	}
}
