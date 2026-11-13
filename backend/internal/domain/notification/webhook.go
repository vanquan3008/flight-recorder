// Package notification defines the webhook configuration and delivery models.
// Delivery intents are persisted by the storage transaction outbox; the
// notification worker only claims and sends those intents.
package notification

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/netaddr"
)

var (
	ErrWebhookNotFound = errors.New("webhook not found")
	ErrInvalidWebhook  = errors.New("invalid webhook configuration")
	ErrNameExists      = errors.New("webhook name already exists")
)

// AllowedEvents is the set of event types that can be subscribed to.
var AllowedEvents = map[string]struct{}{
	"incident.opened":   {},
	"incident.resolved": {},
}

// Webhook is the configuration for an outbound notification endpoint.
type Webhook struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WebhookRecord is the delivery-time view of a webhook. Secret is intentionally
// kept out of Webhook because management responses must never expose it.
type WebhookRecord struct {
	ID        string
	ProjectID string
	URL       string
	Secret    string
}

// IssuedWebhook contains the one-time secret returned after creation. The
// secret is deliberately absent from Webhook and is never returned by reads.
type IssuedWebhook struct {
	Webhook
	Secret string `json:"secret"`
}

// DeliveryAttempt is the claimed, delivery-ready representation used by the
// durable retry worker. It contains the decrypted secret only in memory.
type DeliveryAttempt struct {
	Delivery
	ProjectID string
	URL       string
	Secret    string
	Payload   []byte
}

// WebhookFilter is used to list/paginate webhooks.
type WebhookFilter struct {
	Before   time.Time
	CursorID string
	Limit    int
}

// Delivery represents one outbound HTTP call attempt for a webhook.
type Delivery struct {
	ID            string    `json:"id"`
	WebhookID     string    `json:"webhook_id"`
	EventType     string    `json:"event_type"`
	Status        string    `json:"status"`
	Attempt       int       `json:"attempt"`
	MaxAttempts   int       `json:"max_attempts"`
	HTTPStatus    *int      `json:"http_status,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// DeliveryFilter is used to list deliveries.
type DeliveryFilter struct {
	WebhookID string
	Before    time.Time
	CursorID  string
	Limit     int
}

// CreateWebhookParams is the input for creating a new webhook.
type CreateWebhookParams struct {
	Name   string
	URL    string
	Secret string
	Events []string
}

type UpdateWebhookParams struct {
	Name    string
	URL     string
	Events  []string
	Enabled *bool
}

func (p UpdateWebhookParams) Validate() error {
	return (CreateWebhookParams{Name: p.Name, URL: p.URL, Events: p.Events}).Validate()
}

func (p CreateWebhookParams) Validate() error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len(p.Name) > 128 {
		return fmt.Errorf("%w: webhook name must be 1-128 characters", ErrInvalidWebhook)
	}
	parsed, parseError := url.ParseRequestURI(strings.TrimSpace(p.URL))
	if parseError != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" || strings.Contains(p.URL, "#") {
		return fmt.Errorf("%w: webhook URL must be an HTTPS URL without credentials or fragments", ErrInvalidWebhook)
	}
	if address, addressError := netip.ParseAddr(parsed.Hostname()); addressError == nil && !netaddr.IsPublic(address) {
		return fmt.Errorf("%w: webhook URL must not target a private or reserved IP", ErrInvalidWebhook)
	}
	if len(p.URL) > 2048 {
		return fmt.Errorf("%w: webhook URL is too long", ErrInvalidWebhook)
	}
	if p.Secret != "" && len(p.Secret) < 16 {
		return fmt.Errorf("%w: webhook secret must be at least 16 characters", ErrInvalidWebhook)
	}
	if len(p.Events) == 0 {
		return fmt.Errorf("%w: at least one event type is required", ErrInvalidWebhook)
	}
	for _, event := range p.Events {
		if _, ok := AllowedEvents[event]; !ok {
			return fmt.Errorf("%w: event type is not allowed: %s", ErrInvalidWebhook, event)
		}
	}
	return nil
}

// DecodeEncryptionKey accepts either hex or standard/raw URL-safe base64 and
// requires exactly 32 bytes for AES-256-GCM. It keeps key parsing at the
// configuration boundary so storage never guesses at key formats.
func DecodeEncryptionKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("webhook encryption key is required")
	}
	if decoded, err := hex.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	for _, encoding := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding} {
		if decoded, err := encoding.DecodeString(value); err == nil && len(decoded) == 32 {
			return decoded, nil
		}
	}
	return nil, errors.New("webhook encryption key must encode exactly 32 bytes (hex or base64)")
}

func EncryptSecret(key []byte, secret string) (string, error) {
	if len(key) != 32 {
		return "", errors.New("webhook encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create webhook cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create webhook AEAD: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate webhook nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(secret), nil)
	encoded := append(nonce, ciphertext...)
	return "v1:" + base64.RawStdEncoding.EncodeToString(encoded), nil
}

func DecryptSecret(key []byte, value string) (string, error) {
	if len(key) != 32 {
		return "", errors.New("webhook encryption key must be 32 bytes")
	}
	if !strings.HasPrefix(value, "v1:") {
		return "", errors.New("webhook secret has an unsupported format")
	}
	encoded, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, "v1:"))
	if err != nil {
		return "", fmt.Errorf("decode webhook secret: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create webhook cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create webhook AEAD: %w", err)
	}
	if len(encoded) < gcm.NonceSize() {
		return "", errors.New("webhook secret is truncated")
	}
	plaintext, err := gcm.Open(nil, encoded[:gcm.NonceSize()], encoded[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("webhook secret authentication failed")
	}
	return string(plaintext), nil
}
