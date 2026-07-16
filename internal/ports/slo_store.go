package ports

import (
	"context"

	"plumber/internal/domain"
)

// SLOStore persists a target's declared SLOs and reads them back at judge time
// (SPEC §3/§7). Consumer-owned (the app declares what it needs); implemented by
// the pgx adapter. Kept small: an SLO is set (per target+metric) and listed.
type SLOStore interface {
	// SetSLO declares slo for targetID, replacing any prior SLO for the same
	// metric (a "set", not an append), and returns the SLO's id.
	SetSLO(ctx context.Context, targetID int64, slo domain.SLO) (int64, error)
	// SLOsForTarget returns every SLO declared for targetID, oldest first.
	SLOsForTarget(ctx context.Context, targetID int64) ([]domain.SLO, error)
}
