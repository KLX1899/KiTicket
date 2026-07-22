// Package gateway provides the unified edge with authentication, RBAC, throttling, and routing.
package gateway

import (
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/KLX1899/KiTicket/internal/gateway/ratelimit"
	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/platform/httpx"
)

type Handler struct {
	signer  *auth.Signer
	limiter *ratelimit.Limiter
	routes  []route
}

type route struct {
	prefix string
	proxy  *httputil.ReverseProxy
}

func New(signer *auth.Signer, limiter *ratelimit.Limiter, targets map[string]string) (*Handler, error) {
	if signer == nil || limiter == nil {
		return nil, errors.New("gateway requires token signer and rate limiter")
	}
	prefixes := []struct{ prefix, service string }{
		{"/v1/auth/", "identity"},
		{"/v1/events", "catalog"},
		{"/v1/venues", "catalog"},
		{"/v1/reservation-locks", "reservation"},
		{"/v1/checkouts", "checkout"},
		{"/v1/tickets", "checkout"},
		{"/v1/waiting-room", "waiting-room"},
	}
	handler := &Handler{signer: signer, limiter: limiter}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2: true, MaxIdleConns: 100, MaxIdleConnsPerHost: 20,
		IdleConnTimeout: 60 * time.Second, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 12 * time.Second,
	}
	for _, item := range prefixes {
		target, err := url.Parse(targets[item.service])
		if err != nil || target.Scheme == "" || target.Host == "" {
			return nil, errors.New("gateway target for " + item.service + " is invalid")
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.Transport = transport
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, _ error) {
			httpx.WriteError(w, r, &httpx.Error{Status: http.StatusBadGateway, Code: "upstream_unavailable", Message: "an upstream service is unavailable"})
		}
		originalDirector := proxy.Director
		proxy.Director = func(request *http.Request) {
			originalDirector(request)
			request.Host = target.Host
			request.Header.Set("X-Request-ID", httpx.RequestID(request.Context()))
			request.Header.Set("X-Correlation-ID", httpx.CorrelationID(request.Context()))
		}
		handler.routes = append(handler.routes, route{prefix: item.prefix, proxy: proxy})
	}
	return handler, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	selected := h.route(r.URL.Path)
	if selected == nil {
		httpx.WriteError(w, r, &httpx.Error{Status: http.StatusNotFound, Code: "not_found", Message: "the requested API route does not exist"})
		return
	}
	principal, authenticated, err := h.authenticate(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	identity := clientIP(r)
	if authenticated {
		identity = principal.UserID
	}
	allowed, retryAfter, limitErr := h.limiter.Allow(r.Context(), identity)
	if limitErr != nil {
		httpx.WriteError(w, r, &httpx.Error{Status: http.StatusServiceUnavailable, Code: "rate_limiter_unavailable", Message: "traffic admission is temporarily unavailable", Cause: limitErr})
		return
	}
	if !allowed {
		seconds := int(retryAfter.Round(time.Second) / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		httpx.WriteError(w, r, &httpx.Error{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: "request rate exceeded"})
		return
	}
	selected.ServeHTTP(w, r)
}

func (h *Handler) authenticate(r *http.Request) (auth.Principal, bool, error) {
	public := r.Method == http.MethodPost && (r.URL.Path == "/v1/auth/register" || r.URL.Path == "/v1/auth/login" || r.URL.Path == "/v1/tickets/verify") ||
		r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/events")
	header := strings.Fields(r.Header.Get("Authorization"))
	if len(header) == 0 && public {
		return auth.Principal{}, false, nil
	}
	if len(header) != 2 || !strings.EqualFold(header[0], "Bearer") {
		return auth.Principal{}, false, &httpx.Error{Status: http.StatusUnauthorized, Code: "authentication_required", Message: "a bearer access token is required"}
	}
	principal, err := h.signer.Verify(header[1])
	if err != nil {
		return auth.Principal{}, false, &httpx.Error{Status: http.StatusUnauthorized, Code: "invalid_access_token", Message: "the access token is invalid or expired"}
	}
	if !allowedRole(r, principal.Role) {
		return auth.Principal{}, false, &httpx.Error{Status: http.StatusForbidden, Code: "forbidden", Message: "this role cannot perform the operation"}
	}
	return principal, true, nil
}

func allowedRole(r *http.Request, role string) bool {
	if r.Method == http.MethodPost && r.URL.Path == "/v1/tickets/verify" || r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/events") {
		return true
	}
	if strings.HasPrefix(r.URL.Path, "/v1/venues") || strings.HasPrefix(r.URL.Path, "/v1/events") {
		return role == "organizer" || role == "admin"
	}
	if strings.HasSuffix(r.URL.Path, "/revoke") {
		return role == "admin"
	}
	return role == "buyer"
}

func (h *Handler) route(path string) http.Handler {
	for _, candidate := range h.routes {
		if strings.HasPrefix(path, candidate.prefix) {
			return candidate.proxy
		}
	}
	return nil
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
