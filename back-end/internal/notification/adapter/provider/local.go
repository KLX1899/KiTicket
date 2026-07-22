// Package provider contains development notification providers that never expose recipient data in logs.
package provider

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/KLX1899/KiTicket/internal/notification/application"
)

type Local struct{ logger *slog.Logger }

func NewLocal(logger *slog.Logger) (*Local, error) {
	if logger == nil {
		return nil, errors.New("local notification provider requires a logger")
	}
	return &Local{logger: logger}, nil
}

func (p *Local) SendEmail(ctx context.Context, delivery application.Delivery) error {
	return p.send(ctx, delivery, "email")
}

func (p *Local) SendSMS(ctx context.Context, delivery application.Delivery) error {
	return p.send(ctx, delivery, "sms")
}

func (p *Local) send(ctx context.Context, delivery application.Delivery, channel string) error {
	// A deterministic marker supports failure-path tests without random behavior.
	if strings.HasPrefix(delivery.Recipient, "transient-failure:") {
		return errors.New("simulated transient provider failure")
	}
	p.logger.InfoContext(ctx, "development notification delivered",
		"delivery_id", delivery.ID, "event_id", delivery.EventID,
		"channel", channel, "template", delivery.Template)
	return nil
}
