package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSignerIssueAndVerify(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	signer, err := NewSigner([]byte(strings.Repeat("a", 32)), "kiticket", "kiticket-api")
	if err != nil {
		t.Fatal(err)
	}
	signer.now = func() time.Time { return now }
	token, expires, err := signer.Issue("user-1", "buyer", "token-1", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !expires.Equal(now.Add(15 * time.Minute)) {
		t.Fatalf("unexpected expiry: %s", expires)
	}
	principal, err := signer.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if principal.UserID != "user-1" || principal.Role != "buyer" {
		t.Fatalf("unexpected principal: %+v", principal)
	}

	signer.now = func() time.Time { return expires }
	if _, err := signer.Verify(token); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expected expired token, got %v", err)
	}
}

func TestSignerRejectsTampering(t *testing.T) {
	signer, err := NewSigner([]byte(strings.Repeat("b", 32)), "kiticket", "kiticket-api")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := signer.Issue("user-1", "buyer", "token-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	parts[1] = strings.Repeat("A", len(parts[1]))
	if _, err := signer.Verify(strings.Join(parts, ".")); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected invalid token, got %v", err)
	}
}
