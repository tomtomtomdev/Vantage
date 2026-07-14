// Package domain holds the pure core types: Target, SLO, LoadProfile, Run,
// Repetition, Result, Baseline, and the typed sentinel errors (ErrIncomparable,
// ErrConfounded, ErrTargetNotAllowlisted).
//
// PURITY IS ENFORCED, NOT HOPED FOR (CLAUDE.md §3): this package imports no
// adapter, no pgx, no cobra, no net/http. The arch-test (S0) and depguard fail
// the build on any such import. If a domain type seems to need the clock, a DB,
// or randomness, the design is wrong — pass the data in.
package domain
