package main

import (
	"github.com/spf13/cobra"

	"plumber/internal/platform"
	"plumber/migrations"
)

// newMigrateCmd applies the embedded schema migrations. Idempotent — safe to run
// on every deploy; already-applied versions are skipped.
func newMigrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Apply pending database migrations (SPEC §6).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			pool, closePool, err := connect(ctx)
			if err != nil {
				return err
			}
			defer closePool()

			if err := platform.Migrate(ctx, pool, migrations.FS); err != nil {
				return err
			}
			cmd.Println("migrations applied")
			return nil
		},
	}
}
