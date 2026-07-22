package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/platform/config"
	"github.com/KLX1899/KiTicket/internal/platform/httpx"
	"github.com/KLX1899/KiTicket/internal/platform/id"
	"github.com/KLX1899/KiTicket/internal/platform/metrics"
	"github.com/KLX1899/KiTicket/internal/platform/server"
	"github.com/KLX1899/KiTicket/internal/reservation/adapter/admission"
	reservationhttp "github.com/KLX1899/KiTicket/internal/reservation/adapter/httpapi"
	reservationpg "github.com/KLX1899/KiTicket/internal/reservation/adapter/postgres"
	"github.com/KLX1899/KiTicket/internal/reservation/adapter/redislock"
	reservationapp "github.com/KLX1899/KiTicket/internal/reservation/application"
	"github.com/KLX1899/KiTicket/internal/waitingroom/adapter/redisqueue"
	waitingtoken "github.com/KLX1899/KiTicket/internal/waitingroom/token"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "reservation")
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
	redisURL, err := config.Required("REDIS_URL")
	if err != nil {
		return err
	}
	jwtSecret, err := config.Secret("JWT_SECRET", 32)
	if err != nil {
		return err
	}
	internalSecret, err := config.Secret("INTERNAL_API_SECRET", 32)
	if err != nil {
		return err
	}
	waitingRoomSecret, err := config.Secret("WAITING_ROOM_SIGNING_SECRET", 32)
	if err != nil {
		return err
	}
	idempotencyRetention, err := config.Duration("RESERVATION_IDEMPOTENCY_TTL", 24*time.Hour)
	if err != nil {
		return err
	}
	maxConnections, err := config.Int("POSTGRES_MAX_CONNS", 15, 2, 100)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := server.PostgreSQL(ctx, postgresURL, int32(maxConnections))
	if err != nil {
		return err
	}
	defer pool.Close()
	redisClient, err := redislock.NewClient(redisURL)
	if err != nil {
		return err
	}
	defer func() { _ = redisClient.Close() }()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return err
	}
	lockStore, err := redislock.New(redisClient, "kiticket:reservation", idempotencyRetention)
	if err != nil {
		return err
	}
	bookings, err := reservationpg.New(pool)
	if err != nil {
		return err
	}
	service, err := reservationapp.New(lockStore, bookings, id.Generator{})
	if err != nil {
		return err
	}
	waitingStore, err := redisqueue.New(redisClient)
	if err != nil {
		return err
	}
	waitingSigner, err := waitingtoken.New(waitingRoomSecret, config.Env("WAITING_ROOM_KEY_ID", "local-1"))
	if err != nil {
		return err
	}
	admissionGate, err := admission.New(waitingStore, waitingSigner, config.Env("WAITING_ROOM_PROTECTED", "event_jazz=schedule_jazz_1"))
	if err != nil {
		return err
	}
	service.WithAdmissionGate(admissionGate)
	signer, err := auth.NewSigner(jwtSecret, config.Env("JWT_ISSUER", "kiticket-identity"), config.Env("JWT_AUDIENCE", "kiticket-api"))
	if err != nil {
		return err
	}
	handler, err := reservationhttp.New(service, signer, internalSecret)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	handler.Register(mux)
	server.Health{Service: "reservation", Ready: func(ctx context.Context) error {
		return errors.Join(pool.Ping(ctx), redisClient.Ping(ctx).Err())
	}}.Register(mux)
	registry := metrics.New()
	mux.Handle("GET /metrics", registry)
	middleware := httpx.Middleware{Logger: logger, Metrics: registry, Timeout: 5 * time.Second}
	return server.Run(logger, config.Env("HTTP_ADDRESS", ":8083"), middleware.Wrap(mux))
}
