// Package storetest gives integration tests an isolated PostgreSQL schema.
// Tests are skipped unless TEST_DATABASE_URL points at a database they may use.
package storetest

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"ielts/backend/internal/store"
)

// New returns a migrated Store in a fresh schema that is dropped when the test ends.
func New(t testing.TB) *store.Store {
	t.Helper()
	s, _ := NewWithDB(t)
	return s
}

// NewWithDB is New plus a raw connection to the same schema, for tests that
// need to arrange data directly (for example to backdate a timestamp).
func NewWithDB(t testing.TB) (*store.Store, *sql.DB) {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL is not set, skipping database test")
	}
	schema := fmt.Sprintf("t_%d", time.Now().UnixNano())

	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		_ = admin.Close()
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	ctx := context.Background()
	s, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	return s, raw
}
