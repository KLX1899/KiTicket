// Package httpx defines the common HTTP contract without coupling domain code to HTTP.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/platform/id"
	"github.com/KLX1899/KiTicket/internal/platform/metrics"
)

const maxBodyBytes = 1 << 20

type contextKey string

const (
	requestIDKey   contextKey = "request_id"
	correlationKey contextKey = "correlation_id"
	principalKey   contextKey = "principal"
)

var safeHeaderID = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

type Error struct {
	Status  int
	Code    string
	Message string
	Fields  map[string]string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Code + ": " + e.Cause.Error()
	}
	return e.Code
}

func (e *Error) Unwrap() error { return e.Cause }

type errorEnvelope struct {
	Error struct {
		Code      string            `json:"code"`
		Message   string            `json:"message"`
		RequestID string            `json:"request_id"`
		Fields    map[string]string `json:"fields,omitempty"`
	} `json:"error"`
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	appErr := &Error{Status: http.StatusInternalServerError, Code: "internal_error", Message: "an internal error occurred"}
	var candidate *Error
	if errors.As(err, &candidate) {
		appErr = candidate
	}
	var body errorEnvelope
	body.Error.Code = appErr.Code
	body.Error.Message = appErr.Message
	body.Error.RequestID = RequestID(r.Context())
	body.Error.Fields = appErr.Fields
	WriteJSON(w, appErr.Status, body)
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func DecodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return &Error{Status: http.StatusBadRequest, Code: "invalid_json", Message: "request body is not valid JSON", Cause: err}
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return &Error{Status: http.StatusBadRequest, Code: "invalid_json", Message: "request body must contain exactly one JSON value"}
	}
	return nil
}

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func CorrelationID(ctx context.Context) string {
	value, _ := ctx.Value(correlationKey).(string)
	return value
}

func Principal(ctx context.Context) (auth.Principal, bool) {
	value, ok := ctx.Value(principalKey).(auth.Principal)
	return value, ok
}

type Middleware struct {
	Logger  *slog.Logger
	Metrics *metrics.Registry
	Timeout time.Duration
}

func (m Middleware) Wrap(next http.Handler) http.Handler {
	return m.identify(m.recoverPanic(m.deadline(m.accessLog(next))))
}

func (m Middleware) deadline(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.Timeout <= 0 {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), m.Timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m Middleware) identify(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if !safeHeaderID.MatchString(requestID) {
			requestID, _ = id.New()
		}
		correlationID := strings.TrimSpace(r.Header.Get("X-Correlation-ID"))
		if !safeHeaderID.MatchString(correlationID) {
			correlationID = requestID
		}
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		ctx = context.WithValue(ctx, correlationKey, correlationID)
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Correlation-ID", correlationID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m Middleware) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		capture := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(capture, r)
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		if m.Metrics != nil {
			m.Metrics.Observe(r.Method, route, capture.status)
		}
		if m.Logger != nil {
			m.Logger.InfoContext(r.Context(), "http request",
				"method", r.Method,
				"route", route,
				"status", capture.status,
				"duration_ms", time.Since(started).Milliseconds(),
				"request_id", RequestID(r.Context()),
				"correlation_id", CorrelationID(r.Context()),
			)
		}
	})
}

func (m Middleware) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				if m.Logger != nil {
					m.Logger.ErrorContext(r.Context(), "panic recovered", "request_id", RequestID(r.Context()))
				}
				WriteError(w, r, errors.New("panic recovered"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func Authenticate(signer *auth.Signer, roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			parts := strings.Fields(header)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				WriteError(w, r, &Error{Status: http.StatusUnauthorized, Code: "authentication_required", Message: "a bearer access token is required"})
				return
			}
			principal, err := signer.Verify(parts[1])
			if err != nil {
				WriteError(w, r, &Error{Status: http.StatusUnauthorized, Code: "invalid_access_token", Message: "the access token is invalid or expired"})
				return
			}
			if len(allowed) > 0 {
				if _, ok := allowed[principal.Role]; !ok {
					WriteError(w, r, &Error{Status: http.StatusForbidden, Code: "forbidden", Message: "this role cannot perform the operation"})
					return
				}
			}
			ctx := context.WithValue(r.Context(), principalKey, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
