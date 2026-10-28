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

	"ielts/backend/internal/config"
	"ielts/backend/internal/eval"
	"ielts/backend/internal/httpapi"
	"ielts/backend/internal/store"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// "api healthcheck" is used by Docker: it asks the running server for /api/health.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}

	cfg, err := config.LoadWithDatabase()
	if err != nil {
		log.Error("configuration error", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("cannot connect to the database", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := st.Migrate(ctx); err != nil {
		log.Error("migration failed", "err", err)
		os.Exit(1)
	}
	added, err := st.Seed(ctx)
	if err != nil {
		log.Error("seeding failed", "err", err)
		os.Exit(1)
	}
	if cfg.SeedFile != "" {
		n, err := st.SeedFile(ctx, cfg.SeedFile)
		switch {
		case errors.Is(err, os.ErrNotExist):
			log.Info("extra content file not found, skipping", "path", cfg.SeedFile)
		case err != nil:
			log.Error("extra content failed to load", "path", cfg.SeedFile, "err", err)
			os.Exit(1)
		default:
			added += n
			log.Info("extra content loaded", "path", cfg.SeedFile, "new_exercises", n)
		}
	}
	log.Info("database ready", "new_exercises", added)

	evaluator := eval.NewService(eval.NewClient(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.GeminiBaseURL))
	api := httpapi.New(st, evaluator, cfg.GeminiModel, cfg.EvalRatePerMinute, cfg.NewVocabPerDay, log)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Info("listening", "port", cfg.Port, "model", cfg.GeminiModel, "evals_per_minute", cfg.EvalRatePerMinute)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
	log.Info("shut down cleanly")
}

func healthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/api/health")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}
