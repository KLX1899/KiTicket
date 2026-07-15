// Package token signs queue and one-use admission credentials without personal data beyond opaque IDs.
package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid waiting-room token")

const (
	KindQueue     = "queue"
	KindAdmission = "admission"
)

type Claims struct {
	Kind      string `json:"kind"`
	TokenID   string `json:"tid"`
	UserID    string `json:"sub"`
	EventID   string `json:"event_id"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	KeyID     string `json:"kid"`
}

type Signer struct {
	secret []byte
	keyID  string
	now    func() time.Time
}

func New(secret []byte, keyID string) (*Signer, error) {
	if len(secret) < 32 || keyID == "" {
		return nil, errors.New("waiting-room signer requires a 32-byte secret and key ID")
	}
	return &Signer{secret: append([]byte(nil), secret...), keyID: keyID, now: time.Now}, nil
}

func (s *Signer) Sign(kind, tokenID, userID, eventID string, expiresAt time.Time) (string, error) {
	if kind != KindQueue && kind != KindAdmission || tokenID == "" || userID == "" || eventID == "" {
		return "", ErrInvalid
	}
	now := s.now().UTC()
	if !expiresAt.After(now) {
		return "", ErrInvalid
	}
	claims := Claims{Kind: kind, TokenID: tokenID, UserID: userID, EventID: eventID, IssuedAt: now.Unix(), ExpiresAt: expiresAt.Unix(), KeyID: s.keyID}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := s.mac(encoded)
	return encoded + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (s *Signer) Verify(raw, kind string) (Claims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return Claims{}, ErrInvalid
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(provided, s.mac(parts[0])) {
		return Claims{}, ErrInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrInvalid
	}
	var claims Claims
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil || claims.Kind != kind || claims.KeyID != s.keyID || claims.TokenID == "" || claims.UserID == "" || claims.EventID == "" || claims.IssuedAt <= 0 || claims.ExpiresAt <= claims.IssuedAt {
		return Claims{}, ErrInvalid
	}
	if !s.now().UTC().Before(time.Unix(claims.ExpiresAt, 0)) {
		return Claims{}, ErrInvalid
	}
	return claims, nil
}

func (s *Signer) mac(value string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
