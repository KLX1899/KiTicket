// Package auth signs and verifies the platform's short-lived access tokens.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid access token")
	ErrExpiredToken = errors.New("access token expired")
)

// Principal is the authenticated identity propagated to an application use case.
type Principal struct {
	UserID string
	Role   string
}

// Claims is deliberately small; sensitive profile data never enters access tokens.
type Claims struct {
	Subject   string `json:"sub"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"exp"`
	IssuedAt  int64  `json:"iat"`
	ID        string `json:"jti"`
	Issuer    string `json:"iss"`
	Audience  string `json:"aud"`
}

type Signer struct {
	secret   []byte
	issuer   string
	audience string
	now      func() time.Time
}

func NewSigner(secret []byte, issuer, audience string) (*Signer, error) {
	if len(secret) < 32 || issuer == "" || audience == "" {
		return nil, errors.New("JWT signer requires a 32-byte secret, issuer, and audience")
	}
	return &Signer{secret: append([]byte(nil), secret...), issuer: issuer, audience: audience, now: time.Now}, nil
}

// Issue creates an HS256 access token. The caller controls only identity values and TTL.
func (s *Signer) Issue(subject, role, tokenID string, ttl time.Duration) (string, time.Time, error) {
	if subject == "" || role == "" || tokenID == "" || ttl <= 0 {
		return "", time.Time{}, errors.New("invalid access token claims")
	}
	now := s.now().UTC()
	expires := now.Add(ttl)
	claims := Claims{Subject: subject, Role: role, ExpiresAt: expires.Unix(), IssuedAt: now.Unix(), ID: tokenID, Issuer: s.issuer, Audience: s.audience}
	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("encode JWT header: %w", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("encode JWT claims: %w", err)
	}
	unsigned := encode(header) + "." + encode(payload)
	return unsigned + "." + encode(s.mac(unsigned)), expires, nil
}

// Verify authenticates structure, algorithm, signature, issuer, audience, and lifetime.
func (s *Signer) Verify(token string) (Principal, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Principal{}, ErrInvalidToken
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if err := decodeJSON(parts[0], &header); err != nil || header.Algorithm != "HS256" || header.Type != "JWT" {
		return Principal{}, ErrInvalidToken
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(provided, s.mac(parts[0]+"."+parts[1])) {
		return Principal{}, ErrInvalidToken
	}
	var claims Claims
	if err := decodeJSON(parts[1], &claims); err != nil {
		return Principal{}, ErrInvalidToken
	}
	if claims.Subject == "" || claims.Role == "" || claims.ID == "" || claims.Issuer != s.issuer || claims.Audience != s.audience || claims.IssuedAt <= 0 || claims.ExpiresAt <= claims.IssuedAt {
		return Principal{}, ErrInvalidToken
	}
	if !s.now().UTC().Before(time.Unix(claims.ExpiresAt, 0)) {
		return Principal{}, ErrExpiredToken
	}
	return Principal{UserID: claims.Subject, Role: claims.Role}, nil
}

func (s *Signer) mac(value string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func encode(value []byte) string { return base64.RawURLEncoding.EncodeToString(value) }

func decodeJSON(value string, target any) error {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(decoded)))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}
