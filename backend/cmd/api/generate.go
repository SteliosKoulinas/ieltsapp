package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ielts/backend/internal/config"
	"ielts/backend/internal/eval"
	"ielts/backend/internal/generate"
	"ielts/backend/internal/store"
)

// runGenerate is the manual content refresh command. It is not called by the
// server and nothing schedules it. The operator chooses the type and count.
func runGenerate(args []string, log *slog.Logger) int {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	typ := fs.String("type", "", "vocab or writing")
	count := fs.Int("count", 10, "number of new items to keep (1 to 30)")
	out := fs.String("out", "", "batch directory (default: <CONTENT_DIR>/batches)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *typ != eval.TypeVocab && *typ != eval.TypeWriting {
		fmt.Fprintln(os.Stderr, "generate: -type must be vocab or writing")
		fs.Usage()
		return 2
	}
	if *count < 1 || *count > 30 {
		fmt.Fprintln(os.Stderr, "generate: -count must be between 1 and 30")
		return 2
	}

	cfg, err := config.LoadWithDatabase()
	if err != nil {
		log.Error("configuration error", "err", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("cannot connect to the database", "err", err)
		return 1
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		log.Error("migration failed", "err", err)
		return 1
	}
	// Make sure the existing bank is present, so the duplicate check sees it
	// even if the generator is run before the API has ever started.
	if _, err := st.Seed(ctx); err != nil {
		log.Error("seeding failed", "err", err)
		return 1
	}
	for _, path := range []string{cfg.SeedFile, cfg.SeedDir} {
		if path == "" {
			continue
		}
		var n int
		if path == cfg.SeedDir {
			n, err = st.SeedDir(ctx, path)
		} else {
			n, err = st.SeedFile(ctx, path)
		}
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			log.Error("existing content failed to load", "path", path, "err", err)
			return 1
		}
		if n > 0 {
			log.Info("existing content loaded", "path", path, "new_exercises", n)
		}
	}

	outDir := *out
	if outDir == "" {
		outDir = filepath.Join(cfg.ContentDir, "batches")
	}
	llm := eval.NewClientWithTimeout(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.GeminiBaseURL, 120*time.Second)
	log.Info("generating content", "type", *typ, "count", *count, "out", outDir)
	res, err := generate.Run(ctx, llm, st, generate.Options{Type: *typ, Count: *count, OutDir: outDir})
	if err != nil {
		log.Error("generation failed", "err", err)
		if res != nil && res.Path != "" {
			log.Info("valid batch kept for a later retry", "path", res.Path, "items", res.Accepted)
		}
		return 1
	}
	log.Info("generation finished",
		"type", *typ,
		"requested", res.Requested,
		"generated", res.Generated,
		"accepted", res.Accepted,
		"skipped", res.Skipped,
		"inserted", res.Inserted,
		"batch", res.Path)
	fmt.Printf("accepted %d of %d requested, inserted %d new exercises\nbatch: %s\n",
		res.Accepted, res.Requested, res.Inserted, res.Path)
	if res.Accepted < res.Requested {
		fmt.Fprintf(os.Stderr, "note: only %d of %d valid new items were available; run the command again for more\n",
			res.Accepted, res.Requested)
	}
	return 0
}
