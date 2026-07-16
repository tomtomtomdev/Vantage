package store

import (
	"context"
	"encoding/json"
	"fmt"

	"plumber/internal/domain"
	"plumber/internal/ports"
)

// SaveRun persists run and its N repetitions against targetID in one transaction,
// so a Run is all-or-nothing evidence (SPEC §6/§8). The run is immutable once
// written; there is no UPDATE path here and the schema enforces it (0009).
func (s *PgStore) SaveRun(ctx context.Context, targetID int64, run domain.Run) (int64, error) {
	if err := run.Validate(); err != nil {
		return 0, err
	}

	profileJSON, err := json.Marshal(profileDoc(run.Profile))
	if err != nil {
		return 0, fmt.Errorf("marshalling profile: %w", err)
	}
	envJSON, err := json.Marshal(run.Env)
	if err != nil {
		return 0, fmt.Errorf("marshalling env fingerprint: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after a successful Commit

	var versionMarker *string // NULL when empty (black-box has no SHA)
	if run.VersionMarker != "" {
		versionMarker = &run.VersionMarker
	}

	var runID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO runs
			(target_id, version_marker, label, profile, n_reps, warmup_ms,
			 driver_overhead_ms, started_at, finished_at, env_fingerprint)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id`,
		targetID, versionMarker, run.Label, string(profileJSON), run.NReps,
		int(run.Profile.Warmup.Milliseconds()), run.DriverOverheadMs,
		run.StartedAt, run.FinishedAt, string(envJSON),
	).Scan(&runID)
	if err != nil {
		return 0, fmt.Errorf("inserting run: %w", err)
	}

	for _, rep := range run.Reps {
		errsJSON, err := json.Marshal(nonNilErrors(rep.Errors))
		if err != nil {
			return 0, fmt.Errorf("marshalling rep %d errors: %w", rep.Seq, err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO reps
				(run_id, seq, histogram, p50, p90, p99, p999, max_ms,
				 achieved_rps, error_rate, errors)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			runID, rep.Seq, rep.Histogram, rep.P50Ms, rep.P90Ms, rep.P99Ms,
			rep.P999Ms, rep.MaxMs, rep.AchievedRPS, rep.ErrorRate, string(errsJSON),
		)
		if err != nil {
			return 0, fmt.Errorf("inserting rep %d: %w", rep.Seq, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("committing run: %w", err)
	}
	return runID, nil
}

// profileDoc is the jsonb shape stored in runs.profile — durations as ms ints so
// the row is human-readable, and copy-pasteable back via LoadProfile.String().
func profileDoc(p domain.LoadProfile) map[string]any {
	return map[string]any{
		"kind":        string(p.Kind),
		"rps":         p.RPS,
		"duration_ms": p.Duration.Milliseconds(),
		"warmup_ms":   p.Warmup.Milliseconds(),
	}
}

func nonNilErrors(m map[string]int) map[string]int {
	if m == nil {
		return map[string]int{}
	}
	return m
}

// compile-time proof the pgx adapter satisfies the consumer-owned port.
var _ ports.ResultStore = (*PgStore)(nil)
