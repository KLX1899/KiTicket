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

	"github.com/KLX1899/KiTicket/internal/messaging/outbox"
	"github.com/KLX1899/KiTicket/internal/messaging/rabbit"
	"github.com/KLX1899/KiTicket/internal/platform/config"
	"github.com/KLX1899/KiTicket/internal/platform/httpx"
	"github.com/KLX1899/KiTicket/internal/platform/metrics"
	"github.com/KLX1899/KiTicket/internal/platform/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "messaging-worker")
	if err := run(logger); err != nil {
		logger.Error("worker stopped", "error", err)
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
	pool, err := server.PostgreSQL(startup, postgresURL, 5)
	cancelStartup()
	if err != nil {
		return err
	}
	defer pool.Close()
	store, err := outbox.New(pool)
	if err != nil {
		return err
	}
	broker, err := rabbit.Open(rabbitURL)
	if err != nil {
		return err
	}
	defer broker.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go dispatch(ctx, logger, store, broker)
	mux := http.NewServeMux()
	server.Health{Service: "messaging-worker", Ready: func(ctx context.Context) error {
		if !broker.Healthy() {
			return errors.New("broker connection closed")
		}
		return pool.Ping(ctx)
	}}.Register(mux)
	registry := metrics.New()
	mux.Handle("GET /metrics", registry)
	middleware := httpx.Middleware{Logger: logger, Metrics: registry, Timeout: 5 * time.Second}
	return server.RunContext(ctx, logger, config.Env("HTTP_ADDRESS", ":8086"), middleware.Wrap(mux))
}

func dispatch(ctx context.Context, logger *slog.Logger, store *outbox.Store, broker *rabbit.Broker) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			items, err := store.Claim(ctx, 50, 30*time.Second)
			if err != nil {
				logger.ErrorContext(ctx, "claim outbox failed", "error", err)
				continue
			}
			for _, item := range items {
				publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				err := broker.Publish(publishCtx, item.Envelope)
				cancel()
				if err != nil {
					_ = store.Failed(ctx, item.ID, item.Attempts, err.Error())
					logger.WarnContext(ctx, "outbox publish deferred", "event_id", item.ID, "event_type", item.Type, "attempt", item.Attempts)
					continue
				}
				if err := store.Published(ctx, item.ID); err != nil {
					logger.ErrorContext(ctx, "outbox acknowledgement failed", "event_id", item.ID, "error", err)
				}
			}
		}
	}
}
