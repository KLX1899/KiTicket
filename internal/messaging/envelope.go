// Package messaging defines the versioned wire contract shared by publishers and consumers.
package messaging

import (
	"encoding/json"
	"errors"
	"time"
)

type Envelope struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Version       int             `json:"version"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	CorrelationID string          `json:"correlation_id"`
	CausationID   string          `json:"causation_id,omitempty"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Payload       json.RawMessage `json:"payload"`
}

func (e Envelope) Validate() error {
	if e.ID == "" || e.Type == "" || e.Version < 1 || e.AggregateType == "" || e.AggregateID == "" || e.CorrelationID == "" || e.OccurredAt.IsZero() || !json.Valid(e.Payload) {
		return errors.New("invalid event envelope")
	}
	return nil
}
