package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	TokenAccess = "access"
	TokenIngest = "ingest"
)

const (
	jwtIssuer   = "tracepilot"
	jwtAudience = "tracepilot"
	// MinJWTSecretBytes is the shortest signing secret accepted; HS256 wants at least 256 bits.
	MinJWTSecretBytes = 32
	// AccessTokenTTL is how long a console access token lives; the refresh cookie renews it.
	AccessTokenTTL = 15 * time.Minute
	// clockSkew forgives small differences between the servers that sign and check tokens.
	clockSkew = 30 * time.Second
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

// Claims are what a TracePilot JWT states. Subject is the user ID of an access token, and the
// project ID of an ingest token; ID is the ingest token's row, so it can be revoked.
type Claims struct {
	Kind         string
	Subject      string
	ID           string
	IssuedAt     time.Time
	ExpiresAt    time.Time
	Issuer       string
	Audience     string
	TokenVersion int64
}

type jwtPayload struct {
	Issuer       string `json:"iss"`
	Audience     string `json:"aud"`
	Subject      string `json:"sub"`
	ID           string `json:"jti"`
	Kind         string `json:"tp"`
	IssuedAt     int64  `json:"iat"`
	ExpiresAt    int64  `json:"exp"`
	TokenVersion int64  `json:"tp_ver,omitempty"`
}

var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

// SignJWT signs claims with HMAC-SHA256.
func SignJWT(secret []byte, claims Claims) (string, error) {
	if len(secret) < MinJWTSecretBytes {
		return "", errors.New("JWT secret is too short")
	}
	issuer := claims.Issuer
	if issuer == "" {
		issuer = jwtIssuer
	}
	audience := claims.Audience
	if audience == "" {
		audience = jwtAudience
	}
	body, marshalError := json.Marshal(jwtPayload{
		Issuer: issuer, Audience: audience, Subject: claims.Subject, ID: claims.ID, Kind: claims.Kind,
		IssuedAt: claims.IssuedAt.Unix(), ExpiresAt: claims.ExpiresAt.Unix(),
		TokenVersion: claims.TokenVersion,
	})
	if marshalError != nil {
		return "", fmt.Errorf("encode JWT: %w", marshalError)
	}
	signingInput := jwtHeader + "." + base64.RawURLEncoding.EncodeToString(body)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sign(secret, signingInput)), nil
}

func sign(secret []byte, signingInput string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	return mac.Sum(nil)
}

// VerifyJWT checks the signature, issuer, audience, kind and expiry using defaults.
func VerifyJWT(secret []byte, token, wantKind string, now time.Time) (Claims, error) {
	return VerifyJWTWithIssuerAudience(secret, token, wantKind, jwtIssuer, jwtAudience, now)
}

// VerifyJWTWithIssuerAudience checks the signature, custom issuer and audience, kind and expiry.
func VerifyJWTWithIssuerAudience(secret []byte, token, wantKind, wantIssuer, wantAudience string, now time.Time) (Claims, error) {
	if wantIssuer == "" {
		wantIssuer = jwtIssuer
	}
	if wantAudience == "" {
		wantAudience = jwtAudience
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != jwtHeader {
		return Claims{}, ErrInvalidToken
	}
	signature, signatureError := base64.RawURLEncoding.DecodeString(parts[2])
	if signatureError != nil || subtle.ConstantTimeCompare(signature, sign(secret, parts[0]+"."+parts[1])) != 1 {
		return Claims{}, ErrInvalidToken
	}
	body, bodyError := base64.RawURLEncoding.DecodeString(parts[1])
	if bodyError != nil {
		return Claims{}, ErrInvalidToken
	}
	var payload jwtPayload
	if unmarshalError := json.Unmarshal(body, &payload); unmarshalError != nil {
		return Claims{}, ErrInvalidToken
	}
	if payload.Issuer != wantIssuer || payload.Audience != wantAudience || payload.Kind != wantKind || payload.Subject == "" || payload.ExpiresAt == 0 {
		return Claims{}, ErrInvalidToken
	}
	claims := Claims{
		Kind: payload.Kind, Subject: payload.Subject, ID: payload.ID,
		Issuer: payload.Issuer, Audience: payload.Audience,
		IssuedAt: time.Unix(payload.IssuedAt, 0).UTC(), ExpiresAt: time.Unix(payload.ExpiresAt, 0).UTC(),
		TokenVersion: payload.TokenVersion,
	}
	if !now.Before(claims.ExpiresAt.Add(clockSkew)) {
		return Claims{}, ErrExpiredToken
	}
	if claims.IssuedAt.After(now.Add(clockSkew)) {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}

func NewJWTSecret() ([]byte, error) {
	secret := make([]byte, MinJWTSecretBytes)
	if _, readError := rand.Read(secret); readError != nil {
		return nil, fmt.Errorf("generate JWT secret: %w", readError)
	}
	return secret, nil
}
