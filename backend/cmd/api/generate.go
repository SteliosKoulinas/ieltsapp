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
	"strconv"
	"strings"
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
	wordsFile := fs.String("words", "", "vocabulary target file: word<TAB>tier<TAB>cefr per line")
	chunk := fs.Int("chunk", 24, "words per model call when -words is used (1 to 30)")
	retries := fs.Int("retries", 2, "retries for target words the model skipped (0 to 5)")
	model := fs.String("model", "", "Gemini model override (default: GEMINI_MODEL)")
	thinking := fs.Int("thinking", -1, "thinking budget for generation; 0 disables it, -1 uses the model default")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *thinking < -1 || *thinking > 32768 {
		fmt.Fprintln(os.Stderr, "generate: -thinking must be -1 or between 0 and 32768")
		return 2
	}
	if *typ != eval.TypeVocab && *typ != eval.TypeWriting {
		fmt.Fprintln(os.Stderr, "generate: -type must be vocab or writing")
		fs.Usage()
		return 2
	}
	if *wordsFile != "" {
		if *typ != eval.TypeVocab {
			fmt.Fprintln(os.Stderr, "generate: -words is only supported with -type vocab")
			return 2
		}
		if *chunk < 1 || *chunk > 30 {
			fmt.Fprintln(os.Stderr, "generate: -chunk must be between 1 and 30")
			return 2
		}
		if *retries < 0 || *retries > 5 {
			fmt.Fprintln(os.Stderr, "generate: -retries must be between 0 and 5")
			return 2
		}
	} else if *count < 1 || *count > 30 {
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
	modelName := cfg.GeminiModel
	if *model != "" {
		modelName = *model
	}
	client := eval.NewClientWithTimeout(cfg.GeminiAPIKey, modelName, cfg.GeminiBaseURL, 300*time.Second)
	if *thinking >= 0 {
		client.SetThinkingBudget(*thinking)
	}
	llm := retryLLM{inner: client, log: log, attempts: 4}
	if *wordsFile != "" {
		return runWordList(ctx, log, st, llm, outDir, *wordsFile, *chunk, *retries)
	}

	log.Info("generating content", "type", *typ, "count", *count, "model", modelName, "thinking", *thinking, "out", outDir)
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

// runWordList walks a fixed vocabulary list in chunks. Each chunk is one model
// call. Missing words are retried a few times, and words already in the
// database are skipped so a long run can be resumed safely.
func runWordList(ctx context.Context, log *slog.Logger, st *store.Store, llm generate.LLM, outDir, path string, chunk, retries int) int {
	specs, err := readWordSpecs(path)
	if err != nil {
		log.Error("cannot read the word list", "path", path, "err", err)
		return 1
	}
	if len(specs) == 0 {
		fmt.Fprintln(os.Stderr, "generate: the word list is empty")
		return 2
	}

	idx, err := st.ContentIndex(ctx, eval.TypeVocab)
	if err != nil {
		log.Error("cannot read the existing vocabulary", "err", err)
		return 1
	}
	seen := map[string]bool{}
	pending := make([]generate.WordSpec, 0, len(specs))
	for _, spec := range specs {
		key := normalizeWord(spec.Word)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if idx.Keys[key] {
			continue
		}
		pending = append(pending, spec)
	}
	log.Info("word list loaded",
		"requested", len(specs),
		"already_present", len(specs)-len(pending),
		"remaining", len(pending),
		"chunk", chunk,
		"retries", retries)
	if len(pending) == 0 {
		fmt.Println("All words already exist in the database; nothing to do.")
		return 0
	}

	var totalRequested, totalAccepted, totalInserted int
	var missing []string
	batches := (len(pending) + chunk - 1) / chunk
	for i := 0; i < len(pending); i += chunk {
		end := i + chunk
		if end > len(pending) {
			end = len(pending)
		}
		current := append([]generate.WordSpec(nil), pending[i:end]...)
		batchNo := i/chunk + 1

		for attempt := 0; attempt <= retries; attempt++ {
			res, err := generate.Run(ctx, llm, st, generate.Options{
				Type:   eval.TypeVocab,
				OutDir: outDir,
				Words:  current,
			})
			if res == nil {
				log.Error("generation failed", "batch", batchNo, "of", batches, "err", err)
				return 1
			}
			totalRequested += res.Requested
			totalAccepted += res.Accepted
			totalInserted += res.Inserted
			log.Info("batch finished",
				"batch", batchNo, "of", batches, "attempt", attempt+1,
				"requested", res.Requested,
				"accepted", res.Accepted,
				"inserted", res.Inserted,
				"skipped", res.Skipped,
				"missing", len(res.Missing),
				"batch_file", res.Path)
			if res.Accepted > 0 && err != nil {
				log.Error("batch was written but insertion failed", "batch", batchNo, "err", err)
				return 1
			}
			if len(res.Missing) == 0 {
				current = nil
				break
			}
			if attempt == retries {
				missing = append(missing, res.Missing...)
				current = nil
				break
			}
			current = specsByWord(current, res.Missing)
			if len(current) == 0 {
				break
			}
		}
	}

	fmt.Printf("requested %d, accepted %d, inserted %d\n", totalRequested, totalAccepted, totalInserted)
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "note: %d words were not generated after %d attempts: %s\n",
			len(missing), retries+1, strings.Join(missing, ", "))
		return 1
	}
	return 0
}

func readWordSpecs(path string) ([]generate.WordSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var specs []generate.WordSpec
	seen := map[string]bool{}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		word := strings.TrimSpace(fields[0])
		if word == "" {
			return nil, fmt.Errorf("line %d: empty word", i+1)
		}
		key := normalizeWord(word)
		if seen[key] {
			continue
		}
		seen[key] = true
		spec := generate.WordSpec{Word: word}
		if len(fields) > 1 && strings.TrimSpace(fields[1]) != "" {
			tier, err := strconv.Atoi(strings.TrimSpace(fields[1]))
			if err != nil || tier < 1 || tier > 4 {
				return nil, fmt.Errorf("line %d: tier must be 1 to 4", i+1)
			}
			spec.Tier = tier
		}
		if len(fields) > 2 && strings.TrimSpace(fields[2]) != "" {
			cefr := strings.ToUpper(strings.TrimSpace(fields[2]))
			if cefr != "B1" && cefr != "B2" && cefr != "C1" {
				return nil, fmt.Errorf("line %d: CEFR must be B1, B2 or C1", i+1)
			}
			spec.CEFR = cefr
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

func specsByWord(specs []generate.WordSpec, words []string) []generate.WordSpec {
	wanted := make(map[string]bool, len(words))
	for _, word := range words {
		wanted[normalizeWord(word)] = true
	}
	out := make([]generate.WordSpec, 0, len(words))
	for _, spec := range specs {
		if wanted[normalizeWord(spec.Word)] {
			out = append(out, spec)
		}
	}
	return out
}

func normalizeWord(word string) string {
	return strings.ToLower(strings.TrimSpace(word))
}

// retryLLM retries transient provider failures. A long generation run should
// not lose a whole batch because one call hit a 503 or a per-minute limit. A
// per-day limit is returned immediately: waiting will not clear it.
type retryLLM struct {
	inner    generate.LLM
	log      *slog.Logger
	attempts int
}

func (r retryLLM) Generate(ctx context.Context, system, user string) (string, error) {
	delay := 10 * time.Second
	var lastErr error
	for attempt := 1; attempt <= r.attempts; attempt++ {
		out, err := r.inner.Generate(ctx, system, user)
		if err == nil {
			return out, nil
		}
		lastErr = err
		var rl *eval.RateLimitError
		if errors.As(err, &rl) && rl.Daily() {
			return "", err
		}
		if !errors.Is(err, eval.ErrProvider) && !errors.Is(err, eval.ErrRateLimited) {
			return "", err
		}
		if attempt == r.attempts {
			break
		}
		r.log.Warn("provider call failed, retrying",
			"attempt", attempt, "of", r.attempts, "delay", delay.String(), "err", err)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
		delay *= 2
	}
	return "", lastErr
}
