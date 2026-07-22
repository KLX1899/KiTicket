package main

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/KLX1899/KiTicket/internal/gateway"
	"github.com/KLX1899/KiTicket/internal/gateway/ratelimit"
	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/platform/config"
	"github.com/KLX1899/KiTicket/internal/platform/httpx"
	"github.com/KLX1899/KiTicket/internal/platform/metrics"
	"github.com/KLX1899/KiTicket/internal/platform/server"
	"github.com/KLX1899/KiTicket/internal/reservation/adapter/redislock"
)

//go:embed web/*
var frontendFiles embed.FS

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "api-gateway")
	if err := run(logger); err != nil {
		logger.Error("service stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	jwtSecret, err := config.Secret("JWT_SECRET", 32)
	if err != nil {
		return err
	}
	redisURL, err := config.Required("REDIS_URL")
	if err != nil {
		return err
	}
	redisClient, err := redislock.NewClient(redisURL)
	if err != nil {
		return err
	}
	defer func() { _ = redisClient.Close() }()
	limiter, err := ratelimit.New(redisClient, 60, 20)
	if err != nil {
		return err
	}
	signer, err := auth.NewSigner(jwtSecret, config.Env("JWT_ISSUER", "kiticket-identity"), config.Env("JWT_AUDIENCE", "kiticket-api"))
	if err != nil {
		return err
	}
	handler, err := gateway.New(signer, limiter, map[string]string{
		"identity":     config.Env("IDENTITY_SERVICE_URL", "http://identity:8081"),
		"catalog":      config.Env("CATALOG_SERVICE_URL", "http://catalog:8082"),
		"reservation":  config.Env("RESERVATION_SERVICE_URL", "http://reservation:8083"),
		"checkout":     config.Env("CHECKOUT_SERVICE_URL", "http://checkout:8084"),
		"waiting-room": config.Env("WAITING_ROOM_SERVICE_URL", "http://waiting-room:8085"),
	})
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/v1/", handler)
	registerFrontend(mux)
	server.Health{Service: "api-gateway", Ready: func(ctx context.Context) error { return redisClient.Ping(ctx).Err() }}.Register(mux)
	registry := metrics.New()
	mux.Handle("GET /metrics", registry)
	middleware := httpx.Middleware{Logger: logger, Metrics: registry, Timeout: 15 * time.Second}
	return server.Run(logger, config.Env("HTTP_ADDRESS", ":8080"), middleware.Wrap(mux))
}

func registerFrontend(mux *http.ServeMux) {
	web, err := fs.Sub(frontendFiles, "web")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /", http.FileServerFS(web))
	mux.Handle("GET /app.css", http.FileServerFS(web))
	mux.Handle("GET /app.js", http.FileServerFS(web))
}
