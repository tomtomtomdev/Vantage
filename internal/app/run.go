package app

import (
	"context"
	"fmt"
	"time"

	"github.com/tomtomtomdev/vantage/internal/domain"
	"github.com/tomtomtomdev/vantage/internal/hist"
	"github.com/tomtomtomdev/vantage/internal/ports"
	"github.com/tomtomtomdev/vantage/internal/verdict"
)

// RunService orchestrates one audit Run: it looks up the target, drives N
// repetitions through the load driver, captures the environment fingerprint,
// persists the immutable Run, and judges it against the target's declared SLOs
// (SPEC §3/§7/§8). It wires ports only — the measurement integrity lives in the
// driver, the significance math in verdict.
type RunService struct {
	targets ports.TargetStore
	driver  ports.LoadDriver
	store   ports.ResultStore
	probe   ports.EnvProbe
	slos    ports.SLOStore
	now     func() time.Time
}

// NewRunService injects the ports the use case needs.
func NewRunService(targets ports.TargetStore, driver ports.LoadDriver, store ports.ResultStore, probe ports.EnvProbe, slos ports.SLOStore) *RunService {
	return &RunService{targets: targets, driver: driver, store: store, probe: probe, slos: slos, now: time.Now}
}

// RunParams are the caller's inputs for one Run.
type RunParams struct {
	TargetName    string
	Profile       domain.LoadProfile
	NReps         int
	VersionMarker string
	Label         string
}

// Percentiles is a latency summary in milliseconds.
type Percentiles struct {
	P50, P90, P99, P999, Max float64
}

// RunSummary is the Run-level view derived from its repetitions: pooled
// percentiles (from the merged histograms) plus the co-equal error-rate axis and
// achieved throughput (SPEC §6/§8).
type RunSummary struct {
	Pooled           Percentiles
	ErrorRate        float64
	AchievedRPS      float64
	DriverOverheadMs float64
}

// Result projects the pooled summary into the domain.Result that verdict.Judge
// consumes — the single seam between the app's measurement view and the pure
// significance math. AchievedRPS is the throughput axis.
func (s RunSummary) Result() domain.Result {
	return domain.Result{
		P50Ms:         s.Pooled.P50,
		P90Ms:         s.Pooled.P90,
		P99Ms:         s.Pooled.P99,
		P999Ms:        s.Pooled.P999,
		MaxMs:         s.Pooled.Max,
		ErrorRate:     s.ErrorRate,
		ThroughputRPS: s.AchievedRPS,
	}
}

// RunResult bundles what the CLI needs to report a completed Run. Verdicts is
// nil when the target has no declared SLO — a single audited run is legitimate,
// it just carries no PASS/FAIL (SPEC §8).
type RunResult struct {
	RunID    int64
	Run      domain.Run
	Summary  RunSummary
	Verdicts verdict.Verdicts
}

// Run executes the audit. Guard violations from the driver (SPEC §9) and a
// missing target propagate unchanged; nothing is persisted unless every
// repetition completed.
func (s *RunService) Run(ctx context.Context, p RunParams) (RunResult, error) {
	if p.NReps < 1 {
		return RunResult{}, fmt.Errorf("%w: n_reps must be >= 1, got %d", domain.ErrRunInvalid, p.NReps)
	}

	target, targetID, err := s.targets.GetTargetByName(ctx, p.TargetName)
	if err != nil {
		return RunResult{}, err
	}

	env, err := s.probe.Fingerprint(ctx)
	if err != nil {
		return RunResult{}, fmt.Errorf("capturing env fingerprint: %w", err)
	}

	started := s.now()
	reps := make([]domain.RepResult, 0, p.NReps)
	var maxOverhead float64
	for i := 0; i < p.NReps; i++ {
		rep, err := s.driver.Run(ctx, target, p.Profile)
		if err != nil {
			return RunResult{}, fmt.Errorf("repetition %d: %w", i+1, err)
		}
		rep.Seq = i + 1
		reps = append(reps, rep)
		if rep.DriverOverheadMs > maxOverhead {
			maxOverhead = rep.DriverOverheadMs
		}
	}

	run := domain.Run{
		Profile:          p.Profile,
		NReps:            p.NReps,
		VersionMarker:    p.VersionMarker,
		Label:            p.Label,
		Env:              env,
		Reps:             reps,
		DriverOverheadMs: maxOverhead,
		StartedAt:        started,
		FinishedAt:       s.now(),
	}

	summary, err := Summarise(run)
	if err != nil {
		return RunResult{}, err
	}

	// Judge against the target's declared SLOs, if any. A target with no SLO
	// yields a legitimate run without a PASS/FAIL — Judge is only called when a
	// threshold exists, honouring "no verdict without a declared SLO" (SPEC §8).
	slos, err := s.slos.SLOsForTarget(ctx, targetID)
	if err != nil {
		return RunResult{}, fmt.Errorf("loading SLOs: %w", err)
	}
	var verdicts verdict.Verdicts
	if len(slos) > 0 {
		verdicts, err = verdict.Judge(summary.Result(), slos)
		if err != nil {
			return RunResult{}, fmt.Errorf("judging run: %w", err)
		}
	}

	runID, err := s.store.SaveRun(ctx, targetID, run)
	if err != nil {
		return RunResult{}, err
	}
	return RunResult{RunID: runID, Run: run, Summary: summary, Verdicts: verdicts}, nil
}

// Summarise merges the per-rep histograms into pooled percentiles and computes the
// exact pooled error rate from success counts (histogram totals) and the error
// tallies — never folding errors into latency (SPEC §8).
func Summarise(run domain.Run) (RunSummary, error) {
	pooled := hist.New()
	var successTotal, failedTotal int64
	var rpsSum float64
	for _, rep := range run.Reps {
		h, err := hist.Decode(rep.Histogram)
		if err != nil {
			return RunSummary{}, fmt.Errorf("decoding rep %d histogram: %w", rep.Seq, err)
		}
		pooled.Merge(h)
		successTotal += h.TotalCount()
		for _, c := range rep.Errors {
			failedTotal += int64(c)
		}
		rpsSum += rep.AchievedRPS
	}

	var errorRate float64
	if total := successTotal + failedTotal; total > 0 {
		errorRate = float64(failedTotal) / float64(total)
	}
	var achievedRPS float64
	if len(run.Reps) > 0 {
		achievedRPS = rpsSum / float64(len(run.Reps))
	}

	return RunSummary{
		Pooled: Percentiles{
			P50:  pooled.PercentileMillis(50),
			P90:  pooled.PercentileMillis(90),
			P99:  pooled.PercentileMillis(99),
			P999: pooled.PercentileMillis(99.9),
			Max:  pooled.MaxMillis(),
		},
		ErrorRate:        errorRate,
		AchievedRPS:      achievedRPS,
		DriverOverheadMs: run.DriverOverheadMs,
	}, nil
}
