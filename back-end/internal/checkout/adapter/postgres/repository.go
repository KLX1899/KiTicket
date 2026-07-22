// Package postgres persists the checkout saga, tickets, idempotency, and outbox atomically.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/KLX1899/KiTicket/internal/checkout/application"
	"github.com/KLX1899/KiTicket/internal/checkout/domain"
	"github.com/KLX1899/KiTicket/internal/ticket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type IDGenerator interface{ New() (string, error) }

type Repository struct {
	pool *pgxpool.Pool
	ids  IDGenerator
}

func New(pool *pgxpool.Pool, ids IDGenerator) (*Repository, error) {
	if pool == nil || ids == nil {
		return nil, errors.New("checkout repository requires PostgreSQL and an ID generator")
	}
	return &Repository{pool: pool, ids: ids}, nil
}

func (r *Repository) Begin(ctx context.Context, command application.BeginCommand) (domain.Result, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return domain.Result{}, false, fmt.Errorf("begin checkout transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	scope := "checkout:" + command.Order.BuyerID
	var savedHash, resourceID string
	err = tx.QueryRow(ctx, `
		SELECT request_hash, coalesce(resource_id, '')
		FROM checkout.idempotency_keys
		WHERE scope = $1 AND key_hash = $2 AND expires_at > now()
		FOR UPDATE`, scope, command.IdempotencyKeyHash).Scan(&savedHash, &resourceID)
	if err == nil {
		if savedHash != command.RequestHash {
			return domain.Result{}, false, domain.ErrIdempotencyMismatch
		}
		result, loadErr := loadResult(ctx, tx, resourceID)
		if loadErr != nil {
			return domain.Result{}, false, loadErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return domain.Result{}, false, fmt.Errorf("commit checkout replay: %w", commitErr)
		}
		return result, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Result{}, false, fmt.Errorf("read checkout idempotency key: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT sp.seat_id, sp.price_minor, sp.currency, (bs.seat_id IS NOT NULL) AS booked
		FROM catalog.schedule_seat_prices sp
		LEFT JOIN reservation.booked_seats bs
		  ON bs.schedule_id = sp.schedule_id AND bs.seat_id = sp.seat_id
		WHERE sp.schedule_id = $1 AND sp.seat_id = ANY($2::text[])
		ORDER BY sp.seat_id
		FOR SHARE OF sp`, command.Order.ScheduleID, command.Order.SeatIDs)
	if err != nil {
		return domain.Result{}, false, fmt.Errorf("price order seats: %w", err)
	}
	pricedSeats := make([]string, 0, len(command.Order.SeatIDs))
	prices := make(map[string]int64, len(command.Order.SeatIDs))
	currency := ""
	amount := int64(0)
	for rows.Next() {
		var seatID, rowCurrency string
		var price int64
		var booked bool
		if err := rows.Scan(&seatID, &price, &rowCurrency, &booked); err != nil {
			rows.Close()
			return domain.Result{}, false, fmt.Errorf("scan order seat price: %w", err)
		}
		if booked {
			rows.Close()
			return domain.Result{}, false, fmt.Errorf("%w: seat %s is already booked", domain.ErrInvalidOrder, seatID)
		}
		if currency != "" && currency != rowCurrency {
			rows.Close()
			return domain.Result{}, false, fmt.Errorf("%w: mixed currencies", domain.ErrInvalidOrder)
		}
		currency = rowCurrency
		amount += price
		prices[seatID] = price
		pricedSeats = append(pricedSeats, seatID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return domain.Result{}, false, fmt.Errorf("iterate order seat prices: %w", err)
	}
	wantedSeats := append([]string(nil), command.Order.SeatIDs...)
	slices.Sort(wantedSeats)
	if !slices.Equal(pricedSeats, wantedSeats) || currency == "" {
		return domain.Result{}, false, fmt.Errorf("%w: one or more seats are not sellable", domain.ErrInvalidOrder)
	}
	order := command.Order
	order.AmountMinor = amount
	order.Currency = currency
	if _, err := tx.Exec(ctx, `
		INSERT INTO checkout.orders
			(id, buyer_id, reservation_id, reservation_fence, schedule_id, state, amount_minor, currency, state_version)
		VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7, 1)`,
		order.ID, order.BuyerID, order.ReservationID, order.ReservationFence, order.ScheduleID, amount, currency); err != nil {
		return domain.Result{}, false, mapError("insert checkout order", err)
	}
	for _, seatID := range wantedSeats {
		if _, err := tx.Exec(ctx, `INSERT INTO checkout.order_seats (order_id, seat_id, price_minor) VALUES ($1, $2, $3)`, order.ID, seatID, prices[seatID]); err != nil {
			return domain.Result{}, false, mapError("insert checkout order seat", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO checkout.idempotency_keys
			(scope, key_hash, request_hash, resource_id, expires_at)
		VALUES ($1, $2, $3, $4, $5)`, scope, command.IdempotencyKeyHash, command.RequestHash, order.ID, command.IdempotencyExpiry); err != nil {
		return domain.Result{}, false, mapError("insert checkout idempotency key", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO checkout.order_transitions (order_id, version, from_state, to_state, correlation_id)
		VALUES ($1, 1, NULL, 'pending', $2)`, order.ID, command.CorrelationID); err != nil {
		return domain.Result{}, false, fmt.Errorf("record initial order transition: %w", err)
	}
	if err := r.outbox(ctx, tx, order, "checkout.order.pending", command.CorrelationID); err != nil {
		return domain.Result{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Result{}, false, mapError("commit checkout order", err)
	}
	order.SeatIDs = wantedSeats
	order.CreatedAt = time.Now().UTC()
	order.UpdatedAt = order.CreatedAt
	return domain.Result{Order: order}, false, nil
}

func (r *Repository) Transition(ctx context.Context, order domain.Order, to domain.State, reason, correlationID string, payment *application.PaymentRecord) (domain.Order, error) {
	if !order.CanTransition(to) {
		return domain.Order{}, domain.ErrInvalidTransition
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Order{}, fmt.Errorf("begin order transition: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if payment != nil {
		if _, err := tx.Exec(ctx, `
			INSERT INTO checkout.payments
				(id, order_id, provider, provider_reference, status, amount_minor, currency)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (provider, provider_reference) DO UPDATE
			SET status = EXCLUDED.status, updated_at = now()`,
			payment.ID, order.ID, payment.Provider, payment.ProviderReference, payment.Status, order.AmountMinor, order.Currency); err != nil {
			return domain.Order{}, mapError("persist payment transition", err)
		}
	}
	result, err := tx.Exec(ctx, `
		UPDATE checkout.orders
		SET state = $1, state_version = state_version + 1, failure_code = nullif($2, ''),
		    booking_id = coalesce(nullif($3, ''), booking_id), updated_at = now()
		WHERE id = $4 AND state = $5 AND state_version = $6`,
		to, reason, order.BookingID, order.ID, order.State, order.Version)
	if err != nil {
		return domain.Order{}, fmt.Errorf("update order transition: %w", err)
	}
	if result.RowsAffected() != 1 {
		return domain.Order{}, domain.ErrTransitionConflict
	}
	newVersion := order.Version + 1
	if _, err := tx.Exec(ctx, `
		INSERT INTO checkout.order_transitions
			(order_id, version, from_state, to_state, reason, correlation_id)
		VALUES ($1, $2, $3, $4, nullif($5, ''), $6)`,
		order.ID, newVersion, order.State, to, reason, correlationID); err != nil {
		return domain.Order{}, fmt.Errorf("record order transition: %w", err)
	}
	order.State = to
	order.Version = newVersion
	order.FailureCode = reason
	order.UpdatedAt = time.Now().UTC()
	if payment != nil {
		order.PaymentID = payment.ID
		order.PaymentProvider = payment.Provider
		order.ProviderReference = payment.ProviderReference
	}
	if err := r.outbox(ctx, tx, order, "checkout.order."+string(to), correlationID); err != nil {
		return domain.Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Order{}, mapError("commit order transition", err)
	}
	return order, nil
}

func (r *Repository) Complete(ctx context.Context, order domain.Order, bookingID string, materials []ticket.Material, correlationID string) (domain.Result, error) {
	if order.State != domain.StateBookingConfirmed || bookingID == "" || len(materials) != len(order.SeatIDs) {
		return domain.Result{}, domain.ErrInvalidTransition
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Result{}, fmt.Errorf("begin ticket issuance: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	result, err := tx.Exec(ctx, `
		UPDATE checkout.orders
		SET state = 'ticket_issued', state_version = state_version + 1, booking_id = $1, updated_at = now()
		WHERE id = $2 AND state = 'booking_confirmed' AND state_version = $3`, bookingID, order.ID, order.Version)
	if err != nil {
		return domain.Result{}, fmt.Errorf("mark tickets issued: %w", err)
	}
	if result.RowsAffected() != 1 {
		loaded, loadErr := loadResult(ctx, tx, order.ID)
		if loadErr == nil && loaded.Order.State == domain.StateCompleted {
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return domain.Result{}, commitErr
			}
			return loaded, nil
		}
		return domain.Result{}, domain.ErrTransitionConflict
	}
	issuedVersion := order.Version + 1
	if _, err := tx.Exec(ctx, `
		INSERT INTO checkout.order_transitions (order_id, version, from_state, to_state, correlation_id)
		VALUES ($1, $2, 'booking_confirmed', 'ticket_issued', $3)`, order.ID, issuedVersion, correlationID); err != nil {
		return domain.Result{}, fmt.Errorf("record ticket-issued transition: %w", err)
	}
	tickets := make([]domain.Ticket, 0, len(materials))
	for _, material := range materials {
		if _, err := tx.Exec(ctx, `
			INSERT INTO checkout.tickets
				(id, booking_id, order_id, seat_id, token_digest, qr_signature, qr_payload)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			material.ID, bookingID, order.ID, material.SeatID, material.TokenDigest, material.Signature, material.QRPayload); err != nil {
			return domain.Result{}, mapError("insert ticket", err)
		}
		tickets = append(tickets, domain.Ticket{ID: material.ID, SeatID: material.SeatID, QRPayload: material.QRPayload, IssuedAt: time.Now().UTC()})
	}
	result, err = tx.Exec(ctx, `
		UPDATE checkout.orders
		SET state = 'completed', state_version = state_version + 1, updated_at = now()
		WHERE id = $1 AND state = 'ticket_issued' AND state_version = $2`, order.ID, issuedVersion)
	if err != nil || result.RowsAffected() != 1 {
		return domain.Result{}, domain.ErrTransitionConflict
	}
	completedVersion := issuedVersion + 1
	if _, err := tx.Exec(ctx, `
		INSERT INTO checkout.order_transitions (order_id, version, from_state, to_state, correlation_id)
		VALUES ($1, $2, 'ticket_issued', 'completed', $3)`, order.ID, completedVersion, correlationID); err != nil {
		return domain.Result{}, fmt.Errorf("record completed transition: %w", err)
	}
	order.State = domain.StateCompleted
	order.Version = completedVersion
	order.BookingID = bookingID
	order.UpdatedAt = time.Now().UTC()
	if err := r.outbox(ctx, tx, order, "ticket.issued", correlationID); err != nil {
		return domain.Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Result{}, mapError("commit ticket issuance", err)
	}
	return domain.Result{Order: order, Tickets: tickets}, nil
}

func (r *Repository) Ticket(ctx context.Context, ticketID string, digest []byte) (domain.Ticket, bool, error) {
	var record domain.Ticket
	var revokedAt *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT id, seat_id, qr_payload, issued_at, revoked_at
		FROM checkout.tickets
		WHERE id = $1 AND token_digest = $2`, ticketID, digest).Scan(
		&record.ID, &record.SeatID, &record.QRPayload, &record.IssuedAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Ticket{}, false, nil
	}
	if err != nil {
		return domain.Ticket{}, false, fmt.Errorf("verify ticket record: %w", err)
	}
	record.RevokedAt = revokedAt
	return record, true, nil
}

func (r *Repository) RevokeTicket(ctx context.Context, ticketID, reason string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE checkout.tickets
		SET revoked_at = coalesce(revoked_at, now()), revocation_reason = coalesce(revocation_reason, $2)
		WHERE id = $1`, ticketID, reason)
	if err != nil {
		return fmt.Errorf("revoke ticket: %w", err)
	}
	if result.RowsAffected() != 1 {
		return domain.ErrNotFound
	}
	return nil
}

type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func loadResult(ctx context.Context, query querier, orderID string) (domain.Result, error) {
	var order domain.Order
	err := query.QueryRow(ctx, `
		SELECT o.id, o.buyer_id, o.reservation_id, o.reservation_fence, o.schedule_id,
		       o.state, o.amount_minor, o.currency, o.state_version,
		       coalesce(p.id, ''), coalesce(p.provider, ''), coalesce(p.provider_reference, ''),
		       coalesce(o.booking_id, ''), coalesce(o.failure_code, ''), o.created_at, o.updated_at
		FROM checkout.orders o
		LEFT JOIN LATERAL (
			SELECT id, provider, provider_reference FROM checkout.payments
			WHERE order_id = o.id ORDER BY created_at DESC, id DESC LIMIT 1
		) p ON true
		WHERE o.id = $1`, orderID).Scan(
		&order.ID, &order.BuyerID, &order.ReservationID, &order.ReservationFence, &order.ScheduleID,
		&order.State, &order.AmountMinor, &order.Currency, &order.Version,
		&order.PaymentID, &order.PaymentProvider, &order.ProviderReference,
		&order.BookingID, &order.FailureCode, &order.CreatedAt, &order.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Result{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Result{}, fmt.Errorf("load checkout order: %w", err)
	}
	rows, err := query.Query(ctx, `SELECT seat_id FROM checkout.order_seats WHERE order_id = $1 ORDER BY seat_id`, orderID)
	if err != nil {
		return domain.Result{}, fmt.Errorf("load checkout seats: %w", err)
	}
	for rows.Next() {
		var seatID string
		if err := rows.Scan(&seatID); err != nil {
			rows.Close()
			return domain.Result{}, fmt.Errorf("scan checkout seat: %w", err)
		}
		order.SeatIDs = append(order.SeatIDs, seatID)
	}
	rows.Close()
	if rows.Err() != nil {
		return domain.Result{}, rows.Err()
	}
	tickets := make([]domain.Ticket, 0)
	rows, err = query.Query(ctx, `
		SELECT id, seat_id, qr_payload, issued_at, revoked_at
		FROM checkout.tickets WHERE order_id = $1 ORDER BY seat_id`, orderID)
	if err != nil {
		return domain.Result{}, fmt.Errorf("load checkout tickets: %w", err)
	}
	for rows.Next() {
		var record domain.Ticket
		if err := rows.Scan(&record.ID, &record.SeatID, &record.QRPayload, &record.IssuedAt, &record.RevokedAt); err != nil {
			rows.Close()
			return domain.Result{}, fmt.Errorf("scan checkout ticket: %w", err)
		}
		tickets = append(tickets, record)
	}
	rows.Close()
	return domain.Result{Order: order, Tickets: tickets}, rows.Err()
}

func (r *Repository) outbox(ctx context.Context, tx pgx.Tx, order domain.Order, eventType, correlationID string) error {
	eventID, err := r.ids.New()
	if err != nil {
		return fmt.Errorf("generate outbox event ID: %w", err)
	}
	payload, err := json.Marshal(map[string]any{
		"order_id": order.ID, "buyer_id": order.BuyerID, "state": order.State,
		"schedule_id": order.ScheduleID, "booking_id": order.BookingID,
	})
	if err != nil {
		return fmt.Errorf("encode outbox event: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO messaging.outbox
			(id, owner_context, aggregate_type, aggregate_id, event_type, event_version,
			 correlation_id, payload, occurred_at)
		VALUES ($1, 'checkout', 'order', $2, $3, 1, $4, $5, now())`,
		eventID, order.ID, eventType, correlationID, payload)
	if err != nil {
		return fmt.Errorf("insert transactional outbox event: %w", err)
	}
	return nil
}

func mapError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return domain.ErrTransitionConflict
		case "40001":
			return domain.ErrTransitionConflict
		case "23503", "23514":
			return fmt.Errorf("%w: %s", domain.ErrInvalidOrder, operation)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
