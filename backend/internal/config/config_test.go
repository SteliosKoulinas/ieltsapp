package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLine(t *testing.T) {
	tests := []struct {
		in     string
		k, v   string
		wantOK bool
	}{
		{"A=b", "A", "b", true},
		{"  A = b  ", "A", "b", true},
		{`A="b c"`, "A", "b c", true},
		{"A='b'", "A", "b", true},
		{"A=", "A", "", true},
		{"# comment", "", "", false},
		{"", "", "", false},
		{"no equals", "", "", false},
		{"=x", "", "", false},
	}
	for _, tc := range tests {
		k, v, ok := parseLine(tc.in)
		if k != tc.k || v != tc.v || ok != tc.wantOK {
			t.Errorf("parseLine(%q) = %q,%q,%v", tc.in, k, v, ok)
		}
	}
}

func TestLoadRejectsPlaceholders(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	_ = os.WriteFile(p, []byte("GEMINI_API_KEY=PASTE_YOUR_KEY_HERE\nGEMINI_MODEL=PASTE_MODEL_NAME_HERE\n"), 0o600)
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GEMINI_MODEL", "")
	chdir(t, dir)
	if _, err := Load(); err == nil {
		t.Fatal("expected error for placeholder values")
	}
}

func TestEnvironmentWinsOverFile(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, ".env"), []byte("GEMINI_API_KEY=from-file\nGEMINI_MODEL=m\n"), 0o600)
	t.Setenv("GEMINI_API_KEY", "from-env")
	t.Setenv("GEMINI_MODEL", "")
	chdir(t, dir)
	cfg, err := Load()
	if err != nil || cfg.GeminiAPIKey != "from-env" || cfg.GeminiModel != "m" {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}

// chdir changes the working directory for one test (t.Chdir needs Go 1.24).
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestLoadWithDatabaseRequiresURL(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "k")
	t.Setenv("GEMINI_MODEL", "m")
	t.Setenv("DATABASE_URL", "")
	chdir(t, t.TempDir())
	if _, err := LoadWithDatabase(); err == nil {
		t.Fatal("expected an error when DATABASE_URL is missing")
	}
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := LoadWithDatabase()
	if err != nil || cfg.EvalRatePerMinute != 10 {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
	if cfg.NewVocabPerDay != 0 {
		t.Errorf("default NEW_VOCAB_PER_DAY should be 0 (no limit), got %d", cfg.NewVocabPerDay)
	}
}

func TestNewVocabPerDayValidation(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "k")
	t.Setenv("GEMINI_MODEL", "m")
	chdir(t, t.TempDir())
	for _, bad := range []string{"-1", "abc", "101"} {
		t.Setenv("NEW_VOCAB_PER_DAY", bad)
		if _, err := Load(); err == nil {
			t.Errorf("NEW_VOCAB_PER_DAY=%q should be rejected", bad)
		}
	}
	for _, tc := range []struct {
		in   string
		want int
	}{{"0", 0}, {"5", 5}, {"100", 100}} {
		t.Setenv("NEW_VOCAB_PER_DAY", tc.in)
		if cfg, err := Load(); err != nil || cfg.NewVocabPerDay != tc.want {
			t.Errorf("NEW_VOCAB_PER_DAY=%q: cfg=%+v err=%v", tc.in, cfg, err)
		}
	}
}

func TestEvalRatePerMinuteValidation(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "k")
	t.Setenv("GEMINI_MODEL", "m")
	chdir(t, t.TempDir())
	for _, bad := range []string{"0", "-1", "abc", "9999"} {
		t.Setenv("EVAL_RATE_PER_MINUTE", bad)
		if _, err := Load(); err == nil {
			t.Errorf("EVAL_RATE_PER_MINUTE=%q should be rejected", bad)
		}
	}
	t.Setenv("EVAL_RATE_PER_MINUTE", "12")
	if cfg, err := Load(); err != nil || cfg.EvalRatePerMinute != 12 {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}
