package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	checkouthttp "github.com/KLX1899/KiTicket/internal/checkout/adapter/httpapi"
	"github.com/KLX1899/KiTicket/internal/checkout/adapter/payment"
	checkoutpg "github.com/KLX1899/KiTicket/internal/checkout/adapter/postgres"
	"github.com/KLX1899/KiTicket/internal/checkout/adapter/reservationhttp"
	checkoutapp "github.com/KLX1899/KiTicket/internal/checkout/application"
	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/platform/config"
	"github.com/KLX1899/KiTicket/internal/platform/httpx"
	"github.com/KLX1899/KiTicket/internal/platform/id"
	"github.com/KLX1899/KiTicket/internal/platform/metrics"
	"github.com/KLX1899/KiTicket/internal/platform/server"
	"github.com/KLX1899/KiTicket/internal/ticket"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "checkout")
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
	internalSecret, err := config.Secret("INTERNAL_API_SECRET", 32)
	if err != nil {
		return err
	}
	ticketSecret, err := config.Secret("TICKET_SIGNING_SECRET", 32)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := server.PostgreSQL(ctx, postgresURL, 15)
	if err != nil {
		return err
	}
	defer pool.Close()
	repository, err := checkoutpg.New(pool, id.Generator{})
	if err != nil {
		return err
	}
	reservationClient, err := reservationhttp.New(
		config.Env("RESERVATION_SERVICE_URL", "http://reservation:8083"), internalSecret,
		&http.Client{Timeout: 5 * time.Second},
	)
	if err != nil {
		return err
	}
	ticketIssuer, err := ticket.New(ticketSecret, id.Generator{})
	if err != nil {
		return err
	}
	service, err := checkoutapp.New(repository, reservationClient, payment.NewLocal(), ticketIssuer, id.Generator{})
	if err != nil {
		return err
	}
	signer, err := auth.NewSigner(jwtSecret, config.Env("JWT_ISSUER", "kiticket-identity"), config.Env("JWT_AUDIENCE", "kiticket-api"))
	if err != nil {
		return err
	}
	handler, err := checkouthttp.New(service, signer)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	handler.Register(mux)
	server.Health{Service: "checkout", Ready: pool.Ping}.Register(mux)
	registry := metrics.New()
	mux.Handle("GET /metrics", registry)
	middleware := httpx.Middleware{Logger: logger, Metrics: registry, Timeout: 15 * time.Second}
	return server.Run(logger, config.Env("HTTP_ADDRESS", ":8084"), middleware.Wrap(mux))
}
