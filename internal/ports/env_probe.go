package ports

import (
	"context"

	"github.com/tomtomtomdev/vantage/internal/domain"
)

// EnvProbe captures the environment fingerprint recorded on every Run so Compare
// can guard on condition drift (SPEC §7/§8) — a precise measurement of a
// confounded number is still wrong. The default probe records host + colocation;
// a target-specific probe (e.g. a Sluice row-count query) plugs in here later
// without touching the driver or the app orchestration.
type EnvProbe interface {
	Fingerprint(ctx context.Context) (domain.EnvFingerprint, error)
}
