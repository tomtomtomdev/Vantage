package app

import (
	"context"

	"plumber/internal/domain"
	"plumber/internal/ports"
)

// SLOService is the declare/list use case for a target's SLOs (SPEC §3). It owns
// the rule that every stored SLO is well-formed (via domain.NewSLO) and resolves
// the human target name to its id before touching the SLO store.
type SLOService struct {
	targets ports.TargetStore
	slos    ports.SLOStore
}

// NewSLOService injects the stores the use case reads and writes through.
func NewSLOService(targets ports.TargetStore, slos ports.SLOStore) *SLOService {
	return &SLOService{targets: targets, slos: slos}
}

// Set validates the SLO in the domain, resolves the target by name, and declares
// the SLO (replacing any prior one for the same metric). An invalid SLO is
// rejected before anything touches the database.
func (s *SLOService) Set(ctx context.Context, targetName, metric string, threshold float64, unit, comparator string, atRPS float64) (int64, error) {
	slo, err := domain.NewSLO(metric, threshold, unit, comparator, atRPS)
	if err != nil {
		return 0, err
	}
	_, targetID, err := s.targets.GetTargetByName(ctx, targetName)
	if err != nil {
		return 0, err
	}
	return s.slos.SetSLO(ctx, targetID, slo)
}

// List returns the SLOs declared for the named target, oldest first.
func (s *SLOService) List(ctx context.Context, targetName string) ([]domain.SLO, error) {
	_, targetID, err := s.targets.GetTargetByName(ctx, targetName)
	if err != nil {
		return nil, err
	}
	return s.slos.SLOsForTarget(ctx, targetID)
}
