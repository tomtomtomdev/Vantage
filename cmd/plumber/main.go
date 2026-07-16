// Command plumber is the composition root: the only place adapters meet the
// domain. Wiring happens here by hand (constructor injection, no DI framework)
// and cobra commands are mounted here. See CLAUDE.md §3.
//
// This is a walking skeleton. Subcommands are added per slice (PLAN.md); S0
// ships `migrate` and `target add|list`, each behind a failing test written
// first (CLAUDE.md §2, law 2).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// version is stamped via -ldflags at build time (see Makefile). Runs immutable
// & version-marked is a core invariant (SPEC §8): every Run records the code
// state it measured.
var version = "dev"

func main() {
	// The kill switch is context cancellation (SPEC §9): SIGINT/SIGTERM cancels
	// the root ctx so in-flight work drains and exits.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		// Errors are values; log once, at the boundary. cmd/ is the only place
		// allowed to exit non-zero at the top level (CLAUDE.md §4).
		fmt.Fprintln(os.Stderr, "plumber:", err)
		os.Exit(1)
	}
}
