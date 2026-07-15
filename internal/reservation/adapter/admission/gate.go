// Package admission validates and atomically consumes waiting-room admission tokens.
package admission

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/KLX1899/KiTicket/internal/reservation/domain"
	"github.com/KLX1899/KiTicket/internal/waitingroom/adapter/redisqueue"
	"github.com/KLX1899/KiTicket/internal/waitingroom/token"
)

type Gate struct {
	store               *redisqueue.Store
	signer              *token.Signer
	protectedBySchedule map[string]string
}

func New(store *redisqueue.Store, signer *token.Signer, configuration string) (*Gate, error) {
	if store == nil || signer == nil {
		return nil, errors.New("admission gate requires waiting-room store and signer")
	}
	protected := make(map[string]string)
	for _, association := range strings.Split(configuration, ",") {
		association = strings.TrimSpace(association)
		if association == "" {
			continue
		}
		parts := strings.Split(association, "=")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, errors.New("WAITING_ROOM_PROTECTED must contain event=schedule associations")
		}
		protected[strings.TrimSpace(parts[1])] = strings.TrimSpace(parts[0])
	}
	return &Gate{store: store, signer: signer, protectedBySchedule: protected}, nil
}

func (g *Gate) Authorize(ctx context.Context, userID, eventID, scheduleID, idempotencyKey, rawToken string) error {
	protectedEvent, protected := g.protectedBySchedule[scheduleID]
	if !protected {
		return nil
	}
	claims, err := g.signer.Verify(rawToken, token.KindAdmission)
	if err != nil || claims.UserID != userID || claims.EventID != protectedEvent || eventID != protectedEvent {
		return domain.ErrAdmissionRequired
	}
	hash := sha256.Sum256([]byte(strings.Join([]string{userID, eventID, scheduleID, idempotencyKey}, "\x00")))
	if _, err := g.store.Consume(ctx, eventID, claims.TokenID, hex.EncodeToString(hash[:])); err != nil {
		return domain.ErrAdmissionRequired
	}
	return nil
}
