// Package postgres persists completed bookings as the authoritative inventory state.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/KLX1899/KiTicket/internal/reservation/application"
	"github.com/KLX1899/KiTicket/internal/reservation/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) (*Repository, error) {
	if pool == nil {
		return nil, errors.New("booking repository requires a PostgreSQL pool")
	}
	return &Repository{pool: pool}, nil
}

func (r *Repository) Finalize(ctx context.Context, booking application.Booking) (application.Booking, error) {
	seats := append([]string(nil), booking.SeatIDs...)
	slices.Sort(seats)
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return application.Booking{}, fmt.Errorf("begin booking transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var existing application.Booking
	err = tx.QueryRow(ctx, `
		SELECT id, reservation_id, schedule_id, buyer_id
		FROM reservation.bookings
		WHERE reservation_id = $1`, booking.ReservationID).Scan(
		&existing.ID, &existing.ReservationID, &existing.ScheduleID, &existing.OwnerID)
	if err == nil {
		rows, queryErr := tx.Query(ctx, `
			SELECT seat_id FROM reservation.booked_seats
			WHERE booking_id = $1 ORDER BY seat_id`, existing.ID)
		if queryErr != nil {
			return application.Booking{}, fmt.Errorf("read idempotent booking seats: %w", queryErr)
		}
		for rows.Next() {
			var seatID string
			if scanErr := rows.Scan(&seatID); scanErr != nil {
				rows.Close()
				return application.Booking{}, fmt.Errorf("scan idempotent booking seat: %w", scanErr)
			}
			existing.SeatIDs = append(existing.SeatIDs, seatID)
		}
		rows.Close()
		if rows.Err() != nil {
			return application.Booking{}, fmt.Errorf("iterate idempotent booking seats: %w", rows.Err())
		}
		if existing.ScheduleID != booking.ScheduleID || existing.OwnerID != booking.OwnerID || !slices.Equal(existing.SeatIDs, seats) {
			return application.Booking{}, domain.ErrAlreadyBooked
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return application.Booking{}, fmt.Errorf("commit idempotent booking read: %w", commitErr)
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return application.Booking{}, fmt.Errorf("find idempotent booking: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT seat_id
		FROM catalog.schedule_seat_prices
		WHERE schedule_id = $1 AND seat_id = ANY($2::text[])
		ORDER BY seat_id
		FOR UPDATE`, booking.ScheduleID, seats)
	if err != nil {
		return application.Booking{}, fmt.Errorf("lock sellable seats: %w", err)
	}
	locked := make([]string, 0, len(seats))
	for rows.Next() {
		var seatID string
		if err := rows.Scan(&seatID); err != nil {
			rows.Close()
			return application.Booking{}, fmt.Errorf("scan sellable seat: %w", err)
		}
		locked = append(locked, seatID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return application.Booking{}, fmt.Errorf("read sellable seats: %w", err)
	}
	if !slices.Equal(locked, seats) {
		return application.Booking{}, fmt.Errorf("%w: one or more seats are not sellable for this schedule", domain.ErrInvalidRequest)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO reservation.bookings
			(id, reservation_id, schedule_id, buyer_id, status)
		VALUES ($1, $2, $3, $4, 'confirmed')`,
		booking.ID, booking.ReservationID, booking.ScheduleID, booking.OwnerID)
	if err != nil {
		return application.Booking{}, mapWriteError("insert booking", err)
	}
	for _, seatID := range seats {
		if _, err := tx.Exec(ctx, `
			INSERT INTO reservation.booked_seats (booking_id, schedule_id, seat_id)
			VALUES ($1, $2, $3)`, booking.ID, booking.ScheduleID, seatID); err != nil {
			return application.Booking{}, mapWriteError("insert booked seat", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return application.Booking{}, mapWriteError("commit booking", err)
	}
	return booking, nil
}

func mapWriteError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ErrAlreadyBooked
	}
	if errors.Is(err, pgx.ErrTxCommitRollback) {
		return fmt.Errorf("%s: transaction serialization failed: %w", operation, err)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
