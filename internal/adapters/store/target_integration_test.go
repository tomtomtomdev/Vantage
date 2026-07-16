//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"plumber/internal/adapters/store"
	"plumber/internal/domain"
	"plumber/internal/platform"
	"plumber/migrations"
)

// migratedPool spins up a throwaway Postgres, migrates it, and returns a pool.
func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("plumber_test"),
		tcpostgres.WithUsername("plumber"),
		tcpostgres.WithPassword("plumber"),
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

	if err := platform.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	return pool
}

func TestAddAndListTargets(t *testing.T) {
	ctx := context.Background()
	s := store.New(migratedPool(t))

	mustTarget := func(name, url string, mode domain.Mode, opts ...domain.TargetOption) domain.Target {
		tg, err := domain.NewTarget(name, url, mode, opts...)
		if err != nil {
			t.Fatalf("NewTarget(%q): %v", name, err)
		}
		return tg
	}

	sluice := mustTarget("sluice", "http://localhost:8080", domain.ModeWhiteBox, domain.WithAllowlisted())
	other := mustTarget("other", "https://example.test", domain.ModeBlackBox)

	if _, err := s.AddTarget(ctx, sluice); err != nil {
		t.Fatalf("AddTarget(sluice): %v", err)
	}
	if _, err := s.AddTarget(ctx, other); err != nil {
		t.Fatalf("AddTarget(other): %v", err)
	}

	got, err := s.ListTargets(ctx)
	if err != nil {
		t.Fatalf("ListTargets: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d targets, want 2", len(got))
	}
	// oldest-first ordering: sluice inserted first.
	if got[0].Name != "sluice" || got[1].Name != "other" {
		t.Errorf("order = [%s, %s], want [sluice, other]", got[0].Name, got[1].Name)
	}
	// flags round-trip: sluice is allowlisted + white-box, other is neither.
	if !got[0].Allowlisted || got[0].Mode != domain.ModeWhiteBox {
		t.Errorf("sluice = %+v, want allowlisted white-box", got[0])
	}
	if got[1].Allowlisted || got[1].Mutating {
		t.Errorf("other = %+v, want safe-default flags", got[1])
	}
}

func TestAddTargetDuplicateNameIsDomainError(t *testing.T) {
	ctx := context.Background()
	s := store.New(migratedPool(t))

	tg, err := domain.NewTarget("dup", "http://x.test", domain.ModeBlackBox)
	if err != nil {
		t.Fatalf("NewTarget: %v", err)
	}
	if _, err := s.AddTarget(ctx, tg); err != nil {
		t.Fatalf("first AddTarget: %v", err)
	}

	_, err = s.AddTarget(ctx, tg)
	if !errors.Is(err, domain.ErrTargetExists) {
		t.Fatalf("duplicate name err = %v, want domain.ErrTargetExists", err)
	}
}
