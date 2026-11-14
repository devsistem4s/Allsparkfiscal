package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"os"
	"path/filepath"
)

// Migrate is restricted to the database owner; migrations are checksummed and serialized.
func (s *Store) Migrate(ctx context.Context, dir string) error {
	files, e := filepath.Glob(filepath.Join(dir, "*.sql"))
	if e != nil {
		return e
	}
	if len(files) == 0 {
		return fmt.Errorf("no migrations")
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(770024301)"); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())"); e != nil {
		return e
	}
	for _, file := range files {
		raw, e := os.ReadFile(file)
		if e != nil {
			return e
		}
		hash := sha256.Sum256(raw)
		sum := hex.EncodeToString(hash[:])
		name := filepath.Base(file)
		var previous string
		e = tx.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE name=$1", name).Scan(&previous)
		if e == nil {
			if previous != sum {
				return fmt.Errorf("migration changed: %s", name)
			}
			continue
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if _, e = tx.Exec(ctx, string(raw)); e != nil {
			return fmt.Errorf("migration %s: %w", name, e)
		}
		if _, e = tx.Exec(ctx, "INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)", name, sum); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(ctx, "REVOKE ALL ON schema_migrations FROM allspark"); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
