// Package trace implements ports.ExemplarTraceSource: the tail-exemplar trace
// ingestion for white-box attribution (F3, deferred to S5 — SPEC §5, PLAN S5).
//
// It pulls the p99 request BY trace-id — the slow path itself, not an average
// over the window — and folds its span waterfall into the audit artifact.
package trace
