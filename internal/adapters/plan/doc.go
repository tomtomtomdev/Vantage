// Package plan implements ports.PlanSource: query-plan attribution via EXPLAIN
// (ANALYZE) and pg_stat_statements (S6 — PLAN S6, query-plan-reading.md).
//
// Carried issue #3 (PROGRESS): prefer the exemplar's own DB-span SQL over a
// global pg_stat_statements delta to avoid the ambient-traffic attribution bug.
package plan
