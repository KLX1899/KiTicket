// Package outbox implements lease-based, duplicate-tolerant transactional outbox dispatch.
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/KLX1899/KiTicket/internal/messaging"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) (*Store, error) {
	if pool == nil {
		return nil, errors.New("outbox store requires PostgreSQL")
	}
	return &Store{pool: pool}, nil
}

type Pending struct {
	messaging.Envelope
	Attempts int
}

func (s *Store) Claim(ctx context.Context, limit int, lease time.Duration) ([]Pending, error) {
	if limit < 1 || limit > 100 || lease < time.Second {
		return nil, errors.New("invalid outbox claim options")
	}
	rows, err := s.pool.Query(ctx, `
		WITH candidates AS (
			SELECT id FROM messaging.outbox
			WHERE published_at IS NULL AND available_at <= now()
			ORDER BY available_at, occurred_at, id
			FOR UPDATE SKIP LOCKED LIMIT $1
		)
		UPDATE messaging.outbox o
		SET attempts = o.attempts + 1, available_at = now() + $2::interval
		FROM candidates c WHERE o.id = c.id
		RETURNING o.id, o.event_type, o.event_version, o.aggregate_type, o.aggregate_id,
		          o.correlation_id, coalesce(o.causation_id, ''), o.occurred_at, o.payload, o.attempts`,
		limit, lease.String())
	if err != nil {
		return nil, fmt.Errorf("claim outbox events: %w", err)
	}
	defer rows.Close()
	items := make([]Pending, 0, limit)
	for rows.Next() {
		var item Pending
		var payload []byte
		if err := rows.Scan(&item.ID, &item.Type, &item.Version, &item.AggregateType, &item.AggregateID,
			&item.CorrelationID, &item.CausationID, &item.OccurredAt, &payload, &item.Attempts); err != nil {
			return nil, fmt.Errorf("scan outbox event: %w", err)
		}
		item.Payload = json.RawMessage(payload)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Published(ctx context.Context, eventID string) error {
	result, err := s.pool.Exec(ctx, `UPDATE messaging.outbox SET published_at = now(), last_error = null WHERE id = $1 AND published_at IS NULL`, eventID)
	if err != nil {
		return fmt.Errorf("mark outbox event published: %w", err)
	}
	if result.RowsAffected() != 1 {
		return errors.New("outbox event was not pending")
	}
	return nil
}

func (s *Store) Failed(ctx context.Context, eventID string, attempts int, reason string) error {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	delay := time.Second << min(attempts, 6)
	if delay > time.Minute {
		delay = time.Minute
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE messaging.outbox SET available_at = now() + $2::interval, last_error = $3
		WHERE id = $1 AND published_at IS NULL`, eventID, delay.String(), reason)
	if err != nil {
		return fmt.Errorf("defer outbox event: %w", err)
	}
	return nil
}
