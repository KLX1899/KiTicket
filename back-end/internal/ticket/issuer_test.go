package ticket

import (
	"errors"
	"strings"
	"testing"
)

type fixedID string

func (f fixedID) New() (string, error) { return string(f), nil }

func TestIssueVerifyAndTamper(t *testing.T) {
	issuer, err := New([]byte(strings.Repeat("q", 32)), fixedID("ticket-1"))
	if err != nil {
		t.Fatal(err)
	}
	material, err := issuer.Issue("seat-1")
	if err != nil {
		t.Fatal(err)
	}
	verified, err := issuer.Verify(material.QRPayload)
	if err != nil {
		t.Fatal(err)
	}
	if verified.TicketID != material.ID || !strings.HasPrefix(material.QRPayload, "kt1.ticket-1.") {
		t.Fatalf("unexpected ticket verification: %+v", verified)
	}
	tampered := material.QRPayload[:len(material.QRPayload)-1] + "A"
	if _, err := issuer.Verify(tampered); !errors.Is(err, ErrInvalidQR) {
		t.Fatalf("expected invalid QR, got %v", err)
	}
}
