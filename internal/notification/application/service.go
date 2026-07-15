// Package application contains notification routing and bounded retry policy.
package application

import (
	"context"
	"errors"
	"time"
)

type Delivery struct {
	ID        string
	EventID   string
	Channel   string
	Recipient string
	Template  string
	Attempts  int
}

type Repository interface {
	Accept(context.Context, []byte) (bool, error)
	Claim(context.Context, int, time.Duration) ([]Delivery, error)
	Sent(context.Context, string) error
	Retry(context.Context, string, int, time.Time, string) error
	DeadLetter(context.Context, string, string) error
}

type EmailProvider interface {
	SendEmail(context.Context, Delivery) error
}
type SMSProvider interface {
	SendSMS(context.Context, Delivery) error
}

type Service struct {
	repository Repository
	email      EmailProvider
	sms        SMSProvider
	maxRetries int
	now        func() time.Time
}

func New(repository Repository, email EmailProvider, sms SMSProvider, maxRetries int) (*Service, error) {
	if repository == nil || email == nil || sms == nil || maxRetries < 1 || maxRetries > 20 {
		return nil, errors.New("invalid notification service dependencies")
	}
	return &Service{repository: repository, email: email, sms: sms, maxRetries: maxRetries, now: time.Now}, nil
}

// Accept stores inbox and delivery records atomically. The boolean is false for a duplicate event.
func (s *Service) Accept(ctx context.Context, body []byte) (bool, error) {
	return s.repository.Accept(ctx, body)
}

func (s *Service) Deliver(ctx context.Context, limit int) (int, error) {
	deliveries, err := s.repository.Claim(ctx, limit, 30*time.Second)
	if err != nil {
		return 0, err
	}
	for _, delivery := range deliveries {
		var sendErr error
		switch delivery.Channel {
		case "email":
			sendErr = s.email.SendEmail(ctx, delivery)
		case "sms":
			sendErr = s.sms.SendSMS(ctx, delivery)
		default:
			sendErr = errors.New("unsupported notification channel")
		}
		if sendErr == nil {
			if err := s.repository.Sent(ctx, delivery.ID); err != nil {
				return 0, err
			}
			continue
		}
		if delivery.Attempts >= s.maxRetries {
			if err := s.repository.DeadLetter(ctx, delivery.ID, sendErr.Error()); err != nil {
				return 0, err
			}
			continue
		}
		delay := time.Second << min(delivery.Attempts, 6)
		if delay > time.Minute {
			delay = time.Minute
		}
		if err := s.repository.Retry(ctx, delivery.ID, delivery.Attempts, s.now().UTC().Add(delay), sendErr.Error()); err != nil {
			return 0, err
		}
	}
	return len(deliveries), nil
}
