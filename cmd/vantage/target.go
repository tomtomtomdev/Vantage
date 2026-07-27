package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/tomtomtomdev/vantage/internal/adapters/store"
	"github.com/tomtomtomdev/vantage/internal/app"
)

// newTargetCmd is the `target` command group: register and list the systems
// Vantage is authorized to measure.
func newTargetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "target",
		Short: "Manage measurement targets.",
	}
	cmd.AddCommand(newTargetAddCmd(), newTargetListCmd())
	return cmd
}

func newTargetAddCmd() *cobra.Command {
	var (
		baseURL     string
		mode        string
		mutating    bool
		allowlisted bool
	)
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a target.",
		Long: "Register a target Vantage may measure. Blast-radius flags are safe by " +
			"default: a target is neither mutating nor allowlisted unless opted in (SPEC §9).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			pool, closePool, err := connect(ctx)
			if err != nil {
				return err
			}
			defer closePool()

			svc := app.NewTargetService(store.New(pool))
			id, err := svc.Add(ctx, args[0], baseURL, mode, mutating, allowlisted)
			if err != nil {
				return err
			}
			cmd.Printf("target %q registered (id %d)\n", args[0], id)
			return nil
		},
	}
	cmd.Flags().StringVar(&baseURL, "url", "", "base URL of the target (required)")
	cmd.Flags().StringVar(&mode, "mode", "black-box", "observation mode: black-box | white-box")
	cmd.Flags().BoolVar(&mutating, "mutating", false, "endpoint changes state (SPEC §9; refused by default)")
	cmd.Flags().BoolVar(&allowlisted, "allowlisted", false, "clear the target to receive load (SPEC §9)")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

func newTargetListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered targets.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			pool, closePool, err := connect(ctx)
			if err != nil {
				return err
			}
			defer closePool()

			svc := app.NewTargetService(store.New(pool))
			targets, err := svc.List(ctx)
			if err != nil {
				return err
			}
			if len(targets) == 0 {
				cmd.Println("no targets registered")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tMODE\tALLOWLISTED\tMUTATING\tURL")
			for _, t := range targets {
				fmt.Fprintf(w, "%s\t%s\t%t\t%t\t%s\n",
					t.Name, t.Mode, t.Allowlisted, t.Mutating, t.BaseURL)
			}
			return w.Flush()
		},
	}
}
