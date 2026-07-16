package ports

import (
	"context"

	"plumber/internal/domain"
)

// TargetStore persists and retrieves the targets Plumber is authorized to
// measure. Defined here by the consumer (the app), implemented by the pgx
// adapter in internal/adapters/store (CLAUDE §3). Kept small — persistence of
// runs/reps/baselines is a separate port added when S1 needs it.
type TargetStore interface {
	// AddTarget persists t and returns its assigned id. It returns
	// domain.ErrTargetExists if the name is already taken.
	AddTarget(ctx context.Context, t domain.Target) (int64, error)
	// ListTargets returns all targets, oldest first.
	ListTargets(ctx context.Context) ([]domain.Target, error)
}
