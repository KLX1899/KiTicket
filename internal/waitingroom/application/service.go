// Package application implements waiting-room join, status, and admission use cases.
package application

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/waitingroom/adapter/redisqueue"
	"github.com/KLX1899/KiTicket/internal/waitingroom/token"
)

var (
	ErrForbidden    = errors.New("waiting-room operation is forbidden")
	ErrInvalidEvent = errors.New("invalid waiting-room event")
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type IDGenerator interface{ New() (string, error) }

type Service struct {
	store        *redisqueue.Store
	signer       *token.Signer
	ids          IDGenerator
	queueTTL     time.Duration
	admissionTTL time.Duration
}

type JoinResponse struct {
	QueueToken string    `json:"queue_token"`
	State      string    `json:"state"`
	Position   int64     `json:"position"`
	ExpiresAt  time.Time `json:"expires_at"`
	Replayed   bool      `json:"replayed"`
}

type StatusResponse struct {
	State          string    `json:"state"`
	Position       int64     `json:"position"`
	ExpiresAt      time.Time `json:"expires_at"`
	AdmissionToken string    `json:"admission_token,omitempty"`
}

func New(store *redisqueue.Store, signer *token.Signer, ids IDGenerator, queueTTL, admissionTTL time.Duration) (*Service, error) {
	if store == nil || signer == nil || ids == nil || queueTTL <= admissionTTL || admissionTTL <= 0 {
		return nil, errors.New("waiting-room service requires store, signer, IDs, and valid TTLs")
	}
	return &Service{store: store, signer: signer, ids: ids, queueTTL: queueTTL, admissionTTL: admissionTTL}, nil
}

func (s *Service) Join(ctx context.Context, principal auth.Principal, eventID string) (JoinResponse, error) {
	if principal.Role != "buyer" || principal.UserID == "" {
		return JoinResponse{}, ErrForbidden
	}
	if !safeID.MatchString(eventID) {
		return JoinResponse{}, ErrInvalidEvent
	}
	tokenID, err := s.ids.New()
	if err != nil {
		return JoinResponse{}, err
	}
	joined, err := s.store.Join(ctx, eventID, principal.UserID, tokenID, s.queueTTL)
	if err != nil {
		return JoinResponse{}, err
	}
	queueToken, err := s.signer.Sign(token.KindQueue, joined.TokenID, principal.UserID, eventID, joined.ExpiresAt)
	if err != nil {
		return JoinResponse{}, err
	}
	status, err := s.store.Status(ctx, eventID, principal.UserID, joined.TokenID)
	if err != nil {
		return JoinResponse{}, err
	}
	return JoinResponse{QueueToken: queueToken, State: status.State, Position: status.Position, ExpiresAt: status.ExpiresAt, Replayed: joined.Replayed}, nil
}

func (s *Service) Status(ctx context.Context, principal auth.Principal, queueToken string) (StatusResponse, error) {
	if principal.Role != "buyer" || principal.UserID == "" {
		return StatusResponse{}, ErrForbidden
	}
	claims, err := s.signer.Verify(queueToken, token.KindQueue)
	if err != nil || claims.UserID != principal.UserID {
		return StatusResponse{}, redisqueue.ErrNotQueued
	}
	status, err := s.store.Status(ctx, claims.EventID, claims.UserID, claims.TokenID)
	if err != nil {
		return StatusResponse{}, err
	}
	response := StatusResponse{State: status.State, Position: status.Position, ExpiresAt: status.ExpiresAt}
	if status.State == "admitted" {
		response.AdmissionToken, err = s.signer.Sign(token.KindAdmission, claims.TokenID, claims.UserID, claims.EventID, status.ExpiresAt)
		if err != nil {
			return StatusResponse{}, err
		}
	}
	return response, nil
}

func (s *Service) Admit(ctx context.Context, eventID string, limit int) (int, error) {
	if !safeID.MatchString(eventID) || limit < 1 || limit > 1000 {
		return 0, ErrInvalidEvent
	}
	return s.store.Admit(ctx, eventID, limit, s.admissionTTL)
}
