// Package payment provides a deterministic local provider without accepting card data.
package payment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"

	"github.com/KLX1899/KiTicket/internal/checkout/application"
)

type Local struct {
	mu       sync.Mutex
	states   map[string]application.PaymentOutcome
	refunded map[string]bool
}

func NewLocal() *Local {
	return &Local{states: make(map[string]application.PaymentOutcome), refunded: make(map[string]bool)}
}

func (l *Local) Initiate(_ context.Context, orderID string, amountMinor int64, currency string) (application.Payment, error) {
	if orderID == "" || amountMinor < 0 || currency == "" {
		return application.Payment{}, errors.New("invalid local payment request")
	}
	hash := sha256.Sum256([]byte(orderID))
	reference := "local_" + hex.EncodeToString(hash[:12])
	l.mu.Lock()
	if _, exists := l.states[reference]; !exists {
		l.states[reference] = application.PaymentConfirmed
	}
	l.mu.Unlock()
	return application.Payment{Provider: "local", Reference: reference}, nil
}

func (l *Local) Confirm(_ context.Context, payment application.Payment) (application.PaymentOutcome, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	outcome, exists := l.states[payment.Reference]
	if !exists {
		return application.PaymentUncertain, errors.New("unknown local payment reference")
	}
	return outcome, nil
}

func (l *Local) Refund(_ context.Context, payment application.Payment) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.states[payment.Reference]; !exists {
		return errors.New("unknown local payment reference")
	}
	l.refunded[payment.Reference] = true
	return nil
}
