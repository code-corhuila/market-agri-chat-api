// Command chat-api is the composition root of chat-service (Anexo C, Go layout): it reads the
// configuration, builds the adapters and serves HTTP until SIGTERM, then shuts down gracefully.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"github.com/code-corhuila/market-agri-chat-api/internal/adapter/in/httpapi"
	"github.com/code-corhuila/market-agri-chat-api/internal/adapter/out/catalog"
	"github.com/code-corhuila/market-agri-chat-api/internal/adapter/out/persistence"
	"github.com/code-corhuila/market-agri-chat-api/internal/application/usecase"
	"github.com/code-corhuila/market-agri-chat-api/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)) // one JSON object per line
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("chat-api stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// A wrong key stops the start now, not on the first request (ADR-020 point 7).
	verifier, err := httpapi.NewVerifier(cfg.JWTPublicKey, cfg.JWTIssuer, time.Now)
	if err != nil {
		return err
	}
	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURL).
		SetServerSelectionTimeout(cfg.MongoServerSelection).SetMaxPoolSize(cfg.MongoMaxPool))
	if err != nil {
		return err
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	ping := func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, cfg.PingTimeout)
		defer cancel()
		return client.Ping(ctx, readpref.Primary())
	}

	conversations := &usecase.Conversations{
		Store: persistence.NewMongo(client.Database(cfg.MongoDatabase)),
		Catalog: &catalog.Client{
			BaseURL: cfg.CatalogAPIURL, ServiceToken: cfg.ServiceToken, Timeout: cfg.HTTPTimeout,
			Attempts: cfg.HTTPAttempts, BaseDelay: cfg.HTTPBaseDelay, HTTP: &http.Client{},
			CorrelationID: httpapi.CorrelationID,
		},
		NewID: newUUID,
		// MongoDB keeps milliseconds: the instant is truncated so a reread equals what was written.
		Now: func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) },
	}
	auth := httpapi.Auth{Verifier: verifier}
	handler := httpapi.NewHandler(logger, func(mux *http.ServeMux) {
		httpapi.ReadinessRoute(ping)(mux)
		httpapi.ConversationRoutes(auth, conversations, cfg.MaxBodyBytes)(mux)
	})

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	logger.Info("chat-api listening", "port", cfg.Port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// newUUID: a random (version 4) UUID in lower case, the id of a new conversation.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
