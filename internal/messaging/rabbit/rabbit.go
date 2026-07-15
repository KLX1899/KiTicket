// Package rabbit provides durable RabbitMQ topology and publisher confirms.
package rabbit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/KLX1899/KiTicket/internal/messaging"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	Exchange          = "kiticket.events.v1"
	DeadExchange      = "kiticket.dead.v1"
	NotificationQueue = "kiticket.notifications.v1"
	DeadQueue         = "kiticket.notifications.dead.v1"
)

type Broker struct {
	connection *amqp.Connection
	channel    *amqp.Channel
}

func (b *Broker) Healthy() bool {
	return b != nil && b.connection != nil && !b.connection.IsClosed()
}

func Open(rawURL string) (*Broker, error) {
	if rawURL == "" {
		return nil, errors.New("RabbitMQ URL is required")
	}
	connection, err := amqp.Dial(rawURL)
	if err != nil {
		return nil, fmt.Errorf("connect RabbitMQ: %w", err)
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("open RabbitMQ channel: %w", err)
	}
	b := &Broker{connection: connection, channel: channel}
	if err := b.declare(); err != nil {
		_ = b.Close()
		return nil, err
	}
	if err := channel.Confirm(false); err != nil {
		_ = b.Close()
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}
	return b, nil
}

func (b *Broker) declare() error {
	if err := b.channel.ExchangeDeclare(Exchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare event exchange: %w", err)
	}
	if err := b.channel.ExchangeDeclare(DeadExchange, "fanout", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead-letter exchange: %w", err)
	}
	if _, err := b.channel.QueueDeclare(DeadQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead-letter queue: %w", err)
	}
	if err := b.channel.QueueBind(DeadQueue, "", DeadExchange, false, nil); err != nil {
		return fmt.Errorf("bind dead-letter queue: %w", err)
	}
	arguments := amqp.Table{"x-dead-letter-exchange": DeadExchange}
	if _, err := b.channel.QueueDeclare(NotificationQueue, true, false, false, false, arguments); err != nil {
		return fmt.Errorf("declare notification queue: %w", err)
	}
	if err := b.channel.QueueBind(NotificationQueue, "#", Exchange, false, nil); err != nil {
		return fmt.Errorf("bind notification queue: %w", err)
	}
	return nil
}

func (b *Broker) Publish(ctx context.Context, event messaging.Envelope) error {
	if err := event.Validate(); err != nil {
		return err
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode event envelope: %w", err)
	}
	deferred, err := b.channel.PublishWithDeferredConfirmWithContext(ctx, Exchange, event.Type, true, false, amqp.Publishing{
		DeliveryMode: amqp.Persistent, ContentType: "application/json", Type: event.Type,
		MessageId: event.ID, CorrelationId: event.CorrelationID, Timestamp: event.OccurredAt, Body: body,
	})
	if err != nil {
		return fmt.Errorf("publish event: %w", err)
	}
	if deferred == nil {
		return errors.New("publisher confirmation is unavailable")
	}
	confirmed, err := deferred.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("wait for publisher confirmation: %w", err)
	}
	if !confirmed {
		return errors.New("broker rejected event")
	}
	return nil
}

func (b *Broker) Consume(prefetch int) (<-chan amqp.Delivery, error) {
	if prefetch < 1 || prefetch > 1000 {
		return nil, errors.New("invalid consumer prefetch")
	}
	if err := b.channel.Qos(prefetch, 0, false); err != nil {
		return nil, fmt.Errorf("set consumer prefetch: %w", err)
	}
	deliveries, err := b.channel.Consume(NotificationQueue, "", false, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("consume notifications: %w", err)
	}
	return deliveries, nil
}

func (b *Broker) Close() error {
	if b.channel != nil {
		_ = b.channel.Close()
	}
	if b.connection != nil {
		return b.connection.Close()
	}
	return nil
}
