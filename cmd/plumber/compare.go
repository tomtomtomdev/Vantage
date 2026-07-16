package main

import (
	"fmt"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"plumber/internal/adapters/store"
	"plumber/internal/app"
	"plumber/internal/verdict"
)

// newBaselineCmd is the `baseline` group: designate the reference Run a target's
// candidates are compared against (SPEC §3/§8). A delta needs a baseline; this is
// how you set it.
func newBaselineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "baseline",
		Short: "Manage the baseline run a target is compared against.",
	}
	cmd.AddCommand(newBaselineSetCmd())
	return cmd
}

func newBaselineSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <run-id>",
		Short: "Designate a stored run as its target's baseline.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID, err := parseRunID(args[0])
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			pool, closePool, err := connect(ctx)
			if err != nil {
				return err
			}
			defer closePool()

			svc := app.NewCompareService(store.New(pool))
			targetID, err := svc.SetBaseline(ctx, runID)
			if err != nil {
				return err
			}
			cmd.Printf("baseline for target %d set to run %d\n", targetID, runID)
			return nil
		},
	}
}

// newCompareCmd is the S3 stop-and-use gate: compare a candidate run against its
// target's baseline and print the delta with a bootstrap 95%% CI (SPEC §7). The
// verdict is significant only if the CI excludes zero; env drift downgrades to
// confounded; a mismatched pair is refused as incomparable.
func newCompareCmd() *cobra.Command {
	var (
		seed      int64
		resamples int
	)
	cmd := &cobra.Command{
		Use:   "compare <candidate-run-id>",
		Short: "Compare a candidate run against its target's baseline (bootstrap CI).",
		Long: "Prove a change moved the number beyond noise, and not by accident (SPEC §7).\n" +
			"Resamples the per-rep histograms (cluster bootstrap over reps) to build a 95% CI\n" +
			"on Δpercentile; significant iff the CI excludes 0. Guards refuse incomparable pairs\n" +
			"and downgrade to confounded on environment drift.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID, err := parseRunID(args[0])
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			pool, closePool, err := connect(ctx)
			if err != nil {
				return err
			}
			defer closePool()

			svc := app.NewCompareService(store.New(pool))
			res, err := svc.Compare(ctx, runID, verdict.WithSeed(seed), verdict.WithResamples(resamples))
			if err != nil {
				return err
			}
			printComparison(cmd, res)
			return nil
		},
	}
	cmd.Flags().Int64Var(&seed, "seed", 1, "bootstrap RNG seed (fixed ⇒ reproducible CI for the same runs)")
	cmd.Flags().IntVar(&resamples, "resamples", 2000, "bootstrap iterations")
	return cmd
}

// parseRunID validates the run-id argument before any DB connection, returning an
// error that mentions "run id" so the CLI test can assert on it.
func parseRunID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid run id %q: must be a positive integer", s)
	}
	return id, nil
}

// printComparison renders the honest report shape (perf-measurement-rigor): the
// per-percentile delta with its CI and verdict, headlined by the comparability
// banner. Incomparable prints only the refusal; confounded shows the numbers under
// a caveat; comparable lets the per-metric significance stand.
func printComparison(cmd *cobra.Command, res app.CompareResult) {
	c := res.Comparison
	cmd.Printf("Compare — target %d · baseline run %d (%s) vs candidate run %d (%s)\n",
		res.TargetID, res.BaselineRunID, runLabel(res.BaselineLabel, res.BaselineVersion),
		res.CandidateRunID, runLabel(res.CandidateLabel, res.CandidateVersion))

	switch c.Comparability {
	case verdict.Incomparable:
		cmd.Printf("\nINCOMPARABLE — %s\n", c.Reason)
		cmd.Println("no delta emitted: any number would mislead (SPEC §7).")
		return
	case verdict.Confounded:
		cmd.Printf("\nCONFOUNDED — %s\n", c.Reason)
		cmd.Println("the delta below is real but may be caused by the drift, not your change — precision is not causation (SPEC §8).")
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\nMETRIC\tBASELINE\tCANDIDATE\tΔ\t95% CI\tVERDICT")
	for _, d := range c.Deltas {
		verdictStr := string(d.Significance)
		if c.Comparability == verdict.Confounded {
			verdictStr = "confounded" // no bare "significant" claim survives a confounded pair
		}
		fmt.Fprintf(w, "%s\t%.1fms\t%.1fms\t%+.1fms\t[%+.1f, %+.1f]\t%s\n",
			d.Metric, d.BaselineMs, d.CandidateMs, d.PointMs, d.CILowMs, d.CIHighMs, verdictStr)
	}
	_ = w.Flush()

	// Headline the p99 line — the tail is the product.
	for _, d := range c.Deltas {
		if d.Metric == "p99" {
			verb := "within noise"
			if c.Comparability == verdict.Comparable && d.Significance == verdict.Significant {
				verb = "significant"
			} else if c.Comparability == verdict.Confounded {
				verb = "confounded"
			}
			cmd.Printf("\nverdict: p99 %+.1fms, 95%% CI [%+.1f, %+.1f] → %s\n",
				d.PointMs, d.CILowMs, d.CIHighMs, verb)
			break
		}
	}
}

// runLabel renders a run's identity for the report line, falling back gracefully
// when a label or version marker is absent (black-box has no SHA).
func runLabel(label, version string) string {
	switch {
	case label != "" && version != "":
		return label + ", " + version
	case label != "":
		return label
	case version != "":
		return version
	default:
		return "unlabelled"
	}
}
