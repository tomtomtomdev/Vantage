package verdict

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"

	"plumber/internal/domain"
	"plumber/internal/hist"
)

// Comparability is the run-level verdict of a Compare: whether the two Runs may be
// compared at all, and if so whether the conditions were sound (SPEC §7, skill
// perf-measurement-rigor §6). It is distinct from per-metric Significance — a pair
// can be Comparable while an individual delta is within-noise.
type Comparability string

const (
	// Comparable: same profile/target, N>=2 both sides, matched environment — the
	// deltas below can be read as caused by the code change.
	Comparable Comparability = "comparable"
	// Incomparable: mismatched profile/target or N=1 — no delta is emitted, because
	// any number would mislead (SPEC §7).
	Incomparable Comparability = "incomparable"
	// Confounded: the deltas are computed, but the environment drifted between the
	// two Runs (data growth, different host, cold vs warm cache), so a "significant"
	// change may be the confounder, not the fix. Precision is not causation (SPEC §8).
	Confounded Comparability = "confounded"
)

// Significance is a single metric's outcome from its bootstrap CI: significant iff
// the 95% CI on the delta excludes zero, else within-noise — the honest "you didn't
// actually move it".
type Significance string

const (
	Significant Significance = "significant"
	WithinNoise Significance = "within-noise"
)

// Delta is one percentile's before/after with the bootstrap confidence interval on
// the change. PointMs is candidate − baseline (negative ⇒ faster). The CI is the
// percentile-method interval over the resampled Δ distribution.
type Delta struct {
	Metric       domain.MetricKind
	BaselineMs   float64
	CandidateMs  float64
	PointMs      float64
	CILowMs      float64
	CIHighMs     float64
	Significance Significance
}

// Comparison is Compare's result: the run-level Comparability (with a Reason when
// the pair is incomparable or confounded) and the per-metric Deltas. Deltas is
// empty when Incomparable; it is populated (and shown) even when Confounded, so the
// reader sees the number and the caveat together.
type Comparison struct {
	Comparability Comparability
	Reason        string
	Deltas        []Delta
}

// CompareInput is one side of a comparison: the decoded per-rep success-only
// histograms plus the metadata the guards need. The app decodes the stored HDR
// blobs into this, so verdict stays ignorant of the blob format and the database —
// and is testable with hand-built histograms in microseconds (CLAUDE §3).
type CompareInput struct {
	Reps       []*hist.Histogram
	Profile    domain.LoadProfile
	Env        domain.EnvFingerprint
	TargetName string
}

// metric couples a MetricKind to the numeric percentile it reads from a histogram.
type metric struct {
	kind domain.MetricKind
	pct  float64
}

type config struct {
	seed      int64
	resamples int
	metrics   []metric
}

// Option configures Compare. Defaults: a fixed seed (reproducible, auditable
// evidence — the same two Runs always yield the same CI), 2000 resamples, and the
// p50/p90/p99/p99.9 percentiles.
type Option func(*config)

// WithSeed sets the bootstrap RNG seed. Injected/seedable per CLAUDE §3 — never a
// global source — so tests are deterministic and runs are reproducible.
func WithSeed(seed int64) Option { return func(c *config) { c.seed = seed } }

// WithResamples sets the number of bootstrap iterations (more ⇒ tighter CI estimate,
// slower). The default of 2000 is ample for the percentile method.
func WithResamples(b int) Option {
	return func(c *config) {
		if b > 0 {
			c.resamples = b
		}
	}
}

func defaultConfig() config {
	return config{
		seed:      1,
		resamples: 2000,
		metrics: []metric{
			{domain.MetricP50, 50},
			{domain.MetricP90, 90},
			{domain.MetricP99, 99},
			{domain.MetricP999, 99.9},
		},
	}
}

// Compare is the load-bearing verdict (SPEC §7): does the candidate differ from the
// baseline beyond noise, and is the comparison even sound? It guards first, then
// bootstraps a confidence interval on the delta of each percentile.
//
// The bootstrap is a CLUSTER bootstrap over repetitions, not requests: each
// iteration resamples the N reps with replacement and pools them, so between-rep
// variance (the real run-to-run spread) is what the CI reflects. Resampling
// individual requests would treat one run as the whole truth and produce a
// narrow, over-confident interval — the exact sin this method exists to avoid.
// This is also why N=1 can never support a significance claim.
//
// Pure and total: it never returns an error. An unsound pair is a verdict
// (Incomparable / Confounded), not a failure.
func Compare(baseline, candidate CompareInput, opts ...Option) Comparison {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	// Guard 1: cross-profile or cross-target comparisons are meaningless.
	if baseline.TargetName != candidate.TargetName {
		return Comparison{
			Comparability: Incomparable,
			Reason: fmt.Sprintf("different target: baseline %q vs candidate %q",
				baseline.TargetName, candidate.TargetName),
		}
	}
	if baseline.Profile != candidate.Profile {
		return Comparison{
			Comparability: Incomparable,
			Reason: fmt.Sprintf("different load profile: baseline %s vs candidate %s",
				baseline.Profile, candidate.Profile),
		}
	}

	// Guard 2: N=1 on either side means no observable between-rep variance, so any
	// CI would be fiction.
	if len(baseline.Reps) < 2 || len(candidate.Reps) < 2 {
		return Comparison{
			Comparability: Incomparable,
			Reason: fmt.Sprintf("N=1: need >=2 reps per side for a significance claim (baseline %d, candidate %d)",
				len(baseline.Reps), len(candidate.Reps)),
		}
	}

	deltas := bootstrap(baseline.Reps, candidate.Reps, cfg)

	// Guard 3: environment drift downgrades to confounded — the delta is real, its
	// cause may not be the code. Deltas are still surfaced (downgrade, not refusal).
	if drift := envDrift(baseline.Env, candidate.Env); drift != "" {
		return Comparison{Comparability: Confounded, Reason: drift, Deltas: deltas}
	}

	return Comparison{Comparability: Comparable, Deltas: deltas}
}

// bootstrap builds each metric's Δ distribution by cluster-resampling reps, then
// reads the point estimate off the full data and the 95% CI off the Δ distribution.
func bootstrap(baseReps, candReps []*hist.Histogram, cfg config) []Delta {
	rng := rand.New(rand.NewSource(cfg.seed)) //nolint:gosec // not cryptographic; seedable for reproducible evidence

	// Δ* samples per metric, filled iteration by iteration from the SAME resample so
	// all percentiles are read off one pooled draw per side.
	samples := make([][]float64, len(cfg.metrics))
	for i := range samples {
		samples[i] = make([]float64, 0, cfg.resamples)
	}
	for iter := 0; iter < cfg.resamples; iter++ {
		basePool := resample(rng, baseReps)
		candPool := resample(rng, candReps)
		for i, m := range cfg.metrics {
			d := candPool.PercentileMillis(m.pct) - basePool.PercentileMillis(m.pct)
			samples[i] = append(samples[i], d)
		}
	}

	deltas := make([]Delta, len(cfg.metrics))
	for i, m := range cfg.metrics {
		base := pooled(baseReps).PercentileMillis(m.pct)
		cand := pooled(candReps).PercentileMillis(m.pct)
		sort.Float64s(samples[i])
		lo := quantile(samples[i], 0.025)
		hi := quantile(samples[i], 0.975)
		sig := WithinNoise
		if lo > 0 || hi < 0 { // CI excludes zero
			sig = Significant
		}
		deltas[i] = Delta{
			Metric:       m.kind,
			BaselineMs:   base,
			CandidateMs:  cand,
			PointMs:      cand - base,
			CILowMs:      lo,
			CIHighMs:     hi,
			Significance: sig,
		}
	}
	return deltas
}

// resample draws len(hs) reps with replacement and pools them into a fresh
// histogram — one cluster-bootstrap draw. Drawing the same rep twice merges its
// counts twice, which is exactly how the rep's weight varies between draws.
func resample(rng *rand.Rand, hs []*hist.Histogram) *hist.Histogram {
	pool := hist.New()
	n := len(hs)
	for i := 0; i < n; i++ {
		pool.Merge(hs[rng.Intn(n)])
	}
	return pool
}

// pooled merges all reps into one histogram for the full-data point estimate.
func pooled(hs []*hist.Histogram) *hist.Histogram {
	pool := hist.New()
	for _, h := range hs {
		pool.Merge(h)
	}
	return pool
}

// quantile returns the q-quantile (q in [0,1]) of a sorted slice by linear
// interpolation between order statistics — the percentile method's interval bound.
func quantile(sorted []float64, q float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	pos := q * float64(n-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// envDrift returns "" when the two fingerprints match materially, else a
// human-readable description naming every drifted dimension (host, colocation, and
// each differing Extra key with before→after). Exact-match is deliberately
// conservative: erring toward confounded is the safe default for evidence (a
// numeric row-count tolerance is a reasonable later refinement).
func envDrift(a, b domain.EnvFingerprint) string {
	var drifts []string
	if a.Host != b.Host {
		drifts = append(drifts, fmt.Sprintf("host: %s→%s", a.Host, b.Host))
	}
	if a.Colocation != b.Colocation {
		drifts = append(drifts, fmt.Sprintf("colocation: %s→%s", a.Colocation, b.Colocation))
	}
	for _, k := range unionKeys(a.Extra, b.Extra) {
		av, bv := a.Extra[k], b.Extra[k]
		if av != bv {
			drifts = append(drifts, fmt.Sprintf("%s: %s→%s", k, orDash(av), orDash(bv)))
		}
	}
	if len(drifts) == 0 {
		return ""
	}
	return "environment drift — " + strings.Join(drifts, ", ")
}

// unionKeys returns the sorted union of two maps' keys, so drift reporting is
// deterministic regardless of map iteration order.
func unionKeys(a, b map[string]string) []string {
	seen := map[string]struct{}{}
	for k := range a {
		seen[k] = struct{}{}
	}
	for k := range b {
		seen[k] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func orDash(s string) string {
	if s == "" {
		return "∅"
	}
	return s
}
