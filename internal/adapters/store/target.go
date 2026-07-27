package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tomtomtomdev/vantage/internal/domain"
	"github.com/tomtomtomdev/vantage/internal/ports"
)

// AddTarget inserts t and returns its assigned id. A duplicate name maps to the
// domain sentinel domain.ErrTargetExists so callers inspect with errors.Is
// rather than sniffing pg error strings.
//
// request_template defaults to '{}' and auth is left NULL here: S0 registers the
// target's identity and blast-radius flags; request bodies and (encrypted) auth
// are populated when S1 needs them, behind the secrets gate.
func (s *PgStore) AddTarget(ctx context.Context, t domain.Target) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO targets (name, base_url, mode, mutating, allowlisted)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		t.Name, t.BaseURL, string(t.Mode), t.Mutating, t.Allowlisted,
	).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return 0, fmt.Errorf("%w: %q", domain.ErrTargetExists, t.Name)
		}
		return 0, fmt.Errorf("inserting target %q: %w", t.Name, err)
	}
	return id, nil
}

// ListTargets returns every target, oldest first (stable, id-ordered output for
// the CLI). The row's mode/flags reconstruct a domain.Target; construction is
// trusted here because the schema's CHECKs already guarantee validity.
func (s *PgStore) ListTargets(ctx context.Context) ([]domain.Target, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT name, base_url, mode, mutating, allowlisted
		FROM targets
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("querying targets: %w", err)
	}
	defer rows.Close()

	var targets []domain.Target
	for rows.Next() {
		var (
			t    domain.Target
			mode string
		)
		if err := rows.Scan(&t.Name, &t.BaseURL, &mode, &t.Mutating, &t.Allowlisted); err != nil {
			return nil, fmt.Errorf("scanning target: %w", err)
		}
		t.Mode = domain.Mode(mode)
		targets = append(targets, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating targets: %w", err)
	}
	return targets, nil
}

// GetTargetByName returns the named target and its id, mapping a missing row to
// domain.ErrTargetNotFound so callers inspect with errors.Is rather than checking
// pgx.ErrNoRows.
func (s *PgStore) GetTargetByName(ctx context.Context, name string) (domain.Target, int64, error) {
	var (
		id   int64
		t    domain.Target
		mode string
	)
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, base_url, mode, mutating, allowlisted
		FROM targets
		WHERE name = $1`, name,
	).Scan(&id, &t.Name, &t.BaseURL, &mode, &t.Mutating, &t.Allowlisted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Target{}, 0, fmt.Errorf("%w: %q", domain.ErrTargetNotFound, name)
		}
		return domain.Target{}, 0, fmt.Errorf("querying target %q: %w", name, err)
	}
	t.Mode = domain.Mode(mode)
	return t, id, nil
}

// compile-time proof the pgx adapter satisfies the consumer-owned port.
var _ ports.TargetStore = (*PgStore)(nil)
