// Package httpapi exposes checkout orchestration and ticket verification endpoints.
package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/KLX1899/KiTicket/internal/checkout/application"
	"github.com/KLX1899/KiTicket/internal/checkout/domain"
	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/platform/httpx"
)

type Handler struct {
	service *application.Service
	signer  *auth.Signer
}

func New(service *application.Service, signer *auth.Signer) (*Handler, error) {
	if service == nil || signer == nil {
		return nil, errors.New("checkout HTTP handler requires service and signer")
	}
	return &Handler{service: service, signer: signer}, nil
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("POST /v1/checkouts", httpx.Authenticate(h.signer, "buyer")(http.HandlerFunc(h.checkout)))
	mux.HandleFunc("POST /v1/tickets/verify", h.verify)
	mux.Handle("POST /v1/tickets/{ticket_id}/revoke", httpx.Authenticate(h.signer, "admin")(http.HandlerFunc(h.revoke)))
}

func (h *Handler) checkout(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.Principal(r.Context())
	var body struct {
		ReservationID    string   `json:"reservation_id"`
		ReservationFence int64    `json:"reservation_fence"`
		ScheduleID       string   `json:"schedule_id"`
		SeatIDs          []string `json:"seat_ids"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	result, err := h.service.Checkout(r.Context(), principal, application.CheckoutCommand{
		ReservationID: body.ReservationID, ReservationFence: body.ReservationFence,
		ScheduleID: body.ScheduleID, SeatIDs: body.SeatIDs,
		IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		CorrelationID:  httpx.CorrelationID(r.Context()),
	})
	if err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	var body struct {
		QRPayload string `json:"qr_payload"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	verification, err := h.service.VerifyTicket(r.Context(), body.QRPayload)
	if err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, verification)
}

func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.Principal(r.Context())
	var body struct {
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.service.RevokeTicket(r.Context(), principal, r.PathValue("ticket_id"), body.Reason); err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func mapError(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidOrder):
		return &httpx.Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: err.Error(), Cause: err}
	case errors.Is(err, domain.ErrIdempotencyMismatch):
		return &httpx.Error{Status: http.StatusConflict, Code: "idempotency_mismatch", Message: "idempotency key was reused with different input", Cause: err}
	case errors.Is(err, domain.ErrPaymentUncertain):
		return &httpx.Error{Status: http.StatusAccepted, Code: "payment_uncertain", Message: "payment reconciliation is pending", Cause: err}
	case errors.Is(err, domain.ErrPaymentFailed):
		return &httpx.Error{Status: http.StatusPaymentRequired, Code: "payment_failed", Message: "payment was declined or failed", Cause: err}
	case errors.Is(err, domain.ErrProviderUnavailable):
		return &httpx.Error{Status: http.StatusServiceUnavailable, Code: "payment_provider_unavailable", Message: "payment provider is temporarily unavailable", Cause: err}
	case errors.Is(err, domain.ErrForbidden):
		return &httpx.Error{Status: http.StatusForbidden, Code: "forbidden", Message: "checkout authorization failed", Cause: err}
	case errors.Is(err, domain.ErrNotFound):
		return &httpx.Error{Status: http.StatusNotFound, Code: "not_found", Message: "checkout resource was not found", Cause: err}
	case errors.Is(err, domain.ErrTransitionConflict), errors.Is(err, domain.ErrInvalidTransition):
		return &httpx.Error{Status: http.StatusConflict, Code: "order_state_conflict", Message: "order state changed or the requested transition is invalid", Cause: err}
	default:
		return &httpx.Error{Status: http.StatusInternalServerError, Code: "checkout_unavailable", Message: "checkout service is temporarily unavailable", Cause: err}
	}
}
