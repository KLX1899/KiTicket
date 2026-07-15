package token

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTokenBindingExpiryAndTampering(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	signer, err := New([]byte(strings.Repeat("w", 32)), "key-1")
	if err != nil {
		t.Fatal(err)
	}
	signer.now = func() time.Time { return now }
	raw, err := signer.Sign(KindAdmission, "token-1", "user-1", "event-1", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := signer.Verify(raw, KindAdmission)
	if err != nil || claims.UserID != "user-1" || claims.EventID != "event-1" {
		t.Fatalf("unexpected claims=%+v err=%v", claims, err)
	}
	if _, err := signer.Verify(raw, KindQueue); !errors.Is(err, ErrInvalid) {
		t.Fatalf("admission token accepted as queue token: %v", err)
	}
	signer.now = func() time.Time { return now.Add(time.Minute) }
	if _, err := signer.Verify(raw, KindAdmission); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expired token accepted: %v", err)
	}
}
