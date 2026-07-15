// Package httpapi exposes reservation use cases through authenticated JSON endpoints.
package httpapi

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/platform/httpx"
	"github.com/KLX1899/KiTicket/internal/reservation/application"
	"github.com/KLX1899/KiTicket/internal/reservation/domain"
)

type Handler struct {
	service        *application.Service
	signer         *auth.Signer
	internalSecret []byte
}

func New(service *application.Service, signer *auth.Signer, internalSecret []byte) (*Handler, error) {
	if service == nil || signer == nil || len(internalSecret) < 32 {
		return nil, errors.New("reservation HTTP handler requires service, token signer, and internal secret")
	}
	return &Handler{service: service, signer: signer, internalSecret: append([]byte(nil), internalSecret...)}, nil
}

func (h *Handler) Register(mux *http.ServeMux) {
	buyerOnly := httpx.Authenticate(h.signer, "buyer")
	mux.Handle("POST /v1/reservation-locks", buyerOnly(http.HandlerFunc(h.acquire)))
	mux.Handle("POST /v1/reservation-locks/{reservation_id}/release", buyerOnly(http.HandlerFunc(h.release)))
	mux.HandleFunc("POST /internal/v1/reservation-locks/{reservation_id}/finalize", h.finalize)
	mux.HandleFunc("POST /internal/v1/reservation-locks/{reservation_id}/release", h.internalRelease)
}

func (h *Handler) internalRelease(w http.ResponseWriter, r *http.Request) {
	if !h.authenticateInternal(r) {
		httpx.WriteError(w, r, &httpx.Error{Status: http.StatusUnauthorized, Code: "invalid_service_credential", Message: "internal service authentication failed"})
		return
	}
	var body struct {
		BuyerID    string   `json:"buyer_id"`
		ScheduleID string   `json:"schedule_id"`
		SeatIDs    []string `json:"seat_ids"`
		Fence      int64    `json:"fence"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	err := h.service.Release(r.Context(), auth.Principal{UserID: body.BuyerID, Role: "buyer"}, domain.ReleaseRequest{
		ReservationID: r.PathValue("reservation_id"), ScheduleID: body.ScheduleID,
		OwnerID: body.BuyerID, SeatIDs: body.SeatIDs, Fence: body.Fence,
	})
	if errors.Is(err, domain.ErrLockLost) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) finalize(w http.ResponseWriter, r *http.Request) {
	if !h.authenticateInternal(r) {
		httpx.WriteError(w, r, &httpx.Error{Status: http.StatusUnauthorized, Code: "invalid_service_credential", Message: "internal service authentication failed"})
		return
	}
	var body struct {
		BuyerID    string   `json:"buyer_id"`
		ScheduleID string   `json:"schedule_id"`
		SeatIDs    []string `json:"seat_ids"`
		Fence      int64    `json:"fence"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	booking, err := h.service.Finalize(r.Context(), auth.Principal{UserID: body.BuyerID, Role: "buyer"}, domain.ReleaseRequest{
		ReservationID: r.PathValue("reservation_id"), ScheduleID: body.ScheduleID,
		OwnerID: body.BuyerID, SeatIDs: body.SeatIDs, Fence: body.Fence,
	})
	if err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, booking)
}

func (h *Handler) authenticateInternal(r *http.Request) bool {
	provided := []byte(r.Header.Get("X-Internal-Secret"))
	return len(provided) == len(h.internalSecret) && subtle.ConstantTimeCompare(provided, h.internalSecret) == 1
}

func (h *Handler) acquire(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal(r.Context())
	if !ok {
		httpx.WriteError(w, r, &httpx.Error{Status: http.StatusUnauthorized, Code: "authentication_required", Message: "authentication is required"})
		return
	}
	var body struct {
		EventID    string   `json:"event_id"`
		ScheduleID string   `json:"schedule_id"`
		SeatIDs    []string `json:"seat_ids"`
		TTLSeconds int      `json:"ttl_seconds"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	lock, err := h.service.Acquire(r.Context(), principal, domain.AcquireRequest{
		EventID: body.EventID, ScheduleID: body.ScheduleID, OwnerID: principal.UserID, SeatIDs: body.SeatIDs,
		IdempotencyKey: idempotencyKey, TTL: time.Duration(body.TTLSeconds) * time.Second,
		AdmissionToken: strings.TrimSpace(r.Header.Get("X-Admission-Token")),
	})
	if err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	status := http.StatusCreated
	if lock.Replayed {
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, lock)
}

func (h *Handler) release(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpx.Principal(r.Context())
	if !ok {
		httpx.WriteError(w, r, &httpx.Error{Status: http.StatusUnauthorized, Code: "authentication_required", Message: "authentication is required"})
		return
	}
	var body struct {
		ScheduleID string   `json:"schedule_id"`
		SeatIDs    []string `json:"seat_ids"`
		Fence      int64    `json:"fence"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	err := h.service.Release(r.Context(), principal, domain.ReleaseRequest{
		ReservationID: r.PathValue("reservation_id"), ScheduleID: body.ScheduleID,
		OwnerID: principal.UserID, SeatIDs: body.SeatIDs, Fence: body.Fence,
	})
	if err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func mapError(err error) error {
	var conflict *domain.SeatConflictError
	switch {
	case errors.As(err, &conflict):
		return &httpx.Error{Status: http.StatusConflict, Code: "seat_unavailable", Message: "one or more seats are unavailable", Fields: map[string]string{"seat_id": conflict.SeatID}, Cause: err}
	case errors.Is(err, domain.ErrInvalidRequest):
		return &httpx.Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: err.Error(), Cause: err}
	case errors.Is(err, domain.ErrIdempotencyMismatch):
		return &httpx.Error{Status: http.StatusConflict, Code: "idempotency_mismatch", Message: "idempotency key was reused with different input", Cause: err}
	case errors.Is(err, domain.ErrLockLost):
		return &httpx.Error{Status: http.StatusConflict, Code: "reservation_lock_lost", Message: "reservation lock is absent, expired, or stale", Cause: err}
	case errors.Is(err, domain.ErrAdmissionRequired):
		return &httpx.Error{Status: http.StatusForbidden, Code: "waiting_room_admission_required", Message: "a valid waiting-room admission token is required", Cause: err}
	case errors.Is(err, application.ErrForbidden):
		return &httpx.Error{Status: http.StatusForbidden, Code: "forbidden", Message: "reservation ownership check failed", Cause: err}
	default:
		return &httpx.Error{Status: http.StatusInternalServerError, Code: "reservation_unavailable", Message: "reservation service is temporarily unavailable", Cause: err}
	}
}
