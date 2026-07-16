package store

import (
	"context"
	"fmt"

	"plumber/internal/domain"
	"plumber/internal/ports"
)

// SetSLO declares an SLO for targetID, replacing any prior SLO for the same
// metric (SPEC §3 — a "set", not an append). The upsert keys on the
// (target_id, metric) unique constraint (migration 0010). SLOs are mutable
// config, not evidence, so re-declaration UPDATEs rather than errors.
func (s *PgStore) SetSLO(ctx context.Context, targetID int64, slo domain.SLO) (int64, error) {
	var atRPS *float64 // NULL when unspecified (the "—" in SPEC §3)
	if slo.AtRPS > 0 {
		atRPS = &slo.AtRPS
	}

	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO slos (target_id, metric, threshold, unit, comparator, at_rps)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (target_id, metric) DO UPDATE
		SET threshold  = EXCLUDED.threshold,
		    unit       = EXCLUDED.unit,
		    comparator = EXCLUDED.comparator,
		    at_rps     = EXCLUDED.at_rps,
		    created_at = now()
		RETURNING id`,
		targetID, string(slo.Metric), slo.Threshold, slo.Unit, string(slo.Comparator), atRPS,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upserting SLO %s for target %d: %w", slo.Metric, targetID, err)
	}
	return id, nil
}

// SLOsForTarget returns every SLO declared for targetID, oldest first. The domain
// constructor re-validates each row so a malformed persisted SLO can never reach
// the verdict engine.
func (s *PgStore) SLOsForTarget(ctx context.Context, targetID int64) ([]domain.SLO, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT metric, threshold, unit, comparator, COALESCE(at_rps, 0)
		FROM slos
		WHERE target_id = $1
		ORDER BY id`, targetID)
	if err != nil {
		return nil, fmt.Errorf("querying SLOs for target %d: %w", targetID, err)
	}
	defer rows.Close()

	var slos []domain.SLO
	for rows.Next() {
		var (
			metric, unit, comparator string
			threshold, atRPS         float64
		)
		if err := rows.Scan(&metric, &threshold, &unit, &comparator, &atRPS); err != nil {
			return nil, fmt.Errorf("scanning SLO: %w", err)
		}
		slo, err := domain.NewSLO(metric, threshold, unit, comparator, atRPS)
		if err != nil {
			return nil, fmt.Errorf("reconstructing SLO %q: %w", metric, err)
		}
		slos = append(slos, slo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating SLOs: %w", err)
	}
	return slos, nil
}

// compile-time proof the pgx adapter satisfies the consumer-owned port.
var _ ports.SLOStore = (*PgStore)(nil)
