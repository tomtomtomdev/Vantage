package main

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/tomtomtomdev/vantage/internal/adapters/loaddriver"
	"github.com/tomtomtomdev/vantage/internal/adapters/store"
	"github.com/tomtomtomdev/vantage/internal/app"
	"github.com/tomtomtomdev/vantage/internal/domain"
	"github.com/tomtomtomdev/vantage/internal/platform"
	"github.com/tomtomtomdev/vantage/internal/verdict"
)

// newRunCmd is the S1 vertical seam: drive an open-model constant load against a
// registered target, measure N repetitions (success-only, warmup-discarded), and
// persist an immutable Run with its environment fingerprint (SPEC §3/§8).
func newRunCmd() *cobra.Command {
	var (
		profileStr     string
		reps           int
		workers        int
		maxRPS         int
		allowMutating  bool
		requestTimeout time.Duration
		versionMarker  string
		label          string
		colocation     string
	)
	cmd := &cobra.Command{
		Use:   "run <target-name>",
		Short: "Measure a target under load and store the Run (SPEC §3).",
		Long: "Drive a constant open-model load against a registered target and persist a Run.\n" +
			"Latency is measured from intended dispatch (open-model, no coordinated omission),\n" +
			"over successful requests only, with the warmup window discarded (SPEC §8).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, err := domain.ParseProfile(profileStr)
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			pool, closePool, err := connect(ctx)
			if err != nil {
				return err
			}
			defer closePool()

			st := store.New(pool) // satisfies both TargetStore and ResultStore
			driver := loaddriver.New(loaddriver.Config{
				Workers:        workers,
				MaxRPS:         maxRPS,
				AllowMutating:  allowMutating,
				RequestTimeout: requestTimeout,
			})
			defer driver.CloseIdleConns()

			svc := app.NewRunService(st, driver, st, platform.NewHostProbe(colocation), st)
			res, err := svc.Run(ctx, app.RunParams{
				TargetName:    args[0],
				Profile:       profile,
				NReps:         reps,
				VersionMarker: versionMarker,
				Label:         label,
			})
			if err != nil {
				return err
			}

			printRun(cmd, profile, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&profileStr, "profile", "", "load profile, e.g. constant:rps=100,dur=60s,warmup=10s (required)")
	cmd.Flags().IntVar(&reps, "reps", 5, "number of repetitions (N>=5 recommended for a claim)")
	cmd.Flags().IntVar(&workers, "workers", 32, "bounded worker pool size (models the target's connection limit)")
	cmd.Flags().IntVar(&maxRPS, "max-rps", 1000, "rate ceiling; a profile above it is refused (SPEC §9)")
	cmd.Flags().BoolVar(&allowMutating, "allow-mutating", false, "explicit override to load a mutating target (SPEC §9)")
	cmd.Flags().DurationVar(&requestTimeout, "request-timeout", 30*time.Second, "per-request timeout")
	cmd.Flags().StringVar(&versionMarker, "version-marker", "", "code state under test (git SHA / deploy tag / note)")
	cmd.Flags().StringVar(&label, "label", "", "human label, e.g. before-fix / after-index")
	cmd.Flags().StringVar(&colocation, "colocation", "unknown", "where the driver runs relative to the target (recorded in the fingerprint)")
	_ = cmd.MarkFlagRequired("profile")
	return cmd
}

// printRun renders the per-rep distribution and the pooled result. Errors are
// reported as a co-equal axis alongside latency — a latency number without its
// error rate is not a result (SPEC §8).
func printRun(cmd *cobra.Command, p domain.LoadProfile, res app.RunResult) {
	cmd.Printf("Run %d — %s · %d reps · warmup %s discarded · success-only latency\n",
		res.RunID, p, res.Run.NReps, p.Warmup)
	cmd.Printf("env: host=%s colocation=%s\n\n", res.Run.Env.Host, res.Run.Env.Colocation)

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "REP\tP50(ms)\tP90(ms)\tP99(ms)\tP99.9(ms)\tMAX(ms)\tERR%\tRPS")
	for _, r := range res.Run.Reps {
		fmt.Fprintf(w, "%d\t%.1f\t%.1f\t%.1f\t%.1f\t%.1f\t%.2f\t%.1f\n",
			r.Seq, r.P50Ms, r.P90Ms, r.P99Ms, r.P999Ms, r.MaxMs, r.ErrorRate*100, r.AchievedRPS)
	}
	s := res.Summary
	fmt.Fprintf(w, "pooled\t%.1f\t%.1f\t%.1f\t%.1f\t%.1f\t%.2f\t%.1f\n",
		s.Pooled.P50, s.Pooled.P90, s.Pooled.P99, s.Pooled.P999, s.Pooled.Max, s.ErrorRate*100, s.AchievedRPS)
	_ = w.Flush()

	cmd.Printf("\ndriver overhead: %.2fms (worst rep) — distrust the run if this is large or achieved rps << requested\n",
		s.DriverOverheadMs)

	printVerdicts(cmd, res.Verdicts)
}

// printVerdicts renders the per-metric PASS/FAIL against the target's declared
// SLOs (SPEC §7). A target with no SLO gets a hint, not a fabricated verdict —
// a PASS/FAIL requires a declared threshold (SPEC §8).
func printVerdicts(cmd *cobra.Command, verdicts verdict.Verdicts) {
	if len(verdicts) == 0 {
		cmd.Println("\nno SLO declared for this target — run `vantage slo set` to get a PASS/FAIL verdict")
		return
	}

	overall := "PASS"
	if !verdicts.AllPass() {
		overall = "FAIL"
	}
	cmd.Printf("\nSLO verdict: %s\n", overall)

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "METRIC\tSLO\tACTUAL\t@RPS\tRESULT")
	for _, v := range verdicts {
		atRPS := "—"
		if v.SLO.AtRPS > 0 {
			atRPS = fmt.Sprintf("%.0f", v.SLO.AtRPS)
		}
		fmt.Fprintf(w, "%s\t%s %g%s\t%g %s\t%s\t%s\n",
			v.SLO.Metric, v.SLO.Comparator, v.SLO.Threshold, unitSuffix(v.SLO.Unit),
			v.Actual, v.SLO.Unit, atRPS, v.Status)
	}
	_ = w.Flush()
}

// unitSuffix renders the unit adjacent to the threshold ("150ms", "0.1%"),
// spacing rps so "200 rps" stays readable.
func unitSuffix(unit string) string {
	if unit == "rps" {
		return " rps"
	}
	return unit
}
