package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/tomtomtomdev/vantage/internal/adapters/store"
	"github.com/tomtomtomdev/vantage/internal/app"
)

// newSLOCmd is the `slo` command group: declare and list the thresholds a target
// is judged against (SPEC §3). SLOs are declared before a run; `vantage run` then
// renders PASS/FAIL against them.
func newSLOCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "slo",
		Short: "Declare and list a target's SLOs.",
	}
	cmd.AddCommand(newSLOSetCmd(), newSLOListCmd())
	return cmd
}

func newSLOSetCmd() *cobra.Command {
	var (
		metric     string
		threshold  float64
		unit       string
		comparator string
		atRPS      float64
	)
	cmd := &cobra.Command{
		Use:   "set <target-name>",
		Short: "Declare an SLO (one per metric; re-setting replaces it).",
		Long: "Declare a threshold a target is judged against, e.g.\n" +
			"  vantage slo set sluice --metric p99 --threshold 150 --unit ms --comparator '<=' --at-rps 200\n" +
			"Metrics: p50 p90 p99 p99.9 max (ms), error_rate (%), throughput (rps).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			pool, closePool, err := connect(ctx)
			if err != nil {
				return err
			}
			defer closePool()

			st := store.New(pool)
			svc := app.NewSLOService(st, st)
			id, err := svc.Set(ctx, args[0], metric, threshold, unit, comparator, atRPS)
			if err != nil {
				return err
			}
			cmd.Printf("SLO %s %s %g%s set for %q (id %d)\n", metric, comparator, threshold, unit, args[0], id)
			return nil
		},
	}
	cmd.Flags().StringVar(&metric, "metric", "", "p50|p90|p99|p99.9|max|error_rate|throughput (required)")
	cmd.Flags().Float64Var(&threshold, "threshold", 0, "the number the metric is held to (required)")
	cmd.Flags().StringVar(&unit, "unit", "", "ms | % | rps — must match the metric (required)")
	cmd.Flags().StringVar(&comparator, "comparator", "", "< | <= | > | >= | == (required)")
	cmd.Flags().Float64Var(&atRPS, "at-rps", 0, "the load the SLO applies at (optional)")
	_ = cmd.MarkFlagRequired("metric")
	_ = cmd.MarkFlagRequired("threshold")
	_ = cmd.MarkFlagRequired("unit")
	_ = cmd.MarkFlagRequired("comparator")
	return cmd
}

func newSLOListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <target-name>",
		Short: "List a target's declared SLOs.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			pool, closePool, err := connect(ctx)
			if err != nil {
				return err
			}
			defer closePool()

			st := store.New(pool)
			svc := app.NewSLOService(st, st)
			slos, err := svc.List(ctx, args[0])
			if err != nil {
				return err
			}
			if len(slos) == 0 {
				cmd.Printf("no SLOs declared for %q\n", args[0])
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "METRIC\tCOMPARATOR\tTHRESHOLD\tUNIT\t@RPS")
			for _, s := range slos {
				atRPS := "—"
				if s.AtRPS > 0 {
					atRPS = fmt.Sprintf("%.0f", s.AtRPS)
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%g\t%s\t%s\n", s.Metric, s.Comparator, s.Threshold, s.Unit, atRPS)
			}
			return w.Flush()
		},
	}
}
