package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/jmylchreest/refyne-api/internal/http/mw"
)

// ========================================
// HealthCheck Tests
// ========================================

func TestHealthCheck(t *testing.T) {
	output, err := HealthCheck(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == nil {
		t.Fatal("expected output, got nil")
	}
	if output.Body.Status != "healthy" {
		t.Errorf("Status = %q, want %q", output.Body.Status, "healthy")
	}
	if output.Body.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", output.Body.Version, "1.0.0")
	}
}

// ========================================
// Livez Tests
// ========================================

func TestLivez(t *testing.T) {
	output, err := Livez(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == nil {
		t.Fatal("expected output, got nil")
	}
	if output.Body.Status != "ok" {
		t.Errorf("Status = %q, want %q", output.Body.Status, "ok")
	}
}

// ========================================
// Readyz Tests
// ========================================

// mockDBPinger implements DBPinger for testing
type mockDBPinger struct {
	err error
}

func (m *mockDBPinger) Ping() error {
	return m.err
}

func TestNewReadyzHandler(t *testing.T) {
	db := &mockDBPinger{}
	handler := NewReadyzHandler(db)

	if handler == nil {
		t.Fatal("expected handler, got nil")
	}
	if handler.db != db {
		t.Error("db not set correctly")
	}
}

func TestReadyzHandler_Readyz_Success(t *testing.T) {
	db := &mockDBPinger{err: nil}
	handler := NewReadyzHandler(db)

	output, err := handler.Readyz(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == nil {
		t.Fatal("expected output, got nil")
	}
	if output.Body.Status != "ok" {
		t.Errorf("Status = %q, want %q", output.Body.Status, "ok")
	}
}

func TestReadyzHandler_Readyz_DBError(t *testing.T) {
	db := &mockDBPinger{err: errors.New("connection failed")}
	handler := NewReadyzHandler(db)

	_, err := handler.Readyz(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestReadyzHandler_Readyz_NilDB(t *testing.T) {
	handler := NewReadyzHandler(nil)

	output, err := handler.Readyz(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Body.Status != "ok" {
		t.Errorf("Status = %q, want %q", output.Body.Status, "ok")
	}
}

// ========================================
// getUserID Tests
// ========================================

func TestGetUserID_WithClaims(t *testing.T) {
	claims := &mw.UserClaims{
		UserID: "user-123",
	}
	ctx := context.WithValue(context.Background(), mw.UserClaimsKey, claims)

	userID := getUserID(ctx)
	if userID != "user-123" {
		t.Errorf("getUserID() = %q, want %q", userID, "user-123")
	}
}

func TestGetUserID_NoClaims(t *testing.T) {
	userID := getUserID(context.Background())
	if userID != "" {
		t.Errorf("getUserID() = %q, want empty", userID)
	}
}

// ========================================
// getUserClaims Tests
// ========================================

func TestGetUserClaims_WithClaims(t *testing.T) {
	expected := &mw.UserClaims{
		UserID: "user-456",
		Email:  "test@example.com",
		Tier:   "pro",
	}
	ctx := context.WithValue(context.Background(), mw.UserClaimsKey, expected)

	claims := getUserClaims(ctx)
	if claims == nil {
		t.Fatal("expected claims, got nil")
	}
	if claims.UserID != expected.UserID {
		t.Errorf("UserID = %q, want %q", claims.UserID, expected.UserID)
	}
	if claims.Email != expected.Email {
		t.Errorf("Email = %q, want %q", claims.Email, expected.Email)
	}
	if claims.Tier != expected.Tier {
		t.Errorf("Tier = %q, want %q", claims.Tier, expected.Tier)
	}
}

func TestGetUserClaims_NoClaims(t *testing.T) {
	claims := getUserClaims(context.Background())
	if claims != nil {
		t.Errorf("expected nil, got %+v", claims)
	}
}

// ========================================
// Output Struct Tests
// ========================================

func TestHealthCheckOutput_Fields(t *testing.T) {
	output := HealthCheckOutput{}
	output.Body.Status = "healthy"
	output.Body.Version = "2.0.0"

	if output.Body.Status != "healthy" {
		t.Errorf("Status = %q, want %q", output.Body.Status, "healthy")
	}
	if output.Body.Version != "2.0.0" {
		t.Errorf("Version = %q, want %q", output.Body.Version, "2.0.0")
	}
}

func TestLivezOutput_Fields(t *testing.T) {
	output := LivezOutput{}
	output.Body.Status = "ok"

	if output.Body.Status != "ok" {
		t.Errorf("Status = %q, want %q", output.Body.Status, "ok")
	}
}

func TestReadyzOutput_Fields(t *testing.T) {
	output := ReadyzOutput{}
	output.Body.Status = "ok"

	if output.Body.Status != "ok" {
		t.Errorf("Status = %q, want %q", output.Body.Status, "ok")
	}
}
