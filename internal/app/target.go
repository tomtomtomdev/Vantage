// Package app holds orchestration — the use cases that wire domain rules to the
// ports (CLAUDE §1). It depends inward on domain and ports only; adapters are
// injected at the composition root.
package app

import (
	"context"

	"github.com/tomtomtomdev/vantage/internal/domain"
	"github.com/tomtomtomdev/vantage/internal/ports"
)

// TargetService is the register/list use case for targets. It owns the rule that
// every stored target is well-formed (via domain.NewTarget) and translates the
// CLI's flat flags into the domain's greppable, safe-by-default options.
type TargetService struct {
	store ports.TargetStore
}

// NewTargetService injects the store the use case writes through.
func NewTargetService(store ports.TargetStore) *TargetService {
	return &TargetService{store: store}
}

// Add validates and registers a target, returning its assigned id. Validation
// happens in the domain (NewTarget) before anything touches the store, so an
// invalid target never reaches the database.
func (s *TargetService) Add(ctx context.Context, name, baseURL, mode string, mutating, allowlisted bool) (int64, error) {
	var opts []domain.TargetOption
	if mutating {
		opts = append(opts, domain.WithMutating())
	}
	if allowlisted {
		opts = append(opts, domain.WithAllowlisted())
	}

	t, err := domain.NewTarget(name, baseURL, domain.Mode(mode), opts...)
	if err != nil {
		return 0, err
	}
	return s.store.AddTarget(ctx, t)
}

// List returns all registered targets, oldest first.
func (s *TargetService) List(ctx context.Context) ([]domain.Target, error) {
	return s.store.ListTargets(ctx)
}
