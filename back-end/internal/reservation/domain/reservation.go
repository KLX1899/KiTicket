// Package domain contains reservation invariants independent of infrastructure.
package domain

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	MinimumTTL   = 30 * time.Second
	MaximumTTL   = 15 * time.Minute
	MaximumSeats = 20
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

var (
	ErrInvalidRequest      = errors.New("invalid reservation request")
	ErrIdempotencyMismatch = errors.New("idempotency key was reused with a different request")
	ErrLockLost            = errors.New("reservation lock is absent, expired, or owned by another command")
	ErrAlreadyBooked       = errors.New("one or more seats are already booked")
	ErrAdmissionRequired   = errors.New("a valid waiting-room admission is required")
)

type SeatConflictError struct{ SeatID string }

func (e *SeatConflictError) Error() string { return "seat " + e.SeatID + " is unavailable" }

type Lock struct {
	ReservationID string    `json:"reservation_id"`
	ScheduleID    string    `json:"schedule_id"`
	OwnerID       string    `json:"owner_id"`
	SeatIDs       []string  `json:"seat_ids"`
	Fence         int64     `json:"fence"`
	ExpiresAt     time.Time `json:"expires_at"`
	Replayed      bool      `json:"replayed"`
}

type AcquireRequest struct {
	EventID        string
	ScheduleID     string
	OwnerID        string
	SeatIDs        []string
	IdempotencyKey string
	TTL            time.Duration
	AdmissionToken string
}

func (r AcquireRequest) Validate() ([]string, error) {
	if !safeID.MatchString(r.ScheduleID) || !safeID.MatchString(r.OwnerID) {
		return nil, fmt.Errorf("%w: schedule and owner identifiers are required", ErrInvalidRequest)
	}
	if len(r.IdempotencyKey) < 8 || len(r.IdempotencyKey) > 128 || strings.TrimSpace(r.IdempotencyKey) != r.IdempotencyKey {
		return nil, fmt.Errorf("%w: idempotency key must contain 8-128 non-edge-whitespace characters", ErrInvalidRequest)
	}
	if r.TTL < MinimumTTL || r.TTL > MaximumTTL {
		return nil, fmt.Errorf("%w: TTL must be between %s and %s", ErrInvalidRequest, MinimumTTL, MaximumTTL)
	}
	if len(r.SeatIDs) == 0 || len(r.SeatIDs) > MaximumSeats {
		return nil, fmt.Errorf("%w: request must contain 1-%d seats", ErrInvalidRequest, MaximumSeats)
	}
	seats := append([]string(nil), r.SeatIDs...)
	for _, seat := range seats {
		if !safeID.MatchString(seat) {
			return nil, fmt.Errorf("%w: invalid seat identifier", ErrInvalidRequest)
		}
	}
	slices.Sort(seats)
	for index := 1; index < len(seats); index++ {
		if seats[index] == seats[index-1] {
			return nil, fmt.Errorf("%w: duplicate seat %s", ErrInvalidRequest, seats[index])
		}
	}
	return seats, nil
}

type ReleaseRequest struct {
	ReservationID string
	ScheduleID    string
	OwnerID       string
	SeatIDs       []string
	Fence         int64
}

func (r ReleaseRequest) Validate() ([]string, error) {
	if !safeID.MatchString(r.ReservationID) || !safeID.MatchString(r.ScheduleID) || !safeID.MatchString(r.OwnerID) || r.Fence <= 0 {
		return nil, fmt.Errorf("%w: invalid release identity", ErrInvalidRequest)
	}
	if len(r.SeatIDs) == 0 || len(r.SeatIDs) > MaximumSeats {
		return nil, fmt.Errorf("%w: invalid release seat count", ErrInvalidRequest)
	}
	seats := append([]string(nil), r.SeatIDs...)
	slices.Sort(seats)
	for index, seat := range seats {
		if !safeID.MatchString(seat) || (index > 0 && seats[index-1] == seat) {
			return nil, fmt.Errorf("%w: invalid release seats", ErrInvalidRequest)
		}
	}
	return seats, nil
}
