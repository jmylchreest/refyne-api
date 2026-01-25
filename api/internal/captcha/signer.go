// Package captcha provides a client for the captcha service.
package captcha

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// Signer creates HMAC signatures for captcha service requests.
type Signer struct {
	secret []byte
}

// NewSigner creates a new HMAC signer with the given secret.
func NewSigner(secret string) *Signer {
	return &Signer{
		secret: []byte(secret),
	}
}

// SignatureHeaders represents the headers to include in a signed request.
type SignatureHeaders struct {
	Signature string
	Timestamp string
	UserID    string
	Tier      string
	Features  string
	JobID     string
}

// Sign creates a signature for the given request parameters.
// Returns headers to include in the request to the captcha service.
// Signature format: HMAC-SHA256(timestamp|userID|tier|features|jobID|bodyHash)
func (s *Signer) Sign(userID, tier string, features []string, jobID string, body []byte) SignatureHeaders {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	featuresStr := strings.Join(features, ",")

	// Create message to sign: timestamp|userID|tier|features|jobID|bodyHash
	bodyHash := sha256.Sum256(body)
	message := timestamp + "|" + userID + "|" + tier + "|" + featuresStr + "|" + jobID + "|" + hex.EncodeToString(bodyHash[:])

	// Calculate HMAC-SHA256
	h := hmac.New(sha256.New, s.secret)
	h.Write([]byte(message))
	signature := hex.EncodeToString(h.Sum(nil))

	return SignatureHeaders{
		Signature: signature,
		Timestamp: timestamp,
		UserID:    userID,
		Tier:      tier,
		Features:  featuresStr,
		JobID:     jobID,
	}
}

// Verify verifies a signature against the expected parameters.
// Used by the captcha service to validate incoming requests.
// Signature format: HMAC-SHA256(timestamp|userID|tier|features|jobID|bodyHash)
func (s *Signer) Verify(signature, timestamp, userID, tier, features, jobID string, body []byte) bool {
	// Check timestamp is recent (within 5 minutes)
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if time.Since(time.Unix(ts, 0)) > 5*time.Minute {
		return false
	}

	// Recreate expected signature
	bodyHash := sha256.Sum256(body)
	message := timestamp + "|" + userID + "|" + tier + "|" + features + "|" + jobID + "|" + hex.EncodeToString(bodyHash[:])

	h := hmac.New(sha256.New, s.secret)
	h.Write([]byte(message))
	expected := hex.EncodeToString(h.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expected))
}
