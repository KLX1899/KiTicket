// Package application orchestrates reservation use cases through explicit ports.
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/reservation/domain"
)

var ErrForbidden = errors.New("principal cannot perform reservation operation")

type IDGenerator interface{ New() (string, error) }

type LockCommand struct {
	ReservationID  string
	ScheduleID     string
	OwnerID        string
	SeatIDs        []string
	IdempotencyKey string
	RequestHash    string
	TTLMillis      int64
}

type LockStore interface {
	Acquire(context.Context, LockCommand) (domain.Lock, error)
	Release(context.Context, domain.ReleaseRequest) error
	Validate(context.Context, domain.ReleaseRequest) error
}

type AdmissionGate interface {
	Authorize(context.Context, string, string, string, string, string) error
}

type Booking struct {
	ID            string   `json:"booking_id"`
	ReservationID string   `json:"reservation_id"`
	ScheduleID    string   `json:"schedule_id"`
	OwnerID       string   `json:"owner_id"`
	SeatIDs       []string `json:"seat_ids"`
}

type BookingRepository interface {
	Finalize(context.Context, Booking) (Booking, error)
}

type Service struct {
	locks     LockStore
	bookings  BookingRepository
	ids       IDGenerator
	admission AdmissionGate
}

func (s *Service) WithAdmissionGate(gate AdmissionGate) *Service {
	s.admission = gate
	return s
}

func New(locks LockStore, bookings BookingRepository, ids IDGenerator) (*Service, error) {
	if locks == nil || ids == nil {
		return nil, errors.New("reservation service requires lock store and ID generator")
	}
	return &Service{locks: locks, bookings: bookings, ids: ids}, nil
}

func (s *Service) Acquire(ctx context.Context, principal auth.Principal, request domain.AcquireRequest) (domain.Lock, error) {
	if principal.Role != "buyer" || principal.UserID == "" || principal.UserID != request.OwnerID {
		return domain.Lock{}, ErrForbidden
	}
	seats, err := request.Validate()
	if err != nil {
		return domain.Lock{}, err
	}
	if s.admission != nil {
		if err := s.admission.Authorize(ctx, request.OwnerID, request.EventID, request.ScheduleID, request.IdempotencyKey, request.AdmissionToken); err != nil {
			return domain.Lock{}, err
		}
	}
	reservationID, err := s.ids.New()
	if err != nil {
		return domain.Lock{}, fmt.Errorf("generate reservation ID: %w", err)
	}
	hash := sha256.Sum256([]byte(strings.Join(append([]string{request.ScheduleID, request.OwnerID}, seats...), "\x00")))
	return s.locks.Acquire(ctx, LockCommand{
		ReservationID:  reservationID,
		ScheduleID:     request.ScheduleID,
		OwnerID:        request.OwnerID,
		SeatIDs:        seats,
		IdempotencyKey: request.IdempotencyKey,
		RequestHash:    hex.EncodeToString(hash[:]),
		TTLMillis:      request.TTL.Milliseconds(),
	})
}

func (s *Service) Release(ctx context.Context, principal auth.Principal, request domain.ReleaseRequest) error {
	if principal.Role != "buyer" || principal.UserID == "" || principal.UserID != request.OwnerID {
		return ErrForbidden
	}
	seats, err := request.Validate()
	if err != nil {
		return err
	}
	request.SeatIDs = seats
	return s.locks.Release(ctx, request)
}

func (s *Service) Finalize(ctx context.Context, principal auth.Principal, lock domain.ReleaseRequest) (Booking, error) {
	if principal.Role != "buyer" || principal.UserID == "" || principal.UserID != lock.OwnerID {
		return Booking{}, ErrForbidden
	}
	if s.bookings == nil {
		return Booking{}, errors.New("booking repository is unavailable")
	}
	seats, err := lock.Validate()
	if err != nil {
		return Booking{}, err
	}
	lock.SeatIDs = seats
	if err := s.locks.Validate(ctx, lock); err != nil {
		return Booking{}, err
	}
	bookingID, err := s.ids.New()
	if err != nil {
		return Booking{}, fmt.Errorf("generate booking ID: %w", err)
	}
	booking := Booking{ID: bookingID, ReservationID: lock.ReservationID, ScheduleID: lock.ScheduleID, OwnerID: lock.OwnerID, SeatIDs: seats}
	booking, err = s.bookings.Finalize(ctx, booking)
	if err != nil {
		return Booking{}, err
	}
	// Durable completion has committed. A failed cleanup is safe because the TTL will
	// expire and PostgreSQL's uniqueness constraint remains authoritative.
	_ = s.locks.Release(ctx, lock)
	return booking, nil
}
