package notification

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestSecretEncryptionRoundTripAndAuthentication(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	ciphertext, err := EncryptSecret(key, "webhook-secret-value")
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext == "webhook-secret-value" || !strings.HasPrefix(ciphertext, "v1:") {
		t.Fatalf("ciphertext is not an encrypted envelope: %q", ciphertext)
	}
	plaintext, err := DecryptSecret(key, ciphertext)
	if err != nil || plaintext != "webhook-secret-value" {
		t.Fatalf("round trip plaintext=%q error=%v", plaintext, err)
	}
	key[0]++
	if _, err := DecryptSecret(key, ciphertext); err == nil {
		t.Fatal("tampered key decrypted the secret")
	}
}

func TestDecodeEncryptionKeyAcceptsHexAndBase64(t *testing.T) {
	raw := []byte(strings.Repeat("x", 32))
	if decoded, err := DecodeEncryptionKey(hex.EncodeToString(raw)); err != nil || string(decoded) != string(raw) {
		t.Fatalf("hex key decode = %q, %v", decoded, err)
	}
}

func TestWebhookValidationRejectsSSRFTargets(t *testing.T) {
	for _, url := range []string{"http://example.com/hook", "https://127.0.0.1/hook", "https://user:pass@example.com/hook", "https://example.com/hook#fragment"} {
		err := (CreateWebhookParams{Name: "test", URL: url, Events: []string{"detection.created"}}).Validate()
		if err == nil {
			t.Fatalf("Validate(%q) accepted an unsafe URL", url)
		}
	}
}
