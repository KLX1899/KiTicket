// Package postgres persists notification inbox and delivery retries transactionally.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/KLX1899/KiTicket/internal/messaging"
	"github.com/KLX1899/KiTicket/internal/notification/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type IDGenerator interface{ New() (string, error) }

var ErrInvalidEvent = errors.New("invalid notification event")

type Repository struct {
	pool *pgxpool.Pool
	ids  IDGenerator
}

func New(pool *pgxpool.Pool, ids IDGenerator) (*Repository, error) {
	if pool == nil || ids == nil {
		return nil, errors.New("notification repository dependencies are required")
	}
	return &Repository{pool: pool, ids: ids}, nil
}

func (r *Repository) Accept(ctx context.Context, body []byte) (bool, error) {
	var envelope messaging.Envelope
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Validate() != nil {
		return false, fmt.Errorf("%w: malformed envelope", ErrInvalidEvent)
	}
	var payload struct {
		BuyerID string `json:"buyer_id"`
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil || payload.BuyerID == "" {
		return false, fmt.Errorf("%w: missing buyer", ErrInvalidEvent)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin notification inbox: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	result, err := tx.Exec(ctx, `
		INSERT INTO messaging.inbox (consumer_name, event_id, event_type, aggregate_id, occurred_at)
		VALUES ('notification-service', $1, $2, $3, $4)
		ON CONFLICT (consumer_name, event_id) DO NOTHING`, envelope.ID, envelope.Type, envelope.AggregateID, envelope.OccurredAt)
	if err != nil {
		return false, fmt.Errorf("insert notification inbox: %w", err)
	}
	if result.RowsAffected() == 0 {
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	// Intermediate transitions remain observable through the event stream but do
	// not generate noisy customer messages.
	if notifyEvent(envelope.Type) {
		var email string
		if err := tx.QueryRow(ctx, `SELECT email FROM identity.users WHERE id = $1 AND status = 'active'`, payload.BuyerID).Scan(&email); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return false, fmt.Errorf("%w: recipient unavailable", ErrInvalidEvent)
			}
			return false, fmt.Errorf("resolve notification recipient: %w", err)
		}
		deliveryID, err := r.ids.New()
		if err != nil {
			return false, fmt.Errorf("generate notification delivery ID: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO notification.deliveries
				(id, event_id, channel, recipient_ref, template, state)
			VALUES ($1, $2, 'email', $3, $4, 'pending')
			ON CONFLICT (event_id, channel, recipient_ref, template) DO NOTHING`,
			deliveryID, envelope.ID, email, envelope.Type); err != nil {
			return false, fmt.Errorf("create notification delivery: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit notification inbox: %w", err)
	}
	return true, nil
}

func notifyEvent(eventType string) bool {
	return eventType == "ticket.issued" || eventType == "checkout.order.failed" ||
		eventType == "checkout.order.refunded" || eventType == "checkout.order.payment_uncertain"
}

func (r *Repository) Claim(ctx context.Context, limit int, lease time.Duration) ([]application.Delivery, error) {
	if limit < 1 || limit > 100 || lease < time.Second {
		return nil, errors.New("invalid delivery claim options")
	}
	rows, err := r.pool.Query(ctx, `
		WITH candidates AS (
			SELECT id FROM notification.deliveries
			WHERE state IN ('pending', 'retrying') AND next_attempt_at <= now()
			ORDER BY next_attempt_at, id FOR UPDATE SKIP LOCKED LIMIT $1
		)
		UPDATE notification.deliveries d
		SET attempts = d.attempts + 1, next_attempt_at = now() + $2::interval, updated_at = now()
		FROM candidates c WHERE d.id = c.id
		RETURNING d.id, d.event_id, d.channel, d.recipient_ref, d.template, d.attempts`, limit, lease.String())
	if err != nil {
		return nil, fmt.Errorf("claim notification deliveries: %w", err)
	}
	defer rows.Close()
	items := make([]application.Delivery, 0, limit)
	for rows.Next() {
		var item application.Delivery
		if err := rows.Scan(&item.ID, &item.EventID, &item.Channel, &item.Recipient, &item.Template, &item.Attempts); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Sent(ctx context.Context, id string) error {
	return r.setState(ctx, id, "sent", time.Time{}, "")
}

func (r *Repository) Retry(ctx context.Context, id string, _ int, next time.Time, reason string) error {
	return r.setState(ctx, id, "retrying", next, reason)
}

func (r *Repository) DeadLetter(ctx context.Context, id, reason string) error {
	return r.setState(ctx, id, "dead_letter", time.Time{}, reason)
}

func (r *Repository) setState(ctx context.Context, id, state string, next time.Time, reason string) error {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	reason = strings.TrimSpace(reason)
	var affected int64
	if next.IsZero() {
		result, err := r.pool.Exec(ctx, `UPDATE notification.deliveries SET state=$2, last_error=nullif($3,''), updated_at=now() WHERE id=$1`, id, state, reason)
		if err != nil {
			return fmt.Errorf("update notification delivery: %w", err)
		}
		affected = result.RowsAffected()
	} else {
		result, err := r.pool.Exec(ctx, `UPDATE notification.deliveries SET state=$2, next_attempt_at=$3, last_error=nullif($4,''), updated_at=now() WHERE id=$1`, id, state, next, reason)
		if err != nil {
			return fmt.Errorf("update notification delivery: %w", err)
		}
		affected = result.RowsAffected()
	}
	if affected != 1 {
		return errors.New("notification delivery not found")
	}
	return nil
}
