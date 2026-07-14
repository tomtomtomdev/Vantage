// Package app orchestrates the audit loop: run(N reps) → measure → store →
// judge/compare → report. It depends only on domain, verdict, and ports —
// never on a concrete adapter.
//
// Invariants enforced here and tested (CLAUDE.md §8): number-before-narrative
// (a delta verdict requires a stored baseline; an SLO PASS/FAIL needs only an
// SLO), and blast-radius refusal before any load is generated.
package app
