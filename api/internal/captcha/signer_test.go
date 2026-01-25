package captcha

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

func TestSigner_Sign(t *testing.T) {
	secret := "test-secret-key"
	signer := NewSigner(secret)

	t.Run("generates valid signature", func(t *testing.T) {
		userID := "user_123"
		tier := "pro"
		features := []string{"captcha", "api_access"}
		jobID := "job_456"
		body := []byte(`{"cmd":"request.get","url":"https://example.com"}`)

		headers := signer.Sign(userID, tier, features, jobID, body)

		if headers.Signature == "" {
			t.Error("expected non-empty signature")
		}
		if headers.Timestamp == "" {
			t.Error("expected non-empty timestamp")
		}
		if headers.UserID != userID {
			t.Errorf("UserID = %q, want %q", headers.UserID, userID)
		}
		if headers.Tier != tier {
			t.Errorf("Tier = %q, want %q", headers.Tier, tier)
		}
		if headers.Features != "captcha,api_access" {
			t.Errorf("Features = %q, want %q", headers.Features, "captcha,api_access")
		}
		if headers.JobID != jobID {
			t.Errorf("JobID = %q, want %q", headers.JobID, jobID)
		}
	})

	t.Run("signature format matches expected", func(t *testing.T) {
		userID := "user_123"
		tier := "pro"
		features := []string{"captcha"}
		jobID := "job_789"
		body := []byte(`{"test":"data"}`)

		headers := signer.Sign(userID, tier, features, jobID, body)

		// Manually compute expected signature
		bodyHash := sha256.Sum256(body)
		message := headers.Timestamp + "|" + userID + "|" + tier + "|" + "captcha" + "|" + jobID + "|" + hex.EncodeToString(bodyHash[:])
		h := hmac.New(sha256.New, []byte(secret))
		h.Write([]byte(message))
		expected := hex.EncodeToString(h.Sum(nil))

		if headers.Signature != expected {
			t.Errorf("Signature = %q, want %q", headers.Signature, expected)
		}
	})

	t.Run("empty job ID is valid", func(t *testing.T) {
		headers := signer.Sign("user", "tier", []string{}, "", []byte("body"))

		if headers.Signature == "" {
			t.Error("expected non-empty signature even with empty job ID")
		}
		if headers.JobID != "" {
			t.Errorf("JobID = %q, want empty", headers.JobID)
		}
	})

	t.Run("different bodies produce different signatures", func(t *testing.T) {
		body1 := []byte(`{"url":"https://example1.com"}`)
		body2 := []byte(`{"url":"https://example2.com"}`)

		headers1 := signer.Sign("user", "tier", []string{}, "", body1)
		headers2 := signer.Sign("user", "tier", []string{}, "", body2)

		if headers1.Signature == headers2.Signature {
			t.Error("expected different signatures for different bodies")
		}
	})

	t.Run("different job IDs produce different signatures", func(t *testing.T) {
		body := []byte(`{"url":"https://example.com"}`)

		headers1 := signer.Sign("user", "tier", []string{}, "job_1", body)
		headers2 := signer.Sign("user", "tier", []string{}, "job_2", body)

		if headers1.Signature == headers2.Signature {
			t.Error("expected different signatures for different job IDs")
		}
	})
}

func TestSigner_Verify(t *testing.T) {
	secret := "test-secret-key"
	signer := NewSigner(secret)

	t.Run("verifies valid signature", func(t *testing.T) {
		userID := "user_123"
		tier := "pro"
		features := []string{"captcha"}
		jobID := "job_456"
		body := []byte(`{"test":"data"}`)

		headers := signer.Sign(userID, tier, features, jobID, body)

		valid := signer.Verify(
			headers.Signature,
			headers.Timestamp,
			headers.UserID,
			headers.Tier,
			headers.Features,
			headers.JobID,
			body,
		)

		if !valid {
			t.Error("expected signature to be valid")
		}
	})

	t.Run("rejects expired timestamp", func(t *testing.T) {
		userID := "user_123"
		tier := "pro"
		features := "captcha"
		jobID := ""
		body := []byte(`{"test":"data"}`)

		// Create timestamp 6 minutes ago (expired)
		oldTimestamp := strconv.FormatInt(time.Now().Add(-6*time.Minute).Unix(), 10)

		// Manually create signature with old timestamp
		bodyHash := sha256.Sum256(body)
		message := oldTimestamp + "|" + userID + "|" + tier + "|" + features + "|" + jobID + "|" + hex.EncodeToString(bodyHash[:])
		h := hmac.New(sha256.New, []byte(secret))
		h.Write([]byte(message))
		signature := hex.EncodeToString(h.Sum(nil))

		valid := signer.Verify(signature, oldTimestamp, userID, tier, features, jobID, body)

		if valid {
			t.Error("expected signature with expired timestamp to be invalid")
		}
	})

	t.Run("rejects tampered body", func(t *testing.T) {
		userID := "user_123"
		tier := "pro"
		features := []string{"captcha"}
		jobID := ""
		originalBody := []byte(`{"test":"original"}`)
		tamperedBody := []byte(`{"test":"tampered"}`)

		headers := signer.Sign(userID, tier, features, jobID, originalBody)

		valid := signer.Verify(
			headers.Signature,
			headers.Timestamp,
			headers.UserID,
			headers.Tier,
			headers.Features,
			headers.JobID,
			tamperedBody,
		)

		if valid {
			t.Error("expected signature to be invalid for tampered body")
		}
	})

	t.Run("rejects tampered job ID", func(t *testing.T) {
		userID := "user_123"
		tier := "pro"
		features := []string{"captcha"}
		jobID := "job_123"
		body := []byte(`{"test":"data"}`)

		headers := signer.Sign(userID, tier, features, jobID, body)

		valid := signer.Verify(
			headers.Signature,
			headers.Timestamp,
			headers.UserID,
			headers.Tier,
			headers.Features,
			"tampered_job_id",
			body,
		)

		if valid {
			t.Error("expected signature to be invalid for tampered job ID")
		}
	})

	t.Run("rejects wrong secret", func(t *testing.T) {
		wrongSigner := NewSigner("wrong-secret")

		userID := "user_123"
		tier := "pro"
		features := []string{"captcha"}
		jobID := ""
		body := []byte(`{"test":"data"}`)

		headers := signer.Sign(userID, tier, features, jobID, body)

		valid := wrongSigner.Verify(
			headers.Signature,
			headers.Timestamp,
			headers.UserID,
			headers.Tier,
			headers.Features,
			headers.JobID,
			body,
		)

		if valid {
			t.Error("expected signature to be invalid with wrong secret")
		}
	})

	t.Run("rejects invalid timestamp format", func(t *testing.T) {
		valid := signer.Verify(
			"some-signature",
			"not-a-number",
			"user",
			"tier",
			"features",
			"",
			[]byte("body"),
		)

		if valid {
			t.Error("expected invalid timestamp format to be rejected")
		}
	})
}

func TestSignerInteroperability(t *testing.T) {
	// This test ensures the signer produces signatures that match
	// what the captcha service auth middleware expects
	secret := "shared-secret"
	signer := NewSigner(secret)

	userID := "user_test"
	tier := "premium"
	features := []string{"content_dynamic", "models_premium"}
	jobID := "job_interop_test"
	body := []byte(`{"cmd":"request.get","url":"https://test.example.com"}`)

	headers := signer.Sign(userID, tier, features, jobID, body)

	// Manually verify using the same algorithm the server uses
	bodyHash := sha256.Sum256(body)
	message := headers.Timestamp + "|" + userID + "|" + tier + "|" + headers.Features + "|" + jobID + "|" + hex.EncodeToString(bodyHash[:])

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(headers.Signature), []byte(expectedSig)) {
		t.Errorf("signature mismatch:\ngot:  %s\nwant: %s", headers.Signature, expectedSig)
	}
}
