// Package httpapi exposes authenticated waiting-room join and status APIs.
package httpapi

import (
	"errors"
	"net/http"

	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/platform/httpx"
	"github.com/KLX1899/KiTicket/internal/waitingroom/adapter/redisqueue"
	"github.com/KLX1899/KiTicket/internal/waitingroom/application"
)

type Handler struct {
	service *application.Service
	signer  *auth.Signer
}

func New(service *application.Service, signer *auth.Signer) (*Handler, error) {
	if service == nil || signer == nil {
		return nil, errors.New("waiting-room HTTP handler requires service and JWT signer")
	}
	return &Handler{service: service, signer: signer}, nil
}

func (h *Handler) Register(mux *http.ServeMux) {
	buyer := httpx.Authenticate(h.signer, "buyer")
	mux.Handle("POST /v1/waiting-room/{event_id}/join", buyer(http.HandlerFunc(h.join)))
	mux.Handle("POST /v1/waiting-room/status", buyer(http.HandlerFunc(h.status)))
}

func (h *Handler) join(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.Principal(r.Context())
	response, err := h.service.Join(r.Context(), principal, r.PathValue("event_id"))
	if err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, response)
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.Principal(r.Context())
	var body struct {
		QueueToken string `json:"queue_token"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	response, err := h.service.Status(r.Context(), principal, body.QueueToken)
	if err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

func mapError(err error) error {
	switch {
	case errors.Is(err, application.ErrForbidden):
		return &httpx.Error{Status: http.StatusForbidden, Code: "forbidden", Message: "waiting-room authorization failed", Cause: err}
	case errors.Is(err, application.ErrInvalidEvent):
		return &httpx.Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "event identifier is invalid", Cause: err}
	case errors.Is(err, redisqueue.ErrNotQueued):
		return &httpx.Error{Status: http.StatusGone, Code: "queue_token_expired", Message: "queue token is absent or expired", Cause: err}
	default:
		return &httpx.Error{Status: http.StatusServiceUnavailable, Code: "waiting_room_unavailable", Message: "waiting room is temporarily unavailable", Cause: err}
	}
}
