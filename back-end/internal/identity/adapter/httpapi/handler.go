// Package httpapi exposes identity use cases through a strict JSON API.
package httpapi

import (
	"errors"
	"net/http"

	"github.com/KLX1899/KiTicket/internal/identity/application"
	"github.com/KLX1899/KiTicket/internal/identity/domain"
	"github.com/KLX1899/KiTicket/internal/platform/httpx"
)

type Handler struct{ service *application.Service }

func New(service *application.Service) (*Handler, error) {
	if service == nil {
		return nil, errors.New("identity HTTP handler requires a service")
	}
	return &Handler{service: service}, nil
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/auth/register", h.register)
	mux.HandleFunc("POST /v1/auth/login", h.login)
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	session, err := h.service.Register(r.Context(), application.RegisterCommand(body))
	if err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, session)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	session, err := h.service.Login(r.Context(), application.LoginCommand(body))
	if err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, session)
}

func mapError(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidUser):
		return &httpx.Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: err.Error(), Cause: err}
	case errors.Is(err, domain.ErrEmailTaken):
		return &httpx.Error{Status: http.StatusConflict, Code: "email_taken", Message: "email is already registered", Cause: err}
	case errors.Is(err, domain.ErrInvalidCredentials):
		return &httpx.Error{Status: http.StatusUnauthorized, Code: "invalid_credentials", Message: "email or password is invalid", Cause: err}
	case errors.Is(err, domain.ErrUserDisabled):
		return &httpx.Error{Status: http.StatusForbidden, Code: "user_disabled", Message: "the user account is disabled", Cause: err}
	default:
		return err
	}
}
