package ports

import (
	"context"

	"plumber/internal/domain"
)

// ResultStore persists Runs and their repetitions as append-only evidence
// (SPEC §6/§8). RunService needs only SaveRun; reading runs back and managing the
// baseline pointer is a separate concern (see BaselineStore) so this interface
// stays small and RunService's dependencies don't grow with S3.
type ResultStore interface {
	// SaveRun persists run (with its N RepResults) against targetID in a single
	// transaction and returns the new run's id. The run is immutable once written.
	SaveRun(ctx context.Context, targetID int64, run domain.Run) (int64, error)
}

// BaselineStore reads stored Runs back (with their per-rep histograms) and manages
// the per-target baseline pointer. It is consumed by app.CompareService (S3) and
// implemented by the same concrete *store.PgStore that satisfies ResultStore —
// interface segregation, defined where consumed (CLAUDE §3/§4).
type BaselineStore interface {
	// GetRun loads a stored Run (including its per-rep histograms) and the id of the
	// target it was measured against. Missing ⇒ domain.ErrRunNotFound.
	GetRun(ctx context.Context, runID int64) (domain.Run, int64, error)

	// SetBaseline points targetID's baseline at runID (upsert — the pointer moves;
	// see migrations/0008). This is the one intentionally-mutable evidence pointer.
	SetBaseline(ctx context.Context, targetID, runID int64) error

	// Baseline returns the current baseline run id for targetID. ok is false when no
	// baseline has been set (the caller surfaces domain.ErrNoBaseline).
	Baseline(ctx context.Context, targetID int64) (runID int64, ok bool, err error)
}
