package config

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	GeminiAPIKey  string
	GeminiModel   string
	GeminiBaseURL string // optional, empty means the public Google endpoint
	DatabaseURL   string
	Port          string
	// EvalRatePerMinute caps how many evaluations this server will start per
	// minute, to protect the provider quota from loops and double clicks.
	EvalRatePerMinute int
	// NewVocabPerDay is how many new vocabulary exercises are introduced per
	// local day. Due reviews are always shown, whatever this number is.
	NewVocabPerDay int
	// SeedFile is an optional extra JSON batch loaded at startup.
	SeedFile string
}

const (
	keyPlaceholder   = "PASTE_YOUR_KEY_HERE"
	modelPlaceholder = "PASTE_MODEL_NAME_HERE"
)

// Load reads .env (from the current directory or its parent) and then the
// process environment. Variables that are already set in the environment win
// over the file, so production can use real environment variables instead.
func Load() (Config, error) {
	LoadDotEnv(".env", "../.env")

	cfg := Config{
		GeminiAPIKey:      os.Getenv("GEMINI_API_KEY"),
		GeminiModel:       os.Getenv("GEMINI_MODEL"),
		GeminiBaseURL:     os.Getenv("GEMINI_BASE_URL"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		Port:              os.Getenv("PORT"),
		EvalRatePerMinute: 10,
		NewVocabPerDay:    8,
		SeedFile:          os.Getenv("SEED_FILE"),
	}
	if v := os.Getenv("EVAL_RATE_PER_MINUTE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 600 {
			return cfg, errors.New("EVAL_RATE_PER_MINUTE must be a number between 1 and 600")
		}
		cfg.EvalRatePerMinute = n
	}
	if v := os.Getenv("NEW_VOCAB_PER_DAY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 100 {
			return cfg, errors.New("NEW_VOCAB_PER_DAY must be a number between 0 and 100 (0 disables the limit)")
		}
		cfg.NewVocabPerDay = n
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.GeminiAPIKey == "" || cfg.GeminiAPIKey == keyPlaceholder {
		return cfg, errors.New("GEMINI_API_KEY is not set: open .env and paste your key")
	}
	if cfg.GeminiModel == "" || cfg.GeminiModel == modelPlaceholder {
		return cfg, errors.New("GEMINI_MODEL is not set: open .env and set a Flash model name from AI Studio")
	}
	return cfg, nil
}

// LoadWithDatabase is Load plus a required DATABASE_URL, for the API server.
// The evalcheck tool only needs the AI settings, so it uses Load.
func LoadWithDatabase() (Config, error) {
	cfg, err := Load()
	if err != nil {
		return cfg, err
	}
	if cfg.DatabaseURL == "" {
		return cfg, errors.New("DATABASE_URL is not set")
	}
	return cfg, nil
}

// LoadDotEnv loads KEY=VALUE lines from the first file that exists.
// It never overrides a variable that is already set. Inline comments are not
// supported: a comment must be on its own line.
func LoadDotEnv(paths ...string) {
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			k, v, ok := parseLine(sc.Text())
			if ok && os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
		return
	}
}

func parseLine(line string) (key, val string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	k, v, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	k = strings.TrimSpace(k)
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		v = v[1 : len(v)-1]
	}
	if k == "" {
		return "", "", false
	}
	return k, v, true
}
