//go:build integration

package platform

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/tomtomtomdev/vantage/migrations"
)

// startPostgres spins up a throwaway Postgres and returns a connected pool.
// Shared by the platform and store integration suites.
func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("vantage_test"),
		tcpostgres.WithUsername("vantage"),
		tcpostgres.WithPassword("vantage"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatalf("starting postgres: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestMigrateAppliesSchema(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)

	if err := Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Every SPEC §6 table exists.
	for _, table := range []string{
		"targets", "slos", "runs", "reps",
		"exemplars", "spans", "query_attrib", "baselines",
	} {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`,
			table).Scan(&exists)
		if err != nil {
			t.Fatalf("checking %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %q was not created", table)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)

	if err := Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("second Migrate must be a no-op, got: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("counting migrations: %v", err)
	}
	if n == 0 {
		t.Fatal("no migrations were recorded")
	}
}

// The append-only invariant (SPEC §8) lives in the schema, so prove it: an
// UPDATE on an evidence row must raise, not silently rewrite history.
func TestEvidenceTablesAreImmutable(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	if err := Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var targetID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO targets (name, base_url, mode) VALUES ('t','http://x','black-box') RETURNING id`,
	).Scan(&targetID); err != nil {
		t.Fatalf("seeding target: %v", err)
	}
	var runID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO runs (target_id, profile, n_reps) VALUES ($1, '{}'::jsonb, 1) RETURNING id`,
		targetID,
	).Scan(&runID); err != nil {
		t.Fatalf("seeding run: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE runs SET label = 'tamper' WHERE id = $1`, runID); err == nil {
		t.Error("UPDATE on runs succeeded; evidence table must be immutable")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM runs WHERE id = $1`, runID); err == nil {
		t.Error("DELETE on runs succeeded; evidence table must be immutable")
	}
}

// The secrets gate (SPEC review #5) is a schema CHECK: a plaintext bearer token
// must be rejected at write time, not merely discouraged in a doc.
func TestTargetsRejectPlaintextAuth(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	if err := Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	_, err := pool.Exec(ctx,
		`INSERT INTO targets (name, base_url, mode, auth)
		 VALUES ('leaky','http://x','black-box','{"token":"sk-live-plaintext"}'::jsonb)`)
	if err == nil {
		t.Error("a plaintext token in auth was accepted; the secrets gate CHECK is not holding")
	}

	// An encrypted envelope / reference is allowed.
	if _, err := pool.Exec(ctx,
		`INSERT INTO targets (name, base_url, mode, auth)
		 VALUES ('safe','http://x','black-box','{"scheme":"secretbox","enc":"…","nonce":"…"}'::jsonb)`,
	); err != nil {
		t.Errorf("an encrypted auth envelope was rejected: %v", err)
	}
}
