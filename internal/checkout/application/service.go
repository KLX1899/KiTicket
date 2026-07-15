// Package application orchestrates payment, durable booking, compensation, and ticket issuance.
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/KLX1899/KiTicket/internal/checkout/domain"
	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/ticket"
)

type BeginCommand struct {
	Order              domain.Order
	IdempotencyKeyHash string
	RequestHash        string
	IdempotencyExpiry  time.Time
	CorrelationID      string
}

type PaymentRecord struct {
	ID                string
	Provider          string
	ProviderReference string
	Status            string
}

type Repository interface {
	Begin(context.Context, BeginCommand) (domain.Result, bool, error)
	Transition(context.Context, domain.Order, domain.State, string, string, *PaymentRecord) (domain.Order, error)
	Complete(context.Context, domain.Order, string, []ticket.Material, string) (domain.Result, error)
	Ticket(context.Context, string, []byte) (domain.Ticket, bool, error)
	RevokeTicket(context.Context, string, string) error
}

type Reservation struct {
	ID         string
	BuyerID    string
	ScheduleID string
	SeatIDs    []string
	Fence      int64
}

type Booking struct {
	ID string `json:"booking_id"`
}

type ReservationPort interface {
	Finalize(context.Context, Reservation) (Booking, error)
	Release(context.Context, Reservation) error
}

var ErrReservationRejected = errors.New("reservation finalization was definitively rejected")

type Payment struct {
	Provider  string
	Reference string
}

type PaymentOutcome string

const (
	PaymentConfirmed PaymentOutcome = "confirmed"
	PaymentDeclined  PaymentOutcome = "declined"
	PaymentUncertain PaymentOutcome = "uncertain"
)

type PaymentProvider interface {
	Initiate(context.Context, string, int64, string) (Payment, error)
	Confirm(context.Context, Payment) (PaymentOutcome, error)
	Refund(context.Context, Payment) error
}

type TicketIssuer interface {
	Issue(string) (ticket.Material, error)
	Verify(string) (ticket.Verified, error)
}

type IDGenerator interface{ New() (string, error) }

type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	repository   Repository
	reservations ReservationPort
	payments     PaymentProvider
	tickets      TicketIssuer
	ids          IDGenerator
	clock        Clock
}

type CheckoutCommand struct {
	ReservationID    string
	ReservationFence int64
	ScheduleID       string
	SeatIDs          []string
	IdempotencyKey   string
	CorrelationID    string
}

type TicketVerification struct {
	Valid     bool       `json:"valid"`
	TicketID  string     `json:"ticket_id,omitempty"`
	SeatID    string     `json:"seat_id,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

func New(repository Repository, reservations ReservationPort, payments PaymentProvider, tickets TicketIssuer, ids IDGenerator) (*Service, error) {
	if repository == nil || reservations == nil || payments == nil || tickets == nil || ids == nil {
		return nil, errors.New("checkout service dependencies are required")
	}
	return &Service{repository: repository, reservations: reservations, payments: payments, tickets: tickets, ids: ids, clock: systemClock{}}, nil
}

func (s *Service) Checkout(ctx context.Context, principal auth.Principal, command CheckoutCommand) (domain.Result, error) {
	if principal.Role != "buyer" || principal.UserID == "" {
		return domain.Result{}, domain.ErrForbidden
	}
	if len(command.IdempotencyKey) < 8 || len(command.IdempotencyKey) > 128 || command.CorrelationID == "" {
		return domain.Result{}, domain.ErrInvalidOrder
	}
	seats := append([]string(nil), command.SeatIDs...)
	slices.Sort(seats)
	orderID, err := s.ids.New()
	if err != nil {
		return domain.Result{}, fmt.Errorf("generate order ID: %w", err)
	}
	order := domain.Order{
		ID: orderID, BuyerID: principal.UserID, ReservationID: command.ReservationID,
		ReservationFence: command.ReservationFence, ScheduleID: command.ScheduleID, SeatIDs: seats,
		State: domain.StatePending, Version: 1,
	}
	if err := order.ValidateInput(); err != nil {
		return domain.Result{}, err
	}
	keyHash := sha256.Sum256([]byte(command.IdempotencyKey))
	requestHash := sha256.Sum256([]byte(strings.Join(append([]string{
		principal.UserID, command.ReservationID, fmt.Sprint(command.ReservationFence), command.ScheduleID,
	}, seats...), "\x00")))
	result, _, err := s.repository.Begin(ctx, BeginCommand{
		Order: order, IdempotencyKeyHash: hex.EncodeToString(keyHash[:]), RequestHash: hex.EncodeToString(requestHash[:]),
		IdempotencyExpiry: s.clock.Now().Add(24 * time.Hour), CorrelationID: command.CorrelationID,
	})
	if err != nil {
		return domain.Result{}, err
	}
	return s.resume(ctx, result, command.CorrelationID)
}

func (s *Service) resume(ctx context.Context, result domain.Result, correlationID string) (domain.Result, error) {
	order := result.Order
	reservation := Reservation{ID: order.ReservationID, BuyerID: order.BuyerID, ScheduleID: order.ScheduleID, SeatIDs: order.SeatIDs, Fence: order.ReservationFence}
	for {
		switch order.State {
		case domain.StatePending:
			payment, err := s.payments.Initiate(ctx, order.ID, order.AmountMinor, order.Currency)
			if err != nil {
				s.compensateBeforePayment(ctx, order, reservation, "provider_unavailable", correlationID)
				return domain.Result{}, fmt.Errorf("%w: %v", domain.ErrProviderUnavailable, err)
			}
			paymentID, err := s.ids.New()
			if err != nil {
				return domain.Result{}, err
			}
			order, err = s.repository.Transition(ctx, order, domain.StatePaymentPending, "", correlationID, &PaymentRecord{
				ID: paymentID, Provider: payment.Provider, ProviderReference: payment.Reference, Status: "initiated",
			})
			if err != nil {
				return domain.Result{}, err
			}
		case domain.StatePaymentPending:
			payment := Payment{Provider: order.PaymentProvider, Reference: order.ProviderReference}
			outcome, err := s.payments.Confirm(ctx, payment)
			if err != nil || outcome == PaymentUncertain {
				order, _ = s.repository.Transition(ctx, order, domain.StatePaymentUncertain, "payment_outcome_unknown", correlationID, &PaymentRecord{ID: order.PaymentID, Provider: payment.Provider, ProviderReference: payment.Reference, Status: "uncertain"})
				return domain.Result{Order: order}, domain.ErrPaymentUncertain
			}
			if outcome == PaymentDeclined {
				s.compensateBeforePayment(ctx, order, reservation, "payment_declined", correlationID)
				return domain.Result{}, domain.ErrPaymentFailed
			}
			order, err = s.repository.Transition(ctx, order, domain.StatePaid, "", correlationID, &PaymentRecord{ID: order.PaymentID, Provider: payment.Provider, ProviderReference: payment.Reference, Status: "confirmed"})
			if err != nil {
				return domain.Result{}, err
			}
		case domain.StatePaid:
			booking, err := s.reservations.Finalize(ctx, reservation)
			if err != nil {
				if !errors.Is(err, ErrReservationRejected) {
					// The booking outcome is unknown (network/5xx/decoding failure). Keep the
					// paid saga recoverable and retry the idempotent finalize call later.
					return domain.Result{Order: order}, fmt.Errorf("booking outcome uncertain: %w", err)
				}
				payment := Payment{Provider: order.PaymentProvider, Reference: order.ProviderReference}
				refundErr := s.payments.Refund(ctx, payment)
				order, _ = s.repository.Transition(ctx, order, domain.StateRefundPending, "booking_failed", correlationID, nil)
				if refundErr == nil {
					order, _ = s.repository.Transition(ctx, order, domain.StateRefunded, "booking_failed", correlationID, &PaymentRecord{ID: order.PaymentID, Provider: payment.Provider, ProviderReference: payment.Reference, Status: "refunded"})
				}
				_ = s.reservations.Release(ctx, reservation)
				return domain.Result{Order: order}, fmt.Errorf("finalize booking: %w", err)
			}
			order.BookingID = booking.ID
			order, err = s.repository.Transition(ctx, order, domain.StateBookingConfirmed, "", correlationID, nil)
			if err != nil {
				return domain.Result{}, err
			}
		case domain.StateBookingConfirmed:
			materials := make([]ticket.Material, 0, len(order.SeatIDs))
			for _, seatID := range order.SeatIDs {
				material, err := s.tickets.Issue(seatID)
				if err != nil {
					return domain.Result{Order: order}, fmt.Errorf("issue ticket: %w", err)
				}
				materials = append(materials, material)
			}
			return s.repository.Complete(ctx, order, order.BookingID, materials, correlationID)
		case domain.StateCompleted:
			return result, nil
		case domain.StatePaymentUncertain:
			return domain.Result{Order: order}, domain.ErrPaymentUncertain
		case domain.StateFailed, domain.StateCancelled:
			return domain.Result{Order: order}, domain.ErrPaymentFailed
		case domain.StateRefundPending, domain.StateRefunded:
			return domain.Result{Order: order}, errors.New("checkout was compensated after booking failure")
		default:
			return domain.Result{Order: order}, domain.ErrInvalidTransition
		}
	}
}

func (s *Service) VerifyTicket(ctx context.Context, qrPayload string) (TicketVerification, error) {
	verified, err := s.tickets.Verify(qrPayload)
	if err != nil {
		return TicketVerification{Valid: false}, nil
	}
	record, found, err := s.repository.Ticket(ctx, verified.TicketID, verified.TokenDigest)
	if err != nil {
		return TicketVerification{}, err
	}
	if !found {
		return TicketVerification{Valid: false}, nil
	}
	return TicketVerification{Valid: record.RevokedAt == nil, TicketID: record.ID, SeatID: record.SeatID, RevokedAt: record.RevokedAt}, nil
}

func (s *Service) RevokeTicket(ctx context.Context, principal auth.Principal, ticketID, reason string) error {
	if principal.Role != "admin" || principal.UserID == "" || strings.TrimSpace(reason) == "" {
		return domain.ErrForbidden
	}
	return s.repository.RevokeTicket(ctx, ticketID, strings.TrimSpace(reason))
}

func (s *Service) compensateBeforePayment(ctx context.Context, order domain.Order, reservation Reservation, reason, correlationID string) {
	_, _ = s.repository.Transition(ctx, order, domain.StateFailed, reason, correlationID, nil)
	_ = s.reservations.Release(ctx, reservation)
}
