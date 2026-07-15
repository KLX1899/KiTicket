// Package domain defines the persisted checkout saga state machine.
package domain

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

type State string

const (
	StatePending          State = "pending"
	StatePaymentPending   State = "payment_pending"
	StatePaymentUncertain State = "payment_uncertain"
	StatePaid             State = "paid"
	StateBookingConfirmed State = "booking_confirmed"
	StateTicketIssued     State = "ticket_issued"
	StateCompleted        State = "completed"
	StateCancelled        State = "cancelled"
	StateFailed           State = "failed"
	StateRefundPending    State = "refund_pending"
	StateRefunded         State = "refunded"
)

var (
	ErrInvalidOrder        = errors.New("invalid checkout order")
	ErrInvalidTransition   = errors.New("invalid checkout state transition")
	ErrTransitionConflict  = errors.New("checkout state changed concurrently")
	ErrIdempotencyMismatch = errors.New("checkout idempotency key was reused with different input")
	ErrPaymentFailed       = errors.New("payment failed")
	ErrPaymentUncertain    = errors.New("payment outcome is uncertain")
	ErrProviderUnavailable = errors.New("payment provider is unavailable")
	ErrNotFound            = errors.New("checkout order not found")
	ErrForbidden           = errors.New("checkout operation is forbidden")
)

var transitions = map[State]map[State]struct{}{
	StatePending:          {StatePaymentPending: {}, StateCancelled: {}, StateFailed: {}},
	StatePaymentPending:   {StatePaid: {}, StatePaymentUncertain: {}, StateFailed: {}, StateCancelled: {}},
	StatePaymentUncertain: {StatePaid: {}, StateFailed: {}, StateRefundPending: {}},
	StatePaid:             {StateBookingConfirmed: {}, StateRefundPending: {}},
	StateBookingConfirmed: {StateTicketIssued: {}},
	StateTicketIssued:     {StateCompleted: {}},
	StateRefundPending:    {StateRefunded: {}},
}

type Order struct {
	ID                string    `json:"id"`
	BuyerID           string    `json:"buyer_id"`
	ReservationID     string    `json:"reservation_id"`
	ReservationFence  int64     `json:"reservation_fence"`
	ScheduleID        string    `json:"schedule_id"`
	SeatIDs           []string  `json:"seat_ids"`
	State             State     `json:"state"`
	AmountMinor       int64     `json:"amount_minor"`
	Currency          string    `json:"currency"`
	Version           int64     `json:"version"`
	PaymentID         string    `json:"-"`
	PaymentProvider   string    `json:"-"`
	ProviderReference string    `json:"-"`
	BookingID         string    `json:"booking_id,omitempty"`
	FailureCode       string    `json:"failure_code,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (o Order) CanTransition(to State) bool {
	_, ok := transitions[o.State][to]
	return ok
}

func (o Order) ValidateInput() error {
	if o.ID == "" || o.BuyerID == "" || o.ReservationID == "" || o.ScheduleID == "" || o.ReservationFence <= 0 || len(o.SeatIDs) == 0 || len(o.SeatIDs) > 20 {
		return ErrInvalidOrder
	}
	seats := append([]string(nil), o.SeatIDs...)
	slices.Sort(seats)
	for index, seat := range seats {
		if seat == "" || index > 0 && seats[index-1] == seat {
			return fmt.Errorf("%w: invalid or duplicate seat", ErrInvalidOrder)
		}
	}
	return nil
}

func (o Order) Terminal() bool {
	return o.State == StateCompleted || o.State == StateCancelled || o.State == StateFailed || o.State == StateRefunded
}

type Ticket struct {
	ID        string     `json:"id"`
	SeatID    string     `json:"seat_id"`
	QRPayload string     `json:"qr_payload"`
	IssuedAt  time.Time  `json:"issued_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type Result struct {
	Order   Order    `json:"order"`
	Tickets []Ticket `json:"tickets"`
}
