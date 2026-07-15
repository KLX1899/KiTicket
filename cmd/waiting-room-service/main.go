package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/platform/config"
	"github.com/KLX1899/KiTicket/internal/platform/httpx"
	"github.com/KLX1899/KiTicket/internal/platform/id"
	"github.com/KLX1899/KiTicket/internal/platform/metrics"
	"github.com/KLX1899/KiTicket/internal/platform/server"
	"github.com/KLX1899/KiTicket/internal/reservation/adapter/redislock"
	waitinghttp "github.com/KLX1899/KiTicket/internal/waitingroom/adapter/httpapi"
	"github.com/KLX1899/KiTicket/internal/waitingroom/adapter/redisqueue"
	waitingapp "github.com/KLX1899/KiTicket/internal/waitingroom/application"
	"github.com/KLX1899/KiTicket/internal/waitingroom/token"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "waiting-room")
	if err := run(logger); err != nil {
		logger.Error("service stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	redisURL, err := config.Required("REDIS_URL")
	if err != nil {
		return err
	}
	jwtSecret, err := config.Secret("JWT_SECRET", 32)
	if err != nil {
		return err
	}
	waitingSecret, err := config.Secret("WAITING_ROOM_SIGNING_SECRET", 32)
	if err != nil {
		return err
	}
	queueTTL, err := config.Duration("WAITING_ROOM_QUEUE_TTL", time.Hour)
	if err != nil {
		return err
	}
	admissionTTL, err := config.Duration("WAITING_ROOM_ADMISSION_TTL", 5*time.Minute)
	if err != nil {
		return err
	}
	batch, err := config.Int("WAITING_ROOM_ADMISSION_BATCH", 5, 1, 1000)
	if err != nil {
		return err
	}
	client, err := redislock.NewClient(redisURL)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	store, err := redisqueue.New(client)
	if err != nil {
		return err
	}
	waitingSigner, err := token.New(waitingSecret, config.Env("WAITING_ROOM_KEY_ID", "local-1"))
	if err != nil {
		return err
	}
	service, err := waitingapp.New(store, waitingSigner, id.Generator{}, queueTTL, admissionTTL)
	if err != nil {
		return err
	}
	events := protectedEvents(config.Env("WAITING_ROOM_PROTECTED", "event_jazz=schedule_jazz_1"))
	workerCtx, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	go admitLoop(workerCtx, logger, service, events, batch)

	jwtSigner, err := auth.NewSigner(jwtSecret, config.Env("JWT_ISSUER", "kiticket-identity"), config.Env("JWT_AUDIENCE", "kiticket-api"))
	if err != nil {
		return err
	}
	handler, err := waitinghttp.New(service, jwtSigner)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	handler.Register(mux)
	server.Health{Service: "waiting-room", Ready: func(ctx context.Context) error { return client.Ping(ctx).Err() }}.Register(mux)
	registry := metrics.New()
	mux.Handle("GET /metrics", registry)
	middleware := httpx.Middleware{Logger: logger, Metrics: registry, Timeout: 5 * time.Second}
	return server.Run(logger, config.Env("HTTP_ADDRESS", ":8085"), middleware.Wrap(mux))
}

func admitLoop(ctx context.Context, logger *slog.Logger, service *waitingapp.Service, events []string, batch int) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, eventID := range events {
				admitCtx, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
				count, err := service.Admit(admitCtx, eventID, batch)
				cancel()
				if err != nil {
					logger.Error("waiting-room admission failed", "event_id", eventID, "error", err)
				} else if count > 0 {
					logger.Info("waiting-room users admitted", "event_id", eventID, "count", count)
				}
			}
		}
	}
}

func protectedEvents(configuration string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, association := range strings.Split(configuration, ",") {
		parts := strings.Split(strings.TrimSpace(association), "=")
		if len(parts) != 2 {
			continue
		}
		eventID := strings.TrimSpace(parts[0])
		if _, exists := seen[eventID]; eventID == "" || exists {
			continue
		}
		seen[eventID] = struct{}{}
		result = append(result, eventID)
	}
	return result
}
