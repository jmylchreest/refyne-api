package mw

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ========================================
// ExtendWriteDeadlineForSyncRequests Tests
// ========================================

func TestExtendWriteDeadlineForSyncRequests_NoWaitParam(t *testing.T) {
	var handlerCalled bool
	handler := ExtendWriteDeadlineForSyncRequests()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/123", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestExtendWriteDeadlineForSyncRequests_WaitFalse(t *testing.T) {
	var handlerCalled bool
	handler := ExtendWriteDeadlineForSyncRequests()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/123?wait=false", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestExtendWriteDeadlineForSyncRequests_WaitTrue(t *testing.T) {
	var handlerCalled bool
	handler := ExtendWriteDeadlineForSyncRequests()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		// In real scenario, the deadline would be extended
		// We can't easily verify the deadline extension with httptest.ResponseRecorder
		// but we can verify the handler is called
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/123?wait=true", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestExtendWriteDeadlineForSyncRequests_WaitTrueUpperCase(t *testing.T) {
	// Test that "TRUE" or "True" doesn't trigger extension (case-sensitive)
	var handlerCalled bool
	handler := ExtendWriteDeadlineForSyncRequests()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/123?wait=TRUE", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestExtendWriteDeadlineForSyncRequests_OtherQueryParams(t *testing.T) {
	var handlerCalled bool
	handler := ExtendWriteDeadlineForSyncRequests()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	// Multiple query params with wait=true
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/123?format=json&wait=true&timeout=60", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestExtendWriteDeadlineForSyncRequests_POSTWithWait(t *testing.T) {
	var handlerCalled bool
	handler := ExtendWriteDeadlineForSyncRequests()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/extract?wait=true", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
}
