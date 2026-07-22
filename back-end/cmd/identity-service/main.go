package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	identityhttp "github.com/KLX1899/KiTicket/internal/identity/adapter/httpapi"
	"github.com/KLX1899/KiTicket/internal/identity/adapter/password"
	identitypg "github.com/KLX1899/KiTicket/internal/identity/adapter/postgres"
	identityapp "github.com/KLX1899/KiTicket/internal/identity/application"
	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/platform/config"
	"github.com/KLX1899/KiTicket/internal/platform/httpx"
	"github.com/KLX1899/KiTicket/internal/platform/id"
	"github.com/KLX1899/KiTicket/internal/platform/metrics"
	"github.com/KLX1899/KiTicket/internal/platform/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "identity")
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
	accessTTL, err := config.Duration("ACCESS_TOKEN_TTL", 15*time.Minute)
	if err != nil {
		return err
	}
	maxConnections, err := config.Int("POSTGRES_MAX_CONNS", 10, 2, 100)
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
	repository, err := identitypg.New(pool)
	if err != nil {
		return err
	}
	signer, err := auth.NewSigner(jwtSecret, config.Env("JWT_ISSUER", "kiticket-identity"), config.Env("JWT_AUDIENCE", "kiticket-api"))
	if err != nil {
		return err
	}
	service, err := identityapp.New(repository, password.Argon2id{}, signer, id.Generator{}, accessTTL)
	if err != nil {
		return err
	}
	handler, err := identityhttp.New(service)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	handler.Register(mux)
	server.Health{Service: "identity", Ready: pool.Ping}.Register(mux)
	registry := metrics.New()
	mux.Handle("GET /metrics", registry)
	middleware := httpx.Middleware{Logger: logger, Metrics: registry, Timeout: 10 * time.Second}
	return server.Run(logger, config.Env("HTTP_ADDRESS", ":8081"), middleware.Wrap(mux))
}
