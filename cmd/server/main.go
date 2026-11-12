// Command server runs the comic collection web app and its JSON API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/angelospillos/haiku-pwa/internal/api"
	"github.com/angelospillos/haiku-pwa/internal/catalog"
	"github.com/angelospillos/haiku-pwa/internal/collection"
	"github.com/angelospillos/haiku-pwa/internal/comicsorg"
	"github.com/angelospillos/haiku-pwa/internal/config"
	"github.com/angelospillos/haiku-pwa/internal/inducks"
	"github.com/angelospillos/haiku-pwa/internal/lookup"
	"github.com/angelospillos/haiku-pwa/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger := newLogger(cfg.LogFormat)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.OpenCollection(ctx, cfg.CollectionDBPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	index, err := inducks.Open(ctx, cfg.InducksDBPath)
	switch {
	case errors.Is(err, inducks.ErrNotSynced):
		logger.Warn("inducks index not found; run make sync to enable inducks lookups", "path", cfg.InducksDBPath)
		index = nil
	case err != nil:
		return err
	default:
		defer func() { _ = index.Close() }()
	}

	lookups := lookup.NewService(logger,
		lookup.Source{
			Name: catalog.SourceComicsOrg,
			Searcher: comicsorg.New(comicsorg.Options{
				BaseURL: cfg.ComicsOrgAPIURL,
				Cache:   store.NewCache(db),
				Logger:  logger,
			}),
		},
		lookup.Source{Name: catalog.SourceInducks, Searcher: index},
	)

	srv := &http.Server{
		Addr: fmt.Sprintf(":%d", cfg.Port),
		Handler: api.NewHandler(api.Deps{
			Logger:           logger,
			Collection:       collection.NewStore(db, cfg.PhotosDir),
			Lookup:           lookups,
			InducksAvailable: index != nil,
			WebDir:           cfg.WebDir,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", srv.Addr, "data_dir", cfg.DataDir)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		logger.Info("shutdown requested; finishing in-flight requests")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func newLogger(format string) *slog.Logger {
	options := &slog.HandlerOptions{Level: slog.LevelInfo}
	if format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stderr, options))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, options))
}
