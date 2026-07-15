package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/KLX1899/KiTicket/internal/checkout/domain"
	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/ticket"
)

func TestCheckoutSuccessIssuesTicketsAfterBooking(t *testing.T) {
	repository := &fakeRepository{}
	reservations := &fakeReservations{}
	provider := &fakeProvider{outcome: PaymentConfirmed}
	service, err := New(repository, reservations, provider, fakeIssuer{}, &fakeIDs{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Checkout(context.Background(), auth.Principal{UserID: "buyer-1", Role: "buyer"}, checkoutCommand())
	if err != nil {
		t.Fatal(err)
	}
	if result.Order.State != domain.StateCompleted || result.Order.BookingID != "booking-1" || len(result.Tickets) != 2 {
		t.Fatalf("unexpected checkout result: %+v", result)
	}
	if reservations.finalized != 1 || provider.refunds != 0 {
		t.Fatalf("finalized=%d refunds=%d", reservations.finalized, provider.refunds)
	}
	want := []domain.State{domain.StatePaymentPending, domain.StatePaid, domain.StateBookingConfirmed, domain.StateTicketIssued, domain.StateCompleted}
	if fmt.Sprint(repository.transitions) != fmt.Sprint(want) {
		t.Fatalf("transitions=%v, want %v", repository.transitions, want)
	}
}

func TestCheckoutProviderFailureReleasesReservation(t *testing.T) {
	repository := &fakeRepository{}
	reservations := &fakeReservations{}
	provider := &fakeProvider{initiateErr: errors.New("gateway offline")}
	service, err := New(repository, reservations, provider, fakeIssuer{}, &fakeIDs{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Checkout(context.Background(), auth.Principal{UserID: "buyer-1", Role: "buyer"}, checkoutCommand())
	if !errors.Is(err, domain.ErrProviderUnavailable) {
		t.Fatalf("expected provider unavailable, got %v", err)
	}
	if reservations.released != 1 || repository.order.State != domain.StateFailed {
		t.Fatalf("released=%d state=%s", reservations.released, repository.order.State)
	}
}

func TestCheckoutBookingFailureRefundsAndReleases(t *testing.T) {
	repository := &fakeRepository{}
	reservations := &fakeReservations{finalizeErr: fmt.Errorf("%w: seat lost", ErrReservationRejected)}
	provider := &fakeProvider{outcome: PaymentConfirmed}
	service, err := New(repository, reservations, provider, fakeIssuer{}, &fakeIDs{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Checkout(context.Background(), auth.Principal{UserID: "buyer-1", Role: "buyer"}, checkoutCommand())
	if err == nil {
		t.Fatal("expected booking failure")
	}
	if result.Order.State != domain.StateRefunded || provider.refunds != 1 || reservations.released != 1 {
		t.Fatalf("state=%s refunds=%d releases=%d", result.Order.State, provider.refunds, reservations.released)
	}
}

func checkoutCommand() CheckoutCommand {
	return CheckoutCommand{
		ReservationID: "reservation-1", ReservationFence: 1, ScheduleID: "schedule-1",
		SeatIDs: []string{"seat-2", "seat-1"}, IdempotencyKey: "checkout-key-1", CorrelationID: "correlation-1",
	}
}

type fakeRepository struct {
	mu          sync.Mutex
	order       domain.Order
	transitions []domain.State
}

func (f *fakeRepository) Begin(_ context.Context, command BeginCommand) (domain.Result, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.order.ID != "" {
		return domain.Result{Order: f.order}, true, nil
	}
	f.order = command.Order
	f.order.AmountMinor = 5000
	f.order.Currency = "USD"
	f.order.State = domain.StatePending
	f.order.Version = 1
	return domain.Result{Order: f.order}, false, nil
}

func (f *fakeRepository) Transition(_ context.Context, order domain.Order, to domain.State, reason, _ string, payment *PaymentRecord) (domain.Order, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !order.CanTransition(to) {
		return domain.Order{}, domain.ErrInvalidTransition
	}
	order.State = to
	order.Version++
	order.FailureCode = reason
	if payment != nil {
		order.PaymentID = payment.ID
		order.PaymentProvider = payment.Provider
		order.ProviderReference = payment.ProviderReference
	}
	f.order = order
	f.transitions = append(f.transitions, to)
	return order, nil
}

func (f *fakeRepository) Complete(_ context.Context, order domain.Order, bookingID string, materials []ticket.Material, _ string) (domain.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	order.State = domain.StateTicketIssued
	order.Version++
	f.transitions = append(f.transitions, domain.StateTicketIssued)
	order.State = domain.StateCompleted
	order.Version++
	order.BookingID = bookingID
	f.transitions = append(f.transitions, domain.StateCompleted)
	f.order = order
	tickets := make([]domain.Ticket, 0, len(materials))
	for _, material := range materials {
		tickets = append(tickets, domain.Ticket{ID: material.ID, SeatID: material.SeatID, QRPayload: material.QRPayload})
	}
	return domain.Result{Order: order, Tickets: tickets}, nil
}

func (*fakeRepository) Ticket(context.Context, string, []byte) (domain.Ticket, bool, error) {
	return domain.Ticket{}, false, nil
}

func (*fakeRepository) RevokeTicket(context.Context, string, string) error { return nil }

type fakeReservations struct {
	finalized   int
	released    int
	finalizeErr error
}

func (f *fakeReservations) Finalize(context.Context, Reservation) (Booking, error) {
	f.finalized++
	if f.finalizeErr != nil {
		return Booking{}, f.finalizeErr
	}
	return Booking{ID: "booking-1"}, nil
}

func (f *fakeReservations) Release(context.Context, Reservation) error {
	f.released++
	return nil
}

type fakeProvider struct {
	initiateErr error
	outcome     PaymentOutcome
	refundErr   error
	refunds     int
}

func (f *fakeProvider) Initiate(context.Context, string, int64, string) (Payment, error) {
	if f.initiateErr != nil {
		return Payment{}, f.initiateErr
	}
	return Payment{Provider: "fake", Reference: "payment-1"}, nil
}

func (f *fakeProvider) Confirm(context.Context, Payment) (PaymentOutcome, error) {
	return f.outcome, nil
}

func (f *fakeProvider) Refund(context.Context, Payment) error {
	f.refunds++
	return f.refundErr
}

type fakeIssuer struct{}

func (fakeIssuer) Issue(seatID string) (ticket.Material, error) {
	return ticket.Material{ID: "ticket-" + seatID, SeatID: seatID, QRPayload: "qr-" + seatID}, nil
}

func (fakeIssuer) Verify(string) (ticket.Verified, error) { return ticket.Verified{}, nil }

type fakeIDs struct {
	mu    sync.Mutex
	value int
}

func (f *fakeIDs) New() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.value++
	return fmt.Sprintf("generated-%d", f.value), nil
}

var _ Clock = fixedClock{}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Unix(0, 0) }
