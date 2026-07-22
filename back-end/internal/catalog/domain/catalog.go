// Package domain defines catalog aggregates and validation independent of storage or HTTP.
package domain

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalidCatalog   = errors.New("invalid catalog data")
	ErrNotFound         = errors.New("catalog resource not found")
	ErrScheduleConflict = errors.New("hall has a scheduling conflict")
	ErrForbidden        = errors.New("catalog operation is forbidden")
)

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

type Venue struct {
	ID               string
	OwnerID          string
	Name             string
	CountryCode      string
	City             string
	Address          string
	DeclaredCapacity int
	Halls            []Hall
}

type Hall struct {
	ID               string
	Name             string
	DeclaredCapacity int
	Sections         []Section
}

type Section struct {
	ID        string
	Name      string
	SortOrder int
	Rows      []Row
}

type Row struct {
	ID        string
	Label     string
	SortOrder int
	Seats     []Seat
}

type Seat struct {
	ID         string
	Number     string
	SortOrder  int
	Accessible bool
}

func (v Venue) Validate() error {
	if !validText(v.Name, 2, 200) || !validText(v.City, 2, 120) || !validText(v.Address, 4, 500) || len(v.CountryCode) != 2 || v.CountryCode != strings.ToUpper(v.CountryCode) {
		return fmt.Errorf("%w: venue name, country, city, or address", ErrInvalidCatalog)
	}
	if len(v.Halls) == 0 || len(v.Halls) > 20 {
		return fmt.Errorf("%w: a venue requires 1-20 halls", ErrInvalidCatalog)
	}
	total := 0
	hallNames := map[string]struct{}{}
	for _, hall := range v.Halls {
		name := strings.ToLower(strings.TrimSpace(hall.Name))
		if !validText(hall.Name, 1, 120) {
			return fmt.Errorf("%w: invalid hall name", ErrInvalidCatalog)
		}
		if _, duplicate := hallNames[name]; duplicate {
			return fmt.Errorf("%w: duplicate hall name", ErrInvalidCatalog)
		}
		hallNames[name] = struct{}{}
		capacity, err := hall.validate()
		if err != nil {
			return err
		}
		total += capacity
	}
	if total != v.DeclaredCapacity {
		return fmt.Errorf("%w: venue capacity %d does not equal %d defined seats", ErrInvalidCatalog, v.DeclaredCapacity, total)
	}
	return nil
}

func (h Hall) validate() (int, error) {
	if len(h.Sections) == 0 || len(h.Sections) > 100 {
		return 0, fmt.Errorf("%w: a hall requires 1-100 sections", ErrInvalidCatalog)
	}
	total := 0
	sectionNames := map[string]struct{}{}
	orders := map[int]struct{}{}
	for _, section := range h.Sections {
		name := strings.ToLower(strings.TrimSpace(section.Name))
		if !validText(section.Name, 1, 120) || section.SortOrder < 0 {
			return 0, fmt.Errorf("%w: invalid section", ErrInvalidCatalog)
		}
		if _, exists := sectionNames[name]; exists {
			return 0, fmt.Errorf("%w: duplicate section name", ErrInvalidCatalog)
		}
		if _, exists := orders[section.SortOrder]; exists {
			return 0, fmt.Errorf("%w: duplicate section sort order", ErrInvalidCatalog)
		}
		sectionNames[name] = struct{}{}
		orders[section.SortOrder] = struct{}{}
		capacity, err := section.validate()
		if err != nil {
			return 0, err
		}
		total += capacity
	}
	if total != h.DeclaredCapacity {
		return 0, fmt.Errorf("%w: hall capacity %d does not equal %d defined seats", ErrInvalidCatalog, h.DeclaredCapacity, total)
	}
	return total, nil
}

func (s Section) validate() (int, error) {
	if len(s.Rows) == 0 || len(s.Rows) > 500 {
		return 0, fmt.Errorf("%w: a section requires 1-500 rows", ErrInvalidCatalog)
	}
	total := 0
	labels := map[string]struct{}{}
	orders := map[int]struct{}{}
	for _, row := range s.Rows {
		label := strings.ToLower(strings.TrimSpace(row.Label))
		if !validText(row.Label, 1, 30) || row.SortOrder < 0 || len(row.Seats) == 0 || len(row.Seats) > 500 {
			return 0, fmt.Errorf("%w: invalid row", ErrInvalidCatalog)
		}
		if _, exists := labels[label]; exists {
			return 0, fmt.Errorf("%w: duplicate row label", ErrInvalidCatalog)
		}
		if _, exists := orders[row.SortOrder]; exists {
			return 0, fmt.Errorf("%w: duplicate row sort order", ErrInvalidCatalog)
		}
		labels[label] = struct{}{}
		orders[row.SortOrder] = struct{}{}
		seatNumbers := map[string]struct{}{}
		seatOrders := map[int]struct{}{}
		for _, seat := range row.Seats {
			number := strings.ToLower(strings.TrimSpace(seat.Number))
			if !validText(seat.Number, 1, 30) || seat.SortOrder < 0 {
				return 0, fmt.Errorf("%w: invalid seat", ErrInvalidCatalog)
			}
			if _, exists := seatNumbers[number]; exists {
				return 0, fmt.Errorf("%w: duplicate seat number", ErrInvalidCatalog)
			}
			if _, exists := seatOrders[seat.SortOrder]; exists {
				return 0, fmt.Errorf("%w: duplicate seat sort order", ErrInvalidCatalog)
			}
			seatNumbers[number] = struct{}{}
			seatOrders[seat.SortOrder] = struct{}{}
		}
		total += len(row.Seats)
	}
	return total, nil
}

type Event struct {
	ID          string
	OrganizerID string
	Title       string
	Description string
	Genre       string
	Tags        []string
}

func (e *Event) Validate() error {
	if !validText(e.Title, 2, 200) || !validText(e.Description, 10, 5000) || !validText(e.Genre, 2, 80) {
		return fmt.Errorf("%w: event title, description, or genre", ErrInvalidCatalog)
	}
	if len(e.Tags) > 20 {
		return fmt.Errorf("%w: at most 20 tags are allowed", ErrInvalidCatalog)
	}
	for index := range e.Tags {
		e.Tags[index] = strings.ToLower(strings.TrimSpace(e.Tags[index]))
		if !validText(e.Tags[index], 1, 50) {
			return fmt.Errorf("%w: invalid event tag", ErrInvalidCatalog)
		}
	}
	slices.Sort(e.Tags)
	e.Tags = slices.Compact(e.Tags)
	return nil
}

type Schedule struct {
	ID         string
	EventID    string
	HallID     string
	StartsAt   time.Time
	EndsAt     time.Time
	Categories []PriceCategory
}

type PriceCategory struct {
	ID         string
	Name       string
	PriceMinor int64
	Currency   string
	SeatIDs    []string
}

func (s *Schedule) Validate(now time.Time) error {
	if s.EventID == "" || s.HallID == "" || !s.StartsAt.After(now) || !s.EndsAt.After(s.StartsAt) || s.EndsAt.Sub(s.StartsAt) > 7*24*time.Hour {
		return fmt.Errorf("%w: schedule time or association", ErrInvalidCatalog)
	}
	if len(s.Categories) == 0 || len(s.Categories) > 50 {
		return fmt.Errorf("%w: schedule requires 1-50 price categories", ErrInvalidCatalog)
	}
	assigned := map[string]struct{}{}
	names := map[string]struct{}{}
	currency := ""
	for index := range s.Categories {
		category := &s.Categories[index]
		category.Name = strings.TrimSpace(category.Name)
		category.Currency = strings.ToUpper(strings.TrimSpace(category.Currency))
		if !validText(category.Name, 1, 100) || category.PriceMinor < 0 || !currencyPattern.MatchString(category.Currency) || len(category.SeatIDs) == 0 {
			return fmt.Errorf("%w: invalid pricing category", ErrInvalidCatalog)
		}
		if currency == "" {
			currency = category.Currency
		} else if currency != category.Currency {
			return fmt.Errorf("%w: one schedule cannot mix currencies", ErrInvalidCatalog)
		}
		name := strings.ToLower(category.Name)
		if _, duplicate := names[name]; duplicate {
			return fmt.Errorf("%w: duplicate pricing category", ErrInvalidCatalog)
		}
		names[name] = struct{}{}
		slices.Sort(category.SeatIDs)
		for _, seatID := range category.SeatIDs {
			if seatID == "" {
				return fmt.Errorf("%w: empty seat identifier", ErrInvalidCatalog)
			}
			if _, duplicate := assigned[seatID]; duplicate {
				return fmt.Errorf("%w: seat assigned to multiple prices", ErrInvalidCatalog)
			}
			assigned[seatID] = struct{}{}
		}
	}
	return nil
}

type SearchFilter struct {
	Text          string
	Tag           string
	Genre         string
	CountryCode   string
	City          string
	From          time.Time
	To            time.Time
	MinimumPrice  *int64
	MaximumPrice  *int64
	AvailableOnly bool
	Page          int
	PageSize      int
	Sort          string
}

func (f *SearchFilter) Validate() error {
	f.Text = strings.TrimSpace(f.Text)
	f.Tag = strings.ToLower(strings.TrimSpace(f.Tag))
	f.Genre = strings.TrimSpace(f.Genre)
	f.CountryCode = strings.ToUpper(strings.TrimSpace(f.CountryCode))
	f.City = strings.TrimSpace(f.City)
	if len(f.Text) > 200 || len(f.Tag) > 50 || len(f.Genre) > 80 || len(f.City) > 120 || (f.CountryCode != "" && len(f.CountryCode) != 2) {
		return fmt.Errorf("%w: invalid search text or location filter", ErrInvalidCatalog)
	}
	if !f.From.IsZero() && !f.To.IsZero() && !f.To.After(f.From) {
		return fmt.Errorf("%w: date range is invalid", ErrInvalidCatalog)
	}
	if f.MinimumPrice != nil && *f.MinimumPrice < 0 || f.MaximumPrice != nil && *f.MaximumPrice < 0 || f.MinimumPrice != nil && f.MaximumPrice != nil && *f.MaximumPrice < *f.MinimumPrice {
		return fmt.Errorf("%w: price range is invalid", ErrInvalidCatalog)
	}
	if f.Page < 1 || f.PageSize < 1 || f.PageSize > 100 {
		return fmt.Errorf("%w: page must be positive and page_size between 1 and 100", ErrInvalidCatalog)
	}
	if f.Sort == "" {
		f.Sort = "date_asc"
	}
	if f.Sort != "date_asc" && f.Sort != "date_desc" && f.Sort != "price_asc" && f.Sort != "price_desc" {
		return fmt.Errorf("%w: unsupported sort", ErrInvalidCatalog)
	}
	return nil
}

type DiscoveryItem struct {
	EventID           string    `json:"event_id"`
	Title             string    `json:"title"`
	Genre             string    `json:"genre"`
	ScheduleID        string    `json:"schedule_id"`
	StartsAt          time.Time `json:"starts_at"`
	VenueName         string    `json:"venue_name"`
	City              string    `json:"city"`
	CountryCode       string    `json:"country_code"`
	MinimumPriceMinor int64     `json:"minimum_price_minor"`
	Currency          string    `json:"currency"`
	AvailableSeats    int       `json:"available_seats"`
	AvailabilityAsOf  time.Time `json:"availability_as_of"`
	AvailabilityScope string    `json:"availability_scope"`
}

type ScheduleDetail struct {
	EventID     string         `json:"event_id"`
	Title       string         `json:"title"`
	Genre       string         `json:"genre"`
	Description string         `json:"description"`
	ScheduleID  string         `json:"schedule_id"`
	StartsAt    time.Time      `json:"starts_at"`
	EndsAt      time.Time      `json:"ends_at"`
	VenueName   string         `json:"venue_name"`
	HallName    string         `json:"hall_name"`
	City        string         `json:"city"`
	CountryCode string         `json:"country_code"`
	Seats       []ScheduleSeat `json:"seats"`
}

type ScheduleSeat struct {
	SeatID      string `json:"seat_id"`
	SectionName string `json:"section_name"`
	RowLabel    string `json:"row_label"`
	SeatNumber  string `json:"seat_number"`
	Accessible  bool   `json:"accessible"`
	PriceMinor  int64  `json:"price_minor"`
	Currency    string `json:"currency"`
	Available   bool   `json:"available"`
}

func validText(value string, minimum, maximum int) bool {
	if value != strings.TrimSpace(value) || strings.ContainsAny(value, "\x00\r") {
		return false
	}
	length := utf8.RuneCountInString(value)
	return length >= minimum && length <= maximum
}
