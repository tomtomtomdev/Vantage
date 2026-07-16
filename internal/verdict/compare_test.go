package verdict

import (
	"strings"
	"testing"
	"time"

	"plumber/internal/domain"
	"plumber/internal/hist"
)

// rep builds a single-valued histogram: `count` samples all at centerMs, so every
// percentile of the rep is centerMs. Pooling equal-count single-valued reps then
// makes the pooled p99 the max drawn center and p50 the median drawn center, which
// keeps the bootstrap's behaviour predictable in the fixtures below.
func rep(t *testing.T, centerMs float64) *hist.Histogram {
	t.Helper()
	h := hist.New()
	for i := 0; i < 2000; i++ {
		if err := h.RecordDuration(time.Duration(centerMs * float64(time.Millisecond))); err != nil {
			t.Fatalf("record %vms: %v", centerMs, err)
		}
	}
	return h
}

// reps builds one side's decoded rep histograms from a list of per-rep centers.
func reps(t *testing.T, centersMs ...float64) []*hist.Histogram {
	t.Helper()
	out := make([]*hist.Histogram, len(centersMs))
	for i, c := range centersMs {
		out[i] = rep(t, c)
	}
	return out
}

// constProfile is the shared LoadProfile the comparable fixtures use on both sides.
func constProfile(t *testing.T) domain.LoadProfile {
	t.Helper()
	p, err := domain.NewConstantProfile(100, 60*time.Second, 10*time.Second)
	if err != nil {
		t.Fatalf("NewConstantProfile: %v", err)
	}
	return p
}

// input is a small builder for a comparable side sharing profile/target/env.
func input(t *testing.T, hs []*hist.Histogram) CompareInput {
	t.Helper()
	return CompareInput{
		Reps:       hs,
		Profile:    constProfile(t),
		Env:        domain.EnvFingerprint{Host: "h1", Colocation: "same-host"},
		TargetName: "sluice-ohlc",
	}
}

func deltaFor(t *testing.T, c Comparison, m domain.MetricKind) Delta {
	t.Helper()
	for _, d := range c.Deltas {
		if d.Metric == m {
			return d
		}
	}
	t.Fatalf("no delta for metric %s in %+v", m, c.Deltas)
	return Delta{}
}

func TestCompare_KnownShift_Significant(t *testing.T) {
	// A real improvement: candidate ~40ms faster on every rep, tight between-rep
	// spread. The CI on Δp99 should exclude 0 and the point estimate be negative.
	base := input(t, reps(t, 198, 199, 200, 201, 202))
	cand := input(t, reps(t, 158, 159, 160, 161, 162))

	c := Compare(base, cand, WithSeed(1))
	if c.Comparability != Comparable {
		t.Fatalf("comparability = %q, want comparable (reason: %s)", c.Comparability, c.Reason)
	}
	d := deltaFor(t, c, domain.MetricP99)
	if d.PointMs >= 0 {
		t.Errorf("p99 point = %.1fms, want negative (improvement)", d.PointMs)
	}
	if !(d.CIHighMs < 0) {
		t.Errorf("p99 CI = [%.1f, %.1f], want to exclude 0 (both negative)", d.CILowMs, d.CIHighMs)
	}
	if d.Significance != Significant {
		t.Errorf("p99 significance = %q, want significant", d.Significance)
	}
}

func TestCompare_KnownNoShift_WithinNoise(t *testing.T) {
	// No real change: candidate centers interleave the baseline's, so the Δ
	// distribution straddles 0 and the CI must include it.
	base := input(t, reps(t, 196, 198, 200, 202, 204))
	cand := input(t, reps(t, 197, 199, 201, 203, 205))

	c := Compare(base, cand, WithSeed(1))
	if c.Comparability != Comparable {
		t.Fatalf("comparability = %q, want comparable", c.Comparability)
	}
	d := deltaFor(t, c, domain.MetricP99)
	if d.CILowMs > 0 || d.CIHighMs < 0 {
		t.Errorf("p99 CI = [%.1f, %.1f], want to include 0", d.CILowMs, d.CIHighMs)
	}
	if d.Significance != WithinNoise {
		t.Errorf("p99 significance = %q, want within-noise", d.Significance)
	}
}

func TestCompare_ClusterResampling_BetweenRepVarianceWidensCI(t *testing.T) {
	// Both sides are the SAME wide between-rep spread, so the true delta is ~0.
	// Because Compare resamples at the REP level, that large between-rep variance
	// propagates into a wide CI (within-noise). Had it resampled requests from the
	// pooled histogram instead, both sides would be identical fixed distributions
	// and the CI would collapse to ~[0,0] — this test would then fail, which is the
	// point: it distinguishes cluster resampling from the request-level sin.
	spread := []float64{100, 150, 200, 300, 500}
	base := input(t, reps(t, spread...))
	cand := input(t, reps(t, spread...))

	c := Compare(base, cand, WithSeed(1))
	d := deltaFor(t, c, domain.MetricP99)
	if width := d.CIHighMs - d.CILowMs; width < 100 {
		t.Errorf("p99 CI width = %.1fms, want wide (>=100) from between-rep variance; CI=[%.1f,%.1f]",
			width, d.CILowMs, d.CIHighMs)
	}
	if d.Significance != WithinNoise {
		t.Errorf("p99 significance = %q, want within-noise (true delta ~0)", d.Significance)
	}
}

func TestCompare_NEqualsOne_Incomparable(t *testing.T) {
	for _, tc := range []struct {
		name       string
		base, cand []*hist.Histogram
	}{
		{"baseline N=1", reps(t, 200), reps(t, 160, 161, 162, 163, 164)},
		{"candidate N=1", reps(t, 198, 199, 200, 201, 202), reps(t, 160)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Compare(input(t, tc.base), input(t, tc.cand), WithSeed(1))
			if c.Comparability != Incomparable {
				t.Errorf("comparability = %q, want incomparable", c.Comparability)
			}
			if len(c.Deltas) != 0 {
				t.Errorf("incomparable pair produced %d deltas, want none", len(c.Deltas))
			}
			if !strings.Contains(c.Reason, "N=1") && !strings.Contains(strings.ToLower(c.Reason), "reps") {
				t.Errorf("reason %q should explain the N=1 refusal", c.Reason)
			}
		})
	}
}

func TestCompare_ProfileMismatch_Incomparable(t *testing.T) {
	base := input(t, reps(t, 198, 199, 200, 201, 202))
	cand := input(t, reps(t, 158, 159, 160, 161, 162))
	other, err := domain.NewConstantProfile(200, 60*time.Second, 10*time.Second) // different rps
	if err != nil {
		t.Fatalf("NewConstantProfile: %v", err)
	}
	cand.Profile = other

	c := Compare(base, cand, WithSeed(1))
	if c.Comparability != Incomparable {
		t.Fatalf("comparability = %q, want incomparable", c.Comparability)
	}
	if len(c.Deltas) != 0 {
		t.Errorf("incomparable pair produced %d deltas, want none", len(c.Deltas))
	}
	if !strings.Contains(strings.ToLower(c.Reason), "profile") {
		t.Errorf("reason %q should name the profile mismatch", c.Reason)
	}
}

func TestCompare_EnvDrift_Confounded_NamesTheDrift(t *testing.T) {
	base := input(t, reps(t, 198, 199, 200, 201, 202))
	cand := input(t, reps(t, 158, 159, 160, 161, 162))
	base.Env.Extra = map[string]string{"positions_rows": "1000000"}
	cand.Env.Extra = map[string]string{"positions_rows": "3000000"} // dataset grew between runs

	c := Compare(base, cand, WithSeed(1))
	if c.Comparability != Confounded {
		t.Fatalf("comparability = %q, want confounded", c.Comparability)
	}
	// The drift must be named so the reader knows the delta may be data drift, not
	// their fix.
	if !strings.Contains(c.Reason, "positions_rows") {
		t.Errorf("reason %q should name the drifted key positions_rows", c.Reason)
	}
	if !strings.Contains(c.Reason, "1000000") || !strings.Contains(c.Reason, "3000000") {
		t.Errorf("reason %q should show before→after values", c.Reason)
	}
	// A confounded verdict still shows the numbers (downgrade, not refusal), but the
	// top-line is confounded so no bare "significant" claim stands.
	if len(c.Deltas) == 0 {
		t.Errorf("confounded pair should still surface the deltas")
	}
}

func TestCompare_HostAndColocationDrift_Confounded(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*CompareInput)
		want   string
	}{
		{"host", func(in *CompareInput) { in.Env.Host = "other-host" }, "host"},
		{"colocation", func(in *CompareInput) { in.Env.Colocation = "cross-region" }, "colocation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := input(t, reps(t, 198, 199, 200, 201, 202))
			cand := input(t, reps(t, 158, 159, 160, 161, 162))
			tc.mutate(&cand)
			c := Compare(base, cand, WithSeed(1))
			if c.Comparability != Confounded {
				t.Fatalf("comparability = %q, want confounded", c.Comparability)
			}
			if !strings.Contains(strings.ToLower(c.Reason), tc.want) {
				t.Errorf("reason %q should name the %s drift", c.Reason, tc.want)
			}
		})
	}
}

func TestCompare_Deterministic_SameSeedSameCI(t *testing.T) {
	base := input(t, reps(t, 196, 198, 200, 202, 204))
	cand := input(t, reps(t, 190, 195, 200, 205, 210))

	a := Compare(base, cand, WithSeed(42))
	b := Compare(base, cand, WithSeed(42))
	if len(a.Deltas) != len(b.Deltas) {
		t.Fatalf("delta counts differ: %d vs %d", len(a.Deltas), len(b.Deltas))
	}
	for i := range a.Deltas {
		if a.Deltas[i] != b.Deltas[i] {
			t.Errorf("delta %d differs across identical seeds:\n a=%+v\n b=%+v", i, a.Deltas[i], b.Deltas[i])
		}
	}
}
