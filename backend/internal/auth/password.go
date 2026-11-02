package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	passwordAlgorithm  = "pbkdf2-sha256"
	passwordIterations = 600_000
	passwordSaltBytes  = 16
	passwordKeyBytes   = 32
	// MinPasswordLength follows NIST SP 800-63B: length matters more than composition.
	MinPasswordLength = 12
	// MaxPasswordLength bounds hashing work; PBKDF2 cost grows with input only up to the block size,
	// but an unbounded body is still an easy way to waste memory.
	MaxPasswordLength = 256
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrWeakPassword       = fmt.Errorf("password must be %d-%d characters", MinPasswordLength, MaxPasswordLength)
)

// ValidatePassword checks length and refuses a password that is only a
// repeated character; it does not impose composition rules.
func ValidatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < MinPasswordLength || length > MaxPasswordLength {
		return ErrWeakPassword
	}
	first, _ := utf8.DecodeRuneInString(password)
	if strings.Count(password, string(first)) == length {
		return fmt.Errorf("%w and must not repeat one character", ErrWeakPassword)
	}
	return nil
}

// HashPassword derives a salted PBKDF2-HMAC-SHA256 hash in the form
// "pbkdf2-sha256$iterations$salt$key" so the cost can be raised later.
func HashPassword(password string) (string, error) {
	salt := make([]byte, passwordSaltBytes)
	if _, readError := rand.Read(salt); readError != nil {
		return "", fmt.Errorf("generate password salt: %w", readError)
	}
	return derivePassword(password, salt, passwordIterations)
}

func derivePassword(password string, salt []byte, iterations int) (string, error) {
	key, deriveError := pbkdf2.Key(sha256.New, password, salt, iterations, passwordKeyBytes)
	if deriveError != nil {
		return "", fmt.Errorf("derive password hash: %w", deriveError)
	}
	encoding := base64.RawStdEncoding
	return strings.Join([]string{passwordAlgorithm, strconv.Itoa(iterations), encoding.EncodeToString(salt), encoding.EncodeToString(key)}, "$"), nil
}

// VerifyPassword compares in constant time. A malformed stored hash never matches.
func VerifyPassword(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != passwordAlgorithm {
		return false
	}
	iterations, parseError := strconv.Atoi(parts[1])
	if parseError != nil || iterations < 1 || iterations > 10*passwordIterations {
		return false
	}
	encoding := base64.RawStdEncoding
	salt, saltError := encoding.DecodeString(parts[2])
	want, keyError := encoding.DecodeString(parts[3])
	if saltError != nil || keyError != nil || len(want) == 0 {
		return false
	}
	got, deriveError := pbkdf2.Key(sha256.New, password, salt, iterations, len(want))
	return deriveError == nil && subtle.ConstantTimeCompare(got, want) == 1
}

// DummyPasswordHash lets login spend the same time for an unknown email as for a
// wrong password, so response time does not reveal which accounts exist. It is derived on first
// use, so a process that never signs anyone in does not pay for it at start-up.
var DummyPasswordHash = sync.OnceValue(func() string {
	hash, hashError := derivePassword("tracepilot-dummy-password", make([]byte, passwordSaltBytes), passwordIterations)
	if hashError != nil {
		panic(hashError)
	}
	return hash
})

// NewTemporaryPassword returns a random password for an administrator to hand over once.
func NewTemporaryPassword() (string, error) {
	secret := make([]byte, 18)
	if _, readError := rand.Read(secret); readError != nil {
		return "", fmt.Errorf("generate temporary password: %w", readError)
	}
	return base64.RawURLEncoding.EncodeToString(secret), nil
}
