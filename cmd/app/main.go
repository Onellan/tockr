package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"tockr/internal/db/sqlite"
	"tockr/internal/platform/config"
	httpserver "tockr/internal/platform/http"
	"tockr/internal/webhooks"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := sqlite.Open(ctx, cfg.DatabasePath)
	if err != nil {
		log.Error("open database failed", "err", err)
		os.Exit(1)
	}
	defer store.Close()

	if err := store.SeedAdmin(ctx, cfg.AdminEmail, cfg.AdminPassword, cfg.DefaultTimezone, cfg.DefaultCurrency); err != nil {
		log.Error("seed admin failed", "err", err)
		os.Exit(1)
	}

	worker := webhooks.NewWorker(store, log, cfg.WebhookMaxRetries)
	go worker.Run(ctx)

	app := httpserver.New(cfg, store, log)
	server := &http.Server{Addr: cfg.Addr, Handler: app.Handler()}

	go func() {
		log.Info("server listening", "addr", cfg.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "err", err)
	}
}
