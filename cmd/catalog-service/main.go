package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	cataloghttp "github.com/KLX1899/KiTicket/internal/catalog/adapter/httpapi"
	catalogpg "github.com/KLX1899/KiTicket/internal/catalog/adapter/postgres"
	catalogapp "github.com/KLX1899/KiTicket/internal/catalog/application"
	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/platform/config"
	"github.com/KLX1899/KiTicket/internal/platform/httpx"
	"github.com/KLX1899/KiTicket/internal/platform/id"
	"github.com/KLX1899/KiTicket/internal/platform/metrics"
	"github.com/KLX1899/KiTicket/internal/platform/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "catalog")
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
	jwtSecret, err := config.Secret("JWT_SECRET", 32)
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
	repository, err := catalogpg.New(pool)
	if err != nil {
		return err
	}
	service, err := catalogapp.New(repository, id.Generator{})
	if err != nil {
		return err
	}
	signer, err := auth.NewSigner(jwtSecret, config.Env("JWT_ISSUER", "kiticket-identity"), config.Env("JWT_AUDIENCE", "kiticket-api"))
	if err != nil {
		return err
	}
	handler, err := cataloghttp.New(service, signer)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	handler.Register(mux)
	server.Health{Service: "catalog", Ready: pool.Ping}.Register(mux)
	registry := metrics.New()
	mux.Handle("GET /metrics", registry)
	middleware := httpx.Middleware{Logger: logger, Metrics: registry, Timeout: 10 * time.Second}
	return server.Run(logger, config.Env("HTTP_ADDRESS", ":8082"), middleware.Wrap(mux))
}
