// Package ticket creates and authenticates non-personal QR ticket payloads.
package ticket

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidQR = errors.New("invalid ticket QR payload")

type IDGenerator interface{ New() (string, error) }

type Material struct {
	ID          string
	SeatID      string
	QRPayload   string
	TokenDigest []byte
	Signature   []byte
}

type Verified struct {
	TicketID    string
	TokenDigest []byte
}

type Issuer struct {
	secret []byte
	ids    IDGenerator
}

func New(secret []byte, ids IDGenerator) (*Issuer, error) {
	if len(secret) < 32 || ids == nil {
		return nil, errors.New("ticket issuer requires a 32-byte secret and ID generator")
	}
	return &Issuer{secret: append([]byte(nil), secret...), ids: ids}, nil
}

func (i *Issuer) Issue(seatID string) (Material, error) {
	ticketID, err := i.ids.New()
	if err != nil {
		return Material{}, fmt.Errorf("generate ticket ID: %w", err)
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return Material{}, fmt.Errorf("generate ticket token: %w", err)
	}
	tokenText := base64.RawURLEncoding.EncodeToString(token)
	unsigned := "kt1." + ticketID + "." + tokenText
	signature := i.sign(unsigned)
	digest := sha256.Sum256(token)
	return Material{
		ID: ticketID, SeatID: seatID,
		QRPayload:   unsigned + "." + base64.RawURLEncoding.EncodeToString(signature),
		TokenDigest: digest[:], Signature: signature,
	}, nil
}

func (i *Issuer) Verify(payload string) (Verified, error) {
	parts := strings.Split(payload, ".")
	if len(parts) != 4 || parts[0] != "kt1" || parts[1] == "" {
		return Verified{}, ErrInvalidQR
	}
	token, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(token) != 32 {
		return Verified{}, ErrInvalidQR
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || !hmac.Equal(provided, i.sign(strings.Join(parts[:3], "."))) {
		return Verified{}, ErrInvalidQR
	}
	digest := sha256.Sum256(token)
	return Verified{TicketID: parts[1], TokenDigest: digest[:]}, nil
}

func (i *Issuer) sign(payload string) []byte {
	mac := hmac.New(sha256.New, i.secret)
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}
