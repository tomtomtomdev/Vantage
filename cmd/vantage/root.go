package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/tomtomtomdev/vantage/internal/platform"
)

// newRootCmd assembles the command tree. Each subcommand connects its own
// resources lazily inside RunE, so `--help` and arg validation never require a
// database.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "vantage",
		Short:         "An audit harness that proves a backend got faster (SPEC).",
		Version:       version,
		SilenceUsage:  true, // a runtime error is not a usage error; don't dump help
		SilenceErrors: true, // main() prints the error once, at the boundary
	}
	root.AddCommand(newMigrateCmd(), newTargetCmd(), newRunCmd(), newSLOCmd(), newBaselineCmd(), newCompareCmd())
	return root
}

// connect loads config and opens a pgx pool, returning the pool and a close
// func. It is the composition root's single connection seam — every DB-backed
// command routes through it so config is read in exactly one place (CLAUDE §7).
func connect(ctx context.Context) (*pgxpool.Pool, func(), error) {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return nil, nil, err
	}
	pool, err := platform.Connect(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	return pool, pool.Close, nil
}
