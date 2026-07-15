package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/KLX1899/KiTicket/internal/messaging/rabbit"
	notificationpg "github.com/KLX1899/KiTicket/internal/notification/adapter/postgres"
	"github.com/KLX1899/KiTicket/internal/notification/adapter/provider"
	notificationapp "github.com/KLX1899/KiTicket/internal/notification/application"
	"github.com/KLX1899/KiTicket/internal/platform/config"
	"github.com/KLX1899/KiTicket/internal/platform/httpx"
	"github.com/KLX1899/KiTicket/internal/platform/id"
	"github.com/KLX1899/KiTicket/internal/platform/metrics"
	"github.com/KLX1899/KiTicket/internal/platform/server"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "notification")
	if err := run(logger); err != nil {
		logger.Error("service stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	postgresURL, err := config.Required("POSTGRES_URL")
	if err != nil {
		return err
	}
	rabbitURL, err := config.Required("RABBITMQ_URL")
	if err != nil {
		return err
	}
	startup, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := server.PostgreSQL(startup, postgresURL, 8)
	cancelStartup()
	if err != nil {
		return err
	}
	defer pool.Close()
	repository, err := notificationpg.New(pool, id.Generator{})
	if err != nil {
		return err
	}
	local, err := provider.NewLocal(logger)
	if err != nil {
		return err
	}
	service, err := notificationapp.New(repository, local, local, 5)
	if err != nil {
		return err
	}
	broker, err := rabbit.Open(rabbitURL)
	if err != nil {
		return err
	}
	defer broker.Close()
	deliveries, err := broker.Consume(20)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go consume(ctx, logger, service, deliveries)
	go retry(ctx, logger, service)
	mux := http.NewServeMux()
	server.Health{Service: "notification", Ready: func(ctx context.Context) error {
		if !broker.Healthy() {
			return errors.New("broker connection closed")
		}
		return pool.Ping(ctx)
	}}.Register(mux)
	registry := metrics.New()
	mux.Handle("GET /metrics", registry)
	middleware := httpx.Middleware{Logger: logger, Metrics: registry, Timeout: 5 * time.Second}
	return server.RunContext(ctx, logger, config.Env("HTTP_ADDRESS", ":8087"), middleware.Wrap(mux))
}

func consume(ctx context.Context, logger *slog.Logger, service *notificationapp.Service, deliveries <-chan amqp.Delivery) {
	for {
		select {
		case <-ctx.Done():
			return
		case message, open := <-deliveries:
			if !open {
				logger.ErrorContext(ctx, "notification broker delivery stream closed")
				return
			}
			_, err := service.Accept(ctx, message.Body)
			if err == nil {
				if ackErr := message.Ack(false); ackErr != nil {
					logger.ErrorContext(ctx, "notification acknowledgement failed", "event_id", message.MessageId)
				}
				continue
			}
			if errors.Is(err, notificationpg.ErrInvalidEvent) {
				_ = message.Nack(false, false)
				logger.WarnContext(ctx, "poison notification event dead-lettered", "event_id", message.MessageId)
				continue
			}
			_ = message.Nack(false, true)
			logger.ErrorContext(ctx, "notification event processing deferred", "event_id", message.MessageId, "error", err)
		}
	}
}

func retry(ctx context.Context, logger *slog.Logger, service *notificationapp.Service) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := service.Deliver(ctx, 50); err != nil && !errors.Is(err, context.Canceled) {
				logger.ErrorContext(ctx, "notification delivery cycle failed", "error", err)
			}
		}
	}
}
