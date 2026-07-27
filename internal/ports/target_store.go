package ports

import (
	"context"

	"github.com/tomtomtomdev/vantage/internal/domain"
)

// TargetStore persists and retrieves the targets Vantage is authorized to
// measure. Defined here by the consumer (the app), implemented by the pgx
// adapter in internal/adapters/store (CLAUDE §3). Kept small — persistence of
// runs/reps/baselines is a separate port added when S1 needs it.
type TargetStore interface {
	// AddTarget persists t and returns its assigned id. It returns
	// domain.ErrTargetExists if the name is already taken.
	AddTarget(ctx context.Context, t domain.Target) (int64, error)
	// ListTargets returns all targets, oldest first.
	ListTargets(ctx context.Context) ([]domain.Target, error)
	// GetTargetByName returns the target with the given name and its id. It
	// returns domain.ErrTargetNotFound if no such target exists.
	GetTargetByName(ctx context.Context, name string) (domain.Target, int64, error)
}
