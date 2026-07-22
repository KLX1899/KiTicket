// Package httpapi exposes catalog administration and public discovery endpoints.
package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/KLX1899/KiTicket/internal/catalog/application"
	"github.com/KLX1899/KiTicket/internal/catalog/domain"
	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/platform/httpx"
)

type Handler struct {
	service *application.Service
	signer  *auth.Signer
}

func New(service *application.Service, signer *auth.Signer) (*Handler, error) {
	if service == nil || signer == nil {
		return nil, errors.New("catalog HTTP handler requires service and token signer")
	}
	return &Handler{service: service, signer: signer}, nil
}

func (h *Handler) Register(mux *http.ServeMux) {
	manage := httpx.Authenticate(h.signer, "organizer", "admin")
	mux.Handle("POST /v1/venues", manage(http.HandlerFunc(h.createVenue)))
	mux.Handle("POST /v1/events", manage(http.HandlerFunc(h.createEvent)))
	mux.Handle("POST /v1/events/{event_id}/schedules", manage(http.HandlerFunc(h.createSchedule)))
	mux.Handle("POST /v1/events/{event_id}/publish", manage(http.HandlerFunc(h.publish)))
	mux.HandleFunc("GET /v1/events", h.search)
	mux.HandleFunc("GET /v1/events/{event_id}/schedules/{schedule_id}", h.scheduleDetail)
}

type venueRequest struct {
	OwnerID          string        `json:"owner_id"`
	Name             string        `json:"name"`
	CountryCode      string        `json:"country_code"`
	City             string        `json:"city"`
	Address          string        `json:"address"`
	DeclaredCapacity int           `json:"declared_capacity"`
	Halls            []hallRequest `json:"halls"`
}

type hallRequest struct {
	Name             string           `json:"name"`
	DeclaredCapacity int              `json:"declared_capacity"`
	Sections         []sectionRequest `json:"sections"`
}

type sectionRequest struct {
	Name      string       `json:"name"`
	SortOrder int          `json:"sort_order"`
	Rows      []rowRequest `json:"rows"`
}

type rowRequest struct {
	Label     string        `json:"label"`
	SortOrder int           `json:"sort_order"`
	Seats     []seatRequest `json:"seats"`
}

type seatRequest struct {
	Number     string `json:"number"`
	SortOrder  int    `json:"sort_order"`
	Accessible bool   `json:"accessible"`
}

func (h *Handler) createVenue(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.Principal(r.Context())
	var request venueRequest
	if err := httpx.DecodeJSON(w, r, &request); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	venue, err := h.service.CreateVenue(r.Context(), principal, request.venue())
	if err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"id": venue.ID, "declared_capacity": venue.DeclaredCapacity})
}

func (r venueRequest) venue() domain.Venue {
	venue := domain.Venue{OwnerID: r.OwnerID, Name: r.Name, CountryCode: r.CountryCode, City: r.City, Address: r.Address, DeclaredCapacity: r.DeclaredCapacity}
	for _, hallRequest := range r.Halls {
		hall := domain.Hall{Name: hallRequest.Name, DeclaredCapacity: hallRequest.DeclaredCapacity}
		for _, sectionRequest := range hallRequest.Sections {
			section := domain.Section{Name: sectionRequest.Name, SortOrder: sectionRequest.SortOrder}
			for _, rowRequest := range sectionRequest.Rows {
				row := domain.Row{Label: rowRequest.Label, SortOrder: rowRequest.SortOrder}
				for _, seatRequest := range rowRequest.Seats {
					row.Seats = append(row.Seats, domain.Seat{Number: seatRequest.Number, SortOrder: seatRequest.SortOrder, Accessible: seatRequest.Accessible})
				}
				section.Rows = append(section.Rows, row)
			}
			hall.Sections = append(hall.Sections, section)
		}
		venue.Halls = append(venue.Halls, hall)
	}
	return venue
}

func (h *Handler) createEvent(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.Principal(r.Context())
	var request struct {
		OrganizerID string   `json:"organizer_id"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Genre       string   `json:"genre"`
		Tags        []string `json:"tags"`
	}
	if err := httpx.DecodeJSON(w, r, &request); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	event, err := h.service.CreateEvent(r.Context(), principal, domain.Event{
		OrganizerID: request.OrganizerID, Title: request.Title, Description: request.Description, Genre: request.Genre, Tags: request.Tags,
	})
	if err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"id": event.ID, "status": "draft"})
}

func (h *Handler) createSchedule(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.Principal(r.Context())
	var request struct {
		HallID     string    `json:"hall_id"`
		StartsAt   time.Time `json:"starts_at"`
		EndsAt     time.Time `json:"ends_at"`
		Categories []struct {
			Name       string   `json:"name"`
			PriceMinor int64    `json:"price_minor"`
			Currency   string   `json:"currency"`
			SeatIDs    []string `json:"seat_ids"`
		} `json:"pricing_categories"`
	}
	if err := httpx.DecodeJSON(w, r, &request); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	schedule := domain.Schedule{EventID: r.PathValue("event_id"), HallID: request.HallID, StartsAt: request.StartsAt, EndsAt: request.EndsAt}
	for _, category := range request.Categories {
		schedule.Categories = append(schedule.Categories, domain.PriceCategory{
			Name: category.Name, PriceMinor: category.PriceMinor, Currency: category.Currency, SeatIDs: category.SeatIDs,
		})
	}
	created, err := h.service.CreateSchedule(r.Context(), principal, schedule)
	if err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"id": created.ID, "event_id": created.EventID})
}

func (h *Handler) publish(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.Principal(r.Context())
	if err := h.service.Publish(r.Context(), principal, r.PathValue("event_id")); err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	status := query.Get("status")
	if status != "" && status != "published" {
		httpx.WriteError(w, r, &httpx.Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "public discovery supports only published status"})
		return
	}
	page, err := integer(query.Get("page"), 1)
	if err != nil {
		httpx.WriteError(w, r, invalidQuery("page"))
		return
	}
	pageSize, err := integer(query.Get("page_size"), 20)
	if err != nil {
		httpx.WriteError(w, r, invalidQuery("page_size"))
		return
	}
	minimumPrice, err := optionalInt64(query.Get("min_price"))
	if err != nil {
		httpx.WriteError(w, r, invalidQuery("min_price"))
		return
	}
	maximumPrice, err := optionalInt64(query.Get("max_price"))
	if err != nil {
		httpx.WriteError(w, r, invalidQuery("max_price"))
		return
	}
	from, err := optionalTime(query.Get("from"))
	if err != nil {
		httpx.WriteError(w, r, invalidQuery("from"))
		return
	}
	to, err := optionalTime(query.Get("to"))
	if err != nil {
		httpx.WriteError(w, r, invalidQuery("to"))
		return
	}
	availableOnly, err := optionalBool(query.Get("available_only"))
	if err != nil {
		httpx.WriteError(w, r, invalidQuery("available_only"))
		return
	}
	items, total, err := h.service.Search(r.Context(), domain.SearchFilter{
		Text: query.Get("q"), Tag: query.Get("tag"), Genre: query.Get("genre"),
		CountryCode: query.Get("country"), City: query.Get("city"), From: from, To: to,
		MinimumPrice: minimumPrice, MaximumPrice: maximumPrice, AvailableOnly: availableOnly,
		Page: page, PageSize: pageSize, Sort: query.Get("sort"),
	})
	if err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"page":  map[string]int{"number": page, "size": pageSize, "total_items": total},
	})
}

func (h *Handler) scheduleDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := h.service.ScheduleDetail(r.Context(), r.PathValue("event_id"), r.PathValue("schedule_id"))
	if err != nil {
		httpx.WriteError(w, r, mapError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func mapError(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidCatalog):
		return &httpx.Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: err.Error(), Cause: err}
	case errors.Is(err, domain.ErrScheduleConflict):
		return &httpx.Error{Status: http.StatusConflict, Code: "schedule_conflict", Message: "the hall is already scheduled during this interval", Cause: err}
	case errors.Is(err, domain.ErrNotFound):
		return &httpx.Error{Status: http.StatusNotFound, Code: "not_found", Message: "catalog resource was not found", Cause: err}
	case errors.Is(err, domain.ErrForbidden):
		return &httpx.Error{Status: http.StatusForbidden, Code: "forbidden", Message: "catalog ownership or role check failed", Cause: err}
	default:
		return &httpx.Error{Status: http.StatusInternalServerError, Code: "catalog_unavailable", Message: "catalog service is temporarily unavailable", Cause: err}
	}
}

func integer(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	return strconv.Atoi(raw)
}

func optionalInt64(raw string) (*int64, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	return &value, err
}

func optionalTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func optionalBool(raw string) (bool, error) {
	if strings.TrimSpace(raw) == "" {
		return false, nil
	}
	return strconv.ParseBool(raw)
}

func invalidQuery(field string) error {
	return &httpx.Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "query parameter " + field + " is invalid"}
}
