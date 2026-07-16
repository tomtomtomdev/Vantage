package platform

import (
	"context"
	"fmt"
	"os"

	"plumber/internal/domain"
)

// HostProbe is the default ports.EnvProbe: it records the host the driver ran on
// and the operator-declared colocation relative to the target (SPEC §8 —
// client-side latency includes network RTT, so where the driver runs is part of
// the measurement). Target-specific probes (e.g. a Sluice row-count query) can
// replace or wrap it without touching the driver.
type HostProbe struct {
	colocation string
}

// NewHostProbe constructs a probe that stamps the given colocation note.
func NewHostProbe(colocation string) *HostProbe {
	if colocation == "" {
		colocation = "unknown"
	}
	return &HostProbe{colocation: colocation}
}

// Fingerprint captures the host and colocation at run time.
func (p *HostProbe) Fingerprint(_ context.Context) (domain.EnvFingerprint, error) {
	host, err := os.Hostname()
	if err != nil {
		return domain.EnvFingerprint{}, fmt.Errorf("reading hostname: %w", err)
	}
	return domain.EnvFingerprint{Host: host, Colocation: p.colocation}, nil
}
