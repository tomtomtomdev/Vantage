// Command plumber is the composition root: the only place adapters meet the
// domain. Wiring happens here by hand (constructor injection, no DI framework)
// and cobra commands are mounted here. See CLAUDE.md §3.
//
// This is a walking skeleton. The subcommands (target, run, slo, baseline,
// compare, report) are wired in slices S0–S7 per PLAN.md — each behind a
// failing test written first (CLAUDE.md §2, law 2).
package main

import (
	"fmt"
	"os"
)

// version is stamped via -ldflags at build time (see Makefile). Runs immutable
// & version-marked is a core invariant (SPEC §8): every Run records the code
// state it measured.
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		// Errors are values; log once, at the boundary. cmd/ is the only place
		// allowed to exit non-zero at the top level (CLAUDE.md §4).
		fmt.Fprintln(os.Stderr, "plumber:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fmt.Printf("plumber %s — audit harness (pre-code; see PLAN.md S0)\n", version)
	return nil
}
