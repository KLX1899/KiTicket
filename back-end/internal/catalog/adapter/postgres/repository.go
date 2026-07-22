// Package postgres implements catalog persistence and indexed discovery queries.
package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/KLX1899/KiTicket/internal/catalog/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) (*Repository, error) {
	if pool == nil {
		return nil, errors.New("catalog repository requires a PostgreSQL pool")
	}
	return &Repository{pool: pool}, nil
}

func (r *Repository) CreateVenue(ctx context.Context, venue domain.Venue) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin venue transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO catalog.venues (id, owner_id, name, country_code, city, address, declared_capacity)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		venue.ID, venue.OwnerID, venue.Name, venue.CountryCode, venue.City, venue.Address, venue.DeclaredCapacity); err != nil {
		return mapError("insert venue", err)
	}
	for _, hall := range venue.Halls {
		if _, err := tx.Exec(ctx, `INSERT INTO catalog.halls (id, venue_id, name, declared_capacity) VALUES ($1, $2, $3, $4)`, hall.ID, venue.ID, hall.Name, hall.DeclaredCapacity); err != nil {
			return mapError("insert hall", err)
		}
		for _, section := range hall.Sections {
			if _, err := tx.Exec(ctx, `INSERT INTO catalog.sections (id, hall_id, name, sort_order) VALUES ($1, $2, $3, $4)`, section.ID, hall.ID, section.Name, section.SortOrder); err != nil {
				return mapError("insert section", err)
			}
			for _, row := range section.Rows {
				if _, err := tx.Exec(ctx, `INSERT INTO catalog.seat_rows (id, section_id, label, sort_order) VALUES ($1, $2, $3, $4)`, row.ID, section.ID, row.Label, row.SortOrder); err != nil {
					return mapError("insert row", err)
				}
				for _, seat := range row.Seats {
					if _, err := tx.Exec(ctx, `INSERT INTO catalog.seats (id, row_id, number, sort_order, accessible) VALUES ($1, $2, $3, $4, $5)`, seat.ID, row.ID, seat.Number, seat.SortOrder, seat.Accessible); err != nil {
						return mapError("insert seat", err)
					}
				}
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit venue: %w", err)
	}
	return nil
}

func (r *Repository) CreateEvent(ctx context.Context, event domain.Event) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin event transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO catalog.events (id, organizer_id, title, description, genre)
		VALUES ($1, $2, $3, $4, $5)`, event.ID, event.OrganizerID, event.Title, event.Description, event.Genre); err != nil {
		return mapError("insert event", err)
	}
	for _, tag := range event.Tags {
		tagID := deterministicTagID(tag)
		if _, err := tx.Exec(ctx, `INSERT INTO catalog.tags (id, name) VALUES ($1, $2) ON CONFLICT (name) DO NOTHING`, tagID, tag); err != nil {
			return mapError("insert tag", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO catalog.event_tags (event_id, tag_id)
			SELECT $1, id FROM catalog.tags WHERE name = $2`, event.ID, tag); err != nil {
			return mapError("associate event tag", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit event: %w", err)
	}
	return nil
}

func (r *Repository) EventOwner(ctx context.Context, eventID string) (string, error) {
	var owner string
	if err := r.pool.QueryRow(ctx, `SELECT organizer_id FROM catalog.events WHERE id = $1`, eventID).Scan(&owner); errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	} else if err != nil {
		return "", fmt.Errorf("find event owner: %w", err)
	}
	return owner, nil
}

func (r *Repository) CreateSchedule(ctx context.Context, schedule domain.Schedule) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin schedule transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	rows, err := tx.Query(ctx, `
		SELECT s.id
		FROM catalog.seats s
		JOIN catalog.seat_rows r ON r.id = s.row_id
		JOIN catalog.sections sec ON sec.id = r.section_id
		WHERE sec.hall_id = $1
		ORDER BY s.id
		FOR SHARE`, schedule.HallID)
	if err != nil {
		return fmt.Errorf("read hall seats: %w", err)
	}
	hallSeats := make([]string, 0)
	for rows.Next() {
		var seatID string
		if err := rows.Scan(&seatID); err != nil {
			rows.Close()
			return fmt.Errorf("scan hall seat: %w", err)
		}
		hallSeats = append(hallSeats, seatID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate hall seats: %w", err)
	}
	requestedSeats := make([]string, 0, len(hallSeats))
	for _, category := range schedule.Categories {
		requestedSeats = append(requestedSeats, category.SeatIDs...)
	}
	slices.Sort(requestedSeats)
	if len(hallSeats) == 0 || !slices.Equal(hallSeats, requestedSeats) {
		return fmt.Errorf("%w: pricing must assign every hall seat exactly once", domain.ErrInvalidCatalog)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO catalog.event_schedules (id, event_id, hall_id, starts_at, ends_at)
		VALUES ($1, $2, $3, $4, $5)`, schedule.ID, schedule.EventID, schedule.HallID, schedule.StartsAt, schedule.EndsAt); err != nil {
		return mapError("insert schedule", err)
	}
	for _, category := range schedule.Categories {
		var categoryID string
		err := tx.QueryRow(ctx, `
			INSERT INTO catalog.pricing_categories (id, event_id, name, price_minor, currency)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (event_id, name) DO UPDATE
			SET price_minor = EXCLUDED.price_minor, currency = EXCLUDED.currency
			RETURNING id`, category.ID, schedule.EventID, category.Name, category.PriceMinor, category.Currency).Scan(&categoryID)
		if err != nil {
			return mapError("upsert pricing category", err)
		}
		for _, seatID := range category.SeatIDs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO catalog.schedule_seat_prices
					(schedule_id, seat_id, pricing_category_id, price_minor, currency)
				VALUES ($1, $2, $3, $4, $5)`, schedule.ID, seatID, categoryID, category.PriceMinor, category.Currency); err != nil {
				return mapError("insert schedule seat price", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return mapError("commit schedule", err)
	}
	return nil
}

func (r *Repository) PublishEvent(ctx context.Context, eventID string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE catalog.events e
		SET status = 'published', published_at = now(), updated_at = now()
		WHERE e.id = $1
		  AND e.status = 'draft'
		  AND EXISTS (
		      SELECT 1 FROM catalog.event_schedules es
		      JOIN catalog.schedule_seat_prices sp ON sp.schedule_id = es.id
		      WHERE es.event_id = e.id AND es.status = 'scheduled' AND es.starts_at > now()
		  )`, eventID)
	if err != nil {
		return fmt.Errorf("publish event: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: event must be a draft with a future priced schedule", domain.ErrInvalidCatalog)
	}
	return nil
}

func (r *Repository) Search(ctx context.Context, filter domain.SearchFilter) ([]domain.DiscoveryItem, int, error) {
	order := map[string]string{
		"date_asc":   "starts_at ASC, event_id ASC, schedule_id ASC",
		"date_desc":  "starts_at DESC, event_id ASC, schedule_id ASC",
		"price_asc":  "minimum_price_minor ASC, starts_at ASC, event_id ASC, schedule_id ASC",
		"price_desc": "minimum_price_minor DESC, starts_at ASC, event_id ASC, schedule_id ASC",
	}[filter.Sort]
	query := `
		WITH results AS (
			SELECT
				e.id AS event_id, e.title, e.genre, es.id AS schedule_id, es.starts_at,
				v.name AS venue_name, v.city, v.country_code,
				min(sp.price_minor) AS minimum_price_minor,
				min(sp.currency) AS currency,
				(count(sp.seat_id) - count(bs.seat_id))::integer AS available_seats,
				statement_timestamp() AS availability_as_of
			FROM catalog.events e
			JOIN catalog.event_schedules es ON es.event_id = e.id AND es.status = 'scheduled'
			JOIN catalog.halls h ON h.id = es.hall_id
			JOIN catalog.venues v ON v.id = h.venue_id
			JOIN catalog.schedule_seat_prices sp ON sp.schedule_id = es.id
			LEFT JOIN reservation.booked_seats bs ON bs.schedule_id = sp.schedule_id AND bs.seat_id = sp.seat_id
			WHERE e.status = 'published'
			  AND ($1 = '' OR e.search_document @@ plainto_tsquery('simple', $1))
			  AND ($2 = '' OR lower(e.genre) = lower($2))
			  AND ($3 = '' OR v.country_code = $3)
			  AND ($4 = '' OR lower(v.city) = lower($4))
			  AND ($5::timestamptz IS NULL OR es.starts_at >= $5)
			  AND ($6::timestamptz IS NULL OR es.starts_at < $6)
			  AND ($7 = '' OR EXISTS (
			      SELECT 1 FROM catalog.event_tags et
			      JOIN catalog.tags t ON t.id = et.tag_id
			      WHERE et.event_id = e.id AND t.name = $7
			  ))
			GROUP BY e.id, e.title, e.genre, es.id, es.starts_at, v.name, v.city, v.country_code
			HAVING ($8::bigint IS NULL OR min(sp.price_minor) >= $8)
			   AND ($9::bigint IS NULL OR min(sp.price_minor) <= $9)
			   AND (NOT $10 OR count(sp.seat_id) > count(bs.seat_id))
		)
		SELECT *, count(*) OVER() AS total_count
		FROM results
		ORDER BY ` + order + `
		LIMIT $11 OFFSET $12`
	var from any
	if !filter.From.IsZero() {
		from = filter.From
	}
	var to any
	if !filter.To.IsZero() {
		to = filter.To
	}
	rows, err := r.pool.Query(ctx, query,
		filter.Text, filter.Genre, filter.CountryCode, filter.City, from, to, filter.Tag,
		filter.MinimumPrice, filter.MaximumPrice, filter.AvailableOnly,
		filter.PageSize, (filter.Page-1)*filter.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("search events: %w", err)
	}
	defer rows.Close()
	items := make([]domain.DiscoveryItem, 0, filter.PageSize)
	total := 0
	for rows.Next() {
		var item domain.DiscoveryItem
		if err := rows.Scan(
			&item.EventID, &item.Title, &item.Genre, &item.ScheduleID, &item.StartsAt,
			&item.VenueName, &item.City, &item.CountryCode, &item.MinimumPriceMinor,
			&item.Currency, &item.AvailableSeats, &item.AvailabilityAsOf, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan event search result: %w", err)
		}
		item.AvailabilityScope = "confirmed_bookings_only"
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate event search: %w", err)
	}
	return items, total, nil
}

func (r *Repository) ScheduleDetail(ctx context.Context, eventID, scheduleID string) (domain.ScheduleDetail, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			e.id, e.title, e.genre, e.description,
			es.id, es.starts_at, es.ends_at,
			v.name, h.name, v.city, v.country_code,
			s.id, sec.name, sr.label, s.number, s.accessible,
			sp.price_minor, sp.currency,
			(bs.seat_id IS NULL) AS available
		FROM catalog.events e
		JOIN catalog.event_schedules es ON es.event_id = e.id
		JOIN catalog.halls h ON h.id = es.hall_id
		JOIN catalog.venues v ON v.id = h.venue_id
		JOIN catalog.schedule_seat_prices sp ON sp.schedule_id = es.id
		JOIN catalog.seats s ON s.id = sp.seat_id
		JOIN catalog.seat_rows sr ON sr.id = s.row_id
		JOIN catalog.sections sec ON sec.id = sr.section_id
		LEFT JOIN reservation.booked_seats bs ON bs.schedule_id = es.id AND bs.seat_id = s.id
		WHERE e.id = $1 AND es.id = $2 AND e.status = 'published' AND es.status = 'scheduled'
		ORDER BY sec.sort_order, sr.sort_order, s.sort_order`, eventID, scheduleID)
	if err != nil {
		return domain.ScheduleDetail{}, fmt.Errorf("load schedule detail: %w", err)
	}
	defer rows.Close()

	detail := domain.ScheduleDetail{Seats: make([]domain.ScheduleSeat, 0, 32)}
	found := false
	for rows.Next() {
		var seat domain.ScheduleSeat
		if err := rows.Scan(
			&detail.EventID, &detail.Title, &detail.Genre, &detail.Description,
			&detail.ScheduleID, &detail.StartsAt, &detail.EndsAt,
			&detail.VenueName, &detail.HallName, &detail.City, &detail.CountryCode,
			&seat.SeatID, &seat.SectionName, &seat.RowLabel, &seat.SeatNumber, &seat.Accessible,
			&seat.PriceMinor, &seat.Currency, &seat.Available,
		); err != nil {
			return domain.ScheduleDetail{}, fmt.Errorf("scan schedule detail: %w", err)
		}
		detail.Seats = append(detail.Seats, seat)
		found = true
	}
	if err := rows.Err(); err != nil {
		return domain.ScheduleDetail{}, fmt.Errorf("iterate schedule detail: %w", err)
	}
	if !found {
		return domain.ScheduleDetail{}, domain.ErrNotFound
	}
	return detail, nil
}

func deterministicTagID(tag string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(tag)))
	return "tag_" + hex.EncodeToString(hash[:12])
}

func mapError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23P01":
			return domain.ErrScheduleConflict
		case "23503":
			return domain.ErrNotFound
		case "23505", "23514":
			return fmt.Errorf("%w: %s violates a catalog constraint", domain.ErrInvalidCatalog, operation)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
