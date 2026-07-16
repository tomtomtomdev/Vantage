package domain

import (
	"errors"
	"fmt"
	"time"
)

// Blast-radius guard sentinels (SPEC §9). The driver refuses at its entry path
// before any load is generated; callers inspect with errors.Is. Named here on the
// domain side because they are decisions about a Target's declared safety, not
// transport errors.
var (
	// ErrTargetNotAllowlisted: no allowlist entry, no load — hard stop.
	ErrTargetNotAllowlisted = errors.New("target: not allowlisted — refusing to generate load")
	// ErrMutatingRefused: a mutating endpoint is refused without an explicit override.
	ErrMutatingRefused = errors.New("target: mutating endpoint refused without explicit override")
	// ErrRateCeiling: the requested rate exceeds the driver's configured ceiling.
	ErrRateCeiling = errors.New("profile: requested rate exceeds the driver's rate ceiling")
)

// ErrRunInvalid is the sentinel for a malformed Run aggregate.
var ErrRunInvalid = errors.New("run: invalid")

// EnvFingerprint captures the conditions a Run was measured under, so Compare can
// downgrade to `confounded` on drift (SPEC §7/§8) — precision is not causation.
// Host and Colocation are always captured; Extra holds target-specific probe data
// (row counts, schema version) filled by a richer EnvProbe when pointed at a real
// backend like Sluice.
type EnvFingerprint struct {
	Host       string            `json:"host"`
	Colocation string            `json:"colocation"`
	Extra      map[string]string `json:"extra,omitempty"`
}

// RepResult is one repetition's outcome: the post-warmup, success-only HDR
// histogram (encoded blob) plus the summary percentiles, achieved rate, and the
// error tally kept as a separate axis (SPEC §6/§8). It is a pure data record; the
// driver adapter computes and fills it.
type RepResult struct {
	Seq       int
	Histogram []byte // encoded HDR (see internal/hist), post-warmup, success-only

	P50Ms  float64
	P90Ms  float64
	P99Ms  float64
	P999Ms float64
	MaxMs  float64

	AchievedRPS float64
	ErrorRate   float64        // fraction in [0,1], successful vs total measured
	Errors      map[string]int // classification (http status / "timeout" / …) → count

	DriverOverheadMs float64 // scheduling delay this rep — driver self-overhead (SPEC §8)
}

// Run is one audit execution: N repetitions of the same LoadProfile against a
// Target (SPEC §3). Immutable and version-marked once stored. A pure aggregate;
// the app assembles it from the driver's RepResults and the env probe.
type Run struct {
	Profile       LoadProfile
	NReps         int
	VersionMarker string // git SHA / deploy tag / note; empty ⇒ NULL (black-box)
	Label         string
	Env           EnvFingerprint
	Reps          []RepResult

	// DriverOverheadMs is the run-level driver self-overhead (the worst rep's), so
	// a driver-bound run is visible at a glance (SPEC §8).
	DriverOverheadMs float64
	StartedAt        time.Time
	FinishedAt       time.Time
}

// Validate enforces the Run's invariants before it is persisted as evidence.
func (r Run) Validate() error {
	if r.NReps < 1 {
		return fmt.Errorf("%w: n_reps must be >= 1, got %d", ErrRunInvalid, r.NReps)
	}
	if len(r.Reps) != r.NReps {
		return fmt.Errorf("%w: have %d rep results, n_reps says %d", ErrRunInvalid, len(r.Reps), r.NReps)
	}
	if r.Profile.Kind == "" {
		return fmt.Errorf("%w: profile is unset", ErrRunInvalid)
	}
	return nil
}
