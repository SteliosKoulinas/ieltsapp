// Package store is the only place that talks to PostgreSQL.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

var ErrNotFound = errors.New("not found")

type Store struct{ db *sql.DB }

// Open connects to PostgreSQL and waits up to 30 seconds for it to accept
// connections, because the database container may still be starting.
func Open(ctx context.Context, url string) (*Store, error) {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	deadline := time.Now().Add(30 * time.Second)
	for {
		pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = db.PingContext(pctx)
		cancel()
		if err == nil {
			return &Store{db: db}, nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			_ = db.Close()
			return nil, fmt.Errorf("database not reachable: %w", err)
		}
		time.Sleep(time.Second)
	}
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
