package platform

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// migration is one numbered SQL file. version is the numeric prefix (e.g. "0001"),
// which is what schema_migrations records and orders by.
type migration struct {
	version string
	name    string // full filename, for error messages
	sql     string
}

// ErrMigrationOrder signals a malformed migrations set (bad name or gap in order).
var ErrMigrationOrder = errors.New("platform: malformed migration set")

// parseMigrations reads every *.sql file from fsys and returns them in lexical
// (= numeric, given zero-padded prefixes) order. Pure: no DB, no clock — so the
// ordering rule is unit-tested without a container.
func parseMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("globbing migrations: %w", err)
	}
	sort.Strings(entries)

	migs := make([]migration, 0, len(entries))
	for _, name := range entries {
		version, _, ok := strings.Cut(name, "_")
		if !ok || !allDigits(version) {
			return nil, fmt.Errorf("%w: %q must be NNNN_name.sql", ErrMigrationOrder, name)
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		if strings.TrimSpace(string(body)) == "" {
			return nil, fmt.Errorf("%w: %q is empty", ErrMigrationOrder, name)
		}
		migs = append(migs, migration{version: version, name: name, sql: string(body)})
	}
	return migs, nil
}

// allDigits reports whether s is non-empty and entirely ASCII digits — the
// numeric version prefix that makes lexical order == apply order.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// migrateDB is the slice of pgx the runner needs. pgxpool.Pool satisfies it;
// keeping it small lets a fake stand in and keeps the runner honest about what
// it touches (accept interfaces — CLAUDE §4).
type migrateDB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Migrate applies every pending migration from fsys, in order, each in its own
// transaction, recording applied versions in schema_migrations. It is
// idempotent: already-applied versions are skipped, so it is safe to run on
// every boot. Migrations are append-only — the runner never rewrites or removes
// an applied version (SPEC §8, CLAUDE §7).
func Migrate(ctx context.Context, db migrateDB, fsys fs.FS) error {
	migs, err := parseMigrations(fsys)
	if err != nil {
		return err
	}

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("ensuring schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}

	for _, m := range migs {
		if applied[m.version] {
			continue
		}
		if err := applyOne(ctx, db, m); err != nil {
			return err
		}
	}
	return nil
}

// applyOne runs a single migration and records it in the same transaction, so a
// failure leaves neither the schema change nor the bookkeeping half-done.
func applyOne(ctx context.Context, db migrateDB, m migration) (err error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin %s: %w", m.name, err)
	}
	// Roll back on any error path; the commit below makes this a no-op on success.
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if _, err = tx.Exec(ctx, m.sql); err != nil {
		return fmt.Errorf("applying %s: %w", m.name, err)
	}
	if _, err = tx.Exec(ctx,
		`INSERT INTO schema_migrations (version) VALUES ($1)`, m.version); err != nil {
		return fmt.Errorf("recording %s: %w", m.name, err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit %s: %w", m.name, err)
	}
	return nil
}

// appliedVersions returns the set of versions already recorded.
func appliedVersions(ctx context.Context, db migrateDB) (map[string]bool, error) {
	rows, err := db.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("reading applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scanning applied migration: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating applied migrations: %w", err)
	}
	return applied, nil
}
