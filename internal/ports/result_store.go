package ports

import (
	"context"

	"plumber/internal/domain"
)

// ResultStore persists Runs and their repetitions as append-only evidence
// (SPEC §6/§8). S1 needs only SaveRun; Get/SetBaseline/Baseline arrive with the
// verdict engine in S3, added to this interface when their consumers exist.
type ResultStore interface {
	// SaveRun persists run (with its N RepResults) against targetID in a single
	// transaction and returns the new run's id. The run is immutable once written.
	SaveRun(ctx context.Context, targetID int64, run domain.Run) (int64, error)
}
