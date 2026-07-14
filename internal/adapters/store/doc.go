// Package store implements ports.ResultStore over Postgres via pgx/v5
// (SaveRun/Get/SetBaseline/Baseline — PLAN S0).
//
// Uses pgxpool (size configured and recorded), prepared statements, jsonb via
// pgx native. Per-rep HDR histograms are persisted with hdrhistogram-go's
// compressed encoding into reps.histogram bytea (needed for the cluster
// bootstrap — CLAUDE.md §7). Evidence tables (runs/reps/results) are
// append-only: no UPDATE path, no destructive migrations (SPEC §8).
package store
