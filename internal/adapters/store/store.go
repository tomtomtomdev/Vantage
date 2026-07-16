// Package store implements the persistence ports over Postgres via pgx/v5
// (CLAUDE §1, SPEC §6). It returns a concrete *PgStore that satisfies the
// consumer-owned interfaces in internal/ports ("accept interfaces, return
// structs" — CLAUDE §3/§4).
//
// Evidence tables (runs/reps/…) are append-only: the store has no UPDATE path
// for them, and the schema enforces it (migrations/0009).
package store

import "github.com/jackc/pgx/v5/pgxpool"

// PgStore is the Postgres-backed store. Construct it with New at the composition
// root and inject it where a ports.TargetStore (etc.) is needed.
type PgStore struct {
	pool *pgxpool.Pool
}

// New returns a PgStore over an already-connected pool. The pool's lifecycle is
// the caller's (the composition root opens and closes it).
func New(pool *pgxpool.Pool) *PgStore {
	return &PgStore{pool: pool}
}
