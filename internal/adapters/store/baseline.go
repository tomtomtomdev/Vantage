package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"plumber/internal/domain"
	"plumber/internal/ports"
)

// GetRun loads a stored Run (including every rep's HDR histogram blob) and the id
// of the target it was measured against, reconstructing the LoadProfile and env
// fingerprint from their jsonb. A missing run maps to domain.ErrRunNotFound so
// callers inspect with errors.Is. This is the read side the verdict engine needs:
// the per-rep histograms are what the cluster bootstrap resamples (SPEC §6/§7).
func (s *PgStore) GetRun(ctx context.Context, runID int64) (domain.Run, int64, error) {
	var (
		targetID      int64
		versionMarker *string
		label         string
		profileJSON   []byte
		nReps         int
		overhead      float64
		startedAt     time.Time
		finishedAt    *time.Time
		envJSON       []byte
	)
	err := s.pool.QueryRow(ctx, `
		SELECT target_id, version_marker, label, profile, n_reps,
		       COALESCE(driver_overhead_ms, 0), started_at, finished_at, env_fingerprint
		FROM runs WHERE id = $1`, runID,
	).Scan(&targetID, &versionMarker, &label, &profileJSON, &nReps,
		&overhead, &startedAt, &finishedAt, &envJSON)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Run{}, 0, fmt.Errorf("%w: run %d", domain.ErrRunNotFound, runID)
		}
		return domain.Run{}, 0, fmt.Errorf("querying run %d: %w", runID, err)
	}

	profile, err := profileFromDoc(profileJSON)
	if err != nil {
		return domain.Run{}, 0, fmt.Errorf("run %d profile: %w", runID, err)
	}
	var env domain.EnvFingerprint
	if err := json.Unmarshal(envJSON, &env); err != nil {
		return domain.Run{}, 0, fmt.Errorf("run %d env fingerprint: %w", runID, err)
	}

	reps, err := s.repsForRun(ctx, runID)
	if err != nil {
		return domain.Run{}, 0, err
	}

	run := domain.Run{
		Profile:          profile,
		NReps:            nReps,
		Label:            label,
		Env:              env,
		Reps:             reps,
		DriverOverheadMs: overhead,
		StartedAt:        startedAt,
	}
	if versionMarker != nil {
		run.VersionMarker = *versionMarker
	}
	if finishedAt != nil {
		run.FinishedAt = *finishedAt
	}
	return run, targetID, nil
}

// repsForRun loads a run's repetitions in seq order, each with its histogram blob —
// the raw material the bootstrap resamples.
func (s *PgStore) repsForRun(ctx context.Context, runID int64) ([]domain.RepResult, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT seq, histogram,
		       COALESCE(p50, 0), COALESCE(p90, 0), COALESCE(p99, 0), COALESCE(p999, 0),
		       COALESCE(max_ms, 0), COALESCE(achieved_rps, 0), COALESCE(error_rate, 0), errors
		FROM reps WHERE run_id = $1 ORDER BY seq`, runID)
	if err != nil {
		return nil, fmt.Errorf("querying reps for run %d: %w", runID, err)
	}
	defer rows.Close()

	var reps []domain.RepResult
	for rows.Next() {
		var (
			r        domain.RepResult
			errsJSON []byte
		)
		if err := rows.Scan(&r.Seq, &r.Histogram, &r.P50Ms, &r.P90Ms, &r.P99Ms, &r.P999Ms,
			&r.MaxMs, &r.AchievedRPS, &r.ErrorRate, &errsJSON); err != nil {
			return nil, fmt.Errorf("scanning rep: %w", err)
		}
		if len(errsJSON) > 0 {
			if err := json.Unmarshal(errsJSON, &r.Errors); err != nil {
				return nil, fmt.Errorf("rep %d errors: %w", r.Seq, err)
			}
		}
		reps = append(reps, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating reps for run %d: %w", runID, err)
	}
	return reps, nil
}

// profileFromDoc reconstructs a LoadProfile from the jsonb SaveRun wrote (see
// profileDoc in run.go). Re-running it through NewConstantProfile means a corrupt
// stored profile can't produce an invalid domain value.
func profileFromDoc(b []byte) (domain.LoadProfile, error) {
	var doc struct {
		Kind       string `json:"kind"`
		RPS        int    `json:"rps"`
		DurationMs int64  `json:"duration_ms"`
		WarmupMs   int64  `json:"warmup_ms"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return domain.LoadProfile{}, fmt.Errorf("unmarshalling profile: %w", err)
	}
	if domain.ProfileKind(doc.Kind) != domain.ProfileConstant {
		return domain.LoadProfile{}, fmt.Errorf("%w: unsupported stored kind %q", domain.ErrProfileInvalid, doc.Kind)
	}
	return domain.NewConstantProfile(doc.RPS,
		time.Duration(doc.DurationMs)*time.Millisecond,
		time.Duration(doc.WarmupMs)*time.Millisecond)
}

// SetBaseline points targetID's baseline at runID, moving the pointer if one
// already exists (SPEC §6/§8 — baselines is the one intentionally-mutable pointer;
// runs/reps stay append-only). Upserts on the baselines PK (migration 0008).
func (s *PgStore) SetBaseline(ctx context.Context, targetID, runID int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO baselines (target_id, run_id, set_at)
		VALUES ($1, $2, now())
		ON CONFLICT (target_id) DO UPDATE
		SET run_id = EXCLUDED.run_id, set_at = now()`,
		targetID, runID)
	if err != nil {
		return fmt.Errorf("setting baseline for target %d → run %d: %w", targetID, runID, err)
	}
	return nil
}

// Baseline returns the current baseline run id for targetID; ok is false when none
// is set (the app surfaces domain.ErrNoBaseline). Absence is not an error.
func (s *PgStore) Baseline(ctx context.Context, targetID int64) (int64, bool, error) {
	var runID int64
	err := s.pool.QueryRow(ctx, `SELECT run_id FROM baselines WHERE target_id = $1`, targetID).Scan(&runID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("querying baseline for target %d: %w", targetID, err)
	}
	return runID, true, nil
}

// compile-time proof the pgx adapter satisfies the consumer-owned port.
var _ ports.BaselineStore = (*PgStore)(nil)
