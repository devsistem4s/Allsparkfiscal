package store

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyWebhookSignature(t *testing.T) {
	secret := "whsec_test_secret_123"
	body := []byte(`{"event":"document.authorized","document_id":"123","status":"authorized"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !VerifyWebhookSignature(secret, sig, body) {
		t.Fatal("expected valid HMAC-SHA256 signature to pass verification")
	}
	if VerifyWebhookSignature("wrong_secret", sig, body) {
		t.Fatal("expected wrong secret to fail verification")
	}
	if VerifyWebhookSignature(secret, "sha256=deadbeef", body) {
		t.Fatal("expected tampered signature to fail verification")
	}
	if !isRetryableHTTPStatus(429) || !isRetryableHTTPStatus(503) || isRetryableHTTPStatus(200) || isRetryableHTTPStatus(400) {
		t.Fatal("unexpected retryable HTTP status classification")
	}
}

