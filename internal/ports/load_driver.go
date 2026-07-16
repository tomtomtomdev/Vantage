package ports

import (
	"context"

	"plumber/internal/domain"
)

// LoadDriver generates load against a Target and measures one repetition of a
// LoadProfile. Implemented by the open-model driver in internal/adapters/loaddriver
// (fork F1); defined here because the app is the consumer (CLAUDE §3).
//
// One call = one Repetition. The app runs N of them to make run-to-run variance
// observable (SPEC §3). The driver enforces the blast-radius guards (SPEC §9) at
// its entry path and refuses — returning a domain guard sentinel — before any
// request is sent.
type LoadDriver interface {
	// Run measures one repetition and returns its RepResult (post-warmup,
	// success-only latency histogram plus error tally and driver overhead). It
	// honours ctx cancellation as the kill switch: in-flight requests drain and
	// it returns promptly.
	Run(ctx context.Context, t domain.Target, p domain.LoadProfile) (domain.RepResult, error)
}
